# Changelog

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
