---
type: Task
title: T8 Onboard
description: Module factory; copy; EnsureMise; mise apply; no enable.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m8
---

# Goal

Implement `dotdrift onboard` per [CLI surface](/product/cli-surface.md).

# Tests first

- `TestPathMap_homeAndSystem` — map absolute paths to module-relative source paths and target paths.
- `TestInferApp_configDir` — infer app name from `~/.config/<app>` paths.
- `TestOnboard_copiesTree` — live paths are copied into the module directory.
- `TestOnboard_writesToml` — `module.toml` is written with discovered metadata.
- `TestOnboard_noEnableFlagNeeded` — created module is selected by presence on next `apply`.
- `TestOnboard_orderCopyThenEnsureThenMiseApply` — copy, ensure mise, then mise apply.
- `TestOnboard_defaultModeLink` — default mode is `link`.
- `TestOnboard_conflictKeepsModule` — if target exists, keep module files and fail the command.
- `TestOnboard_dryRun_noSideEffects` — `--dry-run` writes nothing and invokes nothing.

# Implementation notes

- `internal/onboard` implements the onboard logic.
- Copies live paths into `modules/<app>/` under the profile, writes a `module.toml`, then runs `EnsureMise` and `mise dotfiles apply`.
- Default mode is `link`; `--mode copy|template` overrides.
- `--app` overrides inferred app name.
- `--package` and `--tool` add entries to `module.toml`.
- `--host` writes files only to the host overlay directory.
- No `--enable` flag; presence selects the module automatically.

# Acceptance

- Defaults only for happy path; presence enables module.
- Copy → ensure mise → mise apply order is enforced in tests.
- Dry run has no side effects.
