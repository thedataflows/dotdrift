---
type: Milestone
title: M12 Scope
description: Module-level user/system dotfile scope — one apply covers both ~/ and /etc.
tags: [milestone]
timestamp: 2026-07-19T00:00:00Z
order: 12
---

# Goal

Let a module declare `scope = "user" | "system"` so a single `dotdrift apply`
covers both home-directory dotfiles and system dotfiles (e.g. `/etc`), with
the system portion applied via sudo.

# Exit criteria

- `module.toml` gains top-level `scope` (`"user"` default when omitted);
  invalid values are a resolve-time error naming the module and the value.
- `resolve.DotfileEntry` carries its module's scope; mixed user+system
  modules partition correctly.
- Apply pipeline is `hooks-pre → packages → tools → dotfiles →
  dotfiles-system → hooks-post`; `dotfiles-system` is appended only when at
  least one system-scope entry exists, generates its own config
  (`dotfiles-system/mise.toml`), and applies via `sudo -E mise dotfiles apply
  --cd <dir> [--yes]` — directly, without sudo, when EUID == 0. The
  `MISE_TRUSTED_CONFIG_PATHS` trust handling survives the elevation.
- `dotdrift plan` marks system entries (`module: <id> [system]`) and
  `plan --json` carries `scope` on each dotfile entry; `dotdrift modules` is
  unchanged.
- Hooks and packages are unaffected: packages self-elevate via paru, hooks
  carry their own inline privilege.
- `go test ./...`, `go vet ./...`, `golangci-lint run ./...` green.

# Tasks

1. [T-scope](/tasks/t-scope.md)

# Depends on

[M11](m11-hooks.md)
