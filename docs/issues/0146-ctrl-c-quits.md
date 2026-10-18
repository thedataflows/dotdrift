---
type: Issue
title: "0146: ctrl+c quits the TUI, like q"
description: The TUI quits only on q; ctrl+c must run the same quit flow (dirty-draft confirm included) at the base level, in the field editor, and in the nav filter.
tags: [issue, feature, tui]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0146: ctrl+c quits the TUI, like q

- **Type**: feature
- **Status**: open
- **Priority**: medium
- **Labels**: [tui]
- **Assignee**: none
- **Related**: none
- **Related code**: [`internal/tui/keymap.go`](../../internal/tui/keymap.go), [`internal/tui/compositor.go`](../../internal/tui/compositor.go)
- **Closing commits**: none

## Summary

User request: the TUI should also quit at ctrl+c, not just q.

## Details

ctrl+c joins the binding table next to `q` and runs the same
`quit()` flow: a clean shell quits, dirty drafts ask first, and
during an apply the footer still redirects to the apply detail
(cancelling a run is that modal's own ctrl+c, unchanged). The
field editor and the nav filter, which own keys before the table,
route ctrl+c to the same quit instead of swallowing it. Modals
keep their own keys (esc closes; the apply detail's ctrl+c
cancels the run).

## Acceptance Criteria

- [ ] ctrl+c at the base level quits a clean shell and asks first
  with dirty drafts, exactly like q.
- [ ] ctrl+c quits from the field editor and the nav filter.
- [ ] The apply detail modal's ctrl+c still cancels the run, not
  the app.
- [ ] The ? help lists ctrl+c as quit.

## Out of Scope

- ctrl+c inside other modals (esc territory).
