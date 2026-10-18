---
type: Issue
title: TUI Plan view (merged plan sections, diff toggle)
description: The Plan view body on the view-row frame — merged plan sections with counts, per-section detail, copy-mode diff toggle.
tags: [issue, tui, views]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0109: TUI Plan view

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, views]
- **Assignee**: agent
- **Related**: [TUI views design spec](../superpowers/specs/2026-10-18-tui-views-design.md), slice 3
- **Related code**: [`internal/tui/planview.go`](../../internal/tui/planview.go)
- **Closing commits**: 594d8e1

## Summary

The Plan view gets its body: the full merged plan of `dotdrift plan`
(packages, tools, dotfiles, hooks, systemd, mounts, smb) as a section
list with counts on the left, the selected section's merged entries on
the right, plus the copy-mode diff preview the CLI shows under
`--diff` behind a `d` toggle. Slice 3 of the TUI views design.

## Details

The view reads `ReadsArea.Plan` (probe-free) and `ReadsArea.Diff`
through the compositor's narrow `Reads` seam (ADR-0008's doorway), on
the Status view's async pattern: first entry and `r` schedule the read
as a `tea.Cmd`, a second `r` in flight is a no-op. The detail pane
reuses `service.RenderPlanReport` rendered once into a buffer at
landing — the selected section's block is cut from that text, never
re-rendered by hand. `d` toggles the diff list (`service.RenderDiff`,
internal tool) in place of the section entries. The read is
yank-scoped like `p`/`P` (0093): the view title names the scope, a
yank change marks the view stale, and the 0108 stale-seq landing rule
holds — a mark set after the read started survives its landing.

## Acceptance Criteria

- [x] Left: one row per non-empty section in the report's order, with
  counts (packages name install/remove, hooks name pre/post)
- [x] Right: the selected section's entries exactly as
  `RenderPlanReport` renders them; the cursor swaps the detail
- [x] `d` toggles the copy-mode diff list in the detail pane
- [x] Yank scope applies; the title names it (`Plan [shell]`)
- [x] The stale-seq landing rule holds; `r` re-resolves and clears the
  mark when no mark landed mid-read
- [x] Keys only through the one binding table + the view hook

## Out of Scope

- Dep-tree rendering (`plan --deps`)
- Section counts on the view row (slice 5)

## Notes

Pinning tests: `TestPlanView_rendersMergedSections`,
`TestPlanView_diffToggle`, `TestPlanView_yankScopeNamed`,
`TestPlanView_midReadStaleMarkSurvivesLanding`, `TestPlanView_golden`
(`views-plan.golden` 100x30, `views-plan-small.golden` 64x24).
