---
type: Issue
title: Remove the section edge row
description: A filled section header is one row again; the eighth-block edge made it a two-row block with the label at the bottom.
tags: [issue, tui]
timestamp: 2026-10-18T18:50:00Z
---

# ISSUE 0125: Remove the section edge row

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0121](0121-filled-headers-eighth-gap.md), [0123](0123-empty-sections-unfilled.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `6ebced3`

## Summary

A filled section header renders as a two-row block — the 0121
eighth-block edge row on top of the bar — so the header is too high
and its label sits at the bottom of the block. The edge existed to
separate filled bars that touched; since 0123 empty sections render
unfilled, filled bars never touch, and the bar itself is the
separation. The edge row and its theme role are removed.

## Requirements

- A filled section header is exactly one row.
- Separator rows render nothing; the model rows stay (movement and
  section structure are unchanged).
