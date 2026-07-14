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
- `TestPackagesStep_callsPresent` — packages step invokes backend with merged present/absent sets.
- `TestPackagesStep_idempotent` — already-installed packages are skipped.
- Fake backend used in apply integration unit test.

# Implementation notes

- `internal/packages` defines a `Backend` interface.
- `Paru` backend implements `Backend` for Arch/CachyOS.
- `present` runs `paru -S --needed --noconfirm <packages...>`.
- `absent` runs `paru -R --noconfirm <packages...>`.
- `IsInstalled` uses `pacman -Q <pkg>` for idempotency checks.
- The packages step in `internal/apply` uses the backend selected by `facts.Backend` (paru for now; apt/dnf are stubs).
- All `exec` calls go through a runner interface so tests fake them.

# Acceptance

- Idempotent installs; interface ready for future backends.
- No real `exec` in default tests.
