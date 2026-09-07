---
type: Issue
title: Route bare Arch packages back through the paru plugin
description: Decision reversal of 0043's bare-name half — bare Arch names emit paru: again (paru self-prompts sudo), so the embedded paru plugin stays load-bearing and 0045's deletion scope narrows to the aur/ marker switch.
tags: [issue, mise, bootstrap, packages, decision]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0054: Route bare Arch packages back through the paru plugin

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [mise, packages, decision]
- **Assignee**: agent
- **Related**: [0043](0043-builtin-aur-pacman-managers.md) (partially reverted), [0045](0045-delete-paru-plugin-after-aur-release.md) (scope narrowed), [0003](0003-paru-mise-package-plugin.md), [mise bootstrap alignment A4](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go), [`internal/paru/`](../../internal/paru/), [`cmd/paru.go`](../../cmd/paru.go)
- **Closing commits**: d5ec01c

## Summary

0043 routed bare Arch package names to mise's built-in `pacman:` manager.
User decision: go back to paru for **all** Arch packages — repo and AUR
alike. paru prompts for sudo itself when it needs to elevate, which is the
elevation UX the dogfood workflow expects on fresh systems. `pacman:` remains
available only as an explicit author prefix.

## Details

- `PrefixedPackages`: drop the `paru → pacman` backend rewrite. Bare names on
  the paru backend emit `paru:<name>` again; `aur/<pkg>` → `paru:<pkg>` is
  unchanged; explicit `manager:pkg` still passes through (so `pacman:foo`
  stays expressible for anyone who wants the built-in).
- Verified empirically against the installed mise 2026.9.1 + the embedded
  plugin (already in the registry): `mise bootstrap packages status` with
  `"paru:curl"` reports `installed` (8.22.0-1.1), a bogus name reports
  `missing`, exit 0.
- Consequence for [0045](0045-delete-paru-plugin-after-aur-release.md): its
  premise ("delete the plugin stack once aur: ships") no longer holds — bare
  names keep the plugin load-bearing indefinitely. 0045 narrows to the
  `aur/` → `aur:` marker switch + `MinMiseVersion` bump; the embedded plugin,
  `dotdrift paru` commands, and registry maintenance **stay**.
- Unchanged: `internal/packages` Paru backend (removal, `when.packages`
  probes), plugin embedding in `packagesStep.Run`, `pacman -Q` status
  protocol inside the plugin.

## Acceptance Criteria

- [x] Bare names on the paru backend emit `paru:<name>` (no `pacman:` rewrite)
- [x] `aur/<pkg>` still emits `paru:<pkg>`; explicit prefixes pass through
- [x] Empirical: `paru:curl` status reports installed on mise 2026.9.1
- [x] 0043 annotated as partially superseded; 0045 scope amended (plugin stays)
- [x] Docs updated (profile-layout `packages.present` bullet, alignment A4)
- [x] `go test ./...` and `go vet` green

## Out of Scope

- The `aur/` → `aur:` switch itself (blocked 0045, gated on an upstream mise
  release containing PR #12718).
- Deleting any part of the paru plugin stack (now permanent).
- Package version pins (0049).
