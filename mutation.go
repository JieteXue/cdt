package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// Build feedback under the same lock as the change, but publish it only after
// persistence succeeds. A concurrent writer cannot make the report misleading.
func mutate(file, path string, o Options, out io.Writer) error {
	var report strings.Builder
	err := withRegistryLock(file, func() error {
		r, err := load(file)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(r.Entries, func(e Entry) bool { return e.Path == path })
		if index < 0 && o.Op != "add" {
			return fmt.Errorf("directory is not registered: %q", path)
		}
		before := []string{}
		if index >= 0 {
			before = unique(r.Entries[index].Tags)
		}
		if o.Op == "delete" {
			r.Entries = slices.Delete(r.Entries, index, index+1)
			fmt.Fprintf(&report, "cdt: unregistered %q\n  Removed tags (own): %s\n  Directory and child registrations were not deleted.\n", path, tagList(before))
			return save(file, r)
		}
		requested := unique(o.Tags)
		repeated := []string{}
		seen := map[string]bool{}
		for _, tag := range o.Tags {
			if seen[tag] {
				repeated = append(repeated, tag)
			}
			seen[tag] = true
		}
		changed, skipped := []string{}, []string{}
		after := slices.Clone(before)
		for _, tag := range requested {
			present := slices.Contains(before, tag)
			if (o.Op == "add" && !present) || (o.Op == "remove" && present) {
				changed = append(changed, tag)
				if o.Op == "add" {
					after = append(after, tag)
				} else {
					after = slices.DeleteFunc(after, func(t string) bool { return t == tag })
				}
			} else {
				skipped = append(skipped, tag)
			}
		}
		after = unique(after)
		status := "unchanged"
		if index < 0 {
			status = "registered"
			r.Entries = append(r.Entries, Entry{path, after})
		} else if len(changed) > 0 {
			status = "updated"
			r.Entries[index].Tags = after
		}
		fmt.Fprintf(&report, "cdt: %s %q\n", status, path)
		verb, skipLabel := "Added", "Already present (skipped)"
		if o.Op == "remove" {
			verb, skipLabel = "Removed", "Not set here (skipped)"
		}
		fmt.Fprintf(&report, "  %s: %s\n", verb, tagList(changed))
		if len(skipped) > 0 {
			fmt.Fprintf(&report, "  %s: %s\n", skipLabel, tagList(skipped))
		}
		if len(repeated) > 0 {
			fmt.Fprintf(&report, "  Repeated input (ignored): %s\n", tagList(unique(repeated)))
		}
		fmt.Fprintf(&report, "  Previous (own): %s\n  Current (own): %s\n", tagList(before), tagList(after))
		if status == "unchanged" {
			return nil
		}
		return save(file, r)
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, report.String())
	return err
}

func tagList(tags []string) string {
	return "[" + strings.Join(tags, ", ") + "]"
}
