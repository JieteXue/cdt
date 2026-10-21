package cli

import (
	"os"
	"path/filepath"
)

// Use native absolute paths without requiring these synthetic directories.
func testPath(path string) string {
	return filepath.Clean(filepath.VolumeName(os.TempDir()) + filepath.FromSlash(path))
}

func testEntries(entries []Entry) []Entry {
	for i := range entries {
		entries[i].Path = testPath(entries[i].Path)
	}
	return entries
}
