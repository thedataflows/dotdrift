---
type: Issue
title: Thin dark border on section headers
description: A filled section header carries a dark hairline border under the bar, also under the cursor so layout never shifts.
tags: [issue, tui]
timestamp: 2026-10-18T19:00:00Z
---

# ISSUE 0126: Thin dark border on section headers

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0125](0125-remove-section-edge-row.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `1ad563a`

## Summary

Filled section headers get a thin dark border: a dark hairline rule
under the bar, the same device as the tab bar's bottom rule. A full
box border would add two rows per header — the header height was
just rejected — while a hairline stroke reads as a border at one
row. The rule is present under the cursor too, so rows below never
shift when the cursor lands on a header.

## Requirements

- A filled section header renders the bar plus a dark `─` rule
  across the pane under it, cursor or not.
- Unfilled (empty) section headings stay plain text, no rule.
