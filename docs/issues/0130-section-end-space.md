---
type: Issue
title: One empty line after each workspace section
description: Sections run into the next header; render the existing separator row as one empty line after the last item of each section.
tags: [issue, tui]
timestamp: 2026-10-18T20:41:00Z
---

# ISSUE 0130: Empty line after each section

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0117](0117-section-spacing.md), [0125](0125-remove-section-edge-row.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `5265a8c`

## Summary

The separator row between workspace sections renders nothing
(0125), so a section's last item sits directly against the next
section header. Render the separator as one empty line: each
section ends, then one blank line, then the next header. The
separator stays non-selectable and the cursor still skips it.
