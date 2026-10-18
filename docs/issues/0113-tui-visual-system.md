---
type: Issue
title: TUI visual system from gh-dash
description: Give the TUI filled tabs and rows, cards, distinct labels, pills, and a footer bar, and move view switching to < and >.
tags: [issue, tui]
timestamp: 2026-10-18T16:30:00Z
---

# ISSUE 0113: TUI visual system from gh-dash

- **Type**: feature
- **Status**: done
- **Priority**: high
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [TUI specification](../product/tui.md), [0111](0111-tui-generate-volumes-polish.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `0c48c0c`

## Summary

The TUI renders selectable elements, labels, and values as plain text in near
equal shades. Tabs are bold words. The detail panes have no header. The footer
is dim text. Borrow the visual system of gh-dash: filled blocks, cards, pills,
and strong label and value contrast. No new dependency.

## Requirements

- View tabs are padded blocks. The active tab has a filled background. A stale
  view shows an amber dot badge, not the word stale. Counts stay in the tab.
- `<` and `>` switch to the previous and next view. The `s` key is removed.
  Keys `1` to `4` still jump direct.
- The cursor row is a filled block across the pane width, with the accent bar.
- Labels use the indigo hue. Values use the bright neutral hue. A label and a
  value must differ by hue, not by one shade of gray.
- Workspace section headers render as heading bars. Status and Plan detail
  panes start with a header strip and a status pill. Profile groups render as
  titled cards.
- The footer is a background bar. Success uses a green check, errors red.
- Diff lines use green for added lines and red for removed lines.
- Modal frames use severity colors: red for destructive confirms, amber for
  sudo and gates, indigo for information.

## Out of scope

- New dependencies, mouse support, user theme configuration, Nerd Font icons.
- Layout changes: pane sizes, view order, and view behavior stay as they are.
