---
type: Task
title: T-tui-shell
description: The two-pane charm v2 shell — tree (Modules/Accounts/Profile), view stack, keys, chrome — wired to the reads areas.
tags: [task, tdd, tui, shell]
timestamp: 2026-09-12T00:00:00Z
milestone: m14
---

# Goal

Build `dotdrift tui`'s frame per [M14](/milestones/m14-tui.md) and the
approved prototype (issue 0063): tree left, one active view right, the
keybinding baseline, and the read views (module stacks, status, plan) fed
by the reads areas from T-tui-reads. Editors and apply are later tasks —
the shell owns navigation, focus, chrome, and the view stack only.

# Tests first

- Pure state machines + golden `View()` (research 0058's mandate; teatest
  stays out): `TestShell_focusMovesWithTab` / `_oneFocusedPane`,
  `TestTree_groupsAndOrdering` (Modules/Accounts/Profile; one node per
  module across layers — contract 1), `TestTree_overlayOriginsExpand`
  (origins as children: base/hosts/users/superuser),
  `TestTree_accountsNodes` (hosts, users, superuser overlay visibility
  per issue 0029), `TestSelection_resolvedByDefault_toggleRaw`,
  `TestViewStack_openPop` / `_dirtyEditorSurvivesNavigation` (draft
  ledger placeholder semantics),
  `TestChrome_headerShowsRootContextDirty` / `_statusBarHintsFollowFocus`,
  `TestKeys_baseline` (j/k/arrows, tab/shift-tab, enter, esc, q
  dirty-confirm, ?, g/G), `TestPaletteRegistry_noInlineStyles`
  (every style resolved through the ADR-0003 registry, adaptive
  LightDark).
- Golden `View()` for each read view: module (resolved stack + overlay
  toggle), status (canonical renderer output in the scroll view + the
  ADR-0006 notice), plan (per-step diffs; the gate area stubbed until
  T-tui-apply).

# Implementation notes

- charm v2 (`charm.land/bubbletea/v2`, `charm.land/bubbles/v2` — official
  tree bubble), vendored; 0063's recorded simplifications are **not**
  copied: adaptive `LightDark(isDark)` colors, no static ANSI, no stub
  patterns.
- Tree width fixed 30%, clamped 22–44 columns; focus = border color
  (indigo/dim). Header: profile root, host/user context (read-only in
  M14), dirty indicator. Status bar: focused pane's `help.Model` hints.
- Keys + context menus only; global keys reserved by the shell; `/`
  filtering deferred. Command palette and `:` line do not exist (fog).
- Context-menu actions that write (onboard/restore/generate) render as
  disabled stubs behind their task flags until T-tui-writes lands.
- The TUI consumes `service.Service` (reads areas) through consumer-side
  narrow interfaces; the tree builds from the modules read model, views
  resolve on selection (resolved-by-default, 0062-D3).

# Docs

- tui.md stays the spec — any drift discovered while building is fixed in
  the same change; README command table gains the `tui` row; log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
