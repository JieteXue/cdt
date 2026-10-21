package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

var version = "dev"

//go:embed shell/cdt.zsh
var shell string

const help = `cdt - directory tags for zsh

Usage:
  cdt TAG...                    Jump by unordered tags
  cdt -a [-p PATH] [TAG...]      Register a directory / append tags
  cdt -d [-p PATH]               Unregister a directory (never deletes files)
  cdt -r [-p PATH] TAG...        Remove explicit tags
  cdt -l [TAG...]               List records (including stale paths)
  cdt --doctor                  Check registry and recorded paths
  cdt init zsh                  Print shell integration
  cdt -h, --help                Show help
  cdt -V, --version             Show version

Options: -a/--add, -d/--delete, -r/--remove, -l/--list, -p/--path
Short options combine: cdt -ap PATH TAG...; cdt -dp PATH
Use -- before tags beginning with '-'.
Enable jumping: eval "$(command cdt init zsh)"
`

type Options struct {
	Op, Path string
	Tags     []string
	PathSet  bool
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
			op, ok := map[string]string{"add": "add", "delete": "delete", "remove": "remove", "list": "list", "doctor": "doctor", "help": "help", "version": "version"}[name]
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
			op, ok := map[byte]string{'a': "add", 'd': "delete", 'r': "remove", 'l': "list", 'h': "help", 'V': "version"}[a[j]]
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
	if (o.Op == "delete" || o.Op == "doctor" || o.Op == "help" || o.Op == "version") && len(o.Tags) > 0 {
		return o, errors.New("this operation does not accept tags")
	}
	if o.Op == "remove" && len(o.Tags) == 0 {
		return o, errors.New("--remove requires tags")
	}
	return o, validTags(o.Tags)
}

type Response struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

func execute(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 2 && args[0] == "init" && args[1] == "zsh" {
		_, err := io.WriteString(out, shell)
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
	if len(args) > 0 && (args[0] == "__query" || args[0] == "__complete") {
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
		r, err := load(file)
		if err != nil {
			return err
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
	if o.Op == "add" || o.Op == "delete" || o.Op == "remove" {
		path, err := canonical(o.Path, o.Op == "add")
		if err != nil {
			return err
		}
		return update(file, func(r *Registry) error {
			index := -1
			for i, e := range r.Entries {
				if e.Path == path {
					index = i
					break
				}
			}
			if o.Op == "add" {
				if index < 0 {
					r.Entries = append(r.Entries, Entry{path, unique(o.Tags)})
				} else {
					r.Entries[index].Tags = unique(append(r.Entries[index].Tags, o.Tags...))
				}
				return nil
			}
			if index < 0 {
				return fmt.Errorf("directory is not registered: %s", path)
			}
			if o.Op == "delete" {
				r.Entries = append(r.Entries[:index], r.Entries[index+1:]...)
				return nil
			}
			keep := []string{}
			for _, t := range r.Entries[index].Tags {
				remove := false
				for _, target := range o.Tags {
					if t == target {
						remove = true
					}
				}
				if !remove {
					keep = append(keep, t)
				}
			}
			r.Entries[index].Tags = keep
			return nil
		})
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
		fmt.Fprintln(out, "shell: run 'whence -w cdt' in zsh; it should report a function")
		if bad {
			return errors.New("invalid recorded paths; remove with cdt -dp PATH")
		}
		return nil
	}
	if o.Op == "list" {
		for _, e := range matches(r, o.Tags, false) {
			fmt.Fprintln(out, display(e))
		}
		return nil
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

func main() {
	if err := execute(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "cdt:", err)
		os.Exit(1)
	}
}
