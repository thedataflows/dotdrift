---
type: Issue
title: Empty sections render unfilled
description: A section with no items drops the filled bar for a plain heading; the eighth-block edge appears only above filled bars.
tags: [issue, tui]
timestamp: 2026-10-18T18:30:00Z
---

# ISSUE 0123: Empty sections render unfilled

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0121](0121-filled-headers-eighth-gap.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `5400445`

## Summary

With the 0121 filled bars, a typical module — most sections empty —
renders as a wall of purple bars separated by dark notches. Emphasis
must follow content: a section with items keeps the filled bar and
the eighth-block edge above it; an empty section renders as a plain
bold heading with no separator, so empty sections stack calm and
tight, as gh-dash's preview headings do.

## Requirements

- A section header is a filled bar only when the section has entry
  rows; otherwise it is plain bold indigo text in the same column.
- The eighth-block edge renders only above a filled bar; separators
  before unfilled headings render nothing.
- The cursor treatment, the heading column, and the item indent do
  not change.
