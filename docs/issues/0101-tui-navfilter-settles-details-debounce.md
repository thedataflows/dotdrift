---
type: Issue
title: Nav filter settles the workspace on the selection after a debounce
description: While quick-filtering the modules list the workspace kept showing the previously selected module; a 300 ms debounce now syncs the workspace to the current selection once typing settles.
tags: [issue, tui, navfilter]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0101: TUI nav filter settles the workspace on the selection after a debounce

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, navfilter]
- **Assignee**: none
- **Related**: [0081](0081-tui-nav-inplace-filter.md)
- **Related code**: [`internal/tui/nav.go`](../../internal/tui/nav.go), [`internal/tui/compositor.go`](../../internal/tui/compositor.go)

## Summary

Typing in the nav quick filter moves the cursor (`refilter`) but never
syncs the workspace, so the details pane keeps showing the previously
selected module. Syncing per keystroke would spawn a layer file read per
character, so the sync now rides a 300 ms debounce: once the query
settles, the workspace follows the current selection.

## Details

`navFilterKey`'s query-changing paths (typed characters, backspace) and
the compositor's paste path all return `nil` after `refilter`; only
`j`/`k`/arrows and `esc` call `syncWorkspace`. The cursor can therefore
land on another module while the workspace still renders the old one —
and even `enter`, which ends the typing mode with the filter applied,
does not sync, so the stale details survive until the next nav movement.

The fix follows the compositor's existing token pattern (`msgFadeMsg`):
every query change bumps `filterSeq` and returns a `tea.Tick` of 300 ms
carrying the sequence; the tick's message syncs the workspace only when
its sequence is still the latest. The sequence guard also keeps a burst
of keystrokes from spawning duplicate in-flight reads of the same file
(`loadLayer` only dedupes *landed* reads). An empty match set settles to
an empty workspace, the same `syncWorkspace` behavior an empty selection
already produces.

## Acceptance Criteria

- [x] Typing or backspacing a query change does not sync the workspace
      per keystroke (no per-character file read).
- [x] 300 ms after the last query change, the workspace shows the
      module under the cursor.
- [x] Pasted query text settles the same way.
- [x] A superseded tick (an older sequence) does not sync.
- [x] A query with no matches settles to an empty workspace.

## Out of Scope

- Debouncing the `j`/`k` moves within the matches — those sync
  immediately today and stay immediate.
- Changing how `enter`/`esc` behave (enter ends the typing mode; the
  pending settle tick still syncs afterwards).

## Notes
