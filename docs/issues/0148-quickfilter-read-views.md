---
type: Issue
title: "0148: Quickfilter on the Status, Plan, and Profile views"
description: The f and / keys open an in-place quickfilter on the focused panel of the Status, Plan, and Profile views, with the Modules nav filter's keys and line.
tags: [issue, feature, tui]
timestamp: 2026-10-19T00:00:00Z
---

# ISSUE 0148: Quickfilter on the Status, Plan, and Profile views

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui]
- **Assignee**: none
- **Related**: [0081](0081-tui-nav-inplace-filter.md), [0108](0108-tui-status-view.md), [0109](0109-tui-plan-view.md), [0135](0135-shared-text-input.md)
- **Related code**: [`internal/tui/`](../../internal/tui/)
- **Closing commits**: `2d42ad8`

## Summary

User request: on tabs 2-4 (Status, Plan, Profile), `f` or `/` opens a
quickfilter on whatever is in the focused panel of the current tab,
like the Modules nav filter (0081).

## Details

- Status: the focused list panel filters finding rows (the row text and
  its section name match); the focused detail panel filters its lines.
- Plan: the focused section list filters section names; the focused
  detail panel filters its lines (section entries or the diff list).
- Profile: the single panel filters its card rows; a card whose title
  matches keeps all its rows, a card with no matching row drops out.
- The filter uses the shared text input (0135) and the nav filter's
  rules: typing narrows live, `j`/`k` and the arrows still move in the
  list panel, `enter` keeps the filter and ends the typing mode, `esc`
  removes it (while typing, or later from the base esc), `ctrl+c`
  quits. The query line shows at the panel bottom: `/ query` while
  typing, `/ query · esc clears` when applied.
- Matching is a case-insensitive substring test on the plain text of a
  row or line (ANSI codes stripped).
- On the Status view the quickfilter replaces the `f` section cycle
  (0108): typing a section name in the list filter does the same job.
  The Modules view is unchanged (`/` filters the nav, opens the
  palette in the workspace; `f` stays inert there).

## Acceptance Criteria

- [x] `f` and `/` open the quickfilter on the focused panel of the
  Status, Plan, and Profile views; typing narrows that panel live.
- [x] `enter` keeps the filter, `esc` removes it, `ctrl+c` quits
  while typing; movement keys still move in a list panel.
- [x] The query line shows at the filtered panel's bottom.
- [x] The Status section cycle is gone; a section name typed in the
  list filter narrows to that section's findings.
- [x] Help (`?`) names the quickfilter on the read views.

## Out of Scope

- Any change to the Modules view filters or the palette.
- Filtering inside modals.
