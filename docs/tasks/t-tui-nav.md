---
type: Task
title: T-tui-nav
description: The nav tree — module rows with overlay layers as expandable children, dirty markers, expansion memory, async-load placeholders.
tags: [task, tdd, tui, nav]
timestamp: 2026-09-13T00:00:00Z
milestone: m15
---

# Goal

Build the M15 nav pane on the T-tui-compositor platform: modules as rows,
overlay layers (base / user / host) as expandable children, per
[M15](/milestones/m15-tui-compositor.md). The nav is for browsing and
understanding profile structure — teleporting is the palette's job
(T-tui-palette).

# Tests first

- Golden `View()` (composited, fixed sizes): `TestNav_flatModuleList`,
  `TestNav_expandedModuleShowsOverlayChildren` (base · user · host rows
  with layer labels per issue 0029 visibility),
  `TestNav_dirtyMarkersPerFile` (`●` on the module row and the dirty
  layer child), `TestNav_emptyProfile`,
  `TestNav_failedModuleGreyedOut` (load failure names the error on the
  row), `TestNav_loadPlaceholderRows`.
- Message-driven: `TestNav_expandCollapseWithHL` / `_arrows`,
  `TestNav_expansionRememberedPerSession`,
  `TestNav_selectionSyncsWorkspace` (nav cursor and workspace content
  never disagree), `TestNav_cursorSurvivesReload` (async refresh keeps
  the selected module selected when it still exists).

# Implementation notes

- One row per module across layers (contract 1); overlays are children,
  not separate top-level entries. Layer rows are selectable — selecting
  one points the workspace at that layer (the tree position is the layer
  picker, as in M14).
- Tree width and focus behavior inherit the compositor shell; the nav
  consumes the service reads areas through its narrow interface, as M14.
- Dirty state comes from the 0065 draft ledger — a module row is dirty
  when any of its layer files has a draft.
- Deleted-on-disk modules stay visible, greyed, until the next reload
  confirms — never vanish rows out from under the cursor.

# Docs

- tui.md nav section rewritten; log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
