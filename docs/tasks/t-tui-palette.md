---
type: Task
title: T-tui-palette
description: The fuzzy palette on / — modules+layers, contextually valid actions, current-module fields; fzf-style ranking, in-session recents, jump-in-sync.
tags: [task, tdd, tui, palette]
timestamp: 2026-09-13T00:00:00Z
milestone: m15
---

# Goal

Add the `/` fuzzy palette per [M15](/milestones/m15-tui-compositor.md):
the fast path to a known target. The tree is for browsing; the palette is
for teleporting. A navigator, not a command line.

# Tests first

- Golden `View()`: `TestPalette_emptyQueryShowsRecents` (then all
  modules), `TestPalette_midQueryRanked` (fixed section order — modules,
  actions, fields — score within section),
  `TestPalette_rowAnatomy` (glyph + label + faint context;
  `firefox · user` and `firefox · host:buildbox` as separate entries),
  `TestPalette_noResults` (`no matches` + dimmed hint), max 10 visible
  rows with scroll.
- Message-driven: `TestPalette_selectModuleJumpsNavAndWorkspaceInSync`
  (nav row, workspace content, focus), `TestPalette_actionsContextual`
  (no `save draft` when nothing is dirty; only valid actions listed),
  `TestPalette_selectFieldDeepLinks` (workspace scrolls, cursor on the
  field), `TestPalette_closeRestoresFocusExactly` (nav cursor, workspace
  scroll, active draft untouched),
  `TestPalette_ctrlNFromNoResultsPrefillsCreate` (query prefilled, never
  auto-offered), `TestPalette_recentsInSessionOnly` (last 8, tiebreak
  only, nothing persisted), `TestPalette_dirtyDraftNeverWarnsOnJump`.

# Implementation notes

- Ranking: subsequence match with scored gaps via a mature Go fuzzy
  library (vendored) — no hand-rolled matcher. Sections keep fixed order;
  score first within a section, recents as tiebreak.
- Actions run through their usual confirms/modals — selecting `apply` in
  the palette is identical to pressing `P`.
- Explicitly out of scope (fog): shell commands, file search outside the
  profile, keybinding remapping.
- Opens from anywhere in the base screen; captures all keys except
  `esc`/`ctrl+n` while open; mouse click chooses, wheel scrolls.

# Docs

- tui.md palette section; log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
