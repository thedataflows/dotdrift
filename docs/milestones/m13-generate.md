---
type: Milestone
title: M13 Generate
description: dotdrift generate mounts|smb — declarative mount and share specs, generated systemd units and samba configs, CLI and interactive wizard.
tags: [milestone]
timestamp: 2026-07-27T00:00:00Z
order: 13
---

# Goal

Let users declare filesystem mounts and Samba shares as data in
`module.toml` and materialize them as self-contained, system-scoped
modules — `dotdrift generate mounts` renders systemd `.mount`/`.timer`
units, `dotdrift generate smb` renders `shares.conf` and seeds
`smb.conf` — from either strict CLI flags or an interactive wizard,
then lets the regular apply pipeline place (mise) and activate
(mounts/smb steps) them.

# Exit criteria

- **Overlay-only discovery (ADR-0001).** Presence = managed holds per
  layer: a module existing only under `hosts/<hostname>/modules/` or
  `users/<username>/modules/` is discovered and selected like a base
  module (`TestDiscover_hostOnlyModuleSelected`,
  `TestDiscover_userOnlyModuleSelected`). Layers dedup by directory name
  (base → host → user representative precedence); declared-id collisions
  across different directory names stay fatal; resolve merges overlay-only
  modules against an absent/empty base layer and takes module-level
  metadata from the representative layer. Contract invariant 1 updated.
- **Specs in module.toml (ADR-0002).** `[mounts.<name>]`
  (source/destination/type/options/startat/state) and `[smb]`
  (group/users/avahi + `[smb.shares.<name>]`
  path/comment/valid_users/writable/public) parse from `module.toml`.
  Mounts and shares merge whole-entry by name across layers; smb scalars
  replace when set (`users` wholesale, `avahi` distinguishes unset from
  false). Required fields and `state` are validated at resolve naming
  module and entry; mount `type` is never registry-validated at resolve.
  Contract invariant 7 extended.
- **internal/generate.** A filesystem registry (embedded `registry.toml`
  with presets from the pimp-my-cachyos reference script, kernel-gated
  `recommended_if`, per-type packages, whole-entry user override from
  `$XDG_CONFIG_HOME/dotdrift/generate.toml`); `lsblk`-backed volume
  detection with already-managed marks; `EscapePath`, a pure-Go
  `systemd-escape --path` port validated against ground-truth vectors
  from the real binary (systemd 258); pure renderers for `.mount`,
  oneshot `.service`, `.timer` (startat only), and `shares.conf`,
  golden-checked against the envsubst'ed reference templates.
- **Module writer.** `generate.WriteModule` is layer-aware (base/host/
  user) and idempotent: the same input produces a byte-identical module
  tree (`TestWriter_rerunByteIdentical`), stale generated units are
  garbage-collected when mounts shrink, `smb.conf` is seeded once and
  then user-owned, unrelated module.toml sections are preserved, and an
  unknown mount type errors before any write. Generated modules carry
  `scope = "system"`, the spec sections, the package union (registry
  packages plus `samba`/`avahi`), and dotfile entries targeting
  `/usr/local/lib/systemd/system/` and `/etc/samba/`.
- **CLI.** `dotdrift generate mounts|smb` with strict mode selection:
  any input flag → CLI mode with loud required-flag validation; no
  flags + terminal → wizard; no flags + no terminal → actionable error;
  `--tui` forces the wizard (errors without a terminal).
  `generate mounts --list-volumes` prints the detected volume table with
  MANAGED marks.
- **Wizard.** Interactive huh/lipgloss wizard (internal/tui) over a pure
  spec-builder state machine: layer/module selection, volume (lsblk
  MultiSelect, managed pre-marked) or network flows, kernel-recommended
  type defaults, multi-mount accumulation with a single WriteModule per
  run, abort-before-confirm writes nothing. CLI and wizard assemble
  `generate.Input` through the same shared helpers, so identical logical
  inputs produce byte-identical module trees
  (`TestGenerate_cliTuiEquivalence_*`). Contract invariant 15.
- **Activation steps.** Apply pipeline is `hooks-pre → packages → tools
  → dotfiles → dotfiles-system → mounts → smb → hooks-post`; the
  `mounts` step (mkdir destinations, daemon-reload, enable/disable units
  + startat timers, tolerant disable) and the `smb` step (group/user
  membership, avahi, testparm gate, service enablement, samba accounts
  with TTY-gated smbpasswd) are conditional, activation-only, and
  idempotent — they never write config files. Contract invariant 14.
- **Plan/status surfaces.** `dotdrift plan` text gains `mounts:`/`smb:`
  sections (omitted when empty, so no-mounts profiles render
  byte-identical) and `plan --json` carries `mounts`/`smb`; the status
  progress denominator is 8 (`TestStatus_denominatorEight`) with the
  conditional-step caveat documented.
- **Vendoring.** charmbracelet huh v1.0.0, bubbletea v1.3.6, and
  lipgloss v1.1.0 are vendored; offline `GOFLAGS=-mod=vendor go test
  ./...` green.
- `go test ./...`, `go vet ./...`, `golangci-lint run ./...` green.

# Tasks

1. [T-overlay-discovery](/tasks/t-overlay-discovery.md)
2. [T-generate](/tasks/t-generate.md)

# Depends on

[M12](m12-scope.md)
