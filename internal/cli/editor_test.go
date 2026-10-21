package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func seedRegistry(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "cdt", "registry.json")
	if err := update(file, func(r *Registry) error {
		r.Entries = testEntries([]Entry{{"/work", []string{"old"}}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestEditValidation(t *testing.T) {
	for _, tc := range []struct {
		name, content string
	}{
		{"invalid-json", "{broken"},
		{"wrong-version", `{"version":2,"entries":[]}`},
		{"invalid-tag", `{"version":1,"entries":[{"path":"/work","tags":["has space"]}]}`},
		{"relative-path", `{"version":1,"entries":[{"path":"work","tags":[]}]}`},
		{"duplicate-path", `{"version":1,"entries":[{"path":"/work","tags":[]},{"path":"/work","tags":[]}]}`},
		{"missing-fields", `{}`},
		{"empty-file", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := seedRegistry(t)
			before, _ := os.ReadFile(file)
			var draft string
			err := editRegistry(file, func(name string) error {
				draft = name
				info, err := os.Stat(name)
				if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
					t.Fatalf("draft not private: %v %v", info, err)
				}
				return os.WriteFile(name, []byte(tc.content), 0600)
			})
			if err == nil || !strings.Contains(err.Error(), strconv.Quote(draft)) {
				t.Fatal("invalid draft must be retained with its path", err)
			}
			after, _ := os.ReadFile(file)
			got, readErr := os.ReadFile(draft)
			if !bytes.Equal(before, after) || readErr != nil || string(got) != tc.content {
				t.Fatal("registry changed or draft lost", readErr)
			}
		})
	}
}

func TestEditSuccess(t *testing.T) {
	file := seedRegistry(t)
	var draft string
	err := editRegistry(file, func(name string) error {
		draft = name
		data, err := json.Marshal(Registry{1, testEntries([]Entry{{"/work", []string{"new", "new"}}})})
		if err != nil {
			return err
		}
		return os.WriteFile(name, data, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := load(file)
	if err != nil || len(r.Entries) != 1 || len(r.Entries[0].Tags) != 1 || r.Entries[0].Tags[0] != "new" {
		t.Fatal(r, err)
	}
	if _, err := os.Stat(draft); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("successful draft should be removed", err)
	}
	info, _ := os.Stat(file)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("registry permissions", info.Mode())
	}
	before, _ := os.ReadFile(file)
	if err := editRegistry(file, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("unchanged edit changed registry")
	}
}

func TestEditFailure(t *testing.T) {
	for _, remove := range []bool{false, true} {
		file := seedRegistry(t)
		before, _ := os.ReadFile(file)
		err := editRegistry(file, func(name string) error {
			if remove {
				return os.Remove(name)
			}
			return errors.New("editor exited unsuccessfully")
		})
		after, _ := os.ReadFile(file)
		if err == nil || !bytes.Equal(before, after) {
			t.Fatal("editor failure or deleted draft changed registry", err)
		}
	}
}

func TestEditConcurrentChanges(t *testing.T) {
	file := seedRegistry(t)
	var draft string
	err := editRegistry(file, func(name string) error {
		draft = name
		// This must complete while the editor is open, not wait on its lock.
		return update(file, func(r *Registry) error {
			r.Entries[0].Tags = []string{"from-other-terminal"}
			return nil
		})
	})
	if err == nil || !strings.Contains(err.Error(), "changed while editing") {
		t.Fatal("concurrent change was not detected", err)
	}
	r, loadErr := load(file)
	if loadErr != nil || r.Entries[0].Tags[0] != "from-other-terminal" {
		t.Fatal("concurrent update lost", r, loadErr)
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatal("conflicting draft lost", err)
	}
}

func TestEditCreateAndRepair(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cdt", "registry.json")
	if err := editRegistry(file, func(name string) error {
		r, err := load(name)
		if err != nil || r.Version != 1 || len(r.Entries) != 0 {
			t.Fatal(r, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := editRegistry(file, func(name string) error {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != "{broken" {
			t.Fatal("corrupt original not offered for repair", err)
		}
		return os.WriteFile(name, []byte(`{"version":1,"entries":[]}`), 0600)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := load(file); err != nil {
		t.Fatal(err)
	}
}

func TestEditCreateConflict(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cdt", "registry.json")
	err := editRegistry(file, func(string) error {
		return update(file, func(r *Registry) error {
			r.Entries = testEntries([]Entry{{"/other", []string{"tag"}}})
			return nil
		})
	})
	if err == nil || !strings.Contains(err.Error(), "changed while editing") {
		t.Fatal("newly created registry was overwritten", err)
	}
}

func TestPreferredEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if preferredEditor() != defaultEditor {
		t.Fatal("missing default")
	}
	t.Setenv("EDITOR", "nano")
	if preferredEditor() != "nano" {
		t.Fatal("EDITOR ignored")
	}
	t.Setenv("VISUAL", "code --wait")
	if preferredEditor() != "code --wait" {
		t.Fatal("VISUAL should take precedence")
	}
	t.Setenv("VISUAL", "  ")
	if preferredEditor() != "nano" {
		t.Fatal("blank VISUAL should be ignored")
	}
}

func TestExecuteEdit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "space ' $(touch INJECTED)")
	t.Setenv("XDG_CONFIG_HOME", root)
	file, err := registryPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := update(file, func(r *Registry) error {
		r.Entries = testEntries([]Entry{{"/work", []string{"old"}}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CDT_TEST_EDITOR", "1")
	quoted := "'" + strings.ReplaceAll(exe, "'", "'\\''") + "'"
	if runtime.GOOS == "windows" {
		quoted = "'" + strings.ReplaceAll(exe, "'", "''") + "'"
	}
	editorArgs := " -test.run=^TestEditorProcess$ -- replace"
	if runtime.GOOS == "windows" {
		editorArgs = " '-test.run=^TestEditorProcess$' -- replace"
	}
	t.Setenv("VISUAL", quoted+editorArgs)
	t.Setenv("EDITOR", "false")
	var out bytes.Buffer
	if err := Execute([]string{"--edit"}, strings.NewReader(""), &out, "dev"); err != nil {
		t.Fatal(err)
	}
	r, err := load(file)
	if err != nil || r.Entries[0].Tags[0] != "new" || !strings.Contains(out.String(), "Registry saved.") {
		t.Fatal(r, err, out.String())
	}
}

func TestEditorProcess(t *testing.T) {
	if os.Getenv("CDT_TEST_EDITOR") != "1" {
		return
	}
	args := os.Args
	if len(args) < 3 || args[len(args)-2] != "replace" {
		os.Exit(2)
	}
	file := args[len(args)-1]
	data, err := os.ReadFile(file)
	if err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(file, bytes.ReplaceAll(data, []byte(`"old"`), []byte(`"new"`)), 0600); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}
