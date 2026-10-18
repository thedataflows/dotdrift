---
type: Issue
title: Tab cycles views, arrows move between panes
description: Flip view and pane navigation: tab and shift+tab cycle the main tabs; left and right arrows move focus between panes.
tags: [issue, tui]
timestamp: 2026-10-18T17:40:00Z
---

# ISSUE 0118: Tab cycles views, arrows move between panes

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux, keys]
- **Assignee**: agent
- **Related**: [0115](0115-arrow-key-view-navigation.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `3eb9d92`

## Summary

Navigation flips: `tab` moves between the main app tabs (the views),
and the left/right arrows move between the left and right panes of the
current view. This replaces the 0115 scheme (arrows switched views,
`tab` switched panes).

## Requirements

- `tab` cycles to the next view, `shift+tab` to the previous view.
- `left` focuses the left pane, `right` focuses the right pane, in
  every two-pane view (Modules, Status, Plan).
- `1`–`4` view jumps stay. `h`/`l` keep their pane-local jobs (nav
  collapse/expand, workspace value cycling).
- Test sites that used `tab` to focus the workspace migrate to `right`.
