---
type: Issue
title: Modal text inputs must not stretch the modal; long text scrolls
description: Dialog fields render their full value, so typing stretches the modal and text past the edge is unreachable; render a fixed window that follows the cursor.
tags: [issue, tui]
timestamp: 2026-10-18T20:55:00Z
---

# ISSUE 0131: Modal input window

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `6133274`

## Summary

Dialog text fields (`dlgField`, used by the onboard, restore,
generate, and manage dialogs and by the add form) render their
whole value, and the modal wrappers set no width cap. Typing a
long value stretches the modal across the screen, and a field
shows no caret, so the cursor position inside long text is
invisible and text outside the visible area cannot be reached
visually with the arrow keys.

Fix: a dialog field renders a fixed-width window of its value
that follows the cursor (left/right already move the cursor),
with the block caret shown on the focused row. The modal
wrappers cap their width at the frame width as a backstop, and
the elevation password dots are capped at the same width. The
modal width must not change while typing.
