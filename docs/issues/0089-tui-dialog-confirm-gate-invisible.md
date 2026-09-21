---
type: Issue
title: Form dialogs arm an invisible confirm gate — enter freezes the dialog
description: Every form dialog (manage create/move, onboard, restore, generate) renders its y/n confirm prompt unconditionally, so pressing enter arms a gate that swallows every key but y/n with zero visible change; the user experiences a frozen dialog that only esc or n escapes. The delete flow has the mirror bug — n disarms the gate but leaves the mode with no key handling at all.
tags: [bug, tui, dogfooded]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0089: Form dialogs arm an invisible confirm gate — enter freezes the dialog

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0072](0072-module-management-dialogs.md) (manage dialogs)
- **Related code**: [`internal/tui/dialog.go`](../../internal/tui/dialog.go) (`finishView`, onboard/restore/generate dialogs), [`internal/tui/manage.go`](../../internal/tui/manage.go)

## Summary

Found dogfooding: in the create-module dialog, typing in `app` works and
the cursor moves between rows — but pressing enter makes the dialog
unresponsive to everything except esc (compositor-owned pop) and `n`
(which mysteriously restores typing).

Root cause: every form dialog gates its run behind `d.confirm`, and
while the gate is armed `HandleKey` answers only `y`/`n` — by design.
The bug is that the gate is **invisible**: every `finishView` call site
passes its confirm text unconditionally, so `create module "x" in base?
y/n` is rendered the entire time, before and after the gate arms.
Pressing enter produces no visible change; the dialog simply stops
responding.

The delete-module flow has the mirror bug: its gate is armed from the
start, `n` disarms it — but `manageDelete` has no key handling outside
the gate, so after `n` every key is dead while the footer still claims
"y deletes · n/esc back". `n` must step back to the menu.

## Details

The two-step gate (enter → y) is deliberate and tested; only its
visibility changes:

- The confirm prompt renders only while the gate is armed — it appears
  on enter and disappears on `n`.
- While armed, the footer swaps to the gate's vocabulary (`y runs ·
  n/esc back`), because the form hint ("type to edit · enter runs")
  would lie — the gate swallows those keys.
- Delete's `n` returns to the manage menu (the footer's "back" made
  literal), instead of stranding the dialog in a keyless mode.

Affected views: manage create/move and delete
([`manage.go`](../../internal/tui/manage.go)), onboard, restore,
generate ([`dialog.go`](../../internal/tui/dialog.go)).

## Acceptance Criteria

- [x] No form dialog renders a y/n prompt before its gate is armed
- [x] Arming the gate (enter) shows the prompt and a footer naming the
  gate's keys; `n` hides the prompt and restores the form
- [x] Delete module's `n` returns to the manage menu
- [x] The golden of the armed delete gate still matches
- [x] `go test ./...` and `go vet` pass
