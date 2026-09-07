---
type: Issue
title: "TICKET: systemd user units and timers module section"
description: Decision ticket — schema and semantics for a [systemd] module.toml section that emits [bootstrap.linux.systemd.units] (user services + timers).
tags: [issue, wayfinder:grilling, mise, systemd]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0048: TICKET — systemd user units and timers module section

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [wayfinder:grilling, mise, systemd]
- **Assignee**: agent
- **Related**: [map 0047](0047-mise-bootstrap-adoptions-map.md), [alignment B-systemd-units](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/profile/profile.go`](../../internal/profile/profile.go), [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go)
- **Closing commits**: TBD

## Question

What is the module.toml schema for declaring systemd **user** units (services
and timers), and how does it map to mise's `[bootstrap.linux.systemd.units]`?

Open decisions:

1. **Schema shape** — passthrough table mirroring mise's keys (`exec_start`,
   `after`, `wanted_by`, `on_calendar`, …) vs a curated subset. mise's key
   table is ~30 entries; passthrough is least code and zero vocabulary drift,
   curated is friendlier but forks the spec.
2. **Section name and placement** — `[systemd.units.<name>]` mirroring mise,
   or flatter `[units.<name>]`. System-scope mounts already write *system*
   units via files+services; this is the user-scope counterpart.
3. **Scope rules** — user units are inherently user-scope: is `scope =
   "system"` + units an error, or does scope apply only to dotfiles as today?
4. **Emission target** — which generated config carries
   `[bootstrap.linux.systemd.units]`, and does it ride an existing
   `bootstrap --only` phase or get its own (`linux-systemd-units`)? Note
   mise skips this under `sudo` (wrong user manager) — interplay with the
   0038 "sudo dotdrift apply for root" model needs an answer.
5. **Merge semantics** — whole-entry by unit name like mounts (user > host >
   base), presumably; confirm.

Verify against the installed mise before locking: 2026.9.1 ships the
`linux systemd-units` subcommand per `src/cli/bootstrap.rs`.

## Resolution

All five decisions accepted by the user:

1. **Passthrough schema** — `[systemd.units.<name>]` carries any directive
   key mise supports; dotdrift validates structure only (name charset, kind
   derivation, `exec_start` on services, trigger on timers, service-only
   key rejection — mirroring `src/system/systemd.rs`), mise owns semantics.
   Strict mode (contract #19) gained a scoped passthrough filter for
   BurntSushi's any-map Undecoded quirk.
2. **`[systemd.units.<name>]`** — mirrors the emitted section 1:1.
3. **User-scope only** — `scope = "system"` + units is a resolve-time error.
4. **Own `systemd` step** after dotfiles, `mise bootstrap --only
   linux-systemd-units` from `mise/systemd/mise.toml`; no euid branching —
   mise's own skip-under-sudo semantics handle root applies. New
   `--[no-]systemd` section flag; plan renders `systemd:` (text) and
   `systemd[]` (JSON).
5. **mediamtx migration** — offered to the user as a diff, not applied
   silently.

Status drift-probing of units is deliberately out (own consideration, like
the parked status-engine question).
