---
type: Task
title: T9 Polish
description: init, status, docs, example profile.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m9
---

# Goal

v0.1.0 ship readiness.

# Tests first

- `TestInit_createsProfile` — `dotdrift init <dir>` creates a profile skeleton.
- `TestInit_clonesProfile` — `dotdrift init <git-url>` clones an existing profile.
- `TestStatus_showsState` — `dotdrift status` prints current step, last error, and selection fingerprint.
- `TestExitCodes_usage` — invalid args exit 2.
- `TestExitCodes_runtime` — runtime failure exits 1.

# Implementation notes

- `dotdrift init`:
  - If a local path is given, create `dotdrift.toml`, `modules/`, `hosts/`, `users/`.
  - If a git URL is given, clone it into the current directory or a specified path.
- `dotdrift status`:
  - Loads state from `internal/state`.
  - Prints `current`, `status`, `error`, and `selection` fingerprint.
- Exit codes:
  - 0 success.
  - 1 runtime failure (e.g. backend error, conflict).
  - 2 usage/invalid args.
- Ensure no `--enable` or `--resume` flags remain in CLI or docs.
- Ensure `go test ./...` green and binary builds.

# Docs

- README: contract, merge order, onboard, apply resume, mise ensure.
- Migration guide from pimp-my-cachyos.
- `examples/` small profile showing a module, host overlay, and user overlay.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
