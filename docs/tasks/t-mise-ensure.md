---
type: Task
title: Ensure mise
description: PATH detect, min version, install/upgrade policy with TDD.
tags: [task, tdd, mise]
timestamp: 2026-07-14T00:00:00Z
milestone: m5
---

# Goal

Implement [mise bootstrap](/product/mise-bootstrap.md) as `internal/mise.Ensure`.

# Hardcoded constant

```go
const MinMiseVersion = "2025.1.0"
```

# Tests first

- `TestEnsureMise_detectsInPath` — existing binary on PATH, version OK.
- `TestEnsureMise_installsWhenMissing` — no binary, installer succeeds.
- `TestEnsureMise_upgradesWhenTooOld` — binary exists, version < min, upgrade succeeds.
- `TestEnsureMise_returnsErrorWhenInstallFails` — installer fails, error surfaced.
- `TestVersionCompare_calendarVersion` — `2026.6.6` > `2025.1.0`, `2024.12.0` < `2025.1.0`.

# Implementation notes

- `Ensure` accepts an `Executor` interface so tests can fake `exec` and filesystem.
- Real executor looks for `mise` on PATH, runs `mise --version`, parses the version.
- If missing or too old, run the official installer script non-interactively (e.g. `https://mise.run | sh`).
- After install, verify the new binary reports a version ≥ min.
- All network/exec calls go through the executor; no real `exec` in default `go test ./...`.

# Acceptance

- `internal/mise.Ensure` is unit-tested with fakes.
- `go test ./...` runs offline.
