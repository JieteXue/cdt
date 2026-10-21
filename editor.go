package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func preferredEditor() string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if editor := strings.TrimSpace(os.Getenv(name)); editor != "" {
			return editor
		}
	}
	return "vi"
}

func launchEditor(file string, in io.Reader, out io.Writer) error {
	// The editor setting is a trusted user command; the filename remains a
	// positional argument, never interpolated into shell source.
	cmd := exec.Command("/bin/sh", "-c", preferredEditor()+` "$@"`, "cdt-editor", file)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, os.Stderr
	return cmd.Run()
}

func snapshot(file string) ([]byte, bool, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func editRegistry(file string, edit func(string) error) error {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	original, existed, err := snapshot(file)
	if err != nil {
		return err
	}
	initial := original
	if !existed {
		initial, err = json.MarshalIndent(Registry{1, []Entry{}}, "", "  ")
		if err != nil {
			return err
		}
		initial = append(initial, '\n')
	}
	draft, err := os.CreateTemp(filepath.Dir(file), ".edit-*.json")
	if err != nil {
		return err
	}
	name := draft.Name()
	keep := false
	defer func() {
		if !keep {
			os.Remove(name)
		}
	}()
	if _, err := draft.Write(initial); err != nil {
		draft.Close()
		return err
	}
	if err := draft.Close(); err != nil {
		return err
	}
	preserve := func(err error) error {
		keep = true
		if _, statErr := os.Stat(name); errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("%w; edit not applied, draft no longer exists at %q", err, name)
		}
		return fmt.Errorf("%w; edit not applied, draft kept at %q", err, name)
	}
	if err := edit(name); err != nil {
		return preserve(fmt.Errorf("editor failed: %w", err))
	}
	edited, err := os.ReadFile(name)
	if err != nil {
		return preserve(fmt.Errorf("cannot read edited draft: %w", err))
	}
	// A removed draft is not an empty registry. Read it above before load,
	// whose missing-file behavior is intended for first-time registration.
	r, err := load(name)
	if err != nil {
		return preserve(err)
	}
	for i := range r.Entries {
		r.Entries[i].Tags = unique(r.Entries[i].Tags)
	}
	// Do not hold a lock while a person is editing. Recheck the snapshot
	// under the shared writer lock to avoid losing another terminal's changes.
	err = withRegistryLock(file, func() error {
		current, exists, err := snapshot(file)
		if err != nil {
			return err
		}
		if exists != existed || !bytes.Equal(current, original) {
			return errors.New("registry changed while editing; retry with the latest records")
		}
		if existed && bytes.Equal(edited, original) {
			return nil
		}
		return save(file, r)
	})
	if err != nil {
		return preserve(err)
	}
	return nil
}
