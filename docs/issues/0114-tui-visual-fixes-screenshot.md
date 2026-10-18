---
type: Issue
title: TUI visual fixes from the first screenshot
description: Fill section bars to the pane edge, calm the cursor row text, and give the tab bar its bottom rule.
tags: [issue, tui]
timestamp: 2026-10-18T17:00:00Z
---

# ISSUE 0114: TUI visual fixes from the first screenshot

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0113](0113-tui-visual-system.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `d9997eb`

## Summary

The first real-terminal screenshot of the 0113 visual system shows three
defects against the gh-dash reference. Section heading bars stop at the
text width, so the workspace shows patchy blocks of different lengths.
The cursor row text is fuchsia on the indigo fill; gh-dash uses plain
primary text on the fill, with color only in the accent bar. The tab bar
floats with no bottom rule under it.

## Requirements

- Section heading bars and detail header strips fill to the pane edge.
- The cursor row text uses the bright neutral value hue; the fuchsia
  stays only in the left accent bar.
- The tab bar carries a bottom rule across the frame width; the pane
  height accounts for the extra line.
