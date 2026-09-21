---
type: Issue
title: Restore dialog's plan resolution never lands — restorePlanMsg has no production consumer
description: The restore dialog's enter returns d.resolve, whose restorePlanMsg is dropped by dialogModal.update (it handles only KeyPressMsg and writeFinishedMsg), so d.resolved never becomes true and the shipped TUI cannot get past the targets row. Tests pass because they call applyPlan directly.
tags: [bug, tui, dogfooded]
timestamp: 2026-09-21T18:30:00Z
---

# ISSUE 0084: Restore dialog's plan resolution never lands

- **Type**: bug
- **Status**: open
- **Priority**: high
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0083](0083-ponytail-audit-application.md) (found during the audit pass), [0073](0073-tui-compositor-redesign.md)
- **Related code**: [`internal/tui/modals.go`](../../internal/tui/modals.go), [`internal/tui/dialog.go`](../../internal/tui/dialog.go), [`internal/tui/compositor.go`](../../internal/tui/compositor.go)
- **Closing commits**: none

## Summary

In the running TUI, the restore dialog cannot get past target entry:
pressing enter returns the `resolve` cmd, the `restorePlanMsg` it
produces is never applied to the dialog, so the plan rows never render
and the restore write can never run. Found by the deadcode pass in
0083: `restoreDialog.applyPlan` is unreachable from `main`.

## Details

The message path has a missing wire:

1. `restoreDialog.HandleKey("enter")` (unresolved state) returns
   `d.resolve`, a `tea.Cmd` producing `restorePlanMsg`
   (dialog.go).
2. `Compositor.Update` has no `restorePlanMsg` case — the message falls
   through to the modal stack (compositor.go).
3. `dialogModal.update` handles only `tea.KeyPressMsg` and
   `writeFinishedMsg`; a `restorePlanMsg` is dropped (modals.go).
4. `d.resolved` never becomes true, so `HandleKey` stays in the
   targets branch and `View` keeps rendering the targets row.

The suite never noticed because the dialog tests call `applyPlan`
directly — they pin the method, not the delivery.

Fix shape (choose one, prefer the first): `dialogModal.update` gains a
`restorePlanMsg` case calling `m.d.applyPlan(msg)` and returning the
cmd bookkeeping the writeFinished case does (`reload` is manage-only,
so probably just `nil`); or `Compositor.Update` routes the message to
the top modal before the input fallthrough, mirroring how apply-session
messages are compositor-level. Add a compositor-level test that drives
`Update` with the cmd's message and asserts the dialog's resolved view,
so the delivery is pinned, not just the method.

## Acceptance Criteria

- [ ] A compositor-level test drives enter on the restore dialog's targets row through `Update` (not `applyPlan` directly) and reaches the resolved plan view
- [ ] The elevated-target marker and generation pinning render from that resolved plan in the same test
- [ ] Confirming the dialog runs the restore write through the existing `writeFinishedMsg` path (unchanged)
- [ ] The CLI `dotdrift restore` output is untouched (no shared code changed)

## Out of Scope

- Reworking the modal message routing for other dialogs (apply-session
  messages already have their compositor-level pattern).
- Any change to restore's service semantics.

## Notes

The 0083 audit left `applyPlan` in place deliberately: it is
load-bearing-intent code with a missing wire, not dead flexibility —
deleting it would have completed the break instead of fixing it.
