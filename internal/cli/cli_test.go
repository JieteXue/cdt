package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestParse(t *testing.T) {
	for _, args := range [][]string{
		{"-ap", "/tmp/project", "api", "backend"},
		{"-a", "-p", "/tmp/project", "api", "backend"},
		{"--add", "--path=/tmp/project", "api", "backend"},
		{"api", "--add", "--path", "/tmp/project", "backend"},
	} {
		got, err := parse(args)
		want := Options{"add", "/tmp/project", []string{"api", "backend"}, true}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%v: %+v %v", args, got, err)
		}
	}
	for _, args := range [][]string{{"-dp", "/tmp/project"}, {"--delete", "--path", "/tmp/project"}} {
		got, err := parse(args)
		if err != nil || got.Op != "delete" || got.Path != "/tmp/project" {
			t.Fatalf("%v: %+v %v", args, got, err)
		}
	}
	for _, args := range [][]string{{"-e"}, {"--edit"}} {
		got, err := parse(args)
		if err != nil || got.Op != "edit" {
			t.Fatalf("%v: %+v %v", args, got, err)
		}
	}
	for _, args := range [][]string{
		{"-p"}, {"--path"}, {"-ad"}, {"-rl"}, {"-d", "tag"}, {"-r"}, {"-lp", "/tmp"},
		{"-x"}, {"--bad"}, {"--add=yes"}, {"--path=", "-a"}, {"-a", "bad tag"}, {"-a", "bad\n"},
		{"-ae"}, {"-el"}, {"-ep", "/tmp"}, {"--edit", "tag"},
	} {
		if _, err := parse(args); err == nil {
			t.Fatalf("accepted invalid args %v", args)
		}
	}
	o, err := parse([]string{"-a", "--", "-tag", "中文"})
	if err != nil || !reflect.DeepEqual(o.Tags, []string{"-tag", "中文"}) {
		t.Fatal(o, err)
	}
	o, err = parse([]string{"-pa"})
	if err == nil || o.Path != "a" {
		t.Fatal("-pa must consume a as path, not add", o, err)
	}
}

func TestInheritance(t *testing.T) {
	r := Registry{1, testEntries([]Entry{
		{"/work", []string{"work"}},
		{"/work/api", []string{"api", "backend"}},
		{"/personal", []string{"personal"}},
		{"/personal/api", []string{"api"}},
		{"/work/api/deep", []string{"deep"}},
		{"/work/api/empty", []string{}},
		{"/workspace/api", []string{"api"}},
	})}
	for _, tc := range []struct {
		tags  []string
		paths []string
	}{
		{[]string{"work"}, []string{"/work"}},
		{[]string{"work", "api"}, []string{"/work/api"}},
		{[]string{"api", "work"}, []string{"/work/api"}},
		{[]string{"work", "api", "deep"}, []string{"/work/api/deep"}},
		{[]string{"backend", "work"}, []string{"/work/api"}},
		{[]string{"absent"}, []string{}},
	} {
		got := []string{}
		for i := range tc.paths {
			tc.paths[i] = testPath(tc.paths[i])
		}
		for _, e := range matches(r, tc.tags, false) {
			got = append(got, e.Path)
		}
		if !reflect.DeepEqual(got, tc.paths) {
			t.Errorf("%v: %v != %v", tc.tags, got, tc.paths)
		}
	}
	r.Entries = r.Entries[1:]
	if got := matches(r, []string{"work", "api"}, false); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestCRUDAndPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	parent := filepath.Join(root, "a parent")
	childName := "中文 ' $(touch INJECTED)"
	if runtime.GOOS != "windows" {
		childName += "\nend"
	}
	child := filepath.Join(parent, childName)
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := Execute(args, strings.NewReader(""), &out, "dev"); err != nil {
			t.Fatal(args, err)
		}
		return out.String()
	}
	// Child first, parent later: no persisted inherited tags to become stale.
	run("-ap", child, "api", "api")
	run("-ap", parent, "work")
	query := run("__query", "--", "work", "api")
	var resp Response
	if err := json.Unmarshal([]byte(query), &resp); err != nil {
		t.Fatal(err)
	}
	real, _ := canonical(child, true)
	if len(resp.Entries) != 1 || resp.Entries[0].Path != real || len(resp.Entries[0].Tags) != 1 {
		t.Fatal(resp)
	}
	var decoded bytes.Buffer
	if err := Execute([]string{"__decode"}, strings.NewReader(query), &decoded, "dev"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(decoded.String(), real+"\x00") {
		t.Fatal("path framing changed")
	}
	link := filepath.Join(root, "link")
	t.Run("symlink", func(t *testing.T) {
		if err := os.Symlink(child, link); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink creation requires Developer Mode or privilege: %v", err)
			}
			t.Fatal(err)
		}
		run("-ap", link, "extra")
	})
	run("-rp", child, "extra")
	file, _ := registryPath()
	r, err := load(file)
	if err != nil || len(r.Entries) != 2 {
		t.Fatal(r, err)
	}
	run("-dp", parent)
	if got := run("__query", "--", "work", "api"); strings.Contains(got, `"path":`) {
		t.Fatal(got)
	}
	if err := os.RemoveAll(child); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Execute([]string{"--doctor"}, strings.NewReader(""), &out, "dev"); err == nil {
		t.Fatal("doctor accepted stale path")
	}
	run("-dp", real)
}

func TestCorruptionAndAtomicWrites(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cdt", "registry.json")
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := update(file, func(r *Registry) error {
				r.Entries = append(r.Entries, Entry{filepath.Join(testPath("/test"), strings.Repeat("x", i+1)), []string{"tag"}})
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	r, err := load(file)
	if err != nil || len(r.Entries) != n {
		t.Fatal(len(r.Entries), err)
	}
	for _, bad := range []string{`null`, `{}`, `{"version":1}`, `{"entries":[]}`, `{broken`, `{"version":99,"entries":[]}`, `{"version":1,"entries":[]} {}`, `{"version":1,"entries":[{"path":"relative","tags":[]}]}`} {
		if err := os.WriteFile(file, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if err := update(file, func(r *Registry) error { r.Entries = nil; return nil }); err == nil {
			t.Fatal("overwrote corrupt data")
		}
		got, _ := os.ReadFile(file)
		if string(got) != bad {
			t.Fatal("corrupt data changed")
		}
	}
}

func TestHelpAndUninitialized(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-V"}, {"init", "zsh"}, {"init", "powershell"}} {
		var out bytes.Buffer
		if err := Execute(args, strings.NewReader(""), &out, "dev"); err != nil || out.Len() == 0 {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{{"init"}, {"init", "bash"}, {"init", "powershell", "extra"}} {
		if err := Execute(args, strings.NewReader(""), &bytes.Buffer{}, "dev"); err == nil {
			t.Fatal("accepted invalid initialization", args)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := Execute([]string{"api"}, strings.NewReader(""), &bytes.Buffer{}, "dev"); err == nil {
		t.Fatal("standalone jump must require shell")
	}
}
