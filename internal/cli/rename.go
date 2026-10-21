package cli

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

type tagRename struct {
	Path          string
	Before, After []string
	Merged        bool
}

func planTagRename(r Registry, from, to string) (Registry, []tagRename, error) {
	if err := validTags([]string{from, to}); err != nil {
		return r, nil, err
	}
	next := Registry{r.Version, append([]Entry(nil), r.Entries...)}
	changes := []tagRename{}
	found := false
	for i, entry := range r.Entries {
		if !slices.Contains(entry.Tags, from) {
			continue
		}
		found = true
		if from == to {
			continue
		}
		tags := append([]string(nil), entry.Tags...)
		for j, tag := range tags {
			if tag == from {
				tags[j] = to
			}
		}
		next.Entries[i].Tags = unique(tags)
		changes = append(changes, tagRename{
			entry.Path, unique(entry.Tags), next.Entries[i].Tags, slices.Contains(entry.Tags, to),
		})
	}
	if !found {
		return r, nil, fmt.Errorf("tag %q is not registered; records unchanged", from)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return next, changes, nil
}

func renameTagRegistry(file, from, to string, dryRun bool, out io.Writer) error {
	var report strings.Builder
	run := func() error {
		r, err := load(file)
		if err != nil {
			return err
		}
		next, changes, err := planTagRename(r, from, to)
		if err != nil {
			return err
		}
		switch {
		case dryRun:
			fmt.Fprintf(&report, "cdt: tag rename preview (%d records; registry unchanged)\n", len(changes))
		case len(changes) == 0:
			fmt.Fprintf(&report, "cdt: unchanged; tag %q already has this name\n", from)
		default:
			if err := save(file, next); err != nil {
				return err
			}
			fmt.Fprintf(&report, "cdt: renamed tag %q -> %q in %d records\n", from, to, len(changes))
		}
		merged := 0
		for _, change := range changes {
			fmt.Fprintf(&report, "  %q\n    Previous (own): %s\n    Current (own): %s\n",
				change.Path, tagList(change.Before), tagList(change.After))
			if change.Merged {
				merged++
			}
		}
		fmt.Fprintf(&report, "  Existing target tags merged: %d\n", merged)
		fmt.Fprintln(&report, "  Only own tags change; inherited tags follow automatically. Directory contents and current directory unchanged.")
		return nil
	}
	if dryRun {
		if err := run(); err != nil {
			return err
		}
	} else if err := withRegistryLock(file, run); err != nil {
		return err
	}
	_, err := io.WriteString(out, report.String())
	return err
}
