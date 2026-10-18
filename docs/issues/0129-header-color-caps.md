---
type: Issue
title: Section headers get their own background and ALL CAPS labels
description: The filled section bar shares the selection background; give it a distinct tint and render section labels in all caps.
tags: [issue, tui]
timestamp: 2026-10-18T19:30:00Z
---

# ISSUE 0129: Section header color and caps

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0123](0123-empty-sections-unfilled.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `04ef64d`

## Summary

A filled section header and the selected row share the same
background tint, so a header and a selection read as the same state.
The header gets its own deeper tint (the bar's edge padding must use
the same tint), the selection keeps its tint, and workspace section
labels render in ALL CAPS.
