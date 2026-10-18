---
type: Issue
title: TUI shell views + Profile view (slice 1)
description: Add the TUI view row, view cycling, stale/refresh, and the read-only Profile view.
tags: [issue, tui, views]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0107: TUI shell views + Profile view (slice 1)

- **Type**: feature
- **Status**: open
- **Priority**: medium
- **Labels**: [tui, views]
- **Assignee**: none
- **Related**: [TUI views design spec](../superpowers/specs/2026-10-18-tui-views-design.md)
- **Related code**: [`internal/service/`](../../internal/service/), [`internal/tui/`](../../internal/tui/)
- **Closing commits**: none

## Summary

Add the shell's view row (Modules · Status · Plan · Profile), view
cycling, the stale/refresh mechanism, and the read-only Profile view —
slice 1 of the TUI views design, the cheapest read that proves the
frame.

## Details

Spec: `docs/superpowers/specs/2026-10-18-tui-views-design.md`, goals 3
and slice 1. The Profile view lists the `detect` facts (host, user, os,
kernel, distro, GPU, backend), the profile paths, and the resume state.
Read only. The Profile view does not probe (settled at builder-check):
it uses a light read — facts, the plan read, the state file, and the
other-accounts list. Slice 1a adds that read to the service layer's
reads area (`ReadsArea.Profile`, returning `ProfileRead`); ADR-0008's
doorway rule applies — the TUI reads through the service layer only.

## Acceptance Criteria

- [ ] A view row under the header lists Modules · Status · Plan ·
  Profile, the active view marked; one key cycles views, number keys
  jump (keys from the one binding table).
- [ ] The Modules view is today's base screen, unchanged.
- [ ] The Profile view shows the detect facts, the profile paths, the
  resume state, and the other-accounts note. Read only, no probes.
- [ ] View state lives for the session; a write or a finished apply
  marks the read views stale; `r` re-reads the current view, `R`
  re-reads all views.
- [ ] `ReadsArea.Profile` returns facts, state, plan, and other
  accounts without probing; covered by a service test.
- [ ] Tests and goldens for the new frames at 100x30 and 64x24;
  `docs/product/tui.md` and `docs/log.md` updated in the same commit.

## Out of Scope

- Status view, Plan view, apply gate options, generate volumes, polish
  (slices 2–5, own issues).
- Probe timeouts, focus-regain re-reads.

## Notes

Each slice runs TDD red first; contract and ADR change only if a slice
breaks an invariant — none is expected.
