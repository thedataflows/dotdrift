---
type: Issue
title: Link forms' target field browses with ctrl+o
description: Both link forms (add and the 0095 edit modal) let ctrl+o browse for the target with the file picker, mirroring source.
tags: [issue, tui, filepicker]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0098: TUI link forms' target field browses with ctrl+o

- **Type**: feature
- **Status**: done
- **Priority**: low
- **Labels**: [tui, filepicker]
- **Assignee**: none
- **Related**: [0094](0094-tui-typed-inputs-multiline-filepicker.md), [0095](0095-tui-links-edit-opens-link-modal.md), [0096](0096-tui-picker-opens-on-current-value.md)
- **Related code**: [`internal/tui/`](../../internal/tui/)

## Summary

In both link forms only `source` was marked browsable; `target` was a
plain text field. Mark `target` browsable too (`kindEither`), so ctrl+o
opens the file picker on it exactly as it does on `source`.

## Details

0094 deliberately excluded link `target` from browsing: the target is
the dotfiles map key — the destination the link is *created at* — so it
typically names a path that does not exist yet, and a picker over
existing files looked like the wrong tool. Two later facts retired that
argument:

1. 0094's own location bar (ctrl+l) accepts a new name while its parent
   exists — precisely the "doesn't exist yet" case.
2. 0096 seeds the picker on the field's current value, parent expanded,
   entry selected — so in the *edit* link modal ctrl+o on `target` opens
   exactly on the existing target.

The mode is `kindEither` (not `kindFiles`): a link target can be a
directory symlink (e.g. `~/.config/nvim`), and either mode still picks
directories via ctrl+enter. The writes add form's `target` already sets
the precedent for a browsable target (`kindFiles`).

## Acceptance Criteria

- [x] In the add link form, ctrl+o on the `target` row opens the picker
      in either mode; the pick fills the field and the form stays open.
- [x] In the 0095 edit link modal, ctrl+o on the `target` row opens the
      picker in either mode (seeded on the current target per 0096).
- [x] `source` browsing and every other form field are unchanged.

## Out of Scope

- A mode choice row on the link forms (0095's recorded out-of-scope
  item; unchanged here).

## Notes

One-line change per form spec (`target.browse = kindEither`); the
pick-seeding and commit plumbing are 0094/0096's, untouched.
