---
type: Milestone
title: M0 Scaffold
description: Testable Go CLI foundation and CI.
tags: [milestone]
timestamp: 2026-07-14T00:00:00Z
order: 0
---

# Goal

Runnable module with subcommand stubs and test harness.

# Exit criteria

- `go test ./...` passes
- CI runs test + vet (see `.github/workflows/ci.yml`, added 2026-07-19)
- Architecture notes match [product contract](/product/contract.md)

# Tasks

1. [T0 scaffold](/tasks/t0-scaffold.md)