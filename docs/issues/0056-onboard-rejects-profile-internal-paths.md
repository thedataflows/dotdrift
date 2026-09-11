---
type: Issue
title: Reject profile-internal paths in onboard
description: onboard/adopt brings EXTERNAL files into the profile; the 0017 repair that registered profile-internal module files into their own module.toml (silently overriding the mandatory --app) is deleted — such paths are now a loud error.
tags: [issue, onboard, deletion, ux]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0056: Reject profile-internal paths in onboard

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [onboard, deletion, ux]
- **Assignee**: agent
- **Related**: [0017](0017-onboard-adoption-units-and-profile-paths.md) (semantics amended), [0055](0055-onboard-app-flag-mandatory.md) (removes its "deliberate survivor")
- **Related code**: [`internal/onboard/onboard.go`](../../internal/onboard/onboard.go), [`cmd/onboard.go`](../../cmd/onboard.go)
- **Closing commits**: f9e8704

## Summary

onboard/adopt exists to bring **external** (live) files into the profile. The
0017 repair gave profile-internal module files a second job: passing one
registered it into its own layer's `module.toml`, overriding `--app`,
`--host`, and `--user`. After 0055 made `--app` mandatory, that produced an
odd combination: a required flag whose value is silently ignored for these
paths. User decision: the special case is scope creep — delete it and reject
profile-internal paths loudly.

## Details

- A path inside a module layer (`modules/<app>/...`,
  `hosts/<h>/modules/<app>/...`, `users/<u>/modules/<app>/...`) now fails
  immediately, naming the module and layer and the remedy:
  `onboard: <path> is already inside module "easyeffects" (hosts/cri-pc) —
  onboard adopts live paths only; declare it in that module.toml directly`.
  No stat, no copy, no declaration, `--app` never overridden.
- The "inside the profile but not a module file" error is unchanged.
- Deleted: the `directed`/`directedDir` pathway — per-path layer detection
  bookkeeping, the app-name override, the directed target-layer switch case,
  the host-owner derivation for claims, directed refs marking, and the
  directed adoption loop with its notices.
- **Kept (unchanged 0017 substance)**: the orphan sweep's anti-mangling
  protections — adoption claims are bounded by declared targets across all
  layers, dir units never claim shared namespace roots, and orphan files
  still ride any live-path onboard (that sweep is how a hand-placed module
  file gets registered now: onboard any live path of the module, or write
  the `[dotfiles]` entry by hand).

## Acceptance Criteria

- [x] Module-file paths (base/host/user layers, existing or not) fail with the loud naming error
- [x] `--app` value is never silently overridden; `--host`/`--user` always apply
- [x] Orphan sweep on live-path onboard unchanged (0017 protections intact)
- [x] README scenarios, cli-surface row, 0017/0055 annotations, log.md updated
- [x] `go test ./...` and `go vet` green

## Out of Scope

- Changing the orphan sweep itself (bounds, collapse rules).
- A separate `register` verb for undeclared module files (YAGNI — the sweep
  plus hand-editing covers it).
