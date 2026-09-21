---
type: Issue
title: TUI add forms — `a` opens a labeled form instead of a grammar input, writes becomes addable
description: The add gesture opens a centered form with labeled rows and choice cycling, every section's add lands through the same commit path, and the writes section gains an add of its own.
tags: [issue, tui, ergonomics, follow-up]
timestamp: 2026-09-20T00:00:00Z
---

# ISSUE 0077: TUI add forms — `a` opens a labeled form instead of a grammar input, writes becomes addable

- **Type**: task
- **Status**: open
- **Priority**: high
- **Labels**: [tui, ergonomics]
- **Assignee**: none
- **Related**: [0074](0074-structural-section-editing.md), [0075](0075-tui-ergonomics-paging-selection-disclosure.md), [0076](0076-tui-choice-editors-and-location.md)
- **Related code**: [`internal/tui/editing.go`](../../internal/tui/editing.go)
- **Closing commits**: none

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

- [ ] `a` on any addable row opens the form; the form names the entry
      and destination; rows are labeled; no input is ever invisible
- [ ] choices cycle with left/right; enter commits; esc cancels; a
      refused commit stays open with the error rendered in the form
- [ ] every section's commit lands through the unchanged applyEdit
      pipeline (validation, splice, ledger, cursor-follow)
- [ ] writes `a` creates link and line entries; the line text may
      contain spaces
- [ ] the inline add input, its dead render branch, and the grammar
      lore are gone; the full suite stays green

## Out of Scope

- The profile dialogs (onboard/restore/generate) keep their own forms.
- Mouse double-click commit; the form focuses on click, commits on enter.
