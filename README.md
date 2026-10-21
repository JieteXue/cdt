# cdt

**Jump to directories by tags, not long paths.**

cdt is a command-line directory bookmark tool for **macOS zsh** and **Windows
PowerShell 7**. Give a directory a few tags, then combine those tags to get back
to it from anywhere. You choose what to register: cdt does not learn from your
shell history or scan your directory tree.

## How it works

After [installing and enabling shell integration](#install), tag directories
that already exist:

```sh
cdt -ap ~/work work
cdt -ap ~/work/api api
cdt -ap ~/personal/api api

cdt work api       # jump straight to ~/work/api
cdt api work       # same result; tag order does not matter
cdt api            # choose between the two API directories
cdt work           # jump to ~/work, not every directory below it
```

The `api` directory under `~/work` inherits the registered parent's `work` tag.
Adding `work` to your query distinguishes it from your personal API directory,
without adding that tag to every child yourself. All query tags must match,
and at least one must belong to the directory itself. Matching is exact and
case-sensitive, not fuzzy.

A unique match changes directory immediately. Multiple matches open a native
shell selection menu; Tab/arrows select, Enter confirms, and Ctrl-G/Ctrl-C cancel.
Tab also completes tags and path arguments while you type.

## What you get

- **Explicit bookmarks:** register only the directories you want, with multiple
  tags per directory and inherited parent tags for context.
- **Visible results:** browse registrations as a tree with `cdt -l`, discover
  tags with `cdt --tags`, and diagnose matching with `cdt --explain`.
- **Editable local data:** open your tags with `cdt --edit`, rename tags globally,
  or repair recorded paths after moving a project. Preview bulk changes with
  `--dry-run`.
- **Small installation:** precompiled packages for Apple Silicon, Intel Macs,
  Windows x64, and Windows ARM64. No Go, Python, or fzf is needed at runtime.

Registration commands change cdt's local records, never directory contents.
Only a successful jump changes your shell's current directory.

[Install](#install) · [Usage](#usage) · [Data and safety](#data-and-safety) ·
[Upgrade](#upgrade) · [Development](#development)

## Install

### macOS

**Install the executable, then configure zsh once.** Homebrew does not modify
your startup files, so installing the package alone does not enable jumping.

```sh
brew install JieteXue/tap/cdt
```

Open your zsh startup file (create it if necessary):

```sh
nano "${ZDOTDIR:-$HOME}/.zshrc"
```

This is normally `~/.zshrc`. If you use `ZDOTDIR`, edit `.zshrc` in that directory
instead. Add the following block **once**, near the end of the file, after your
`PATH` and completion setup:

```zsh
# Enable cdt navigation and completion when installed.
if (( $+commands[cdt] )); then
  eval "$(command cdt init zsh)"
fi
```

For Oh My Zsh, put the block after `source "$ZSH/oh-my-zsh.sh"` and any later
`compinit` call. The guard avoids startup errors if cdt is later uninstalled.
Keep only one copy of the block.

Save the file, then **open a new terminal window or tab**. Future interactive zsh
sessions will load cdt automatically; no manual `eval` is needed each time.
To enable it immediately in the terminal you already have open, run this once:

```sh
eval "$(command cdt init zsh)"
```

Verify in the new terminal:

```sh
whence -w cdt
# Expected: cdt: function
cdt --version
```

If `whence` reports a command rather than a function, or jumping prints
`cdt: jumping requires zsh integration`, the initialization has not loaded in
that session. Check that the block is saved in the correct `.zshrc`, after
Homebrew's `bin` directory is added to `PATH`, and reopen the terminal.
`command cdt --version` should work before the block runs.

The shell function is needed because a standalone executable cannot change its
parent shell's directory. Running `eval` only at the prompt enables the current
session; saving the block in `.zshrc` makes it persistent.

Homebrew automatically downloads the precompiled macOS package for Apple Silicon
or Intel; installing cdt does not install Go or compile source code. Each release
also includes both architecture archives and a `checksums.txt` file for manual
verification. Only the package matching your Mac is needed.

### Windows

Install PowerShell 7, then download the matching ZIP from this repository's
GitHub Releases: `windows_amd64` for Intel/AMD x64, or `windows_arm64` for
Windows on ARM. Extract it into a permanent directory such as
`$HOME\Applications\cdt`, and add that directory to your user `PATH`.
Neither Go nor a compiler is needed. The ZIP includes `cdt.exe`, the license,
and documentation; initialization is embedded in the executable.

Verify the downloaded archive against the release's `checksums.txt`:

```powershell
Get-FileHash .\cdt_0.5.0_windows_amd64.zip -Algorithm SHA256
```

Configure automatic loading once in **PowerShell 7**. Create your profile if it
does not exist, then open it:

```powershell
if (-not (Test-Path -LiteralPath $PROFILE)) {
    New-Item -ItemType File -Path $PROFILE -Force | Out-Null
}
notepad $PROFILE
```

Add this block once, after any `PATH` and completion setup:

```powershell
if (Get-Command cdt.exe -CommandType Application -ErrorAction SilentlyContinue) {
    cdt.exe init powershell | Out-String | Invoke-Expression
}
```

Save the profile and open a new PowerShell session. `(Get-Command cdt).CommandType`
should print `Function`. To enable the current session immediately, run
`cdt.exe init powershell | Out-String | Invoke-Expression` once at the prompt.
Sessions started with `-NoProfile` do not load this configuration.

Installation does not edit your profile or execution policy. Windows PowerShell
5.1 and `cmd.exe` shell integration are not supported.

```powershell
cdt -ap 'C:\Projects\api' work api
cdt work api
cdt -l
```

Unique matches jump directly. Multiple matches open a numbered console menu:
Tab/Down move forward, Shift-Tab/Up move backward, Enter confirms, and
Escape/Ctrl-G/Ctrl-C cancel. Tag and `-p` directory completion use PowerShell's
normal completion system. The integration restores console Ctrl-C handling
after selection and does not install PSReadLine key bindings.

Check `$LASTEXITCODE` after a `cdt` function call: `0` means success, `1` means
failure, `2` means a noninteractive ambiguous query, and `130` means cancellation.
Successful noninteractive jumps are silent; failures leave the location unchanged.
PowerShell consumes a bare `--`, so the integration recovers it from syntax
without evaluating values. When splatting arguments, pass the marker as a quoted
string, for example `cdt @tags '--' -special`.

## Usage

```sh
cdt -a work                         # tag the current directory
cdt -ap ~/projects/api api backend  # register another directory and add tags
cdt work api                        # jump by unordered tags
cdt -l                             # show registered paths and own tags as a tree
cdt -l work api                     # list matching records without jumping
cdt -rp ~/projects/api backend      # remove an explicit tag
cdt -dp ~/projects/api              # unregister; never deletes the directory
cdt -e                             # edit tags in the registry file
cdt --doctor                       # report invalid recorded paths
cdt --tags                         # list tags and direct directory counts
cdt --explain work api             # explain matches without jumping
cdt --move OLD NEW --dry-run       # preview relocation of a directory record
cdt --move OLD NEW --recursive     # relocate records, not real directories
cdt --rename-tag api backend --dry-run  # preview a global tag rename
cdt --rename-tag api backend        # rename, merging existing target tags
```

`-ap PATH` equals `-a -p PATH` and `--add --path PATH`. Similarly, `-dp`
unregisters a specified path. `-p` consumes the rest of a short-option group or
the next argument: `-pa` means path `a`, not add. Without `-p`, mutations target
the current directory. `-a` with no tags registers a directory with no own tags.
`-r` requires tags. Operations, including `--tags` and `--explain`, are mutually
exclusive. `--tags` accepts no tags; `--explain` requires at least one.

Tags are exact, case-sensitive and unordered; Unicode is supported, whitespace
and control characters are not. Use `--` for tags beginning with `-`:

```sh
cdt -a -- -special
cdt -- -special
```

Mutations report the canonical path, tags added or removed, and previous/current
own tags. For example, adding `api backend api` to a directory already tagged
`api` reports:

```text
cdt: updated "/Users/you/projects/api"
  Added: [backend]
  Already present (skipped): [api]
  Repeated input (ignored): [api]
  Previous (own): [api]
  Current (own): [api, backend]
```

Duplicate tags have always been deduplicated. Repeated additions now explicitly
report `unchanged` and do not rewrite the registry. Removal reports tags not set
on that directory separately; inherited tags are not own tags. Unregistering
reports the removed own tags and preserves the directory and child records.
An interactive jump confirms the selected path and own tags; cancelling confirms
that the directory is unchanged. Successful noninteractive jumps remain silent,
and internal JSON responses contain no human-facing messages.

All query tags must match the path's own or inherited tags, and **at least one
must match an own tag**. With `/work` tagged `work`, `/work/api` tagged `api`,
and `/personal/api` also tagged `api`:

| Query | Results |
| --- | --- |
| `cdt work` | `/work` only |
| `cdt api` | both API directories |
| `cdt work api` | `/work/api` only |
| `cdt api work` | `/work/api` only |

Only registered paths are candidates. No directory tree is scanned. Parent tags
take effect even when the parent was registered later; removing a parent record
immediately removes its inherited tags without unregistering children.

### Inspect tags and matches

`cdt --tags` lists tags in case-sensitive sorted order and counts the registered
directories that explicitly own each tag. Repeated tags in a record count once;
inherited tags do not increase the counts. Stale records are included so forgotten
tags remain discoverable:

```text
TAG   DIRECTORIES
api   2
work  1
```

`cdt --explain work api` explains every registered record using the same matching
rules as jumping. It shows matching own tags, each inherited query tag and its
registered ancestor sources, and missing query tags. An inherited-only match is
excluded and says why. Tags owned by the record are shown as own matches even if
an ancestor also supplies them.

Records that satisfy the tag rules but are not usable directories are marked
`UNAVAILABLE`; they are excluded from the match count, just as for jumping.
Only tag-matching records are checked on disk; no directory tree is scanned.
An explanation succeeds even when no paths match: it is a diagnostic report,
not a jump. Repeated query tags are combined and displayed in sorted order.

Both commands are read-only, never change the shell's current directory, and do
not create a registry when one does not yet exist. Use
`cdt --explain -- -special` to inspect a tag beginning with `-`.

`cdt -l` groups records under their nearest registered ancestor. Tree roots show
absolute paths; descendants show paths relative to their displayed parent. Only
own tags appear, including `[]` for a record without own tags:

```text
/Users/you/projects  [work]
├── api  [api, backend]
│   └── tests  [test]
└── web  [web]
/Users/you/personal  [personal]
```

`cdt -l TAG...` builds the same tree from matching records only. An unmatched
parent is omitted, so its matching descendant becomes an absolute-path root.
Unregistered intermediate directories stay in a relative label such as
`services/api`; they are not extra records. Listing does not scan the filesystem
and continues to include stale paths for inspection.

A unique match jumps directly. Multiple matches open a native zsh completion
menu: Tab/arrows select, Enter confirms, Ctrl-G/Ctrl-C cancel. Only explicit tags
are displayed. Tab also completes tags while typing and paths after `-p`.
Invalid paths are excluded from jumping, but remain visible in `-l` and
`--doctor`. Noninteractive ambiguous queries fail without changing directory.
The standalone binary cannot change a parent shell's directory: enable the
zsh or PowerShell function before jumping.

## Data and safety

On macOS records are stored in
`${XDG_CONFIG_HOME:-~/.config}/cdt/registry.json`. On Windows the default is
`%APPDATA%\cdt\registry.json`; an absolute `XDG_CONFIG_HOME` overrides it.
`XDG_CONFIG_HOME`, when set, must be absolute. Only canonical real paths and
explicit tags are persisted (schema version 1). Symlink aliases share a record.
Spaces, quotes and shell metacharacters in paths are passed as data, never
evaluated. Mutations use a file lock and atomic replacement. Corrupt or
unsupported data is rejected without overwriting it.

Windows uses Win32 file locks and same-directory replacement, with readers
retaining their original snapshots. File access follows Windows ACLs inherited
from the configuration directory; POSIX `0600` permission bits apply only on Unix.
Keep registries local to each machine: drive paths and Unix paths are not
portable across platforms. UNC ancestry is supported, but network-share locking
and replacement guarantees depend on the remote filesystem.

### Edit tags in a file

```sh
cdt -e
cdt --edit                         # same command
EDITOR=nano cdt -e                 # use nano if VISUAL is not set
VISUAL="code --wait" cdt -e         # wait for a graphical editor to close
```

The editor is chosen from `$VISUAL`, then `$EDITOR`, then system `vi` on macOS
or `notepad.exe` on Windows. Editor flags are supported; use PowerShell command
syntax for Windows editor settings, such as `$env:VISUAL = 'code --wait'`.
The command shows the registry location and opens a private
JSON draft. Edit each entry's `tags` array; inherited tags are computed, so do not
copy them into child entries. For example:

```json
{
  "version": 1,
  "entries": [
    {
      "path": "/Users/you/projects/api",
      "tags": ["api", "backend"]
    }
  ]
}
```

Keep `version` as `1`, use absolute normalized real paths (not `~`), and keep
tags free of whitespace. Save and close the editor to apply the changes.
Duplicate tags are deduplicated. You can also repair a corrupt registry this way.

The registry is atomically replaced only after the editor exits successfully
and the draft passes validation. Invalid edits, editor failures, or concurrent
registry changes leave the current records untouched and retain the draft at
the path printed in the error. Other `cdt` commands are not blocked while you edit.

Removing the final explicit tag leaves the directory registered.

### Relocate directory records

After moving or renaming a real directory yourself, update its recorded path
without re-entering its tags:

```sh
cdt --move /old/projects /new/projects --recursive --dry-run
cdt --move /old/projects /new/projects --recursive
```

**This only changes registry records. It does not move, rename, create, or delete
real directories, and it never changes your shell's current directory.**

Without `--recursive`, only the source directory's record is changed; any
registered descendants stay at their original paths and are reported separately.
With `--recursive`, all registered records under the source prefix are relocated
by relative path. The prefix itself need not be registered in recursive mode.
Similar names such as `/projects-old` are not descendants of `/projects`.

The old path may be missing. The new path and every relocated directory must
already exist. Paths with spaces and Unicode are supported; quote them normally.
Use `--` before paths beginning with `-`. Existing symlink aliases resolve to
real destinations; a recursive child that resolves outside the destination
subtree is rejected.

`--dry-run` validates and prints the complete planned change without modifying
the registry or creating a writer lock. Run the same command without `--dry-run`
to apply it; the plan is recalculated against the latest records under the writer
lock. Own tags are preserved, and inherited tags follow the new ancestry.

Conflicts with existing destination registrations, multiple sources resolving
to the same target, missing child directories, and overlapping recursive prefixes
are rejected as a whole. Tags are never silently merged or overwritten. A
same-path migration is a no-op and does not rewrite the registry.
`--recursive` is only valid with `--move`; `-p` is not used.
Both shells complete the destination as a directory and can suggest recorded
source paths even after those directories have moved.

### Rename a tag

```sh
cdt --rename-tag api backend --dry-run
cdt --rename-tag api backend
```

Renaming changes the explicit tag on **all registered records**, including stale
paths. Each affected path shows its previous and current own tags. If a record
already owns the target tag, the two tags are merged without duplicates; the
report counts these merges. Other tags and registrations are preserved.
Inherited tags follow renamed ancestors automatically and are never copied into
child records. No real directories or shell locations are changed.

Tags remain exact and case-sensitive. An unknown source tag is an error; renaming
an existing tag to itself succeeds without rewriting the registry. Both shells
complete either operand from all registered tags, including stale-only tags.
New names need not already exist. Use `--` before names starting with `-`:
`cdt --rename-tag -- -old -new`.

`--dry-run` is valid with `--move` and `--rename-tag`. It prints a snapshot preview
without writing records or creating a lock; applying recalculates the change under
the writer lock and atomically replaces the registry once. `--rename-tag` takes
exactly two tags, is mutually exclusive with other operations, and does not accept
`-p` or `--recursive`.

The internal `__query`/`__complete`/`__paths`/`__tags` JSON protocol is versioned
separately; `__decode` converts it to NUL-framed records for zsh without an external parser.
These internal commands are not a user-facing API.

## Development

Go 1.24+ and Python 3 (only for PTY tests), plus zsh on macOS or PowerShell 7
on Windows:

```sh
go test -race ./...
go vet ./...
go build -o bin/cdt ./cmd/cdt
python3 tests/zsh_pty.py -v
```

On Windows:

```powershell
go test ./...
go vet ./...
go build -o bin/cdt.exe ./cmd/cdt
pwsh -NoProfile -NonInteractive -File tests/powershell.ps1
python tests/powershell_pty.py -v
```

CI tests Apple Silicon/Intel macOS and x64/ARM64 Windows natively. Race tests run
on macOS and Windows x64; Windows ARM64 runs ordinary Go tests. Windows menu
tests use a real ConPTY terminal, not a mocked key reader.
Go is needed only by contributors and release builders, not installed users.

### Repository layout

```text
cmd/cdt/         Executable entry point and release version
internal/cli/    Commands, registry, matching, editor, and unit tests
shell/           Embedded zsh and PowerShell integrations
tests/           Cross-platform shell and terminal integration tests
bin/             Local binaries (ignored by Git)
dist/            Release archives and cross-builds (ignored by Git)
.github/         CI and release workflows
```

OS-specific `platform_*.go` and `editor_*.go` files stay beside their shared
implementations in `internal/cli`; Go selects the correct files automatically.
Unit tests stay beside the implementation they exercise. Shell integration tests
default to the executable in `bin/`; set `CDT_BINARY` to an absolute path to test
another installation.

### Publishing

Push a new immutable `vMAJOR.MINOR.PATCH` tag after checks pass. The release
workflow builds a CGO-free binary on each native architecture, tests the actual
binary with native shell terminal tests, and packages it with the license and
documentation. All four builds must pass before the workflow publishes the
macOS TAR archives, Windows ZIP archives, and
SHA-256 checksums. A failed upload leaves a draft, not a published partial release.
Downloads use readable platform labels: macOS - Apple Silicon, macOS - Intel,
Windows - x64 (Intel / AMD), and Windows - ARM64. Archive filenames remain stable
for package managers and scripts; release notes include labeled download links.

Then update the personal tap's architecture-specific URLs and SHA-256 values from
that release. Run the tap's audit, install, and smoke tests before pushing it.
Keep released tags and archives immutable so the tap's checksums remain valid.

## Upgrade

```sh
brew update
brew upgrade cdt
```

The executable updates immediately. Shell functions and completion already loaded
in an open terminal remain in memory. If a release changes shell integration,
open a new terminal to load it through `.zshrc`, or refresh the current session:

```sh
eval "$(command cdt init zsh)"
```

You do not need to run this manually after every upgrade. With the initialization
line in `.zshrc`, new interactive shells load the installed integration automatically.

On Windows, replace the files in your installation directory with the matching
release ZIP. A new PowerShell session loads the integration through `$PROFILE`;
to refresh an existing session, run:

```powershell
cdt.exe init powershell | Out-String | Invoke-Expression
```

Upgrading preserves your registry. No profile edits or data migration are needed
when upgrading to v0.5.0.

## Uninstall

```sh
brew uninstall cdt
```

Remove the cdt initialization block from your zsh startup file when uninstalling.
Your registry is retained; remove it separately only when you no longer need
the records.

On Windows, remove the installation directory from `PATH`, delete it, and remove
initialization from `$PROFILE`. Registry data is retained separately.

MIT licensed.
