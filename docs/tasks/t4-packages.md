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
- `TestPacman_isInstalledNotFound` / `TestPacman_isInstalledErrorPropagates` — exit code 1 means "not installed"; any other runner error propagates.
- `TestPackagesStep_callsPresent` — packages step invokes backend with merged present set.
- `TestPackagesStep_callsAbsentAndPresent` — packages step invokes `Absent` before `Present`, executing explicit exceptions.
- `TestPackagesStep_removeErrorFails` — a failing remove stops the pipeline.
- `TestPackagesStep_propagatesContext` — the step passes its `ctx` through to the backend.
- `TestPackagesStep_idempotent` — already-installed packages are skipped.
- `TestFor_unknown` / `TestFor_auto_fallbackOnError` — unknown distro or failed auto-resolution yields a backend whose `Present`/`Absent`/`IsInstalled` return an explicit `no supported package backend for distro "X"` error instead of silently succeeding.
- `TestExecRunner_Run_alreadyCancelledContext` / `TestExecRunner_Run_contextCancelKillsCommand` — a cancelled context kills the child process (e.g. `sleep`) promptly.
- `TestApt_*` / `TestDnf_*` — argv for install/remove, `dpkg -l` / `rpm -q` installed vs exit-1 not-found, and non-exit-1 errors propagate from `IsInstalled`.
- Fake backend used in apply integration unit test.

# Implementation notes

- `internal/packages` defines a `Backend` interface.
- `Paru` backend implements `Backend` for Arch/CachyOS.
- `present` runs `paru -S --needed --noconfirm <packages...>`.
- `absent` runs `paru -R --noconfirm <packages...>`.
- `IsInstalled` uses `pacman -Q <pkg>` for idempotency checks.
- The packages step calls `backend.Absent(ctx, plan.Packages.Remove)` before `backend.Present(ctx, plan.Packages.Install)` so explicit exceptions are actually uninstalled.
- All `exec` calls go through a runner interface so tests fake them.
- `Runner.Run(ctx, name, args...)` is context-aware: `ExecRunner` uses `exec.CommandContext`, so Ctrl-C (context cancellation) kills the in-flight `paru`/`apt-get`/`dnf` child process instead of leaving it running.
- `Backend` methods (`Present`/`Absent`/`IsInstalled`) take `ctx` and forward it to the runner; the apply `Step.Run(ctx)` is the source of that context.
- Unknown backend names or failed `auto` resolution return a failing `noop` backend: every operation errors with `no supported package backend for distro "X"`, so the packages step fails loudly rather than reporting success without installing anything.
- `IsInstalled` treats exit code 1 as "not installed" `(false, nil)` on all backends (paru/pacman, apt/dpkg, dnf/rpm); any other runner error is propagated.
- `Apt.Present` runs `apt-get update` before `apt-get install -y`: a fresh machine/container has an empty or stale index and install fails with `E: Unable to locate package` (live-reproduced by the debian e2e container). This is intentionally asymmetric with `Dnf.Present` — dnf refreshes repository metadata as part of install and needs no separate index-update step. `Apt.Absent` does no update either (removal only reads the local dpkg database).

# Acceptance

- Idempotent installs; interface ready for future backends.
- No real `exec` in default tests.
- Unsupported distros fail the apply loudly, never silently.
- Cancelling the apply context terminates spawned package-manager processes.
