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
- `TestMergePackages_absentInRemoveList` — absent packages land in the remove list.
- `TestMergeDotfiles_userWinsSameTarget` — user overlay overrides host/base for the same target.
- `TestMergeTools_userWins` — user tool version overrides host/base.
- `TestFileOverlay_userReplacesHost` — user overlay file replaces host overlay file for the same relative path.
- `TestSelectionFingerprint_stable` — same profile + facts → same fingerprint; different selection → different fingerprint.
- `TestResolveSource_traversalRejected` — `source = "../../..."` escaping the layer root is a resolve-time error naming module and source.
- `TestResolveSource_missingFileErrors` — declared source file missing from every layer is a resolve-time error.
- `TestResolve_overlayTOMLErrorPropagated` — malformed host/user overlay `module.toml` propagates with overlay path context.
- `TestResolve_crossModulePackageConflict` — package present in one module and absent in another is a conflict error naming package and both modules.
- `TestResolve_emptyHostnameErrors` / `TestResolve_emptyUsernameErrors` — empty facts with selected modules are explicit resolve errors.
- `TestCLI_plan_golden` — `dotdrift plan` prints resolved plan without side effects.

# Implementation notes

- `internal/resolve` builds a `Plan` from a loaded `profile.Profile`.
- For each selected module, merge base → host → user overlays:
  - `packages.present` accumulates; `packages.absent` removes entries from the set.
  - `tools` map keys: higher layer wins.
  - `dotfiles` target keys: higher layer wins; lower-layer entries for the same target are dropped.
- Resolve fails fast (see [contract](/product/contract.md) invariants 8–10):
  - empty `hostname`/`username` facts with selected modules;
  - unreadable/malformed host or user overlay `module.toml` (missing overlay is fine);
  - dotfile `source` escaping the layer root, or not found in any layer;
  - a package present in one selected module and absent in another.
- The `Plan` contains ordered steps: packages, tools, dotfiles, hooks.
- `dotdrift plan` calls `resolve.Plan(profile, facts)` and prints the plan.

# Acceptance

- User > host > module for all layered values.
- `modules.disable` union is enforced before resolve.
- Contained, existing dotfile sources only; conflicts and empty facts are errors, not silent behavior.
- `dotdrift plan` has no side effects.
