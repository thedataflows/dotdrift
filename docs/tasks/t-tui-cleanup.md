---
type: Task
title: T-tui-cleanup
description: Delete the M14 view stack and orphaned views, prune dead styles, final docs sweep — the proof that nothing depends on the old paths.
tags: [task, tdd, tui, cleanup]
timestamp: 2026-09-13T00:00:00Z
milestone: m15
---

# Goal

Remove the old presentation layer per
[M15](/milestones/m15-tui-compositor.md): the view stack push/pop, the
per-view input capture fallthrough, the details text-dump rendering, and
the editor-form pushed views — all superseded by tasks 1–7. Deletion is
last by design: both layers coexisted during the migration so every task
stayed green; this task is the proof nothing depends on the old paths.

# Tests first

- The deletion is behavior-preserving for the new layer: the full suite
  (fixed-size composited goldens + interaction-flow tests) must pass
  unchanged with the old code gone and the task flags removed.
- `TestNoDeadStyles` — the ADR-0003 registry carries no entries only the
  deleted views referenced.
- Flow tests re-run with flags removed: `TestShell_newLayerIsTheOnlyLayer`
  (no flag, no fallback path).

# Implementation notes

- Prefer deletion over adaptation: anything reachable only through the
  old view stack goes; anything shared moves to the new layer's packages
  with its tests.
- Prune the task flags that gated the migration; the new shell is the
  only `dotdrift tui`.
- Final gates: `go test ./...`, `go vet ./...`,
  `golangci-lint run ./...`, offline `GOFLAGS=-mod=vendor go test ./...`
  — all green on the final tree.
- If any task forced a domain test change during the milestone, surface
  it here for review — the redesign is presentation-layer; domain churn
  is a design smell, not a silent accommodation.

# Docs

- `docs/product/tui.md` final rewrite pass (no M14-era view-stack
  descriptions remain); README; M14 milestone annotated as superseded in
  presentation; issue 0073 closed; `docs/log.md` entry closing M15.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
