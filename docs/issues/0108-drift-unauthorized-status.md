---
type: Issue
title: Drift unauthorized status for permission-denied findings
description: Add the drift status `unauthorized` for dotfiles findings whose probe failed with permission denied.
tags: [issue, drift, status]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0108: Drift unauthorized status

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [drift, status]
- **Assignee**: agent
- **Related**: [TUI views design spec](../superpowers/specs/2026-10-18-tui-views-design.md), contract invariant 17
- **Related code**: [`internal/drift/`](../../internal/drift/)
- **Closing commits**: none

## Summary

Add a drift status `unauthorized` for dotfiles findings whose probe
failed with permission denied. A failed or declined sudo elevation must
read `unauthorized`, not `unknown`. Slice 2a of the TUI views design;
the CLI and the later TUI Status view both consume it.

## Details

Contract invariant 17: file-access probes retry elevated via `sudo`.
When that retry fails or is declined, the finding kept the generic
`unknown` status, which hides the difference between "probe broke" and
"not allowed to read". The new status `Unauthorized` sits after
`Unknown` in `internal/drift/drift.go` (`Status.String()` =
`"unauthorized"`). One classification pass at the end of `Check` flips
a dotfiles-section `Unknown` finding to `Unauthorized` when its detail
carries a permission error (`permission denied`, case-insensitive).
`Render` shows the word `unauthorized` (no `(?)` suffix), counts
unauthorized findings separately, and appends `, U unauthorized` to the
summary when U > 0. The palette role is the unknown role (error).

## Acceptance Criteria

- [x] `Unauthorized` status after `Unknown`; `Status.String()` = `"unauthorized"`
- [x] End-of-`Check` classification: dotfiles `Unknown` with a permission-denied detail (case-insensitive) becomes `Unauthorized`
- [x] Render shows `unauthorized` (no `(?)`); summary appends `, U unauthorized` when U > 0
- [x] Other unknowns (mise missing, non-permission errors) stay `unknown`
- [x] Palette role for unauthorized = the unknown role
- [x] Contract invariant 17 amended

## Out of Scope

- TUI Status view rendering (slice 2b, consumes this status)
- Typed error causes on probes (the detail string is the carrier)

## Notes

Pinning tests: `TestCheck_permissionDeniedIsUnauthorized`,
`TestCheck_otherUnknownStaysUnknown`, `TestRender_unauthorizedCounts`;
`TestCheck_dotfileEditUnreadable` updated to expect `Unauthorized`.
