# Changelog

## Unreleased

- Add global `--rename-tag OLD NEW` with read-only previews, per-path before/after
  feedback, target-tag merging, and atomic application without touching directories.
- Complete both rename operands from all recorded tags, including stale-only tags.
- Add `--move OLD NEW` to relocate registrations without moving real files.
- Support recursive prefix migration, read-only previews, stale source completion,
  collision rejection, and atomic application under the registry writer lock.
- Add `--tags` with sorted tags and direct registered-directory counts.
- Add read-only `--explain TAG...` with own matches, ancestor sources, missing
  tags, inherited-only exclusions, and unavailable directory diagnostics.
- Share tag assessment between navigation and explanations; extend native shell
  tests and tag completion for diagnostic queries.

## 0.4.1

- Add Windows PowerShell 7 navigation, tag/path completion, and console selection.
- Add Win32 registry locking, snapshot-safe replacement, and editor launching.
- Use `%APPDATA%\cdt\registry.json` on Windows, with XDG overrides supported.
- Test Windows x64 and ARM64 natively, including real ConPTY menu interactions.
- Publish macOS and Windows archives under one shared version and release.
- Close macOS test terminals before reaping shell processes to avoid cleanup hangs.

## 0.3.1

- Publish tested macOS arm64 and amd64 binary archives with SHA-256 checksums.
- Install precompiled packages through Homebrew without a Go dependency.
- Automate native-architecture release validation and publication.

## 0.3.0

- Report mutation paths, new/removed tags, skipped tags, and before/after own tags.
- Explain repeated input and existing tags; avoid rewriting unchanged records.
- Confirm interactive zsh jumps and cancelled menu selections.
- Keep noninteractive jumps silent and internal JSON responses unchanged.
- Display `-l` results as a path tree with own tags and matching records only.

## 0.2.0

- Add `cdt -e` / `--edit` to edit registry tags with VISUAL, EDITOR, or vi.
- Validate edits before applying them, retain invalid drafts, and detect concurrent
  changes without blocking other commands while an editor is open.

## 0.1.0

- Register directories and manage explicit tags with GNU-style grouped options.
- Jump by unordered tags with ancestor tags for disambiguation.
- Native zsh selection menu and tag/path completion, without fzf.
- Versioned local JSON storage with atomic writes and concurrent-writer locking.
- Registry diagnostics, safe path handling, and Apple Silicon/Intel macOS CI.
