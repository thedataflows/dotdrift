---
type: Issue
title: "0144: Profile view: remove the outer card around the group cards"
description: The Profile view wraps its group cards (FACTS, PROFILE, OTHER ACCOUNTS) in a larger pane border; remove it and let the cards stretch the full width minus 2 chars.
tags: [issue, bug, tui, profile]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0144: Profile view: remove the outer card around the group cards

- **Type**: bug
- **Status**: open
- **Priority**: low
- **Labels**: [tui, profile]
- **Assignee**: none
- **Related**: [0107](0107-tui-shell-views-profile.md)
- **Related code**: [`internal/tui/views.go`](../../internal/tui/views.go), [`internal/tui/profileview.go`](../../internal/tui/profileview.go)
- **Closing commits**: none

## Summary

User request: in the Profile view (tab 4), the two group cards sit
inside a larger card — the outer pane border the view wraps its
body in. Remove the larger card; the small cards must stretch the
full width minus 2 chars of padding.

## Details

`viewBody` renders the Profile body inside `m.pane(...)`, a bordered
frame, and each group card inside it is inset again by the pane's
border cells. The Profile view returns `profileBody` unwrapped,
called with the full frame width, so each card renders at
`cardBorder.Width(w-2)`: frame width minus 2. The Status and Plan
views keep their panes (they are two-pane layouts). The Profile
body loses its fixed pane height with the border — the cards size
naturally.

## Acceptance Criteria

- [ ] No outer pane border around the Profile view's cards.
- [ ] Each group card spans the frame width minus 2 chars.
- [ ] Profile goldens are regenerated and reviewed.

## Out of Scope

- The Status and Plan panes, and the cards' inner layout.
