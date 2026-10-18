---
type: Issue
title: Section headings as text, hairline spacing, indented items
description: Drop the filled section bar for the gh-dash heading treatment, indent items under headings, and replace the blank separator with a hairline rule.
tags: [issue, tui]
timestamp: 2026-10-18T17:50:00Z
---

# ISSUE 0119: Section headings as text, hairline spacing

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0117](0117-section-spacing.md), [0116](0116-section-bar-alignment.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `9d6c648`

## Summary

Three section defects from the terminal. The blank separator row is a
full character row — far more than the wanted hairline gap. After the
0116 alignment fix, section items start in the same column as the
section title. And a selected item uses the same filled background as
the section title bar, so selection and heading read as one thing.

gh-dash solves all three the same way: preview section headings are
bold underlined text with no background, the selected row is the only
filled element, and spacing comes from faint hairline rules.

## Requirements

- A section heading is bold underlined indigo text, no background.
  The cursor row stays the only filled row treatment.
- Section items indent one column under the heading; the heading text
  column does not move under the cursor.
- The separator between sections renders as a faint hairline rule
  across the pane, not a blank row.
