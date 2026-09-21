---
type: Issue
title: TUI file picker opens on the field's current value, entry selected
description: The picker resolved the seed to the right directory but left the cursor at the top — and a directory seed opened inside itself. A valid seed now shows its parent directory with the seed's own entry selected, so enter re-confirms the current value and siblings are one keystroke away.
tags: [feature, tui, dogfooded]
timestamp: 2026-09-22T00:00:00Z
---

# ISSUE 0096: TUI file picker opens on the field's current value, entry selected

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0094](0094-tui-typed-inputs-multiline-filepicker.md) (the picker and the seed plumbing this builds on)
- **Related code**: [`internal/tui/filepicker.go`](../../internal/tui/filepicker.go)

## Summary

Dogfooded request: in the opened file/dir picker, if the existing path
is valid, open and select to that path directly, expanding the current
directory as needed. 0094 had already wired the seed end to end (every
call site passes the field's current value; `pickStartDir` resolved it
to a directory), but two things were missing: the cursor sat at the top
of the listing instead of on the seed's own entry, and a **directory**
seed opened inside itself — so nothing about the current value was
*selected*, and re-confirming it meant navigating back out.

## Details

- `pickStartDir` becomes `pickStart`, returning `(dir, name)`: an
  existing seed — file or directory — opens its **parent** and selects
  its own base name in the listing. Enter then re-confirms the current
  value (dirs mode picks the highlighted directory; either mode
  descends into it, files mode picks the highlighted file), and the
  siblings are one keystroke away — the desktop-dialog behavior.
- The filesystem root opens itself (there is no parent to select in).
- A missing seed still climbs to the nearest existing ancestor and
  selects nothing; an empty or unresolvable seed still opens home.
- A **hidden** seed (`.config`, `.hidden`) reveals dotfiles
  (`showHidden`) so the listing can select it — otherwise the picker
  would open on a value it refuses to show.
- A seed entry the mode filters out (a file seed in dirs mode) opens
  the parent with the cursor at the top — it is not a valid answer
  there.

Behavior change pinned by updated tests: a directory seed no longer
opens inside itself (0094's `TestPicker_seedResolution` and the smb
path wiring test asserted that), and a bare `~` seed — a valid existing
directory — gets the same parent-plus-selection treatment as any other
path. The picker unit tests that used the fixture root as a browsing
start now construct through a `browsePicker` helper (a missing leaf
seed climbs to the root with nothing selected).

## Acceptance Criteria

- [x] A directory seed opens its parent with the seed's entry selected;
  enter in dirs mode re-picks the seeded path
- [x] A file seed opens its parent with the file selected; enter
  re-picks it (files/either modes)
- [x] Either mode: enter on the selected directory descends into it
- [x] A missing seed climbs to the nearest existing ancestor, cursor at
  the top
- [x] A hidden seed reveals dotfiles and is selected
- [x] The filesystem root opens itself without panicking
- [x] Workspace path rows (smb `path`) open the picker on the field's
  current value with that value selected (compositor wiring test)
- [x] `go test ./...` and `go vet ./...` pass; docs updated

## Notes

TDD: 5 new tests red first (cursor at the top, no hidden reveal, dir
seed opening itself) plus 2 characterization pins (missing seed, root).
The GREEN was small and exactly where the seed plumbing anticipated it
— `newFilePicker` already received the value from every call site
(workspace rows, form ctrl+o, dialog browse), so no wiring changed.
GREEN-side fixes were test-side: the browsing-state constructions moved
to `browsePicker`, and the two tests that pinned "a directory seed
opens itself" were updated to the new intended UX.
