---
type: Issue
title: "0143: Number the view tabs"
description: The view tab bar labels carry their shortcut numbers (1. Modules, 2. Status, 3. Plan, 4. Profile), matching the 1-4 jump keys.
tags: [issue, feature, tui]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0143: Number the view tabs

- **Type**: feature
- **Status**: open
- **Priority**: low
- **Labels**: [tui]
- **Assignee**: none
- **Related**: [0107](0107-tui-shell-views-profile.md)
- **Related code**: [`internal/tui/views.go`](../../internal/tui/views.go)
- **Closing commits**: none

## Summary

The view tabs read `1. Modules`, `2. Status`, `3. Plan`,
`4. Profile`: each tab names the number key that jumps to it
(the `1`–`4` bindings already exist). The change was made in the
working tree outside the agent session and kept by the user
(2026-10-18); this issue records it.

## Acceptance Criteria

- [ ] Tab labels carry their shortcut numbers.
- [ ] TUI goldens are regenerated for the new labels.

## Out of Scope

- Changing the tab order or the jump keys.
