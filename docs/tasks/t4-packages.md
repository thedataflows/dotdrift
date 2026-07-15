---
type: Task
title: T4 Packages backend
description: Paru/pacman package operations.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m4
---

# Goal

Package step for apply.

# Tests first

- `TestParu_presentCommandLine` — paru present command uses `--needed` and sorted packages.
- `TestParu_absent` — paru `-R` command for absent packages.
- `TestPacman_isInstalled` — check installed state via `pacman -Q`.
- `TestPackagesStep_callsPresent` — packages step invokes backend with merged present set.
- `TestPackagesStep_callsAbsentAndPresent` — packages step invokes `Absent` before `Present`, executing explicit exceptions.
- `TestPackagesStep_removeErrorFails` — a failing remove stops the pipeline.
- `TestPackagesStep_idempotent` — already-installed packages are skipped.
- Fake backend used in apply integration unit test.

# Implementation notes

- `internal/packages` defines a `Backend` interface.
- `Paru` backend implements `Backend` for Arch/CachyOS.
- `present` runs `paru -S --needed --noconfirm <packages...>`.
- `absent` runs `paru -R --noconfirm <packages...>`.
- `IsInstalled` uses `pacman -Q <pkg>` for idempotency checks.
- The packages step calls `backend.Absent(plan.Packages.Remove)` before `backend.Present(plan.Packages.Install)` so explicit exceptions are actually uninstalled.
- All `exec` calls go through a runner interface so tests fake them.

# Acceptance

- Idempotent installs; interface ready for future backends.
- No real `exec` in default tests.
