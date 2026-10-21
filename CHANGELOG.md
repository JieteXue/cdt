# Changelog

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
