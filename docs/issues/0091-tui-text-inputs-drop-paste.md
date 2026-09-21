---
type: Issue
title: TUI text inputs drop bracketed paste — PasteMsg has no consumer
description: Every text input in the TUI (workspace field edit, dialogs, add form, nav filter, palette, elevation password) routes only tea.KeyPressMsg; bubbletea delivers bracketed paste as tea.PasteMsg, so pasting into any field silently does nothing.
tags: [bug, tui, dogfooded]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0091: TUI text inputs drop bracketed paste — PasteMsg has no consumer

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0077](0077-tui-add-forms.md) (add forms), [0081](0081-tui-nav-inplace-filter.md) (nav filter)
- **Related code**: [`internal/tui/compositor.go`](../../internal/tui/compositor.go) (Update routing), [`internal/tui/editing.go`](../../internal/tui/editing.go) (`editKey`), [`internal/tui/dialog.go`](../../internal/tui/dialog.go) (`dlgField`, dialogs), [`internal/tui/addform.go`](../../internal/tui/addform.go), [`internal/tui/palette.go`](../../internal/tui/palette.go), [`internal/tui/nav.go`](../../internal/tui/nav.go) (`navFilterKey`), [`internal/tui/modals.go`](../../internal/tui/modals.go) (`elevationModel`)

## Summary

Found dogfooding: pasting into any TUI text field does nothing. The
terminal delivers bracketed paste as `tea.PasteMsg{Content}` (bubbletea
v2), and every input-owning update path in the shell type-switches on
`tea.KeyPressMsg` only — the paste message matches nothing and is
silently dropped.

## Details

Affected inputs, each dropping `PasteMsg` at its own layer:

- the workspace field/line editor — the compositor's editing branch
  routes `KeyPressMsg` to `editKey`, nothing else;
- the form dialogs (onboard, restore, generate) and the manage
  create/move dialog — `dialogModal.update` routes `KeyPressMsg` to
  `HandleKey(string)`, and the dialogs' default branch only inserts a
  single rune anyway;
- the `a` add form — `addForm.update` handles `KeyPressMsg` and mouse
  only;
- the nav quick filter — the compositor's filtering branch routes
  `KeyPressMsg` to `navFilterKey` only;
- the command palette — `paletteModel.update` handles `KeyPressMsg` and
  mouse only;
- the sudo elevation password prompt — `elevationModel.update` is
  `KeyPressMsg`-only (pasting a password from a password manager is a
  primary use).

Paste content is sanitized for the shell's single-line inputs: control
runes (newlines, CR, tab) drop out. Insertion respects the caret where
one exists (workspace editor, dialog fields); append-only inputs (nav
filter, palette, password) extend the buffer.

## Acceptance Criteria

- [x] Pasting into the workspace field editor inserts at the caret
- [x] Pasting into any dialog field (onboard, restore, generate, manage
  create/move) inserts at the caret
- [x] Pasting into the add form, the nav filter, and the palette works
- [x] Pasting into the elevation password prompt works
- [x] Newlines and other control runes in pasted content are stripped
- [x] `go test ./...` passes

## Notes

Fixed in the closing commit below.
