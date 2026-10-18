---
type: Issue
title: "0142: Dialog form rows carry the same leading space"
description: Bug — the add/edit modals opened from the Modules workspace (and every dialog form) render rows with one leading space too many, the same defect 0141 fixed in the Modules panes.
tags: [issue, bug, tui]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0142: Dialog form rows carry the same leading space

- **Type**: bug
- **Status**: open
- **Priority**: low
- **Labels**: [tui]
- **Assignee**: none
- **Related**: [0141](0141-modules-panes-leading-space.md)
- **Related code**: [`internal/tui/dialog.go`](../../internal/tui/dialog.go)
- **Closing commits**: none

## Summary

Follow-up to 0141, user report: the modals opened from the Modules
details (workspace) — the add/edit form for each section — show the
same extra leading space. The focused row renders `│ name value`
and plain rows two spaces before the label, where 0141's rule is
the pipe directly next to the label.

## Details

Every dialog form row renders through `dlgRow.renderRow`: the
per-section add/edit forms from the workspace, and also the
onboard, generate, and manage dialogs. Fix it once there:

- Focused: `cursorRow.Render(body)` instead of
  `cursorRow.Render(" "+body)` — the pipe sits next to the label.
- Plain: `" "+body` instead of `"  "+body` — the label keeps the
  same column as the focused row.

Other modal kinds (choice list, palette, apply gate) are separate
renderers and stay unchanged.

## Acceptance Criteria

- [ ] A focused dialog row renders `│<label> <value>`; a plain row
  renders one space before the label.
- [ ] TUI goldens are regenerated and reviewed.

## Out of Scope

- The choice, palette, and apply-gate modals.
