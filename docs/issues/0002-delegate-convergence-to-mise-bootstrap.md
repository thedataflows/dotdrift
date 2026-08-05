---
type: Issue
title: Delegate convergence mechanics to mise bootstrap
description: Translate module.toml to [bootstrap.*] and drive mise bootstrap, deleting hand-rolled package/mounts/smb convergence.
tags: [issue, feature, mise, architecture]
timestamp: 2026-08-05T00:00:00Z
---

# ISSUE 0002: Delegate convergence mechanics to mise bootstrap

- **Type**: feature
- **Status**: open
- **Priority**: high
- **Labels**: [mise, architecture, migration]
- **Assignee**: none
- **Related**: [ADR-0004](../adr/0004-delegate-convergence-to-mise-bootstrap.md), [contract invariants 2/3/10/11](../product/contract.md), [mise bootstrap](../product/mise-bootstrap.md)
- **Related code**: [`internal/packages/`](../../internal/packages/), [`internal/mounts/`](../../internal/mounts/), [`internal/smb/`](../../internal/smb/), [`internal/mise/`](../../internal/mise/), [`internal/apply/`](../../internal/apply/)
- **Closing commits**: none

## Summary

mise v2026.8.2 turned `mise bootstrap` into a declarative host-provisioning
system that subsumes the convergence mechanics of dotdrift's `packages`,
`dotfiles-system`, `mounts`, and `smb` steps and adds capabilities v0.1 scoped
out (secrets, users/groups, compose, firewall, SSH-remote, macOS). dotdrift
should become a translator + orchestrator over `mise bootstrap`, keeping only
its irreducible authoring model (layering, resolve, generate, onboard, resume).

## Details

ADR-0004 records the decision and rationale. This issue tracks the work.

**The overlap (verified against the release notes, CLI docs, and JSON
schema):**

| dotdrift step | Current impl | Delegation target |
|---|---|---|
| `hooks-pre` | generated `[tasks]` → `mise run` | `[bootstrap.hooks.pre-packages]` |
| `packages` | `internal/packages`: paru/apt/dnf | `[bootstrap.packages]` `manager:pkg` |
| `tools` | `mise install` | bootstrap `tools` phase (already delegated) |
| `dotfiles` (user) | `mise dotfiles apply` | bootstrap `dotfiles` phase (already delegated) |
| `dotfiles-system` | `sudo -E mise dotfiles apply` | `[bootstrap.files]` (atomic, hidden sudo) |
| `mounts` | `internal/mounts`: systemctl | `[bootstrap.directories]` + `[bootstrap.linux.systemd.units]` |
| `smb` | `internal/smb`: group/user/svc | `[bootstrap.users]`/`[groups]`/`[services]` + hook |
| `hooks-post` | generated `[tasks]` → `mise run` | `[bootstrap.hooks.post-dotfiles]`/`final` |

**Deletable (~1100 prod LOC + ~2600 test LOC):**

- `internal/packages` (541) — entire backend set → `[bootstrap.packages]`.
- `internal/mounts` (174) — activation only; unit *rendering* stays in `generate`.
- `internal/smb` (268) — activation only; config *rendering* stays in `generate`.
- `internal/mise` system-dotfiles sudo path (~150) → `[bootstrap.files]`.

**Irreducible (~6300 prod LOC, 75%):** `internal/resolve`, `internal/profile`,
`internal/generate`, `internal/tui`, `internal/onboard`, `internal/state`,
`internal/detect`/`facts`, and the ensure + translator half of `internal/mise`.

**Two blockers before `internal/packages` can be deleted:**

1. **AUR / `paru`.** dotdrift's Arch backend is `paru -S` (pacman + AUR);
   mise's `pacman:` built-in-manager is plain pacman. The migration doc
   references the AUR package `ntfsprogs-plus`. Resolve how AUR packages
   install under mise before deleting the backend.
2. **Resume semantics.** mise bootstrap is convergent but has no cross-run
   cursor tied to selection fingerprint + plan content hash (contract
   invariants 2, 10). Keep dotdrift's resume layer as a thin orchestrator
   driving `mise bootstrap --only/--skip <phases>`; do not drop it.

## Acceptance Criteria

- [ ] `MinMiseVersion` raised to `2026.8.2`; `EnsureMise` upgrade path tested.
- [ ] AUR/paru decision documented (ADR amendment or inline): keep a paru
      slice, require a mise plugin, or accept AUR modules stay dotdrift-native.
- [ ] `[packages]` translates to `[bootstrap.packages]` (`manager:pkg` keys
      resolved from detected backend); `internal/packages` deleted (or the paru
      slice retained per the AUR decision).
- [ ] System-scope dotfiles translate to `[bootstrap.files]`; the
      `sudo -E mise dotfiles apply` path and `DotfilesSystemStep` deleted.
- [ ] Mounts activation translates to `[bootstrap.directories]` +
      `[bootstrap.linux.systemd.units]`; `internal/mounts` activation deleted,
      unit rendering kept in `generate`.
- [ ] SMB activation translates to `[bootstrap.users]`/`[groups]`/`[services]`
      (+ a hook for `smbpasswd`/`testparm`); `internal/smb` activation deleted,
      config rendering kept in `generate`.
- [ ] `dotdrift apply` drives `mise bootstrap` (single command or `--only`/
      `--skip` slices) with the resume orchestrator preserved.
- [ ] Verbose UX decision documented: keep dotdrift's `+ argv` echo wrapper or
      accept mise's `MISE_LOG_LEVEL`/`MISE_DEBUG`.
- [ ] `go test ./...` green; `./tests/e2e/run.sh` green across the debian
      family and CachyOS; existing invariants (cross-module conflict detection,
      layer merge, resume fingerprint + plan hash) still hold.

## Out of Scope

- Adopting the *new* mise capabilities (secrets, compose, firewall, remote,
  macOS) as new `module.toml` sections — that is additive product growth,
  tracked separately after the delegation lands.
- Changing dotdrift's user-facing `module.toml` schema or CLI surface — the
  translation is internal; users keep authoring layered modules.
- Dropping the resume state machine — it is retained as the orchestrator over
  `mise bootstrap` phase slices.

## Notes

Phased rollout (no big-bang), lowest-risk-first:

1. Bump `MinMiseVersion` → `2026.8.2`.
2. Resolve AUR/paru.
3. `[packages]` → `[bootstrap.packages]` (biggest LOC win, lowest risk).
4. System dotfiles → `[bootstrap.files]`.
5. Mounts/smb activation → bootstrap services/users/groups; shift `generate`
   output from dotfile entries to `[bootstrap.*]` tables.
6. Collapse the 8-step pipeline to `mise bootstrap` slices driven by resume.

The new `[bootstrap.files/directories/secrets/users/groups/services/compose]`
sections ship in the v2026.8.2 release and CLI docs but are **not yet in the
published JSON schema** (`mise.jdx.dev/schema/mise.json` lags); editor
validation will catch up. Non-blocking.

mise's `[bootstrap.packages]` requires explicit `manager:pkg` keys; dotdrift
uses bare names + detected backend, so the manager prefix moves into the
resolver as part of the translation.
