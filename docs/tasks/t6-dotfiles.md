---
type: Task
title: T6 Mise dotfiles step
description: Generate dotfiles; apply --yes; conflict stops.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m6
---

# Goal

Dotfiles only through mise.

# Tests first

- `TestGenerate_dotfilesEntries` (link/copy/template goldens) — `[dotfiles]` section from resolved plan.
- `TestMise_dotfilesApplyArgs_noForce` — `mise dotfiles apply` is invoked without `--force`.
- `TestDotfilesStep_conflictStops` — fake runner returns conflict; state current=dotfiles, error saved.
- `TestDotfilesStep_ok` — successful apply advances state.
- `TestApplyPipeline_fullFake` — full fake pipeline resolve→packages→tools→dotfiles green.

(Hooks: the `HooksStep` noop placeholder and `TestHooksStep_noop` were removed for v0.1, decision D4a — hooks are out of scope for the pipeline.)

# Implementation notes

- `internal/mise` generates the `[dotfiles]` section of `mise.toml` from the resolved plan.
- Dotfiles step writes the merged `mise.toml` and runs `mise dotfiles apply` (or `mise apply` if that is the correct command) with `--yes` only when `--yes` is passed to dotdrift.
- On conflict, surface the runner output and stop the pipeline. State current=dotfiles, error set.
- No auto `--force`.

# Acceptance

- Symlink, copy, and template entries are generated correctly.
- Conflicts stop the pipeline and keep resume state.
- Full fake pipeline test passes.
