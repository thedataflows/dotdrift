---
type: Issue
title: Filled section headers with an eighth-row gap
description: Restore the full-background section header and make the space between sections an eighth of a row via a block edge row.
tags: [issue, tui]
timestamp: 2026-10-18T18:10:00Z
---

# ISSUE 0121: Filled section headers with an eighth-row gap

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0119](0119-section-headings-hairlines.md), [0120](0120-heading-ansi-leak.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `8819433`

## Summary

The 0119 underlined-text heading reads badly in the terminal. The
filled background header was the better treatment; its only defect was
the separator taking a full character row. A terminal row cannot be
split, but a block character can: the separator row renders `▇`
(lower seven-eighths block) in the header background color, so the
fill of the next section starts one eighth of a row late — a gap of
about 2px at normal font sizes, not a full row.

## Requirements

- The section header is a filled bar across the pane again (no
  underline — the 0120 leak rule still holds, and its byte-level
  regression test stays).
- The separator between sections is the eighth-block edge row, not a
  blank row and not a hairline rule.
- Items keep their one-column indent under the heading; the heading
  text does not shift under the cursor.
