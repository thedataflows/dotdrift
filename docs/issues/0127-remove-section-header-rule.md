---
type: Issue
title: Remove the section header rule
description: The dark hairline under a filled header reads as the end of the section above; the bar sits directly on its items again.
tags: [issue, tui]
timestamp: 2026-10-18T19:10:00Z
---

# ISSUE 0127: Remove the section header rule

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0126](0126-section-header-dark-border.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `997198d`

## Summary

The 0126 dark hairline under a filled section header is worse than
no border: a rule under a header reads as the end of the section
above it, cutting the header off from its own items. A border means
end of something. The rule and its `sectionRule` theme role are
removed; the filled bar sits directly on its items.
