---
type: Playbook
title: TDD rules
description: Binding red-green-refactor rules for every implementation task.
tags: [engineering, tdd]
timestamp: 2026-07-14T00:00:00Z
---

# Rules

1. Write or extend **failing** tests first for the behavior in the task.
2. Implement the minimum code to pass.
3. Refactor only while tests stay green.
4. Prefer fakes for `exec` (paru, mise, curl|sh). No network in default `go test ./...`.
5. Optional `//go:build integration` tests may use real mise if present.
6. Task is incomplete until listed tests pass and acceptance criteria hold.
7. Do not add enable/resume flags even if “convenient.”