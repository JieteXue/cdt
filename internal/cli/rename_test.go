package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRenameTagOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--rename-tag", "old", "新", "--dry-run"},
		{"--dry-run", "old", "--rename-tag", "新"},
		{"--rename-tag", "--dry-run", "--", "-old", "--new"},
	} {
		got, err := parse(args)
		if err != nil || got.Op != "rename-tag" || !got.DryRun || len(got.Tags) != 2 {
			t.Fatal(got, err)
		}
	}
	for _, args := range [][]string{
		{"--rename-tag"}, {"--rename-tag", "old"}, {"--rename-tag", "old", "new", "extra"},
		{"--rename-tag", "", "new"}, {"--rename-tag", "old", "bad tag"},
		{"--rename-tag", "old", "bad\n"}, {"--rename-tag", "old", string([]byte{0xff})},
		{"--rename-tag=old", "new"}, {"--rename-tag", "old", "new", "--recursive"},
		{"--rename-tag", "old", "new", "-p", "."},
	} {
		if _, err := parse(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	for _, op := range []string{"-a", "-d", "-r", "-l", "-e", "--doctor", "--tags", "--explain", "--move", "-h", "-V"} {
		for _, args := range [][]string{{op, "--rename-tag", "old", "new"}, {"--rename-tag", "old", "new", op}} {
			if _, err := parse(args); err == nil {
				t.Errorf("accepted conflicting operations %v", args)
			}
		}
	}
}

func TestPlanTagRename(t *testing.T) {
	root := moveTestRoot(t)
	child := filepath.Join(root, "child")
	stale := filepath.Join(root, "stale ' 中文")
	r := Registry{1, []Entry{
		{root, []string{"old", "old", "new", "keep"}},
		{child, []string{"leaf"}}, {stale, []string{"old"}},
		{filepath.Join(root, "other"), []string{"Old", "untouched", "untouched"}},
	}}
	before, _ := json.Marshal(r)
	next, changes, err := planTagRename(r, "old", "new")
	if err != nil || len(changes) != 2 || !changes[0].Merged || changes[1].Merged {
		t.Fatal(next, changes, err)
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("planning mutated the source registry")
	}
	if !reflect.DeepEqual(next.Entries[0].Tags, []string{"keep", "new"}) ||
		!reflect.DeepEqual(next.Entries[2].Tags, []string{"new"}) ||
		!reflect.DeepEqual(next.Entries[3].Tags, r.Entries[3].Tags) {
		t.Fatal("rename changed unrelated tags or retained duplicates", next)
	}
	if got := matches(next, []string{"new", "leaf"}, false); len(got) != 1 || got[0].Path != child ||
		!reflect.DeepEqual(got[0].Tags, []string{"leaf"}) {
		t.Fatal("inherited tag did not follow renamed parent", got)
	}
	if got := matches(next, []string{"old", "leaf"}, false); len(got) != 0 {
		t.Fatal("old inherited tag retained", got)
	}
	if _, changes, err := planTagRename(r, "old", "old"); err != nil || len(changes) != 0 {
		t.Fatal(changes, err)
	}
	for _, tags := range [][2]string{{"missing", "new"}, {"missing", "missing"}, {"old", ""}, {"old", "bad tag"}} {
		if _, _, err := planTagRename(r, tags[0], tags[1]); err == nil {
			t.Fatal("accepted invalid rename", tags)
		}
	}
	if next, _, err := planTagRename(r, "old", "中文'$(literal)"); err != nil ||
		!reflect.DeepEqual(next.Entries[2].Tags, []string{"中文'$(literal)"}) {
		t.Fatal(next, err)
	}
}

func TestRenameTagPreviewAndApply(t *testing.T) {
	root := moveTestRoot(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	file, _ := registryPath()
	makeMoveDirs(t, filepath.Dir(file))
	if err := save(file, Registry{1, []Entry{
		{root, []string{"old", "new"}}, {filepath.Join(root, "stale"), []string{"old", "keep"}},
	}}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(file)
	info, _ := os.Stat(file)
	var out bytes.Buffer
	if err := Execute([]string{"--rename-tag", "old", "new", "--dry-run"}, nil, &out, "dev"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"tag rename preview (2 records; registry unchanged)", "Previous (own): [new, old]",
		"Current (own): [new]", "Existing target tags merged: 1", filepath.Join(root, "stale")} {
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
	if err := Execute([]string{"--rename-tag", "old", "new"}, nil, &out, "dev"); err != nil ||
		!strings.Contains(out.String(), `renamed tag "old" -> "new" in 2 records`) {
		t.Fatal(out.String(), err)
	}
	r, err := load(file)
	if err != nil || len(matches(r, []string{"old"}, false)) != 0 || len(matches(r, []string{"new"}, false)) != 2 {
		t.Fatal(r, err)
	}
	info, _ = os.Stat(file)
	out.Reset()
	if err := renameTagRegistry(file, "new", "new", false, &out); err != nil || !strings.Contains(out.String(), "unchanged") {
		t.Fatal(out.String(), err)
	}
	afterInfo, _ = os.Stat(file)
	if !os.SameFile(info, afterInfo) || !info.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("no-op rewrote registry")
	}
	if err := renameTagRegistry(file, "new", "next", true, failedTreeWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}

func TestRenameTagRejectsWithoutWriting(t *testing.T) {
	root := moveTestRoot(t)
	file := moveTestRegistry(t, root, []Entry{{root, []string{"old"}}})
	for _, corrupt := range []bool{false, true} {
		if corrupt {
			if err := os.WriteFile(file, []byte("{broken"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		before, _ := os.ReadFile(file)
		for _, dry := range []bool{false, true} {
			var out bytes.Buffer
			if err := renameTagRegistry(file, "missing", "new", dry, &out); err == nil || out.Len() != 0 {
				t.Fatal("failed rename emitted success", out.String(), err)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("failed rename changed registry")
			}
		}
	}
	absent := filepath.Join(root, "absent", "registry.json")
	if err := renameTagRegistry(absent, "old", "new", true, io.Discard); err == nil {
		t.Fatal("accepted unknown source tag")
	}
	if _, err := os.Stat(filepath.Dir(absent)); !os.IsNotExist(err) {
		t.Fatal("preview created configuration", err)
	}
}

func TestRenameTagConcurrentWriters(t *testing.T) {
	root := moveTestRoot(t)
	file := moveTestRegistry(t, root, []Entry{{root, []string{"old"}}})
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			var err error
			if i == 0 {
				err = renameTagRegistry(file, "old", "new", false, io.Discard)
			} else {
				err = update(file, func(r *Registry) error {
					r.Entries = append(r.Entries, Entry{filepath.Join(root, fmt.Sprint(i)), []string{"keep"}})
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
	if err != nil || len(r.Entries) != 10 || len(matches(r, []string{"new"}, false)) != 1 {
		t.Fatal(r, err)
	}
}

func TestUnfilteredTagCompletion(t *testing.T) {
	root := moveTestRoot(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	var out bytes.Buffer
	if err := Execute([]string{"__tags"}, nil, &out, "dev"); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(out.Bytes(), &response); err != nil || response.Version != 1 || response.Entries == nil {
		t.Fatal(response, err)
	}
	if _, err := os.Stat(filepath.Join(root, "config")); !os.IsNotExist(err) {
		t.Fatal("completion created config", err)
	}
	file, _ := registryPath()
	makeMoveDirs(t, filepath.Dir(file))
	if err := save(file, Registry{1, []Entry{{filepath.Join(root, "stale"), []string{"中文", "api", "api", "API"}}}}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Execute([]string{"__tags", "--"}, nil, &out, "dev"); err != nil {
		t.Fatal(err)
	}
	want := Response{1, []Entry{{"API", []string{}}, {"api", []string{}}, {"中文", []string{}}}}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil || !reflect.DeepEqual(response, want) {
		t.Fatal(response, err)
	}
	for _, args := range [][]string{{"__tags", "api"}, {"__tags", "--rename-tag", "api", "new"}, {"__query", "--rename-tag", "api", "new"}} {
		out.Reset()
		if err := Execute(args, nil, &out, "dev"); err == nil || out.Len() != 0 {
			t.Fatal("accepted invalid internal command", args, err)
		}
	}
	if err := os.WriteFile(file, []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Execute([]string{"__tags"}, nil, &out, "dev"); err == nil || out.Len() != 0 {
		t.Fatal("completion accepted corrupt registry", err)
	}
}
