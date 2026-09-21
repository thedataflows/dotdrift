---
type: Issue
title: TUI add forms — `a` opens a labeled form instead of a grammar input, writes becomes addable
description: The add gesture opens a centered form with labeled rows and choice cycling, every section's add lands through the same commit path, and the writes section gains an add of its own.
tags: [issue, tui, ergonomics, follow-up]
timestamp: 2026-09-20T00:00:00Z
---

# ISSUE 0077: TUI add forms — `a` opens a labeled form instead of a grammar input, writes becomes addable

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [tui, ergonomics]
- **Assignee**: none
- **Related**: [0074](0074-structural-section-editing.md), [0075](0075-tui-ergonomics-paging-selection-disclosure.md), [0076](0076-tui-choice-editors-and-location.md)
- **Related code**: [`internal/tui/editing.go`](../../internal/tui/editing.go), [`internal/tui/addform.go`](../../internal/tui/addform.go)
- **Closing commits**: e66c7b7, 28d62be, 8169659

## Summary

`a` in the workspace opens an inline input that renders nowhere: the row
renderer gates the in-place editor behind `!editing.add`, so the add
input captures every key while the user types blind. The grammar it
wants (`target source`, `post: cmd`, `field = value`, `-name`) is
invisible until enter refuses. The writes section refuses `a`
altogether (`addable("writes")` is false), so a line-write entry cannot
be created from the TUI at all.

## Details

Three tasks, TDD-first, one commit each:

- **T-tui-addform** — `a` opens a centered form modal (the 0076
  picker's box) titled with the entry and its destination
  (`add package · demo`). Labeled rows reuse the dialog primitives:
  text fields with hint placeholders, closed sets as `< >`-cycled
  choices (packages present/absent, hooks pre/post). Enter synthesizes
  the grammar string the pipeline already parses and commits through
  the same `applyEdit` path; a refusal keeps the form open with the
  error inside it. The invisible inline add input and its dead `+ `
  render branch are deleted.
- **T-tui-addform-structural** — the structural sections get their
  row specs: systemd a unit name or a directive + value inside a unit,
  secrets name + env or a field choice inside an entry, mounts a name
  or a field choice, smb a what-choice (share/group/users/avahi) at
  the root and a field choice inside a share, when a kind-choice
  (leaf/and/or/not) plus field choice + value for a leaf.
- **T-tui-writes-add** — writes becomes addable: a kind choice
  (link/line); link adds a symlink entry into links, line adds a
  line-write entry (target + line text, spaces allowed in the text).

## Acceptance Criteria

- [x] `a` on any addable row opens the form; the form names the entry
      and destination; rows are labeled; no input is ever invisible —
      `TestAddForm_opensWithLabeledRows` (also pins
      `c.ws.editing == nil` behind the form), the structural add tests
      pin the per-section titles (`add directive · demo.service`,
      `add smb · demo`)
- [x] choices cycle with left/right; enter commits; esc cancels; a
      refused commit stays open with the error rendered in the form —
      `TestAddForm_packagesAbsent` (state), `TestAddForm_hooksPhasePost`
      (phase), `TestSmb_scalarAndShareEdits` + `TestAddForm_smbWhatRelabelsValue`
      (smb what, plus the relabeling value row), `TestWhen_*` (kind),
      `TestAddForm_emptyNameRefusesInPlace`, `TestSystemd_unitNameTier1`,
      `TestWhen_notAddAndDuplicateRefuse`, `TestAddForm_writesEmptyRefuses`
- [x] every section's commit lands through the unchanged applyEdit
      pipeline (validation, splice, ledger, cursor-follow) — the form
      only synthesizes the grammar string; `commitAddAt` builds the
      same `wsEdit` an inline input would have produced
- [x] writes `a` creates link and line entries; the line text may
      contain spaces — `TestAddForm_writesLineCommit`
      (`export PATH=added` lands whole, no mode), `TestAddForm_writesLinkCommit`
      (symlink entry into links)
- [x] the inline add input, its dead render branch, and the grammar
      lore are gone; the full suite stays green — `rowView`'s
      `!editing.add` gate and `editLines`' unreachable `+ ` prefix
      deleted; every structural add test migrated from poking
      `c.ws.editing` to driving the form

## Notes

Found during T1: the form must not treat letters as navigation — the
first cut mapped `j`/`k` to up/down and typed `backup` as `bacup` (the
RED run caught it as a mounts walk failure). The form's up/down are
arrow-only; j/k type like any other rune. The writes line kind travels
through the pipeline as `target<pathSep>line` (the path separator is the
structural rows' own marker) because a line may hold spaces and `=`,
which the `target source` link grammar cannot express unambiguously.

## Out of Scope

- The profile dialogs (onboard/restore/generate) keep their own forms.
- Mouse double-click commit; the form focuses on click, commits on enter.
