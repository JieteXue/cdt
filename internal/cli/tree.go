package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// Only matching registrations are nodes. Missing intermediate directories are
// kept in the relative label rather than invented as untagged records.
func writeTree(out io.Writer, entries []Entry) error {
	entries = slices.Clone(entries)
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })
	index := make(map[string]int, len(entries))
	for i, e := range entries {
		index[e.Path] = i
	}
	children := make(map[int][]int)
	for i, e := range entries {
		parent := -1
		for p := filepath.Dir(e.Path); p != e.Path; {
			if found, ok := index[p]; ok {
				parent = found
				break
			}
			next := filepath.Dir(p)
			if next == p {
				break
			}
			p = next
		}
		children[parent] = append(children[parent], i)
	}
	var visit func(int, int, string, bool) error
	visit = func(i, parent int, prefix string, last bool) error {
		e := entries[i]
		nextPrefix := ""
		connector := ""
		if parent >= 0 {
			relative, err := filepath.Rel(entries[parent].Path, e.Path)
			if err != nil {
				return err
			}
			e.Path = relative
			connector = "\u251c\u2500\u2500 "
			nextPrefix = prefix + "\u2502   "
			if last {
				connector = "\u2514\u2500\u2500 "
				nextPrefix = prefix + "    "
			}
		}
		if _, err := fmt.Fprintln(out, prefix+connector+display(e)); err != nil {
			return err
		}
		for n, child := range children[i] {
			if err := visit(child, i, nextPrefix, n == len(children[i])-1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range children[-1] {
		if err := visit(root, -1, "", true); err != nil {
			return err
		}
	}
	return nil
}
