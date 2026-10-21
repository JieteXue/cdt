// Package cli implements cdt commands and registry operations.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"unicode"

	"github.com/JieteXue/cdt/shell"
)

const help = `cdt - directory tags for zsh and PowerShell 7

Usage:
  cdt TAG...                    Jump by unordered tags
  cdt -a [-p PATH] [TAG...]      Register a directory / append tags
  cdt -d [-p PATH]               Unregister a directory (never deletes files)
  cdt -r [-p PATH] TAG...        Remove explicit tags
  cdt -l [TAG...]               List records as a tree (including stale paths)
  cdt -e, --edit                Edit registry with $VISUAL / $EDITOR / system editor
  cdt --doctor                  Check registry and recorded paths
  cdt --tags                    List tags and direct directory counts
  cdt --explain TAG...          Explain matches without jumping
  cdt --move OLD NEW            Relocate a registration (does not move files)
    --recursive                 Include registered descendants
    --dry-run                   Preview relocation without changing records
  cdt init zsh                  Print shell integration
  cdt init powershell           Print PowerShell 7 integration
  cdt -h, --help                Show help
  cdt -V, --version             Show version

Options: -a/--add, -d/--delete, -r/--remove, -l/--list, -e/--edit, -p/--path
Short options combine: cdt -ap PATH TAG...; cdt -dp PATH
Use -- before tags beginning with '-'.
Enable jumping: eval "$(command cdt init zsh)"
PowerShell 7: cdt.exe init powershell | Out-String | Invoke-Expression
`

type Options struct {
	Op, Path          string
	Tags              []string
	PathSet           bool
	MoveFrom          string
	MoveTo            string
	Recursive, DryRun bool
}

func parse(args []string) (Options, error) {
	o := Options{Op: "jump", Path: ".", Tags: []string{}}
	setOp := func(op string) error {
		if o.Op != "jump" && o.Op != op {
			return errors.New("operation options are mutually exclusive")
		}
		o.Op = op
		return nil
	}
	end := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if end || a == "-" || !strings.HasPrefix(a, "-") {
			o.Tags = append(o.Tags, a)
			continue
		}
		if a == "--" {
			end = true
			continue
		}
		if strings.HasPrefix(a, "--") {
			name, val, attached := strings.Cut(a[2:], "=")
			if name == "path" {
				if !attached {
					i++
					if i >= len(args) {
						return o, errors.New("--path requires a path")
					}
					val = args[i]
				}
				if val == "" {
					return o, errors.New("path cannot be empty")
				}
				o.Path, o.PathSet = val, true
				continue
			}
			if attached {
				return o, fmt.Errorf("--%s does not take a value", name)
			}
			if name == "recursive" {
				o.Recursive = true
				continue
			}
			if name == "dry-run" {
				o.DryRun = true
				continue
			}
			op, ok := map[string]string{"add": "add", "delete": "delete", "remove": "remove", "list": "list", "edit": "edit", "doctor": "doctor", "tags": "tags", "explain": "explain", "move": "move", "help": "help", "version": "version"}[name]
			if !ok {
				return o, fmt.Errorf("unknown option --%s", name)
			}
			if err := setOp(op); err != nil {
				return o, err
			}
			continue
		}
		for j := 1; j < len(a); j++ {
			if a[j] == 'p' {
				val := a[j+1:]
				if val == "" {
					i++
					if i >= len(args) {
						return o, errors.New("-p requires a path")
					}
					val = args[i]
				}
				if val == "" {
					return o, errors.New("path cannot be empty")
				}
				o.Path, o.PathSet = val, true
				break
			}
			op, ok := map[byte]string{'a': "add", 'd': "delete", 'r': "remove", 'l': "list", 'e': "edit", 'h': "help", 'V': "version"}[a[j]]
			if !ok {
				return o, fmt.Errorf("unknown option -%c", a[j])
			}
			if err := setOp(op); err != nil {
				return o, err
			}
		}
	}
	if o.PathSet && o.Op != "add" && o.Op != "delete" && o.Op != "remove" {
		return o, errors.New("--path requires --add, --delete or --remove")
	}
	if o.Op != "move" && (o.Recursive || o.DryRun) {
		return o, errors.New("--recursive and --dry-run require --move")
	}
	if o.Op == "move" {
		if len(o.Tags) != 2 || o.Tags[0] == "" || o.Tags[1] == "" {
			return o, errors.New("--move requires exactly two nonempty paths: OLD NEW")
		}
		o.MoveFrom, o.MoveTo = o.Tags[0], o.Tags[1]
		o.Tags = []string{}
		return o, nil
	}
	if (o.Op == "delete" || o.Op == "edit" || o.Op == "doctor" || o.Op == "tags" || o.Op == "help" || o.Op == "version") && len(o.Tags) > 0 {
		return o, errors.New("this operation does not accept tags")
	}
	if o.Op == "remove" && len(o.Tags) == 0 {
		return o, errors.New("--remove requires tags")
	}
	if o.Op == "explain" && len(o.Tags) == 0 {
		return o, errors.New("--explain requires tags")
	}
	return o, validTags(o.Tags)
}

type Response struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Execute runs a command without exiting the process.
func Execute(args []string, in io.Reader, out io.Writer, version string) error {
	if len(args) > 0 && args[0] == "init" {
		if len(args) != 2 || (args[1] != "zsh" && args[1] != "powershell") {
			return errors.New("usage: cdt init zsh|powershell")
		}
		integration := shell.Zsh
		if args[1] == "powershell" {
			integration = shell.PowerShell
		}
		_, err := io.WriteString(out, integration)
		return err
	}
	if len(args) > 0 && args[0] == "__decode" {
		var r Response
		dec := json.NewDecoder(in)
		if err := dec.Decode(&r); err != nil {
			return err
		}
		if r.Version != 1 {
			return errors.New("unsupported shell protocol")
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return errors.New("invalid shell protocol")
		}
		for _, e := range r.Entries {
			if strings.ContainsRune(e.Path, 0) {
				return errors.New("invalid shell path")
			}
			// NUL framing preserves whitespace, newlines and shell metacharacters.
			fmt.Fprintf(out, "%s\x00%s\x00", e.Path, display(e))
		}
		return nil
	}
	internal := ""
	if len(args) > 0 && (args[0] == "__query" || args[0] == "__complete" || args[0] == "__paths") {
		internal, args = args[0], args[1:]
	}
	o, err := parse(args)
	if err != nil {
		return err
	}
	if internal == "" {
		if o.Op == "help" || len(args) == 0 {
			_, err := io.WriteString(out, help)
			return err
		}
		if o.Op == "version" {
			fmt.Fprintln(out, "cdt "+version)
			return nil
		}
	}
	file, err := registryPath()
	if err != nil {
		return err
	}
	if internal != "" {
		if o.Op != "jump" {
			return errors.New("query only accepts tags")
		}
		if internal == "__paths" && len(o.Tags) > 0 {
			return errors.New("path completion does not accept tags")
		}
		r, err := load(file)
		if err != nil {
			return err
		}
		if internal == "__paths" {
			return json.NewEncoder(out).Encode(Response{1, matches(r, nil, false)})
		}
		if internal == "__complete" {
			// Suggest any effective tag that can still produce a direct match.
			tags := []string{}
			for _, e := range r.Entries {
				tags = append(tags, e.Tags...)
			}
			entries := []Entry{}
			for _, t := range unique(tags) {
				if len(matches(r, append(append([]string{}, o.Tags...), t), true)) > 0 {
					entries = append(entries, Entry{Path: t, Tags: []string{}})
				}
			}
			return json.NewEncoder(out).Encode(Response{1, entries})
		}
		return json.NewEncoder(out).Encode(Response{1, matches(r, o.Tags, true)})
	}
	if o.Op == "edit" {
		fmt.Fprintf(out, "Editing registry: %q\n", file)
		if err := editRegistry(file, func(draft string) error {
			return launchEditor(draft, in, out)
		}); err != nil {
			return err
		}
		fmt.Fprintln(out, "Registry saved.")
		return nil
	}
	if o.Op == "move" {
		return moveRegistry(file, o, out)
	}
	if o.Op == "add" || o.Op == "delete" || o.Op == "remove" {
		path, err := canonical(o.Path, o.Op == "add")
		if err != nil {
			return err
		}
		return mutate(file, path, o, out)
	}
	r, err := load(file)
	if err != nil {
		return err
	}
	if o.Op == "doctor" {
		bad := false
		for _, e := range r.Entries {
			info, err := os.Stat(e.Path)
			if err != nil || !info.IsDir() {
				fmt.Fprintf(out, "invalid: %q\n", e.Path)
				bad = true
			}
		}
		fmt.Fprintf(out, "registry: %s (%d records)\n", file, len(r.Entries))
		if runtime.GOOS == "windows" {
			fmt.Fprintln(out, "shell: run 'Get-Command cdt' in PowerShell; it should report a function")
		} else {
			fmt.Fprintln(out, "shell: run 'whence -w cdt' in zsh; it should report a function")
		}
		if bad {
			return errors.New("invalid recorded paths; relocate with cdt --move OLD NEW or remove with cdt -dp PATH")
		}
		return nil
	}
	if o.Op == "list" {
		return writeTree(out, matches(r, o.Tags, false))
	}
	if o.Op == "tags" {
		return writeTags(out, r)
	}
	if o.Op == "explain" {
		return writeExplanation(out, r, o.Tags)
	}
	if runtime.GOOS == "windows" {
		return errors.New("jumping requires PowerShell integration: cdt.exe init powershell | Out-String | Invoke-Expression")
	}
	return errors.New("jumping requires zsh integration: eval \"$(command cdt init zsh)\"")
}

func display(e Entry) string {
	p := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '\uFFFD'
		}
		return r
	}, e.Path)
	return p + "  [" + strings.Join(e.Tags, ", ") + "]"
}
