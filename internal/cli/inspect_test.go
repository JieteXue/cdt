package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestInspectionOptions(t *testing.T) {
	for _, tc := range []struct {
		args []string
		op   string
		tags []string
	}{
		{[]string{"--tags"}, "tags", []string{}},
		{[]string{"--explain", "work", "api"}, "explain", []string{"work", "api"}},
		{[]string{"api", "--explain", "work"}, "explain", []string{"api", "work"}},
		{[]string{"--explain", "--", "--tags"}, "explain", []string{"--tags"}},
	} {
		got, err := parse(tc.args)
		if err != nil || got.Op != tc.op || !reflect.DeepEqual(got.Tags, tc.tags) {
			t.Fatalf("%v: %+v, %v", tc.args, got, err)
		}
	}
	for _, args := range [][]string{
		{"--tags", "api"}, {"--tags=api"}, {"--explain"}, {"--explain=api"},
		{"--tags", "-p", "."}, {"--explain", "-p", ".", "api"},
		{"--explain", "bad tag"}, {"--explain", "--tags"},
	} {
		if _, err := parse(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	for _, op := range []string{"--tags", "--explain"} {
		for _, other := range []string{"-a", "-d", "-r", "-l", "-e", "--doctor", "-h", "-V"} {
			for _, args := range [][]string{{op, other}, {other, op}} {
				if _, err := parse(args); err == nil {
					t.Errorf("accepted conflicting operations %v", args)
				}
			}
		}
	}
}

func TestTagOverview(t *testing.T) {
	r := Registry{1, testEntries([]Entry{
		{"/work", []string{"work", "api", "api", "API", "中文"}},
		{"/work/api", []string{"api"}},
		{"/work/api/deep", []string{}},
		{"/missing", []string{"stale"}},
	})}
	var out bytes.Buffer
	if err := writeTags(&out, r); err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(out.String())
	want := []string{"TAG", "DIRECTORIES", "API", "1", "api", "2", "stale", "1", "work", "1", "中文", "1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v != %v", got, want)
	}
	for _, entries := range [][]Entry{nil, {{testPath("/empty"), []string{}}}} {
		out.Reset()
		if err := writeTags(&out, Registry{1, entries}); err != nil || out.String() != "No tags registered.\n" {
			t.Fatal(out.String(), err)
		}
	}
}

func TestMatchAssessment(t *testing.T) {
	r := Registry{1, testEntries([]Entry{
		{"/work", []string{"work", "shared"}},
		{"/work/team", []string{"team", "shared", "shared"}},
		{"/work/team/api", []string{"api", "shared"}},
		{"/workspace/api", []string{"api"}},
	})}
	entry := r.Entries[2]
	detail := evaluateMatch(r, entry, []string{"work", "team", "api", "api", "shared"})
	if !detail.Matches || !reflect.DeepEqual(detail.Own, []string{"api", "shared"}) ||
		!reflect.DeepEqual(detail.Inherited["work"], []string{testPath("/work")}) ||
		!reflect.DeepEqual(detail.Inherited["team"], []string{testPath("/work/team")}) ||
		len(detail.Inherited["shared"]) != 0 {
		t.Fatalf("%+v", detail)
	}
	detail = evaluateMatch(r, entry, []string{"work", "missing", "api"})
	if detail.Matches || !reflect.DeepEqual(detail.Missing, []string{"missing"}) {
		t.Fatalf("%+v", detail)
	}
	if evaluateMatch(r, entry, []string{"work"}).Matches {
		t.Fatal("inherited-only query matched")
	}
	if evaluateMatch(r, r.Entries[3], []string{"work", "api"}).Matches {
		t.Fatal("similar prefix treated as ancestor")
	}
	entry.Tags = []string{"api"}
	detail = evaluateMatch(r, entry, []string{"api", "shared"})
	if !detail.Matches || !reflect.DeepEqual(detail.Inherited["shared"], []string{testPath("/work"), testPath("/work/team")}) {
		t.Fatalf("multiple inheritance sources lost: %+v", detail)
	}
	for _, query := range [][]string{nil, {"api"}, {"work"}, {"team", "api"}, {"api", "work", "shared"}, {"absent"}} {
		for _, entry := range r.Entries {
			matched := false
			for _, candidate := range matches(r, query, false) {
				matched = matched || candidate.Path == entry.Path
			}
			if evaluateMatch(r, entry, query).Matches != matched {
				t.Fatalf("query/explanation mismatch for %s, %v", entry.Path, query)
			}
		}
	}
	// The same assessment must reflect parent removal immediately.
	r.Entries = r.Entries[1:]
	if evaluateMatch(r, entry, []string{"api", "work"}).Matches {
		t.Fatal("removed parent still supplies tags")
	}
}

func TestExplanation(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "work")
	child := filepath.Join(parent, "api ' [literal]")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := Registry{1, []Entry{
		{parent, []string{"work"}},
		{child, []string{"api", "中文"}},
		{filepath.Join(child, "missing"), []string{"api"}},
		{file, []string{"api", "work"}},
	}}
	var out bytes.Buffer
	if err := writeExplanation(&out, r, []string{"work", "api", "work"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Query: [api, work]", "\nMATCH " + strconv.Quote(child),
		"\nNO MATCH " + strconv.Quote(parent), "Own matches: [api]",
		"Inherited: work <- " + strconv.Quote(parent), "Missing: [api]",
		"\nUNAVAILABLE " + strconv.Quote(file), "registered path is not a directory",
		"\nUNAVAILABLE " + strconv.Quote(filepath.Join(child, "missing")), "directory unavailable:",
		"Matches: 1 of 4 registered directories.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, &out)
		}
	}
	out.Reset()
	if err := writeExplanation(&out, r, []string{"work"}); err != nil ||
		!strings.Contains(out.String(), "Reason: no query tag matches an own tag") ||
		strings.Contains(out.String(), "\nMATCH "+strconv.Quote(child)) {
		t.Fatal(out.String(), err)
	}
	out.Reset()
	if err := writeExplanation(&out, r, []string{"中文"}); err != nil ||
		!strings.Contains(out.String(), "Own matches: [中文]") {
		t.Fatal(out.String(), err)
	}
}

func TestInspectionIsReadOnly(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	file, err := registryPath()
	if err != nil {
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
	if got := run("--tags"); got != "No tags registered.\n" {
		t.Fatal(got)
	}
	if got := run("--explain", "absent"); !strings.Contains(got, "No registered directories.") {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Dir(file)); !os.IsNotExist(err) {
		t.Fatal("read-only commands created configuration", err)
	}
	run("-ap", root, "--", "--tags")
	before, _ := os.ReadFile(file)
	info, _ := os.Stat(file)
	run("--tags")
	if got := run("--explain", "--", "--tags"); !strings.Contains(got, "\nMATCH ") {
		t.Fatal(got)
	}
	after, _ := os.ReadFile(file)
	afterInfo, _ := os.Stat(file)
	if !bytes.Equal(before, after) || !info.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("inspection rewrote the registry")
	}
	const corrupt = "{broken"
	if err := os.WriteFile(file, []byte(corrupt), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--tags"}, {"--explain", "api"}, {"__query", "--tags"}, {"__complete", "--explain", "api"}} {
		var out bytes.Buffer
		if err := Execute(args, strings.NewReader(""), &out, "dev"); err == nil || out.Len() != 0 {
			t.Fatal("accepted corruption or invalid protocol query", args, err, out.String())
		}
	}
	after, _ = os.ReadFile(file)
	if string(after) != corrupt {
		t.Fatal("inspection changed corrupt registry")
	}
}

func TestInspectionWriterErrors(t *testing.T) {
	r := Registry{1, []Entry{{testPath("/work"), []string{"work"}}}}
	for _, entries := range [][]Entry{nil, r.Entries} {
		if err := writeTags(failedTreeWriter{}, Registry{1, entries}); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
		if err := writeExplanation(failedTreeWriter{}, Registry{1, entries}, []string{"work"}); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	}
}
