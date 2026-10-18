---
type: Issue
title: Space between workspace sections
description: A blank row separates workspace sections so empty sections do not merge into one background block.
tags: [issue, tui]
timestamp: 2026-10-18T17:30:00Z
---

# ISSUE 0117: Space between workspace sections

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0116](0116-section-bar-alignment.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `853d25e`

## Summary

Since 0113 a section header is a filled bar across the pane. In a module
with empty sections the bars sit directly on each other and merge into
one block of the same background. Sections need a visible gap.

## Requirements

- One blank row separates every pair of workspace sections.
- The separator is never selectable; cursor movement skips it (the
  existing hint-row rule).
- No separator before the first section.
