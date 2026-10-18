---
type: Issue
title: One reusable text input component for every input
description: The hook editor and every other input are separate implementations that drift; extract one textInput component and migrate all inputs to it.
tags: [issue, tui]
timestamp: 2026-10-18T22:00:00Z
---

# ISSUE 0135: Single text input component

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0131](0131-modal-input-window.md), [0132](0132-caret-window-all-inputs.md), [0133](0133-input-continuation-marks.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `724dff6`

## Summary

The TUI has parallel text input implementations — dialog fields,
the workspace inline editor, the multi-line editor behind the
"edit hook" modal, and the end-only query inputs (nav filter,
palette, file picker) — each with its own buffer, caret, and
window code. Fixes 0131–0133 had to be applied once per
implementation, and behavior still differs: the inline and
dialog inputs have no home/end/delete, and the query inputs
cannot move their caret at all.

Extract one `textInput` component (buffer, caret, window edge,
and a `Cells` prop for the visible width) with shared key
handling (left/right/home/end/backspace/delete/typing) and
shared window rendering (caret block, faded `...` marks), and
migrate every input to it: dialog fields, the workspace inline
editor, the multi-line editor's caret line, and the nav, palette,
and picker queries — which gain caret movement where their keys
were free. The elevation password stays on its own byte buffer:
it is zeroed on submit and has no caret by design.
