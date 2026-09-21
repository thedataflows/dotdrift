---
type: Issue
title: TUI intuition round — undo/redo, visible editability, onboard from the selection, in-place choice cycling
description: ctrl+z/ctrl+shift+z undo the staged draft, rows show where editing lands (✎/＋/◂▸), `o` onboards into the selected module prefilled, and closed-set fields cycle in place with left/right.
tags: [issue, tui, ergonomics, follow-up]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0079: TUI intuition round

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [tui, ergonomics]
- **Assignee**: none
- **Related**: [0078](0078-tui-enter-fallback-and-footer-hints.md), [0077](0077-tui-add-forms.md), [0076](0076-tui-choice-editors-and-location.md)
- **Related code**: [`internal/tui/editing.go`](../../internal/tui/editing.go), [`internal/tui/workspace.go`](../../internal/tui/workspace.go), [`internal/tui/keymap.go`](../../internal/tui/keymap.go), [`internal/tui/dialog.go`](../../internal/tui/dialog.go)
- **Closing commits**: 90edb79, 0baf0eb, f22586e, 2908613

## Summary

Four frictions from real use: a staged draft can only be undone by
discarding all of it (`D`), nothing on the surface says which rows
editing will land on, the onboard flow is buried two keys deep behind
`w` with an empty form, and closed-set fields (scope, state, Type)
require a modal round trip to flip one value.

## Details

Four tasks, TDD-first, one commit each:

- **T-tui-undo** — every staged mutation funnels through three sites
  (`applyEdit`'s commit block, `applyRawLine`, `removeRow`). Each pushes
  a snapshot of the draft (raw, mode, maps, counters, cursor, broken-file
  banner) before mutating; history rides the draft, so it survives
  navigation. ctrl+z pops one step — including back past the first
  change, which drops the draft and returns the exact landed file;
  ctrl+shift+z redoes. New commits truncate the redo stack; save and
  discard drop history with the draft. Depth cap 50.
- **T-tui-marks** — rows announce their gesture, computed from the same
  registry that decides behavior so the mark cannot lie: an editable
  field row shows a dim `✎`, a closed-set row shows `◂▸` (it cycles), an
  addable section header shows `＋`, a structural container shows `＋`
  (enter adds into it). Read-only rows stay plain.
- **T-tui-onboard-here** — `o` on either pane opens the existing onboard
  dialog prefilled: from a nav row it carries that module's id and layer;
  from the workspace, the module and active tab. The fields stay
  editable — prefill, not preset. The palette gains the same action;
  `w → onboard` stays as the from-scratch path.
- **T-tui-cycle** — left/right on a closed-set row cycles its value in
  place through the same commit seam as the picker (`commitFieldAt`):
  no modal for a one-key flip. enter keeps the full picker (the list,
  the current marker). Type-into-row was rejected deliberately: j/k/a/d
  are printable navigation keys here, so free typing on a row would
  corrupt values.

## Acceptance

- Two committed edits, ctrl+z once: the first edit is gone from the
  draft, the surface and dirty marker follow; ctrl+z again: no staged
  changes, the landed file shows; ctrl+shift+z walks forward again.
  (`TestUndo_fieldEditStepsBackAndForward`.)
- Undo after removing a row restores the row at its old cursor
  (`TestUndo_removeRestoresRowAndCursor`, through the d/y confirm).
- A new commit truncates the redo stack (`TestUndo_newCommitTruncatesRedo`).
- Undoing a raw-mode repair reinstates the broken file, its rows, and
  the banner (`TestUndo_rawModeRestoresBrokenState`).
- The footer names ctrl+z only while a draft holds staged changes
  (`TestUndo_footerHintsUndoOnlyWhenStaged`).
- The meta scope row shows `◂▸`; right flips user→system without a
  modal, the cycle wraps, left steps back, and each flip is one undo
  step (`TestCycle_choiceRowsCycleInPlace`); free-text rows ignore the
  keys (`TestCycle_nonChoiceRowsIgnoreLeftRight`).
- The description row shows `✎`, the scope row `◂▸`, addable headers
  and containers `＋`, read-only rows nothing, and a dirty row keeps `●`
  instead of its mark (`TestMarks_rowsAnnounceTheirGesture`,
  `TestMarks_dirtyRowShowsTheDotNotTheMark`).
- `o` on a nav module row opens ONBOARD with `app demo` and the base
  layer preset; on a layer child, that child's layer; from the
  workspace, the active tab's; the fields stay editable; a shell
  without a selection refuses loudly (`TestOnboardHere_*`).
- The palette lists `onboard into demo` when a module is selected and
  carries its own target (`TestOnboardHere_paletteListsTheAction`).

## Verification

Fresh runs on the final tree: `go test ./... -count=1` — 20 packages
ok, 0 FAIL; `go vet ./...` clean; gofmt clean; golangci-lint
`run ./internal/tui/...` — 0 issues. Golden sweep: 26 frames
regenerated across the round (footer hint, marks, the palette's new
action row); verified mark-only via stripped diff (marks removed from
the new frames collapse onto the old ones modulo padding). Ponytail
audit: nothing speculative to cut — one snapshot type, two small
compositor functions sharing the `openOnboardInto` seam, marks derived
from the behavior predicates themselves.

