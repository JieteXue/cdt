package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

type Entry struct {
	Path string   `json:"path"`
	Tags []string `json:"tags"`
}

type Registry struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

func registryPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("XDG_CONFIG_HOME must be absolute")
	}
	return filepath.Join(base, "cdt", "registry.json"), nil
}

func validTags(tags []string) error {
	for _, tag := range tags {
		if tag == "" || !utf8.ValidString(tag) || strings.ContainsFunc(tag, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return fmt.Errorf("invalid tag %q: whitespace and control characters are not allowed", tag)
		}
	}
	return nil
}

func unique(tags []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, t := range tags {
		if !seen[t] {
			result = append(result, t)
			seen[t] = true
		}
	}
	sort.Strings(result)
	return result
}

func load(file string) (Registry, error) {
	r := Registry{Version: 1, Entries: []Entry{}}
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	defer f.Close()
	r = Registry{}
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return r, fmt.Errorf("invalid registry (left unchanged): %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return r, errors.New("invalid registry: trailing data")
	}
	if r.Version != 1 {
		return r, fmt.Errorf("unsupported registry version %d", r.Version)
	}
	if r.Entries == nil {
		return r, errors.New("invalid registry: entries must be an array")
	}
	seen := map[string]bool{}
	for _, e := range r.Entries {
		if !filepath.IsAbs(e.Path) || filepath.Clean(e.Path) != e.Path || seen[e.Path] || strings.ContainsRune(e.Path, 0) {
			return r, errors.New("invalid registry path or duplicate entry")
		}
		if err := validTags(e.Tags); err != nil {
			return r, err
		}
		seen[e.Path] = true
	}
	return r, nil
}

func save(file string, r Registry) error {
	sort.Slice(r.Entries, func(i, j int) bool { return r.Entries[i].Path < r.Entries[j].Path })
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".registry-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), file); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(file))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func update(file string, change func(*Registry) error) error {
	return withRegistryLock(file, func() error {
		r, err := load(file)
		if err != nil {
			return err
		}
		if err := change(&r); err != nil {
			return err
		}
		return save(file, r)
	})
}

func withRegistryLock(file string, action func() error) error {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(file+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return action()
}

func canonical(path string, requireDirectory bool) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !utf8.ValidString(abs) {
		return "", errors.New("path must be valid UTF-8")
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if !requireDirectory && errors.Is(err, os.ErrNotExist) {
			return abs, nil
		}
		return "", err
	}
	if requireDirectory {
		info, err := os.Stat(real)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%q is not a directory", path)
		}
	}
	return real, nil
}

func ancestor(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Compute inheritance from explicit records, so late parent registration and
// parent removal take effect without persisting duplicated tags.
func matches(r Registry, tags []string, existingOnly bool) []Entry {
	result := []Entry{}
	for _, e := range r.Entries {
		effective := map[string]bool{}
		own := map[string]bool{}
		for _, t := range e.Tags {
			effective[t] = true
			own[t] = true
		}
		for _, p := range r.Entries {
			if ancestor(p.Path, e.Path) {
				for _, t := range p.Tags {
					effective[t] = true
				}
			}
		}
		all, direct := true, len(tags) == 0
		for _, t := range tags {
			if !effective[t] {
				all = false
			}
			if own[t] {
				direct = true
			}
		}
		if !all || !direct {
			continue
		}
		if existingOnly {
			info, err := os.Stat(e.Path)
			if err != nil || !info.IsDir() {
				continue
			}
		}
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}
