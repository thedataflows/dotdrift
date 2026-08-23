---
type: Issue
title: Onboard adopts orphaned module files
description: Onboard neither reports nor consumes orphaned files; it can even create them. Orphans in the onboarded module are adopted into module.toml with the run's mode.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0015: Onboard adopts orphaned module files

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [onboard, orphans, profile]
- **Assignee**: none
- **Related**: [issue 0012](0012-status-orphans-section.md), [issue 0014](0014-dir-source-subtree-orphan-false-positive.md), [profile layout](../product/profile-layout.md)
- **Related code**: [`internal/onboard/onboard.go`](../../internal/onboard/onboard.go), [`cmd/onboard.go`](../../cmd/onboard.go)
- **Closing commits**: pending

## Summary

Onboarding a module leaves pre-existing orphaned files (unreferenced by
any `[dotfiles]` entry) untouched and unreported, and re-onboarding an
entry whose source path changed strands the old source as a NEW orphan.
Instead, onboard **adopts** orphans: each one becomes a `[dotfiles]`
entry in the corresponding module.toml, using the run's mode.

## Details

An orphan in the onboarded module dir is a file (module.toml excluded)
outside the reference set of the merged entries — the union of the
existing module.toml and this run's entries, with issue-0014 semantics
(a source directory references its whole subtree). Adoption:

- **Invertible paths only.** `home/<rel>` maps back to target `~/<rel>`,
  `system/<rel>` to `/<rel>`. Files elsewhere in the module dir (hook
  scripts, notes) have no derivable target and are skipped.
- **Collapse.** A directory whose entire content is orphaned is adopted
  as one whole-dir entry (exactly what onboarding that directory would
  produce), not per-file entries.
- **Snapshot.** When the derived target exists live, the live content is
  copied over the module source first, so the forced takeover apply
  stays lossless — same rule as the paths the user passed.
- **Targets.** Adoptions whose derived target collides with an existing
  or this-run entry target are skipped, not double-claimed.
- **Dry run.** `--dry-run` lists would-be adoptions (`would adopt:`)
  without touching disk; a real run prints `adopted:` lines.
- Adopted entries ride the same mise config/apply as the run's own
  paths, so the module converges in one command.

## Acceptance Criteria

- [x] An orphaned `home/...` file in the onboarded module gains a
      `[dotfiles]` entry with the run's mode after onboard.
- [x] A fully-orphaned directory becomes ONE entry, not one per file.
- [x] Re-onboarding an entry whose old source differs leaves no orphan:
      the old source is adopted.
- [x] Live snapshots: when the live target exists it wins over the stale
      module copy before apply.
- [x] `--dry-run` prints the would-be adoptions and changes nothing.
- [x] Status after onboard shows no orphans for the module (given
      issue 0014).

## Out of Scope

- Orphans in modules other than the one being onboarded (status covers
  those).
- Adopting non-invertible files (module root junk stays for status to
  report).
- Cross-module target checks: adoption only guards targets within the
  module being onboarded; a collision with another module's claim is left
  to resolve's cross-module conflict error (contract #16), which fails
  loudly at the next apply (`ponytail:` noted in code).

## Notes

Reuses `mapPath`'s inverse; keeps adoption inside `internal/onboard`
(the reference-set walk is ~30 lines over the merged entries map).
