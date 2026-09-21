---
type: Issue
title: Picker location bar accepts paths that do not exist at all
description: A typed path in the file picker's ctrl+l location bar picks as typed even when its parents do not exist; existence rules belong to commit validation.
tags: [issue, tui, filepicker]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0099: TUI picker location bar accepts paths that do not exist at all

- **Type**: feature
- **Status**: done
- **Priority**: low
- **Labels**: [tui, filepicker]
- **Assignee**: none
- **Related**: [0094](0094-tui-typed-inputs-multiline-filepicker.md), [0098](0098-tui-link-forms-target-browse.md)
- **Related code**: [`internal/tui/filepicker.go`](../../internal/tui/filepicker.go)

## Summary

The picker's ctrl+l location bar refused a typed path whose parent
directory does not exist ("a typo in the path's spine refuses"). Drop
the parent check: a not-yet-existing path picks exactly as typed.

## Details

0094's deviation #2 let the location bar accept a new name *while its
parent exists* — enough for a fresh mountpoint under `/mnt`, but not
for a link `target` like `~/.config/app/thing` when `.config/app` is
not there yet, nor for a server-side share path whose spine only
exists on the server. With 0098 both link forms browse `target`, so
the picker must not be stricter than the plain text field it stands in
for (manual typing into the field always accepted anything).

Existence is not the picker's business in the first place: fields that
require an existing path (e.g. link `source`) enforce it in
`commitFieldAt`/form validation, and the refusal renders inside the
modal — the same layer every other field error uses. What the picker
still refuses: picking an existing *file* in dirs mode (a type
mismatch, not an existence question), and it still never *navigates*
into an unreadable directory.

## Acceptance Criteria

- [x] A typed path with missing parents (e.g. `<tmp>/no/such/place`)
      picks as typed and closes the bar.
- [x] Existing behavior is unchanged: an existing directory navigates,
      an existing file picks outside dirs mode, tilde expands, and a
      file typed in dirs mode is refused with the mode-mismatch error.

## Out of Scope

- Resolving relative typed paths against the shown directory (the bar
  is seeded with the absolute cwd; tilde expands — relative input is
  undefined behavior, unchanged from 0094).

## Notes

One-branch change in `locEnter`'s not-found case; the removed refusal
half of `TestPicker_locationBarRefusesBadPaths` became the new
pick-asserting test (the intentional behavior change).
