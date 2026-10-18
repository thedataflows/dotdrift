---
type: Issue
title: "0147: Empty line between the header text and the tab bar"
description: The TUI frame needs one empty line above the tabs, between the header text (dotdrift · root) and the tab line, for air.
tags: [issue, feature, tui]
timestamp: 2026-10-19T00:00:00Z
---

# ISSUE 0147: Empty line between the header text and the tab bar

- **Type**: feature
- **Status**: done
- **Priority**: low
- **Labels**: [tui]
- **Assignee**: none
- **Related**: [0114](0114-tui-visual-fixes-screenshot.md)
- **Related code**: [`internal/tui/compositor.go`](../../internal/tui/compositor.go)
- **Closing commits**: `9f73a67`

## Summary

User request: add an empty line above the tabs at the top,
between the header text and the tab line.

## Details

`baseFrame` joins header, tab bar, body, footer. Insert one blank
line between the header and the tab bar, and take the line out of
the pane budget (`layout()` pane height `h-7` → `h-8`) so the
frame still fits the terminal exactly at every size.

## Acceptance Criteria

- [x] One empty line sits between the header text and the tab
  bar in every view.
- [x] Frames still fit the terminal height at small sizes.
- [x] Goldens are regenerated and reviewed.

## Out of Scope

- Any other spacing change.
