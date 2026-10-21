package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsConfigLocation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", t.TempDir())
	got, err := registryPath()
	want := filepath.Join(os.Getenv("APPDATA"), "cdt", "registry.json")
	if err != nil || got != want {
		t.Fatal(got, want, err)
	}
}

func TestWindowsPaths(t *testing.T) {
	for _, tc := range []struct {
		parent, child string
		want          bool
	}{
		{`C:\work`, `C:\work\api`, true},
		{`C:\work`, `C:\workspace\api`, false},
		{`C:\work`, `D:\work\api`, false},
		{`\\server\share\work`, `\\server\share\work\api`, true},
		{`\\server\share\work`, `\\server\other\work\api`, false},
	} {
		if got := ancestor(tc.parent, tc.child); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	dir := filepath.Join(t.TempDir(), "MixedCase")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	normal, err := canonical(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	upper, err := canonical(strings.ToUpper(dir), true)
	if err != nil || normal != upper {
		t.Fatal("case aliases were not canonicalized", normal, upper, err)
	}
}
