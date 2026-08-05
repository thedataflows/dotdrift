---
type: Issue
title: paru mise package plugin
description: A dotdrift-backed mise package-manager plugin so Arch/AUR packages install via paru instead of mise's plain pacman built-in.
tags: [issue, feature, mise, packages, arch]
timestamp: 2026-08-05T00:00:00Z
---

# ISSUE 0003: paru mise package plugin

- **Type**: feature
- **Status**: open
- **Priority**: high
- **Labels**: [mise, packages, arch, plugin]
- **Assignee**: none
- **Related**: [ADR-0004](../adr/0004-delegate-convergence-to-mise-bootstrap.md), [issue 0002](0002-delegate-convergence-to-mise-bootstrap.md), [mise package plugin spec](https://mise.jdx.dev/package-plugin-development.html)
- **Related code**: [`internal/packages/`](../../internal/packages/), [`internal/mise/`](../../internal/mise/)
- **Closing commits**: none

## Summary

dotdrift provides a mise package-manager plugin named `paru` so that Arch
packages (including AUR) install via `paru -S` instead of mise's plain
`pacman:` built-in. This unblocks deleting `internal/packages` (issue 0002,
tension 1) by preserving AUR support inside the mise convergence model.

## Details

mise's `[bootstrap.packages]` `pacman:` built-in-manager is plain pacman — no
AUR. dotdrift's Arch backend is `paru -S` (pacman + AUR), and the migration
doc references AUR packages (`ntfsprogs-plus`). mise package plugins
([spec](https://mise.jdx.dev/package-plugin-development.html)) are the
extension point: a Lua-based vfox plugin with batch
`PackageInstalled`/`PackageInstall` hooks, declared in `[bootstrap.plugins]`
and keyed as `paru:<pkg>` in `[bootstrap.packages]`.

### Design

Two parts: a thin Lua shim plugin (protocol adapter) and a `dotdrift paru`
subcommand (all logic in Go).

**Lua plugin** (dotdrift writes it to `$XDG_DATA_HOME/dotdrift/mise-plugins/paru/`):

```
paru/
├── metadata.lua
├── mise.plugin.toml      # [package-manager] requires = ["paru"], os = ["linux"]
└── hooks/
    ├── package_installed.lua   # ctx → dotdrift paru installed → {name,state,version}[]
    └── package_install.lua     # ctx → dotdrift paru install [--dry-run] [--update]
```

Each hook serializes the vfox `ctx.packages` batch to JSON, calls
`dotdrift paru <sub>`, and parses the JSON return into the vfox response
table. The Lua is a stable ~15-line shim; no paru logic lives in Lua.

**`dotdrift paru` subcommand** (Go, reuses `internal/packages` argv builder):

| Subcommand | Behavior |
|---|---|
| `dotdrift paru installed` | Reads the package batch (JSON on stdin); checks each via `pacman -Q <name>`; emits `[{name, state: "installed"\|"missing", version?}]` JSON. Side-effect free, never elevates. |
| `dotdrift paru install` | Reads the batch; runs `paru -S --needed --noconfirm <names>`. Honors `--dry-run` (print, don't run) and `--update` (paru refreshes metadata itself). |

**Resolver integration** (issue 0002): bare Arch package names translate to
`paru:<pkg>` keys; an explicit `pacman:<pkg>` prefix uses mise's built-in
pacman instead. dotdrift registers the plugin in the generated `mise.toml`
(`[bootstrap.plugins]` pointing at the local plugin path) on first Arch
apply, or via `mise plugin install package:paru <local-path>`.

### The sudo tension (primary risk)

The plugin hard contract states: *"Package plugins must never invoke `sudo`
in any hook. mise never elevates for them."* But `paru -S` self-elevates —
it calls `sudo pacman -U` internally to install. The plugin (via dotdrift)
runs `paru`, and paru calls sudo. Whether mise tolerates a plugin whose
wrapped command self-elevates is **unverified against v2026.8.2**.

This must be empirically validated before deleting `internal/packages`. The
validation: declare the plugin, run `mise bootstrap packages apply` with an
AUR package on a real Arch host, and confirm paru's sudo prompt reaches the
TTY and the install succeeds (or fails with a specific mise error).

**Fallback if mise blocks it:** dotdrift runs `paru -S --needed` for Arch
packages as a `[bootstrap.hooks.pre-packages]` task (dotdrift owns elevation
directly, mise never sees a plugin), and Arch packages are omitted from
`[bootstrap.packages]`. This loses mise's install-status tracking for Arch
but preserves AUR. `internal/packages`' paru slice (~100 LOC) is retained.

## Acceptance Criteria

- [ ] `dotdrift paru installed` correctly reports installed/missing for repo
      and AUR packages via `pacman -Q` (side-effect free, no elevation).
- [ ] `dotdrift paru install` runs `paru -S --needed --noconfirm`, honoring
      `--dry-run` and `--update`.
- [ ] The Lua shim plugin delegates to the subcommand and returns the vfox
      response shape (`{name, state, version?}` per package).
- [ ] **Sudo validation:** on a real Arch/CachyOS host with mise v2026.8.2,
      `mise bootstrap packages apply` with a `paru:<aur-pkg>` entry installs
      the AUR package (paru's sudo prompt reaches the TTY) — OR the fallback
      (`pre-packages` hook path) is implemented and documented.
- [ ] dotdrift registers the plugin (local path in `[bootstrap.plugins]` or
      `mise plugin install package:paru`) and emits `paru:<pkg>` keys for
      bare Arch package names.
- [ ] `go test ./...` green; an integration test covers the `dotdrift paru`
      subcommand argv and JSON I/O.

## Out of Scope

- Package removal/prune — mise's package-plugin v1 does not support uninstall
  (the spec reserves room for a future `PackageUninstall` hook); `packages.absent`
  handling moves to issue 0002's broader translation.
- Non-Arch platforms — only the `paru` manager; `apt`/`dnf` use mise built-ins.
- Version pins for AUR packages — `supports_version_pins = false` (AUR has no
  meaningful version pinning).

## Notes

The `requires = ["paru"]` in `mise.plugin.toml` means mise adds its shims and
global toolset bin paths to PATH but does not install paru. paru must be
pre-installed (or declared as a dotdrift-managed package/bootstrapped
separately); the e2e CachyOS image already ships paru from the CachyOS binary
repo.

`PackageInstalled` must be "side-effect free, fast, non-interactive, and never
elevate" — `pacman -Q` satisfies all four (read-only query, no sudo). Only
`PackageInstall` (via `paru -S`) triggers the sudo tension.
