---
type: Issue
title: Workspace entry rows shade the key half darker than the value
description: Every workspace key/value row rendered key and value in one flat shade; the key half (description, source, hosts, a unit directive, …) now recedes one step toward the label hues while the value keeps the bright content hue.
tags: [issue, tui, theme]
timestamp: 2026-09-22T00:00:00Z
---

# ISSUE 0103: TUI workspace entry rows shade the key half darker than the value

- **Type**: enhancement
- **Status**: done
- **Priority**: low
- **Labels**: [tui, theme]
- **Assignee**: none
- **Related**: [0080](0080-tui-value-label-colors.md)
- **Related code**: [`internal/tui/theme.go`](../../internal/tui/theme.go), [`internal/tui/workspace.go`](../../internal/tui/workspace.go)

## Summary

0080 split values from fixed labels: values take the bright neutral
`value` hue, dialog/form labels recede to muted. The workspace's entry
rows never got the second half of that rule — a row like
`description the demo module` rendered key and value in the one flat
`rowText` shade, so nothing separated the field's name from its content.
The key half now renders in a new `key` hue, slightly darker than the
value, everywhere a row carries a key/value split.

## Details

A new palette hue `key` sits one step between `value` and `muted`:
ANSI 250 on dark (value 255, muted 245), ANSI 238 on light (value 234,
muted 245) — close enough to the value to read as the same text, far
enough to recede. It registers as `rowKey` in the ADR-0003 registry
(one style per visible concept; `TestNoDeadStyles` keeps it honest).

`wsRow` gains `valueAt`, the byte offset in `text` where the value half
begins (0 marks a single-shade row — packages, containers, headers,
hints). `wsRows` sets it at every key/value build site, with everything
up to and including the separator on the key side:

- meta: `id `, `description `, `scope ` (a bare `disabled` has no value
  and stays flat)
- links/writes: the target is the key half (`← source (mode)` and
  `(edit: line|block)` stay with the value)
- when leaves: indent + field + space
- hooks: `pre: ` / `post: `
- systemd unit directives and tools: through ` = `
- secrets/mounts/smb field rows and smb scalars: through the field name

`rowView` renders a split plain row as `rowKey(key) + rowText(value)`
inside the existing `rowText.MaxWidth` wrap — the proven 0080/0102
pattern of self-styled runs under one ANSI-aware clamp, so truncation
and the appended gesture marks/dirty dots/errors are untouched. The
cursor row keeps the flat accent: selection already owns the row's
color, and a split there would fight the bar.

## Acceptance criteria

- [x] A plain key/value entry row renders its key half in `rowKey` and
  its value in `rowText` — visibly different shades, key darker.
- [x] The split covers every key/value section: meta, links, writes,
  when leaves, hooks, systemd directives, tools, secrets/mounts/smb
  fields and scalars.
- [x] Single-shade rows (packages, containers, headers, hints, raw
  rows) render unchanged.
- [x] The cursor row keeps the flat 0075 accent — no split under
  selection.
- [x] The `key` hue differs from `value`, `muted`, and `dim` on both
  backgrounds and registers as a registry style.

## Test plan

- `TestTheme_rowKey` pins the hue (250 dark / 238 light) and its
  distinctness from value/muted/dim on both backgrounds.
- `TestKeySplit_entryRowsShadeTheKey` renders one row per key/value
  section with the cursor elsewhere and asserts the key half appears in
  `rowKey`'s run and the value in `rowText`'s.
- `TestKeySplit_singleShadeRowsStayFlat` pins packages, a container,
  and a section header as flat `rowText`/`sectionLabel` rows.
- `TestKeySplit_cursorRowKeepsTheAccent` asserts the selected row does
  not split.

## Out of scope

- Dialogs and forms already shade labels vs values (0080's
  `fieldLabel`/`rowText`); the palette, picker, and keymap already
  differentiate their halves. Nothing to change there.

## Notes

The raw-text mode of a broken file keeps its flat rendering: those rows
carry no key metadata, so `valueAt` stays 0 by construction.
