---
type: Issue
title: TUI `d` (remove row) is dead on smb scalars/fields, secrets/mounts fields, when leaves, and writes block rows
description: confirmRemoveRow gates secrets/mounts/smb/when to container rows only, so pressing d on an smb scalar (group/users/avahi) or share field row silently does nothing — unlike packages, where every row prompts. Writes "edit: block" rows carry no family, so d is dead there too. The fix makes d remove the row's thing everywhere, with field removal unsetting the field (save-time validation names required ones).
tags: [bug, tui, dogfooded]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0087: TUI `d` (remove row) is dead on smb scalars/fields, secrets/mounts fields, when leaves, and writes block rows

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0074](0074-structural-section-editing.md) (the container/field grammar this amends), [0075](0075-tui-ergonomics-paging-selection-disclosure.md)
- **Related code**: [`internal/tui/editing.go`](../../internal/tui/editing.go) (`confirmRemoveRow`, `removeRow`), [`internal/tui/workspace.go`](../../internal/tui/workspace.go) (`wsRows`)
- **Closing commits**: 66c0a50

## Summary

In the module workspace, pressing `d` on an smb line does not prompt to
remove it — unlike packages, where every row prompts. The audit the
report asked for shows smb is not alone: every structural field row is
dead to `d`, and writes "edit: block" rows carry no family at all, so
they are dead to `d` too.

## Details

`confirmRemoveRow` gates `secrets`/`mounts`/`smb`/`when` to
`row.container` — "d removes entries and groups, not their fields". In
practice that means silent no-ops on rows that look exactly like the
removable rows of other sections:

- **smb scalars** (`group`, `users`, `avahi`) — dead.
- **smb share fields** (`path`, `comment`, `valid_users`, `writable`,
  `public`) — dead. Only the share's container row prompts.
- **secrets fields** (`env`, `description`, `allow_empty`) — dead.
- **mounts fields** (`source`, `destination`, `type`, `options`,
  `startat`, `state`) — dead.
- **when leaves** — dead (the 0074 design said "leaves clear by editing
  them to empty"; true but undiscoverable, and inconsistent with
  systemd directives, which `d` does remove).
- **writes block rows** (`target (edit: block)`) — built with `family
  ""`, so `d` hits the default case: dead. (Line writes and links carry
  `FamilyDotfiles` and remove fine.)

0074's container-only rule predates the disclosure UX; the user-facing
model that survived dogfooding is simpler: **`d` removes the row's
thing, everywhere** — an entry, a group, a scalar, or one field of an
entry. systemd already works this way (directives are removable rows).

Fix shape:

1. `confirmRemoveRow`: drop the container restriction for
   secrets/mounts/smb/when — every row of those families prompts.
2. `removeRow`: handle the non-container rows — smb scalars clear the
   scalar (`group` → "", `users` → nil, `avahi` → nil), share fields and
   secrets/mounts fields unset the addressed field, when leaves clear
   the leaf (the same zero-value writes the encoders already drop).
3. Writes block rows get `family: FamilyDotfiles` so `d` removes the
   entry; `startEdit` must keep refusing to text-edit a block entry
   (committing there would write into `Source`), falling back to the
   add form as today.

Removing a required field (mount `source`/`destination`/`type`, share
`path`, secret `env`) leaves an invalid entry; the existing tier-1/tier-2
checks and the save pipeline name it at save, and undo restores. That is
the same posture as an emptied systemd unit.

meta (`id`/`app`/`description`/`scope`/`disabled`) stays dead to `d` by
design — identity fields, clearable by editing to empty where the schema
allows it.

## Acceptance Criteria

- [x] `d` on an smb scalar row (group/users/avahi) prompts; yes clears
  the scalar and the row disappears
- [x] `d` on an smb share field row prompts; yes unsets the field
- [x] `d` on secrets and mounts field rows prompts; yes unsets the field
- [x] `d` on a when leaf prompts; yes clears the leaf
- [x] `d` on a writes block row prompts; yes removes the dotfile entry,
  and enter on it still does not open a text edit
- [x] Container removals (shares, secrets, mounts, when groups, systemd
  units) are unchanged
- [x] meta rows, section headers, and hint rows still have no remove
  confirm
- [x] Every new removal is one undo step (ctrl+z restores)
- [x] [`docs/product/tui.md`](../product/tui.md) states the uniform rule

## Out of Scope

- Removing whole sections in one keystroke.
- Any change to save-time validation of entries left invalid by a field
  removal (the existing checks already name them).
