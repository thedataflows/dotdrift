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

**Resolved decisions (ADR-0004):**

1. **AUR / `paru`** — dotdrift ships a mise package plugin (tracked in
   [issue 0003](0003-paru-mise-package-plugin.md)). Bare Arch packages resolve
   to `paru:<pkg>` and install via `paru -S` (pacman + AUR), not the plain
   `pacman:` built-in. **Open risk:** the plugin contract forbids `sudo` in
   any hook, but paru self-elevates — unverified against v2026.8.2; fallback
   is a dotdrift-native paru path for AUR packages.
2. **Resume semantics — kept.** dotdrift's resume layer stays as a thin
   orchestrator over `mise bootstrap --only/--skip`; not dropped.
3. **Verbosity — kept.** `-v` wraps the `mise bootstrap` invocation at the
   dotdrift to mise boundary.
4. **Package prefixes — bare by default.** Bare names assume the detected
   platform manager (Arch → `paru` plugin; Debian → `apt`; Fedora → `dnf`);
   the resolver adds the `manager:` prefix. An explicit prefix passes through
   to the named built-in or plugin unchanged.
5. **Schema lag — accepted** (non-blocking).
6. **Trust + state-dir — unchanged.**

## Acceptance Criteria

- [ ] `MinMiseVersion` raised to `2026.8.2`; `EnsureMise` upgrade path tested.
- [ ] [Issue 0003](0003-paru-mise-package-plugin.md) (paru mise package plugin)
      landed; AUR packages install via `paru:<pkg>` through the plugin.
- [ ] `[packages]` translates to `[bootstrap.packages]`: bare names get the
      detected-manager prefix (Arch → `paru`, Debian → `apt`, Fedora → `dnf`);
      explicit prefixes pass through unchanged. `internal/packages` deleted
      (or the paru slice retained if the plugin sudo-risk materializes).
- [ ] System-scope dotfiles translate to `[bootstrap.files]`; the
      `sudo -E mise dotfiles apply` path and `DotfilesSystemStep` deleted.
- [ ] Mounts activation translates to `[bootstrap.directories]` +
      `[bootstrap.linux.systemd.units]`; `internal/mounts` activation deleted,
      unit rendering kept in `generate`.
- [ ] SMB activation translates to `[bootstrap.users]`/`[groups]`/`[services]`
      (+ a hook for `smbpasswd`/`testparm`); `internal/smb` activation deleted,
      config rendering kept in `generate`.
- [ ] `dotdrift apply` drives `mise bootstrap` (single command or `--only`/
      `--skip` slices) with the resume orchestrator preserved (invariants
      2/10/11).
- [ ] `-v`/`--verbose` wraps the `mise bootstrap` invocation (`+ argv` echo,
      live streaming, probes silent) — kept at the dotdrift-to-mise boundary.
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
2. Implement [issue 0003](0003-paru-mise-package-plugin.md) (paru mise plugin);
   validate the sudo tension against real mise v2026.8.2.
3. `[packages]` → `[bootstrap.packages]` (biggest LOC win, lowest risk).
4. System dotfiles → `[bootstrap.files]`.
5. Mounts/smb activation → bootstrap services/users/groups; shift `generate`
   output from dotfile entries to `[bootstrap.*]` tables.
6. Collapse the 8-step pipeline to `mise bootstrap` slices driven by resume.

The new `[bootstrap.files/directories/secrets/users/groups/services/compose]`
sections ship in the v2026.8.2 release and CLI docs but are **not yet in the
published JSON schema** (`mise.jdx.dev/schema/mise.json` lags); editor
validation will catch up. Non-blocking.

mise's `[bootstrap.packages]` requires explicit `manager:pkg` keys. dotdrift
keeps bare names in `module.toml` and adds the prefix during translation: bare
→ detected platform manager (Arch → `paru` via [issue 0003](0003-paru-mise-package-plugin.md),
Debian → `apt`, Fedora → `dnf`); an explicit prefix (`pacman:foo`) passes
through to the named built-in or plugin unchanged.
