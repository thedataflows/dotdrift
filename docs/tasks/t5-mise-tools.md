---
type: Task
title: T5 Mise tools step
description: Generate tools section and mise install.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m5
---

# Goal

Tools step after EnsureMise.

# Tests first

- `TestGenerate_toolsSection` (golden) — `mise.toml` `[tools]` section from resolved plan.
- `TestToolsStep_callsMiseInstall` — fake runner records `mise install` invocation.
- `TestToolsStep_runsEnsureFirst` — `EnsureMise` is called before `mise install`.
- `TestToolsStep_failurePersistsState` — failure updates state current=tools and error.

# Depends on

[Ensure mise](t-mise-ensure.md)

# Implementation notes

- `internal/mise` generates a `mise.toml` with the merged `[tools]` section from the resolved plan.
- The tools step writes the generated file to a temp/state directory, then invokes `mise install` via the runner.
- `EnsureMise` runs first; if it fails, the step fails and state is updated.
- Use a `Runner` interface for `exec` so tests can fake it.

# Acceptance

- `dotdrift apply` installs tools for selected modules before dotfiles.
- No real network/exec in default tests.
