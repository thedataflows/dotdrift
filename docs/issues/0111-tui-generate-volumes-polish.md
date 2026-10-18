---
type: Issue
title: TUI generate volume picker and view polish
description: A volume picker on the generate mounts source row, per-view counts on the view row, footer op-end marks, detail pane scrolling.
tags: [issue, tui, views]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0111: TUI generate volume picker and view polish

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, views]
- **Assignee**: agent
- **Related**: [TUI views design spec](../superpowers/specs/2026-10-18-tui-views-design.md), slice 5
- **Related code**: [`internal/tui/dialog.go`](../../internal/tui/dialog.go), [`internal/tui/views.go`](../../internal/tui/views.go)
- **Closing commits**: de438e1

## Summary

Slice 5 of the TUI views design, closing the TUI-views work: the
generate mounts dialog picks a volume instead of typing a source blind,
the view row numbers each loaded view's rows, the footer marks an ended
op ✓/✗, and the Status/Plan detail panes scroll instead of clipping
silently.

## Details

**Volume picker.** On the generate dialog's mounts `source` row,
`ctrl+o` lists the detected volumes through `Writes.Volumes` (the
consumer interface over `WritesArea.Volumes`, previously unused by the
TUI). The call snapshots the dialog's current layer/module/host/user
fields into the `generate.Selection`. The picker renders each volume's
UUID, filesystem type, label, size, and mountpoints, with a MANAGED
mark on a volume the target module already mounts. A pick fills
`source` with `UUID=<uuid>` only.

**View-row counts.** The slice 1b counts slot fills in: a loaded read
numbers its rows on the view title — Status counts its findings, Plan
counts the plan's entries. The count renders last on the title so the
`(stale)` word and the `[scope]` suffix stay contiguous.

**Footer end marks.** An `opFinishedMsg` note carries the op's end
mark: ✓ on success, ✗ on failure. The success mark rides the existing
4 s message-slot fade; the failure mark persists like any failure note.

**Detail pane scrolling** (parked from the Task 8 review). The Status
and Plan detail panes clipped long content silently. With the detail
pane focused (`tab`), the movement keys scroll it — the list panes'
windowing idiom without a cursor — and a clipped side carries a dim
overflow mark naming the way to the tail. Moving the list cursor resets
the detail scroll; a landing read does too.

**Stale headers.** The three audit-§8 stale header comments are gone:
the `doc.go` package comment (still described the absorbed huh wizard),
the `compositor.go` header (named the deleted M14 `shell.go` and the
finished cleanup task), the `workspace.go` header (M15 milestone
framing).

## Acceptance Criteria

- [x] `ctrl+o` on the mounts `source` row lists the volumes; the call
  snapshots the dialog's identity fields; a pick fills `UUID=<uuid>`
  only
- [x] The picker renders UUID, type, label, size, mountpoint; only
  managed volumes carry the MANAGED mark
- [x] `ctrl+o` off the source row lists nothing
- [x] The view row counts a loaded Status view's findings and a loaded
  Plan view's entries; an unloaded view shows no count
- [x] The footer shows ✓ after a successful op (fading with the slot)
  and ✗ after a failed one (persisting)
- [x] A long Status diff or Plan section marks the clipped tail; `tab`
  + the movement keys scroll to it; a cursor move resets the scroll
- [x] Keys stay in `keyTable()`/the view hooks; the picker's keys live
  in the contextual `?` help
- [x] The three stale header comments are deleted

## Out of Scope

- A filter row in the volume picker (few volumes per machine)
- Mouse support in the picker (keyboard is primary)
- Counts on the Modules/Profile titles (the Modules view has no count
  vocabulary; Profile is a flat fact list)

## Notes

Pinning tests: `TestGenerate_volumesPickFillsSource`,
`TestGenerate_volumesManagedMarked`, `TestViews_countsOnTabs`,
`TestFooter_opEndMark`, `TestStatusView_detailScrolls`,
`TestPlanView_detailScrolls`. The four view goldens regenerated —
each diff exactly the new count on the view row.
