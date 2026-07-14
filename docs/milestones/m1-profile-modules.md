---
type: Milestone
title: M1 Profile and modules
description: Discover modules; presence means selected; modules CLI lists.
tags: [milestone]
timestamp: 2026-07-14T00:00:00Z
order: 1
---

# Goal

Load `dotdrift.toml` and `modules/*`; implement selection without enable lists.

# Exit criteria

- `dotdrift modules` lists selected/skipped + reason
- Disable union and `when` filters tested
- Fixture profile under `testdata/`

# Tasks

1. [T1 profile](/tasks/t1-profile.md)

# Depends on

[M0](m0-scaffold.md)