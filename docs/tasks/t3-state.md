---
type: Task
title: T3 State and apply driver
description: Resume cursor and pipeline orchestration.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m3
---

# Goal

State + apply driver with fake steps.

# Steps order

`resolve` → `packages` → `tools` → `dotfiles` → `hooks`

# Tests first

- `TestState_roundTrip` — save and load state preserves fields.
- `TestState_resetOnSelectionChange` — different fingerprint clears completed/current/error.
- `TestApply_continuesAfterFailure` — resumes from the failed step.
- `TestApply_successRerunsFullPipeline` — completed run reruns all steps.
- `TestStepOrder` — steps run in resolve → packages → tools → dotfiles → hooks order.
- `TestApply_noResumeFlag` — no `--resume` in CLI or API.

# Implementation notes

- `internal/state` persists JSON state to `~/.local/share/dotdrift/state.json` (or a configurable path).
- State fields: `selection` (fingerprint), `completed` (map of step names), `current` (step name), `status` (enum), `error` (string).
- `internal/apply` orchestrates the pipeline. Each step is an interface (`Step`) with `Run(ctx, plan, state) error`.
- The apply driver:
  1. Resolves the plan.
  2. Compares selection fingerprint to persisted state; if different, resets state.
  3. Runs steps in order, skipping already-completed steps on resume.
  4. On failure, sets `current` and `error` and returns the error.
  5. On success, marks all steps completed.
- Steps are backed by fakes in tests; no real backends.

# Acceptance

- No `--resume` flag.
- State fields only: selection, completed, current, status, error.
- Fresh, mid-fail, success, and selection-change behaviors are tested.
