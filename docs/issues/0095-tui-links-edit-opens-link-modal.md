---
type: Issue
title: TUI links editing opens the link modal instead of a bare text input
description: Enter/e on a module-details links row opened a plain text edit of the source alone — the target (the map key) and the mode were unreachable. It now opens the add-link form's twin, prefilled, committing the two-field grammar through the same edit pipeline.
tags: [bug, tui, dogfooded]
timestamp: 2026-09-22T00:00:00Z
---

# ISSUE 0095: TUI links editing opens the link modal instead of a bare text input

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0077](0077-tui-add-forms.md) (the add forms this modal twins), [0087](0087-tui-remove-row-dead-on-structural-rows.md) (the same "every row verb does its thing" audit)
- **Related code**: [`internal/tui/editing.go`](../../internal/tui/editing.go), [`internal/tui/addform.go`](../../internal/tui/addform.go)

## Summary

Dogfooded report: editing an existing link in the module details view
behaved like a regular text edit. `startEdit` had no links case, so a
links row fell to the generic field input seeded with `row.value` — the
**source only** — and `mutateField` wrote only `d.Source`. The target
(the dotfiles map key) and the mode were unreachable from the TUI,
while `a` on the same section already opened the proper link form.

## Details

- `enter`/`e` on a links row now opens **the link modal**: the add
  form's twin (`edit link · <module>`), prefilled with the entry's
  target and source. The footer's enter verb reads `commits`, not
  `adds` (the form's verb is a field; add forms keep `adds`).
- The commit synthesizes the same two-field grammar the add form
  produces (`target source`) and rides the existing field-edit seam —
  `commitFieldAt` (which now returns the refusal for the form to render
  in place), `applyEdit`, `mutateField` — so validation, the splice
  round-trip, the ledger, undo, and the cursor following a rename are
  the pipeline's, unchanged.
- `mutateField`'s dotfiles edit branch parses the pair: changing the
  target **renames** the entry (the mode rides along); renaming onto an
  existing target refuses (`target %q already exists`); anything but
  two fields refuses (`edit as "target source"`). Writes rows (line
  text, same family) keep their free-text inline edit — the branch is
  unchanged for `Line != ""`, and `renamedKey` only follows a rename
  for `section == "links"`.

Out of scope: editing the link's **mode** (the add form hardcodes
`symlink` too — a mode choice row is a feature of its own, for both
forms at once).

## Acceptance Criteria

- [x] `enter`/`e` on a links row opens the link modal prefilled with
  target and source; no inline input hides behind it
- [x] Committing edits the source and keeps target and mode
- [x] Renaming the target moves the entry (mode preserved); the cursor
  lands on the renamed row
- [x] Renaming onto an existing target refuses in place; nothing stages
- [x] An emptied field refuses in place; nothing stages
- [x] `esc` pops the modal and stages nothing
- [x] A writes line row still opens the inline text edit (regression
  pinned)
- [x] `go test ./...` and `go vet ./...` pass; docs updated

## Notes

TDD: `editlink_test.go` red first (no modal opened; enter started the
inline edit), then green. Two test-side bugs surfaced during green and
were fixed in the tests, not the code: the `keyPress` helper has no
`backspace` case (real terminals deliver `KeyBackspace`), and
`~/.bashrc` is nine runes, not eight.
