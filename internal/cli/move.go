package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type pathMove struct {
	From, To string
	Tags     []string
}

func samePath(a, b string) bool {
	relative, err := filepath.Rel(a, b)
	return err == nil && relative == "."
}

func sameDirectory(a, b string) bool {
	if samePath(a, b) {
		return true
	}
	first, err := os.Stat(a)
	if err != nil {
		return false
	}
	second, err := os.Stat(b)
	return err == nil && os.SameFile(first, second)
}

func moveIncludes(from, path string, recursive bool) bool {
	return samePath(from, path) || (recursive && ancestor(from, path))
}

// A stale path may still start with an existing symlink, such as /var on macOS.
// Resolve its existing prefix without requiring the moved directory to exist.
func resolveMissingPath(path string) (string, error) {
	prefix := path
	tail := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(prefix)
		if err == nil {
			parts := []string{resolved}
			for i := len(tail) - 1; i >= 0; i-- {
				parts = append(parts, tail[i])
			}
			return filepath.Join(parts...), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(prefix)
		if parent == prefix {
			return "", err
		}
		tail = append(tail, filepath.Base(prefix))
		prefix = parent
	}
}

func moveSource(r Registry, path string, recursive bool) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !utf8.ValidString(abs) || strings.ContainsRune(abs, 0) {
		return "", errors.New("source path must be valid UTF-8 without NUL characters")
	}
	find := func(candidate string) (string, bool) {
		for _, entry := range r.Entries {
			if samePath(candidate, entry.Path) {
				return entry.Path, true
			}
		}
		if recursive {
			for _, entry := range r.Entries {
				if ancestor(candidate, entry.Path) {
					return candidate, true
				}
			}
		}
		return "", false
	}
	// Prefer a literal recorded path even when it has since become a symlink.
	if from, found := find(abs); found {
		return from, nil
	}
	resolved, err := resolveMissingPath(abs)
	if err != nil {
		return "", err
	}
	if from, found := find(resolved); found {
		return from, nil
	}
	return "", fmt.Errorf("no registered source paths: %q", abs)
}

func planMove(r Registry, o Options) (Registry, []pathMove, int, error) {
	from, err := moveSource(r, o.MoveFrom, o.Recursive)
	if err != nil {
		return r, nil, 0, err
	}
	to, err := canonical(o.MoveTo, true)
	if err != nil {
		return r, nil, 0, fmt.Errorf("invalid destination: %w", err)
	}
	if o.Recursive && !samePath(from, to) && (ancestor(from, to) || ancestor(to, from)) {
		return r, nil, 0, errors.New("recursive source and destination must not overlap")
	}
	selected := map[int]bool{}
	left := 0
	for i, entry := range r.Entries {
		if moveIncludes(from, entry.Path, o.Recursive) {
			selected[i] = true
		} else if ancestor(from, entry.Path) {
			left++
		}
	}
	next := Registry{r.Version, append([]Entry(nil), r.Entries...)}
	changes := []pathMove{}
	for i, entry := range r.Entries {
		if !selected[i] || samePath(from, to) {
			continue
		}
		relative, err := filepath.Rel(from, entry.Path)
		if err != nil {
			return r, nil, 0, err
		}
		target, err := canonical(filepath.Join(to, relative), true)
		if err != nil {
			return r, nil, 0, fmt.Errorf("cannot relocate %q: %w", entry.Path, err)
		}
		if o.Recursive && !moveIncludes(to, target, true) {
			return r, nil, 0, fmt.Errorf("target %q resolves outside destination subtree %q", target, to)
		}
		for j, existing := range r.Entries {
			if !selected[j] && sameDirectory(target, existing.Path) {
				return r, nil, 0, fmt.Errorf("destination is already registered: %q; records left unchanged", target)
			}
		}
		for _, change := range changes {
			if sameDirectory(target, change.To) {
				return r, nil, 0, fmt.Errorf("multiple records resolve to destination %q; records left unchanged", target)
			}
		}
		next.Entries[i].Path = target
		changes = append(changes, pathMove{entry.Path, target, unique(entry.Tags)})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].From < changes[j].From })
	return next, changes, left, nil
}

func moveRegistry(file string, o Options, out io.Writer) error {
	var report strings.Builder
	run := func() error {
		r, err := load(file)
		if err != nil {
			return err
		}
		next, changes, left, err := planMove(r, o)
		if err != nil {
			return err
		}
		switch {
		case o.DryRun:
			fmt.Fprintf(&report, "cdt: migration preview (%d records; registry unchanged)\n", len(changes))
		case len(changes) == 0:
			fmt.Fprintln(&report, "cdt: unchanged; source and destination refer to the same directory")
		default:
			if err := save(file, next); err != nil {
				return err
			}
			fmt.Fprintf(&report, "cdt: migrated %d records\n", len(changes))
		}
		for _, change := range changes {
			fmt.Fprintf(&report, "  %q -> %q\n    Own tags: %s\n", change.From, change.To, tagList(change.Tags))
		}
		if left > 0 {
			fmt.Fprintf(&report, "  Registered descendants left unchanged: %d (use --recursive to include them).\n", left)
		}
		fmt.Fprintln(&report, "  Directory contents were not moved; current directory unchanged.")
		return nil
	}
	if o.DryRun {
		if err := run(); err != nil {
			return err
		}
	} else if err := withRegistryLock(file, run); err != nil {
		return err
	}
	_, err := io.WriteString(out, report.String())
	return err
}
