---
type: Issue
title: Link source browse is module-relative, not cwd-relative
description: The link forms' source field opens the file picker against the module layer directory and stores the pick relative to it; a pick outside the module directory is refused.
tags: [issue, tui, filepicker]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0100: TUI link source browse is module-relative, not cwd-relative

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, filepicker]
- **Assignee**: none
- **Related**: [0094](0094-tui-typed-inputs-multiline-filepicker.md), [0096](0096-tui-picker-opens-on-current-value.md), [0098](0098-tui-link-forms-target-browse.md)
- **Related code**: [`internal/tui/editing.go`](../../internal/tui/editing.go), [`internal/tui/addform.go`](../../internal/tui/addform.go), [`internal/tui/dialog.go`](../../internal/tui/dialog.go), [`internal/resolve/resolve.go`](../../internal/resolve/resolve.go)

## Summary

A link's stored `source` is module-layer-relative — `resolveSource`
joins it against each layer's module directory and hard-refuses escapes
and missing files. But the ctrl+o browse opened the picker on the
field's raw text (empty → `$HOME`; a relative value → `os.Stat`
resolved it against the *process* cwd) and the pick wrote the
**absolute** path verbatim into the field — a value resolve later
rejects. The link `source` browse now roots at the active layer's
module directory and stores the pick relative to it; a pick outside
the module directory is refused inside the picker.

## Details

`dlgField` gains `browseBase`: when set, `browseField`

1. seeds the picker against the base — an empty field opens *inside*
   the module directory, a relative value (the edit modal's prefilled
   source) seeds `base/value` so 0096 opens the parent listing with
   the entry selected;
2. converts the pick: an absolute path inside the base stores its
   base-relative path, a relative location-bar path stores as typed
   (cleaned), and either form outside the base refuses with "must be
   inside the module directory" — the picker stays open, the same
   contract as every other in-picker refusal.

The refusal is not a new restriction: `resolveSource` already rejects
any source outside the layer directories at resolve time; the picker
just fails at input time instead of leaving a broken draft. Only the
link forms' `source` fields (add link, edit link) set `browseBase`;
`target` (a live-system path) and every other browsable field keep
verbatim absolute picks.

## Acceptance Criteria

- [x] Add link form: ctrl+o on an empty `source` opens the picker
      inside the module layer dir; picking a file there stores the
      module-relative name.
- [x] Edit link modal: the prefilled relative source seeds the picker
      in the module dir with its own entry selected (0096).
- [x] A pick outside the module dir (absolute or escaping `..`)
      refuses with "must be inside the module directory"; the picker
      stays open.
- [x] A relative location-bar path stores as typed (cleaned) — the
      file may exist in another layer, so existence is not checked.
- [x] The link `target` browse and all other fields keep verbatim
      absolute picks (0094/0098 behavior pinned by existing tests).

## Out of Scope

- Resolving relative location-bar paths against the shown directory
  (0099's note stands: relative input there is the module-relative
  shorthand only on `browseBase` fields).
- Copying an outside file into the module on pick.

## Notes

The `addFormSpec`/`editLinkFormSpec` specs take the module layer dir
(`ws.activeDir()`) — the same dir the splice saves to, so the stored
relative path and the entry live in the same layer.
