---
type: Issue
title: Caret window navigation for every text input
description: The 0131 window-follows-caret fix covered dialog fields only; the workspace inline editor and every other input still clip text away from the caret.
tags: [issue, tui]
timestamp: 2026-10-18T21:10:00Z
---

# ISSUE 0132: Caret window for all inputs

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0131](0131-modal-input-window.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `1fb71fc`

## Summary

0131 gave dialog fields a visible window that follows the caret.
The same defect remains in every other text input:

- the workspace inline editor (`wsEdit` — for example the
  `description` field under meta) renders the whole input and
  truncates it at the pane edge, so the caret and the text to its
  right are clipped and unreachable;
- the multi-line editor truncates the caret line the same way;
- the nav filter, the palette query, and the file picker's filter
  and path inputs render unbounded text (the palette and picker
  modals stretch; the nav caret clips at the pane edge).

Fix at one owner: a shared rune-window helper that every input
renders through — the window contains the caret and follows it,
so arrow keys can reach all text. End-only inputs (filters,
palette query, picker path) render the tail window so their
caret at the end is always visible. The elevation password has
no caret by design and stays as capped in 0131.
