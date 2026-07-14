---
type: Task
title: T2 Resolve and plan
description: Merge layers; plan command.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m2
---

# Goal

Implement [merge rules](/product/merge-rules.md) and produce a side-effect-free plan.

# Tests first

- `TestMergePackages_userAbsentBeatsHostPresent` — user `packages.absent` cancels host `packages.present`.
- `TestMergePackages_presentIdempotent` — duplicate present entries collapse.
- `TestMergeDotfiles_userWinsSameTarget` — user overlay overrides host/base for the same target.
- `TestMergeTools_userWins` — user tool version overrides host/base.
- `TestFileOverlay_userReplacesHost` — user overlay file replaces host overlay file for the same relative path.
- `TestSelectionFingerprint_stable` — same profile + facts → same fingerprint; different selection → different fingerprint.
- `TestCLI_plan_golden` — `dotdrift plan` prints resolved plan without side effects.

# Implementation notes

- `internal/resolve` builds a `Plan` from a loaded `profile.Profile`.
- For each selected module, merge base → host → user overlays:
  - `packages.present` accumulates; `packages.absent` removes entries from the set.
  - `tools` map keys: higher layer wins.
  - `dotfiles` target keys: higher layer wins; lower-layer entries for the same target are dropped.
- The `Plan` contains ordered steps: packages, tools, dotfiles, hooks.
- `dotdrift plan` calls `resolve.Plan(profile, facts)` and prints the plan.

# Acceptance

- User > host > module for all layered values.
- `modules.disable` union is enforced before resolve.
- `dotdrift plan` has no side effects.
