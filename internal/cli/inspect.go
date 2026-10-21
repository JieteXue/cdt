package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

type tagMatch struct {
	Own       []string
	Inherited map[string][]string
	Missing   []string
	Matches   bool
}

// Both queries and explanations use the same tag assessment. An explicit tag
// takes precedence over inheritance; inherited sources retain their paths.
func evaluateMatch(r Registry, e Entry, tags []string) tagMatch {
	result := tagMatch{Inherited: map[string][]string{}}
	own := map[string]bool{}
	for _, tag := range e.Tags {
		own[tag] = true
	}
	wanted := map[string]bool{}
	for _, tag := range unique(tags) {
		if own[tag] {
			result.Own = append(result.Own, tag)
		} else {
			wanted[tag] = true
		}
	}
	if len(wanted) == 0 {
		result.Matches = true
		return result
	}
	for _, parent := range r.Entries {
		if !ancestor(parent.Path, e.Path) {
			continue
		}
		for _, tag := range unique(parent.Tags) {
			if wanted[tag] {
				result.Inherited[tag] = append(result.Inherited[tag], parent.Path)
			}
		}
	}
	for tag := range wanted {
		if len(result.Inherited[tag]) == 0 {
			result.Missing = append(result.Missing, tag)
		} else {
			sort.Strings(result.Inherited[tag])
		}
	}
	sort.Strings(result.Missing)
	result.Matches = len(result.Missing) == 0 && (len(tags) == 0 || len(result.Own) > 0)
	return result
}

func writeTags(out io.Writer, r Registry) error {
	counts := map[string]int{}
	tags := []string{}
	for _, entry := range r.Entries {
		for _, tag := range unique(entry.Tags) {
			if counts[tag] == 0 {
				tags = append(tags, tag)
			}
			counts[tag]++
		}
	}
	if len(tags) == 0 {
		_, err := fmt.Fprintln(out, "No tags registered.")
		return err
	}
	sort.Strings(tags)
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "TAG\tDIRECTORIES")
	for _, tag := range tags {
		fmt.Fprintf(table, "%s\t%d\n", tag, counts[tag])
	}
	return table.Flush()
}

func writeExplanation(out io.Writer, r Registry, tags []string) error {
	var report strings.Builder
	tags = unique(tags)
	fmt.Fprintf(&report, "Query: [%s]\n", strings.Join(tags, ", "))
	fmt.Fprintln(&report, "Rule: all query tags must match, including at least one own tag.")
	entries := append([]Entry(nil), r.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	matched := 0
	for _, entry := range entries {
		detail := evaluateMatch(r, entry, tags)
		status, unavailable := "NO MATCH", ""
		if detail.Matches {
			info, err := os.Stat(entry.Path)
			switch {
			case err != nil:
				status, unavailable = "UNAVAILABLE", fmt.Sprintf("directory unavailable: %q", err.Error())
			case !info.IsDir():
				status, unavailable = "UNAVAILABLE", "registered path is not a directory"
			default:
				status = "MATCH"
				matched++
			}
		}
		fmt.Fprintf(&report, "\n%s %q\n", status, entry.Path)
		fmt.Fprintf(&report, "  Own matches: [%s]\n", strings.Join(detail.Own, ", "))
		for _, tag := range tags {
			if sources := detail.Inherited[tag]; len(sources) > 0 {
				quoted := make([]string, len(sources))
				for i, source := range sources {
					quoted[i] = fmt.Sprintf("%q", source)
				}
				fmt.Fprintf(&report, "  Inherited: %s <- %s\n", tag, strings.Join(quoted, ", "))
			}
		}
		if len(detail.Missing) > 0 {
			fmt.Fprintf(&report, "  Missing: [%s]\n", strings.Join(detail.Missing, ", "))
		}
		if len(detail.Own) == 0 {
			fmt.Fprintln(&report, "  Reason: no query tag matches an own tag")
		}
		if unavailable != "" {
			fmt.Fprintln(&report, "  Reason: "+unavailable)
		}
	}
	if len(entries) == 0 {
		fmt.Fprintln(&report, "\nNo registered directories.")
	}
	fmt.Fprintf(&report, "\nMatches: %d of %d registered directories.\n", matched, len(entries))
	_, err := io.WriteString(out, report.String())
	return err
}
