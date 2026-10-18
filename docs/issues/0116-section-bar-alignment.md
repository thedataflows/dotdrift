---
type: Issue
title: Section bars align with the cursor row and fill the pane
description: Selecting a section must not shift its text right; section and detail bars fill the full pane width.
tags: [issue, tui]
timestamp: 2026-10-18T17:20:00Z
---

# ISSUE 0116: Section bar alignment

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0114](0114-tui-visual-fixes-screenshot.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `1dcc1dd`

## Summary

A workspace section header renders its text at column 1 (the bar style's
padding), while the same header under the cursor renders at column 2
(the cursor bar cell plus a space). Moving the cursor onto a section
shifts its text one character right. The section background must also
fill the full pane width in the detail panes, not stop at the text.

## Requirements

- A section header's text starts in the same column with and without
  the cursor.
- Section bars and detail header strips fill the full pane width.
- A regression test pins the column and the width.
