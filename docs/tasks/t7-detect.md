---
type: Task
title: T7 Detect facts
description: Host, user, os, gpu for layers and when.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m7
---

# Goal

Facts provider injectable into resolve and selection.

# Tests first

- `TestDetectOS_osReleaseFixture` — parse sample `/etc/os-release` files into `OS` and `Distro`.
- `TestDetectGPU_nvidiaAmdIntelUnknown` — classify GPU strings to `nvidia`, `amd`, `intel`, or `unknown`.
- `TestDetectBackend_fromDistro` — map distro to package backend (`paru` for arch/cachyos, `apt` for debian/ubuntu, `dnf` for fedora).
- `TestCLI_detect_output` — `dotdrift detect` prints stable line-oriented facts.

# Implementation notes

- `internal/detect` exposes `Detect() (*facts.Facts, error)`.
- OS detection reads `/etc/os-release` (ID, ID_LIKE).
- GPU detection runs `lspci` or reads `/proc/driver/nvidia/version` as a fallback; fail open to `unknown`.
- Backend defaults to `paru` for Arch/CachyOS; `apt` for Debian/Ubuntu; `dnf` for Fedora; `unknown` otherwise.
- All external reads are injected through interfaces so tests use fixtures.

# Acceptance

- `dotdrift detect` stable output.
- GPU fail-open to `unknown`.
- No real system calls in default tests.
