package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestMoveOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--move", "old space", "new ' 中文", "--recursive", "--dry-run"},
		{"--dry-run", "old space", "--move", "--recursive", "new ' 中文"},
	} {
		got, err := parse(args)
		if err != nil || got.Op != "move" || got.MoveFrom != "old space" ||
			got.MoveTo != "new ' 中文" || !got.Recursive || !got.DryRun || len(got.Tags) != 0 {
			t.Fatal(got, err)
		}
	}
	got, err := parse([]string{"--move", "--", "-old", "--new"})
	if err != nil || got.MoveFrom != "-old" || got.MoveTo != "--new" {
		t.Fatal(got, err)
	}
	for _, args := range [][]string{
		{"--move"}, {"--move", "old"}, {"--move", "old", "new", "extra"},
		{"--move", "", "new"}, {"--move", "old", ""},
		{"--move=old", "new"}, {"--move", "old", "new", "--recursive=yes"},
		{"--move", "old", "new", "--dry-run=yes"}, {"--recursive"}, {"--dry-run", "api"},
		{"--move", "-p", ".", "old", "new"},
	} {
		if _, err := parse(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	for _, op := range []string{"-a", "-d", "-r", "-l", "-e", "--doctor", "--tags", "--explain", "-h", "-V"} {
		for _, args := range [][]string{{op, "--move", "old", "new"}, {"--move", "old", "new", op}, {op, "--dry-run"}, {op, "--recursive"}} {
			if _, err := parse(args); err == nil {
				t.Errorf("accepted conflicting options %v", args)
			}
		}
	}
}

func moveTestRoot(t *testing.T) string {
	t.Helper()
	root, err := canonical(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func makeMoveDirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
}

func moveTestRegistry(t *testing.T, root string, entries []Entry) string {
	t.Helper()
	file := filepath.Join(root, "registry.json")
	if err := save(file, Registry{1, entries}); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestMovePreviewAndApply(t *testing.T) {
	for _, recursive := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "recursive"}[recursive], func(t *testing.T) {
			root := moveTestRoot(t)
			oldScope, newScope := filepath.Join(root, "old-scope"), filepath.Join(root, "new-scope")
			old, target := filepath.Join(oldScope, "old space ' 中文"), filepath.Join(newScope, "new [literal]")
			child, neighbor := filepath.Join(old, "api"), filepath.Join(oldScope, "old-neighbor")
			makeMoveDirs(t, child, neighbor, newScope)
			file := moveTestRegistry(t, root, []Entry{
				{oldScope, []string{"work"}}, {newScope, []string{"personal"}},
				{old, []string{"project"}}, {child, []string{"api"}}, {neighbor, []string{"api"}},
			})
			if err := os.Rename(old, target); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(file)
			info, _ := os.Stat(file)
			o := Options{Op: "move", MoveFrom: old, MoveTo: target, Recursive: recursive, DryRun: true}
			var out bytes.Buffer
			if err := moveRegistry(file, o, &out); err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{"migration preview", "registry unchanged", "Own tags: [project]", "Directory contents were not moved"} {
				if !strings.Contains(out.String(), text) {
					t.Errorf("missing %q in %s", text, &out)
				}
			}
			after, _ := os.ReadFile(file)
			afterInfo, _ := os.Stat(file)
			if !bytes.Equal(before, after) || !os.SameFile(info, afterInfo) || !info.ModTime().Equal(afterInfo.ModTime()) {
				t.Fatal("preview modified registry")
			}
			if _, err := os.Stat(file + ".lock"); !os.IsNotExist(err) {
				t.Fatal("preview created writer lock", err)
			}
			out.Reset()
			o.DryRun = false
			if err := moveRegistry(file, o, &out); err != nil {
				t.Fatal(err)
			}
			r, err := load(file)
			if err != nil || len(r.Entries) != 5 {
				t.Fatal(r, err)
			}
			if got := matches(r, []string{"personal", "project"}, true); len(got) != 1 || got[0].Path != target {
				t.Fatal("parent migration failed", got)
			}
			childTarget := child
			if recursive {
				childTarget = filepath.Join(target, "api")
				if got := matches(r, []string{"personal", "project", "api"}, true); len(got) != 1 || got[0].Path != childTarget {
					t.Fatal("new ancestry not applied", got)
				}
			} else if !strings.Contains(out.String(), "Registered descendants left unchanged: 1") {
				t.Fatal(out.String())
			}
			for _, entry := range r.Entries {
				if entry.Path == old || (entry.Path == childTarget && !reflect.DeepEqual(entry.Tags, []string{"api"})) {
					t.Fatal("old path retained or own tags changed", entry)
				}
			}
			if got := matches(r, []string{"work", "project"}, true); len(got) != 0 {
				t.Fatal("old parent tags retained", got)
			}
			if _, err := os.Stat(old); !os.IsNotExist(err) {
				t.Fatal("migration recreated source", err)
			}
			if _, err := os.Stat(filepath.Join(target, "api")); err != nil {
				t.Fatal("migration touched destination files", err)
			}
		})
	}
}

func TestMoveUnregisteredPrefix(t *testing.T) {
	root := moveTestRoot(t)
	old, target := filepath.Join(root, "old"), filepath.Join(root, "new")
	makeMoveDirs(t, filepath.Join(target, "nested", "api"))
	r := Registry{1, []Entry{{filepath.Join(old, "nested", "api"), []string{"api"}}}}
	if _, _, _, err := planMove(r, Options{MoveFrom: old, MoveTo: target}); err == nil {
		t.Fatal("single migration accepted unregistered source")
	}
	next, changes, _, err := planMove(r, Options{MoveFrom: old, MoveTo: target, Recursive: true})
	if err != nil || len(changes) != 1 || next.Entries[0].Path != filepath.Join(target, "nested", "api") {
		t.Fatal(next, changes, err)
	}
	if r.Entries[0].Path != filepath.Join(old, "nested", "api") {
		t.Fatal("planning mutated input")
	}
}

func TestMoveRejectsAtomically(t *testing.T) {
	for _, problem := range []string{"conflict", "child-conflict", "missing-child", "file-child", "missing-target", "overlap", "unregistered", "invalid-utf8", "nul", "corrupt"} {
		t.Run(problem, func(t *testing.T) {
			root := moveTestRoot(t)
			old, target := filepath.Join(root, "old"), filepath.Join(root, "new")
			makeMoveDirs(t, filepath.Join(old, "api"), filepath.Join(target, "api"))
			entries := []Entry{{old, []string{"work"}}, {filepath.Join(old, "api"), []string{"api"}}}
			o := Options{Op: "move", MoveFrom: old, MoveTo: target, Recursive: true}
			switch problem {
			case "conflict":
				entries = append(entries, Entry{target, []string{"existing"}})
			case "child-conflict":
				entries = append(entries, Entry{filepath.Join(target, "api"), []string{"existing"}})
			case "missing-child", "file-child":
				if err := os.Remove(filepath.Join(target, "api")); err != nil {
					t.Fatal(err)
				}
				if problem == "file-child" {
					if err := os.WriteFile(filepath.Join(target, "api"), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "missing-target":
				o.MoveTo = filepath.Join(root, "absent")
			case "overlap":
				o.MoveTo = filepath.Join(old, "api")
			case "unregistered":
				o.MoveFrom = filepath.Join(root, "absent")
			case "invalid-utf8":
				o.MoveFrom = string([]byte{0xff})
			case "nul":
				o.MoveFrom = "bad\x00path"
			}
			file := moveTestRegistry(t, root, entries)
			if problem == "corrupt" {
				if err := os.WriteFile(file, []byte("{broken"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(file)
			for _, dry := range []bool{true, false} {
				o.DryRun = dry
				var out bytes.Buffer
				if err := moveRegistry(file, o, &out); err == nil || out.Len() != 0 {
					t.Fatal("failed migration emitted success or was accepted", err, out.String())
				}
				after, _ := os.ReadFile(file)
				if !bytes.Equal(before, after) {
					t.Fatal("failed migration partially rewrote registry")
				}
			}
			if _, err := os.Stat(filepath.Join(old, "api")); err != nil {
				t.Fatal("failed migration touched source directory", err)
			}
		})
	}
}

func TestMoveNoOpAndReadOnlyFailure(t *testing.T) {
	root := moveTestRoot(t)
	file := moveTestRegistry(t, root, []Entry{{root, []string{"work"}}})
	before, _ := os.Stat(file)
	var out bytes.Buffer
	o := Options{MoveFrom: root, MoveTo: root, Recursive: true}
	if err := moveRegistry(file, o, &out); err != nil || !strings.Contains(out.String(), "unchanged") {
		t.Fatal(out.String(), err)
	}
	after, _ := os.Stat(file)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("no-op rewrote registry")
	}
	o.DryRun = true
	if err := moveRegistry(file, o, failedTreeWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	o.MoveFrom = filepath.Join(root, "missing")
	missingFile := filepath.Join(root, "missing-config", "registry.json")
	if err := moveRegistry(missingFile, o, &out); err == nil {
		t.Fatal("accepted absent registry")
	}
	if _, err := os.Stat(filepath.Dir(missingFile)); !os.IsNotExist(err) {
		t.Fatal("preview created configuration", err)
	}
}

func moveTestSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation requires Developer Mode or privilege: %v", err)
		}
		t.Fatal(err)
	}
}

func TestMoveSymlinks(t *testing.T) {
	t.Run("existing-destination-alias", func(t *testing.T) {
		root := moveTestRoot(t)
		old, target, alias := filepath.Join(root, "old"), filepath.Join(root, "new"), filepath.Join(root, "alias")
		makeMoveDirs(t, target)
		moveTestSymlink(t, target, alias)
		r := Registry{1, []Entry{{old, []string{"work"}}, {alias, []string{"existing"}}}}
		if _, _, _, err := planMove(r, Options{MoveFrom: old, MoveTo: target}); err == nil {
			t.Fatal("accepted destination already registered through an alias")
		}
	})
	t.Run("missing-source-through-alias", func(t *testing.T) {
		root := moveTestRoot(t)
		old, target := filepath.Join(root, "old"), filepath.Join(root, "new")
		alias := filepath.Join(t.TempDir(), "alias")
		makeMoveDirs(t, target)
		moveTestSymlink(t, root, alias)
		r := Registry{1, []Entry{{old, []string{"work"}}}}
		next, _, _, err := planMove(r, Options{MoveFrom: filepath.Join(alias, "old"), MoveTo: filepath.Join(alias, "new")})
		if err != nil || next.Entries[0].Path != target {
			t.Fatal(next, err)
		}
		// A compatibility symlink at OLD must not hide its literal old record.
		moveTestSymlink(t, target, old)
		next, _, _, err = planMove(r, Options{MoveFrom: old, MoveTo: target})
		if err != nil || next.Entries[0].Path != target {
			t.Fatal(next, err)
		}
	})
	for _, escape := range []bool{false, true} {
		t.Run(map[bool]string{false: "duplicate-target", true: "escape-subtree"}[escape], func(t *testing.T) {
			root := moveTestRoot(t)
			old, target := filepath.Join(root, "old"), filepath.Join(root, "new")
			linkTarget := filepath.Join(target, "shared")
			makeMoveDirs(t, linkTarget)
			if escape {
				linkTarget = moveTestRoot(t)
			}
			moveTestSymlink(t, linkTarget, filepath.Join(target, "api"))
			r := Registry{1, []Entry{
				{old, []string{"work"}}, {filepath.Join(old, "api"), []string{"api"}},
				{filepath.Join(old, "shared"), []string{"shared"}},
			}}
			if _, _, _, err := planMove(r, Options{MoveFrom: old, MoveTo: target, Recursive: true}); err == nil {
				t.Fatal("accepted unsafe symlink mapping")
			}
		})
	}
}

func TestMoveConcurrentWriters(t *testing.T) {
	root := moveTestRoot(t)
	old, target := filepath.Join(root, "old"), filepath.Join(root, "new")
	makeMoveDirs(t, old, target)
	file := moveTestRegistry(t, root, []Entry{{old, []string{"work"}}})
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			var err error
			if i == 0 {
				err = moveRegistry(file, Options{MoveFrom: old, MoveTo: target}, io.Discard)
			} else {
				err = update(file, func(r *Registry) error {
					r.Entries = append(r.Entries, Entry{filepath.Join(root, strings.Repeat("x", i)), []string{"other"}})
					return nil
				})
			}
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	r, err := load(file)
	if err != nil || len(r.Entries) != 9 {
		t.Fatal(r, err)
	}
	if got := matches(r, []string{"work"}, false); len(got) != 1 || got[0].Path != target {
		t.Fatal("migration lost during concurrent writes", got)
	}
}

func TestRegisteredPathCompletion(t *testing.T) {
	root := moveTestRoot(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	var out bytes.Buffer
	if err := Execute([]string{"__paths"}, strings.NewReader(""), &out, "dev"); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(out.Bytes(), &response); err != nil || response.Version != 1 || response.Entries == nil {
		t.Fatal(response, err)
	}
	if _, err := os.Stat(filepath.Join(root, "config")); !os.IsNotExist(err) {
		t.Fatal("completion created config", err)
	}
	makeMoveDirs(t, filepath.Join(root, "config", "cdt"))
	file, _ := registryPath()
	if err := save(file, Registry{1, []Entry{{filepath.Join(root, "stale ' path"), []string{"api"}}}}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Execute([]string{"__paths", "--"}, strings.NewReader(""), &out, "dev"); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil || len(response.Entries) != 1 {
		t.Fatal(response, err)
	}
	for _, args := range [][]string{{"__paths", "tag"}, {"__paths", "--move", "old", "new"}, {"__query", "--move", "old", "new"}} {
		out.Reset()
		if err := Execute(args, strings.NewReader(""), &out, "dev"); err == nil || out.Len() != 0 {
			t.Fatal("accepted invalid internal query", args, err, out.String())
		}
	}
}
