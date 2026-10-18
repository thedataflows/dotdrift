---
type: Issue
title: Faded ... continuation marks in clipped inputs
description: Inputs window long text around the caret but give no sign that text is hidden; render a faded ... at each edge where text continues.
tags: [issue, tui]
timestamp: 2026-10-18T21:25:00Z
---

# ISSUE 0133: Input continuation marks

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0131](0131-modal-input-window.md), [0132](0132-caret-window-all-inputs.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `fb8ef94`

## Summary

Since 0131/0132 every text input renders a window of its value
around the caret, but nothing shows that text is hidden beyond
an edge. Render `...` in the faded text style at each window
edge where more text continues — left when the window starts
after the first rune, right when it ends before the last. The
marks take cells from the window budget, so no input grows.
Applies to the workspace inline editor, the multi-line editor's
caret line, dialog fields, the nav filter, the palette query,
and the file picker's filter and typed path.
