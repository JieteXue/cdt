package cli

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestTree(t *testing.T) {
	entries := testEntries([]Entry{
		{"/work/web", []string{"web"}},
		{"/workspace/api", []string{"api"}},
		{"/work/api/deep", []string{"deep"}},
		{"/work", []string{"work"}},
		{"/work/api", []string{"api"}},
		{"/work/services/中文 '", []string{"service"}},
	})
	var out bytes.Buffer
	if err := writeTree(&out, entries); err != nil {
		t.Fatal(err)
	}
	want := testPath("/work") + "  [work]\n" +
		"\u251c\u2500\u2500 api  [api]\n" +
		"\u2502   \u2514\u2500\u2500 deep  [deep]\n" +
		"\u251c\u2500\u2500 " + filepath.Join("services", "中文 '") + "  [service]\n" +
		"\u2514\u2500\u2500 web  [web]\n" +
		testPath("/workspace/api") + "  [api]\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", &out, want)
	}
	if entries[0].Path != testPath("/work/web") {
		t.Fatal("tree changed input order")
	}
	out.Reset()
	if err := writeTree(&out, matches(Registry{1, entries}, []string{"work", "api"}, false)); err != nil {
		t.Fatal(err)
	}
	if out.String() != testPath("/work/api")+"  [api]\n" {
		t.Fatalf("filtered tree leaked nonmatching nodes: %s", &out)
	}
}

func TestTreeRootEmptyAndControlCharacters(t *testing.T) {
	for _, tc := range []struct {
		entries []Entry
		want    string
	}{
		{nil, ""},
		{testEntries([]Entry{{"/", []string{}}, {"/a\nb", []string{"tag"}}}), testPath("/") + "  []\n\u2514\u2500\u2500 a\uFFFDb  [tag]\n"},
	} {
		var out bytes.Buffer
		if err := writeTree(&out, tc.entries); err != nil || out.String() != tc.want {
			t.Fatalf("%q != %q (%v)", &out, tc.want, err)
		}
	}
}

func TestListTree(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	file, _ := registryPath()
	if err := update(file, func(r *Registry) error {
		r.Entries = testEntries([]Entry{{"/missing", []string{"parent"}}, {"/missing/child", []string{"child"}}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Execute([]string{"-l"}, strings.NewReader(""), &out, "dev"); err != nil {
		t.Fatal(err)
	}
	if out.String() != testPath("/missing")+"  [parent]\n\u2514\u2500\u2500 child  [child]\n" {
		t.Fatal("stale paths missing or inherited tags shown:", &out)
	}
}

type failedTreeWriter struct{}

func (failedTreeWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestTreeWriteFailure(t *testing.T) {
	if err := writeTree(failedTreeWriter{}, []Entry{{"/a", []string{}}}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}
