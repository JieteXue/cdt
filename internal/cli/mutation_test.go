package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMutationFeedback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := t.TempDir()
	run := func(want []string, args ...string) {
		t.Helper()
		var out bytes.Buffer
		if err := Execute(args, strings.NewReader(""), &out, "dev"); err != nil {
			t.Fatal(err)
		}
		for _, text := range want {
			if !strings.Contains(out.String(), text) {
				t.Fatalf("missing %q in %s", text, &out)
			}
		}
	}
	run([]string{"registered", "Added: [api]", "Previous (own): []", "Repeated input (ignored): [api]"}, "-ap", path, "api", "api")
	run([]string{"updated", "Added: [work]", "Already present (skipped): [api]", "Previous (own): [api]", "Current (own): [api, work]"}, "-ap", path, "work", "api")
	file, _ := registryPath()
	before, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	run([]string{"unchanged", "Added: []", "Already present (skipped): [api]"}, "-ap", path, "api")
	after, _ := os.Stat(file)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("no-op replaced registry")
	}
	run([]string{"Removed: [api]", "Not set here (skipped): [missing]", "Previous (own): [api, work]", "Current (own): [work]"}, "-rp", path, "api", "missing")
	before, _ = os.Stat(file)
	run([]string{"unchanged", "Removed: []", "Not set here (skipped): [api]"}, "-rp", path, "api")
	after, _ = os.Stat(file)
	if !os.SameFile(before, after) {
		t.Fatal("no-op removal replaced registry")
	}
	child := filepath.Join(path, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	run([]string{"registered"}, "-ap", child, "child")
	run([]string{"unregistered", "Removed tags (own): [work]", "were not deleted"}, "-dp", path)
	r, err := load(file)
	if err != nil || len(r.Entries) != 1 || r.Entries[0].Tags[0] != "child" {
		t.Fatalf("child registration lost: %+v, %v", r, err)
	}
	if _, err := os.Stat(child); err != nil {
		t.Fatal(err)
	}
}

func TestMutationInheritedTagAndFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for _, args := range [][]string{{"-ap", parent, "work"}, {"-ap", child, "api"}, {"-ap", child, "work"}} {
		out.Reset()
		if err := Execute(args, strings.NewReader(""), &out, "dev"); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(out.String(), "Added: [work]") {
		t.Fatal("inherited tag incorrectly treated as own duplicate:", &out)
	}
	file, _ := registryPath()
	if err := os.WriteFile(file, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Execute([]string{"-ap", child, "new"}, strings.NewReader(""), &out, "dev"); err == nil || out.Len() != 0 {
		t.Fatalf("failure printed success: %q, %v", &out, err)
	}
}
