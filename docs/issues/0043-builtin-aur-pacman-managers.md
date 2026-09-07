---
type: Issue
title: Adopt built-in aur/pacman managers, delete paru plugin
description: Upstream mise implements aur (yay/paru) and pacman natively; dotdrift's embedded paru package plugin, its registry maintenance, and the dotdrift paru commands become dead weight.
tags: [issue, mise, bootstrap, packages, deletion]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0043: Adopt built-in aur/pacman managers, delete paru plugin

- **Type**: task
- **Status**: open
- **Priority**: medium
- **Labels**: [mise, packages, deletion]
- **Assignee**: none
- **Related**: [mise bootstrap alignment A4](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go), [`internal/paru/`](../../internal/paru/), [`cmd/paru.go`](../../cmd/paru.go)
- **Closing commits**: none

## Summary

mise ships `aur` (prefers yay, falls back to paru) and `pacman` (with
`state = "absent"`) package managers natively. dotdrift maps `aur/<pkg>` →
`paru:` plugin entries, prefixes bare Arch names with `paru:`, embeds a paru
plugin copied into mise's registry, and exposes `dotdrift paru
install/installed` commands that exist to serve that plugin. All of it is
superseded.

## Details

- `PrefixedPackages`: `aur/<pkg>` → `aur:<pkg>`; bare names on Arch →
  `pacman:<pkg>` (was `paru:`).
- Delete: the embedded plugin + `paru.EnsureInstalled` registry maintenance
  in `packagesStep.Run`, `cmd/paru.go` (verify the plugin is its only
  consumer first), and related tests/docs.
- Keep: `internal/packages` Paru backend — it is dotdrift's own system
  package backend (removal, `when.packages` probes), independent of the mise
  plugin.

Behavioral deltas to document: built-in aur prefers yay over paru when both
exist; aur/pacman version pins are status-only/skipped upstream (dotdrift
already pins everything to `"latest"` — no change).

## Acceptance Criteria

- [ ] Emitted `[bootstrap.packages]` keys use `aur:`/`pacman:` on Arch; no `paru:` keys remain
- [ ] Paru plugin embedding, registry maintenance, and `dotdrift paru` commands deleted
- [ ] `internal/packages` Paru backend still drives removal and probes
- [ ] Docs updated (cli-surface, profile-layout, contract as applicable)
- [ ] `go test ./...` and `go vet` green

## Out of Scope

- Package version pins (alignment A5).
- Emitting `state = "absent"` for pacman entries (own-backend removal stays).
