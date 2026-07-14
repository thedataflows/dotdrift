---
type: Task
title: T0 Scaffold
description: Module, cmd, packages, testutil, CI.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m0
---

# Goal

Create Go module and CLI skeleton.

# Tests first

- `TestHelp_exitsZero` — `dotdrift --help`
- `TestSubcommands_registered` — modules, plan, apply, status, detect, onboard, init

# Implementation notes

- Packages per [package layout](/engineering/package-layout.md)
- Subcommands may return “not implemented” until later milestones

# Acceptance

- `go test ./...` green; binary builds