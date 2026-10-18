---
type: Issue
title: Remove background fill from footer and filter inputs
description: The footer bar and the quickfilter/palette input lines lose their background slab; keys become plain text again.
tags: [issue, tui]
timestamp: 2026-10-18T18:40:00Z
---

# ISSUE 0124: Remove background fill from footer and filter inputs

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0113](0113-tui-visual-system.md), [0122](0122-footer-key-chips.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `82cbfc0`

## Summary

The 0113 background fill spread to places where it carries no
meaning: the whole footer became a background slab, and the nav
quickfilter and palette input lines render on the cursor row's fill.
Neither looks better for it. Background stays only where it means
something: the selected row, the active tab, a filled section bar,
and detail header strips.

## Requirements

- The footer renders as plain lines: status slot (success green,
  errors red, by foreground) and help hints with plain keys — no
  background anywhere in the footer, no key chips (supersedes 0122).
- The nav filter line and the palette query line render in a plain
  input text style, not on the cursor fill.
- The cursor row's edge fill keeps working (the fill role is renamed
  to say what it is).
