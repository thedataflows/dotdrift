---
type: Milestone
title: M11 Hooks
description: Pre/post apply hooks declared in module.toml, executed as mise tasks.
tags: [milestone]
timestamp: 2026-07-19T00:00:00Z
order: 11
---

# Goal

Let modules run user-declared shell commands before and after the apply pipeline.

# Exit criteria

- `[hooks]` `pre`/`post` schema parsed from `module.toml`; layers append
  (base → host → user) and modules aggregate in selection order.
- Apply pipeline is `hooks-pre → packages → tools → dotfiles → hooks-post`;
  hooks run as mise tasks from the profile root with the `DOTDRIFT_*` facts
  environment.
- Hooks visible in `dotdrift plan` (text and `--json`); suppressible via
  `dotdrift apply --no-hooks` or `DOTDRIFT_NO_HOOKS=1`.
- `go test ./...`, `go vet ./...`, `golangci-lint run ./...` green.

# Tasks

1. [T-hooks](/tasks/t-hooks.md)

# Depends on

[M9](m9-polish.md)
