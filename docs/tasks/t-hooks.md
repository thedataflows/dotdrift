---
type: Task
title: T-hooks
description: Pre/post apply hooks via mise tasks, with plan visibility and opt-out.
tags: [task, tdd, mise, hooks]
timestamp: 2026-07-19T00:00:00Z
milestone: m11
---

# Goal

Implement [hooks](/milestones/m11-hooks.md): user-declared `pre`/`post` shell
commands from `module.toml`, merged by append across layers, executed as mise
tasks around the apply pipeline.

# Tests first

- `TestMergeHooks_layerAppendOrder` — base, host, user commands concatenate in order.
- `TestMergeHooks_multiModuleSelectionOrder` — modules aggregate in selection order.
- `TestMergeHooks_emptyHooks` / `TestMergeHooks_preOnly` — missing/partial sections yield empty lists.
- `TestGenerateHookTasks_preAndPost` / `_empty` / `_preOnly` / `_escapesCommands` — `[tasks."hooks:pre"/"hooks:post"]` generation with `run` list, `dir`, and `DOTDRIFT_*` env; TOML round-trip via BurntSushi.
- `TestGenerateApplyConfig_includesToolsDotfilesAndTasks` / `_noHooks` — full apply config composes all sections.
- `TestHooksStep_runsTask` / `_ctxPropagates` / `_emptyCommandsSkipsRunner` / `_runnerErrorPropagates` / `_nilExecErrors` — step behavior against a fake mise runner.
- `TestApply_happyPath` (extended) — pipeline order hooks-pre first, hooks-post last via recorded events.
- `TestApply_noHooksFlag` / `TestApply_noHooksEnv` — `--no-hooks` and `DOTDRIFT_NO_HOOKS=1` each suppress both hooks steps.
- `TestStatus_*` (updated) — progress denominator is 5.
- `TestCLI_plan_hooksSection` / `TestCLI_plan_jsonOutput` (extended) — hooks commands rendered in text and JSON plans.

# Implementation notes

- `profile.ModuleConfig` gains `Hooks Hooks` (`toml:"hooks"`); `resolve.Plan`
  regains `Hooks HooksStep{Pre, Post []string}`, populated per module as
  base→host→user append inside the existing layer loop (no separate merge
  function: append happens where base/host/user configs are already loaded).
- mise task representation: `run` as a **list of command strings** (mise runs
  them in order), plus `dir` (absolute profile root) and an inline `env`
  table with `DOTDRIFT_PROFILE`, `DOTDRIFT_HOSTNAME`, `DOTDRIFT_USERNAME`,
  `DOTDRIFT_OS`, `DOTDRIFT_BACKEND`. Pinned by TOML round-trip tests.
- `mise.GenerateApplyConfig(plan, profileRoot, facts)` composes
  `GenerateConfig` + `GenerateHookTasks`; `GenerateConfig` keeps its old
  signature so `internal/onboard` is untouched.
- `ExecMise.RunTask(ctx, configPath, taskName)` runs `mise run --cd <dir>
  <task>` through the existing ctx-aware runner; the `Runner` interface and
  `FakeRunner` are unchanged.
- `mise.HooksStep{Exec, Commands, ConfigPath, Task, StepName}` implements
  `apply.Step`. Empty handling: `cmd/apply.go` skips construction for empty
  command lists AND `Run` no-ops on empty as a second line of defense.
- `pipelineStepNames` is `["hooks-pre", "packages", "tools", "dotfiles",
  "hooks-post"]`; `status` progress denominator becomes 5.
- Failure semantics: a hook failure fails its step and resume re-runs it —
  hooks-pre failing means nothing ran, hooks-post failing means everything
  else completed. Post-hooks must be idempotent (contract invariant 12).

# Docs

- contract invariant 12; profile-layout `[hooks]` schema; cli-surface
  `--no-hooks` / `DOTDRIFT_NO_HOOKS`; README hooks subsection.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
