# cdt

Directory tags for macOS zsh and Windows PowerShell 7. Register paths, combine
tags, and jump. One Go core, one version, and platform-specific release packages.

## Install

### macOS

```sh
brew install JieteXue/tap/cdt
```

Add this line to your `.zshrc`, after your existing completion setup:

```sh
eval "$(command cdt init zsh)"
```

Run the same line in an existing zsh session to enable it immediately. Installation
does not edit your shell configuration. No fzf, Python, or Go is needed at runtime.
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
Get-FileHash .\cdt_0.4.0_windows_amd64.zip -Algorithm SHA256
```

In PowerShell 7, initialize the function:

```powershell
cdt.exe init powershell | Out-String | Invoke-Expression
```

Add that initialization line to your PowerShell `$PROFILE` after any existing
completion setup to enable it in future sessions. Installation does not edit
your profile or execution policy. Windows PowerShell 5.1 and `cmd.exe` shell
integration are not supported.

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
```

`-ap PATH` equals `-a -p PATH` and `--add --path PATH`. Similarly, `-dp`
unregisters a specified path. `-p` consumes the rest of a short-option group or
the next argument: `-pa` means path `a`, not add. Without `-p`, mutations target
the current directory. `-a` with no tags registers a directory with no own tags.
`-r` requires tags. Mutation/list operations are mutually exclusive.

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

Moved directories are not guessed automatically. Unregister the old absolute
path with `cdt -dp OLD`, then register the new path. Removing the final explicit
tag leaves the directory registered.

The internal `__query`/`__complete` JSON protocol is versioned separately;
`__decode` converts it to NUL-framed records for zsh without an external parser.
These internal commands are not a user-facing API.

## Development

Go 1.24+ and Python 3 (only for PTY tests), plus zsh on macOS or PowerShell 7
on Windows:

```sh
go test -race ./...
go vet ./...
go build -o cdt .
python3 tests/zsh_pty.py -v
```

On Windows:

```powershell
go test ./...
go vet ./...
go build -o cdt.exe .
pwsh -NoProfile -NonInteractive -File tests/powershell.ps1
python tests/powershell_pty.py -v
```

CI tests Apple Silicon/Intel macOS and x64/ARM64 Windows natively. Race tests run
on macOS and Windows x64; Windows ARM64 runs ordinary Go tests. Windows menu
tests use a real ConPTY terminal, not a mocked key reader.
Go is needed only by contributors and release builders, not installed users.

Shared parsing, matching, storage schema, and tree rendering stay in the root
package. `platform_*.go` and `editor_*.go` isolate OS-specific operations;
`shell/cdt.zsh` and `shell/cdt.ps1` own shell navigation and completion.

### Publishing

Push a new immutable `vMAJOR.MINOR.PATCH` tag after checks pass. The release
workflow builds a CGO-free binary on each native architecture, tests the actual
binary with native shell terminal tests, and packages it with the license and
documentation. All four builds must pass before the workflow publishes the
macOS TAR archives, Windows ZIP archives, and
SHA-256 checksums. A failed upload leaves a draft, not a published partial release.

Then update the personal tap's architecture-specific URLs and SHA-256 values from
that release. Run the tap's audit, install, and smoke tests before pushing it.
Keep released tags and archives immutable so the tap's checksums remain valid.

## Upgrade and uninstall

```sh
brew update
brew upgrade cdt
brew uninstall cdt
```

Remove the initialization line from `.zshrc` when uninstalling. Your registry is
retained; remove it separately only when you no longer need the records.

On Windows, replace the files in your installation directory to upgrade. To
uninstall, remove that directory from `PATH`, delete it, and remove initialization
from `$PROFILE`. Registry data is retained separately.

MIT licensed.
