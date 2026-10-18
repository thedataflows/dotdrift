---
type: Issue
title: Enter in the Modules nav expands/collapses the module
description: Enter on the left pane moves focus to the workspace; it must toggle the module's expansion instead, since arrows already move between panes.
tags: [issue, tui]
timestamp: 2026-10-18T21:40:00Z
---

# ISSUE 0134: Nav Enter toggles expansion

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0118](0118-tab-cycles-views-arrows-panes.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `f8e4351`

## Summary

In the Modules view, Enter on the left (nav) pane moves focus to
the workspace pane. Pane movement already belongs to the left
and right arrows (0118), so Enter has no nav job of its own on a
module row. Enter must toggle the current module's expansion:
expand a collapsed module, collapse an expanded one. Focus
stays on the nav. On a layer child (base, user, host), Enter
keeps its old job and focuses the workspace on that layer —
the toggle belongs to the parent module row only. The footer
hint and the `?` help modal take the new help text from the
binding table, and the TUI specification's nav bullet is updated
(it also still claims `right` expands, stale since 0118).
