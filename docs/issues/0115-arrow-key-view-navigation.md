---
type: Issue
title: Arrow keys switch views
description: Left and right arrows cycle the views; the literal < and > keys and their pane jobs move to h and l.
tags: [issue, tui]
timestamp: 2026-10-18T17:10:00Z
---

# ISSUE 0115: Arrow keys switch views

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux, keys]
- **Assignee**: agent
- **Related**: [0113](0113-tui-visual-system.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `dbf9b52`

## Summary

Issue 0113 bound view cycling to the literal `<` and `>` keys, which need
Shift on most layouts. The request meant the arrow keys. `←` and `→` now
cycle to the previous and next view in every view and pane.

## Requirements

- `left` and `right` arrows switch views; the `<` and `>` bindings are removed.
- Nav collapse and expand keep working on `h` and `l` (already bound).
- Workspace value cycling moves from the arrows to `h` and `l`.
- Dialogs, the file picker, and text editors keep their own arrow keys:
  they do not route through the binding table.
- Footer and `?` help name the arrows; no key requires Shift for view switching.
