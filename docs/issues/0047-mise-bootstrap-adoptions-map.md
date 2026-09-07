---
type: Issue
title: "MAP: Mise bootstrap feature adoptions"
description: Wayfinder map for the remaining mise bootstrap alignment backlog — every adoptable feature either implemented as a dotdrift issue or consciously ruled out of scope.
tags: [issue, wayfinder:map, mise, bootstrap]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0047: MAP — Mise bootstrap feature adoptions

- **Type**: task
- **Status**: open
- **Priority**: medium
- **Labels**: [wayfinder:map, mise, bootstrap]
- **Assignee**: none
- **Related**: [mise bootstrap alignment](../product/mise-bootstrap-alignment.md), [ADR-0004](../adr/0004-delegate-convergence-to-mise-bootstrap.md)
- **Related code**: [`internal/mise/`](../../internal/mise/), [`internal/resolve/`](../../internal/resolve/)
- **Closing commits**: none

## Destination

Every adoptable feature from the [mise bootstrap gap analysis](../product/mise-bootstrap-alignment.md)
is either implemented as a dotdrift issue (schema decided, TDD'd, documented)
or consciously ruled out of scope with the reason recorded.

## Notes

- Domain: dotdrift translates layered `module.toml` profiles into mise
  `[bootstrap.*]` config; upstream `jdx/mise @ main` `docs/bootstrap/` is the
  normative spec (verified implemented in `src/cli/bootstrap.rs` + `src/system/`).
- Standing preferences (from prior sessions): terse answers, recommended
  options accepted wholesale; fail-loud actionable messages over catch-alls;
  prefer deletion over addition; reject over-engineering early ("stand down"
  signal); TDD + issue + docs + log per change; execution follows decisions
  in the same effort.
- Verify every adoption empirically against the installed mise (2026.9.1)
  before locking schema — docs/spec drift has burned us twice (aur manager
  merged post-release; `secret()` absent from `[dotfiles]` templates).

## Decisions so far

- [0040: Bootstrap users missing required primary group](0040-bootstrap-users-primary-group.md): smb group emitted as both primary and supplementary.
- [0041: Pin working directory for all mise invocations](0041-mise-invocation-cwd-pinning.md): `Mise.WorkDir` pins every real subprocess, default home.
- [0042: System files via bootstrap.files](0042-system-files-bootstrap-files.md): whole-file entries + mount dirs via `--only files`; sudo survives for edit entries only.
- [0043: Adopt built-in aur/pacman managers](0043-builtin-aur-pacman-managers.md): bare Arch names → `pacman:`; aur half split out (below).
- [0044: Adopt mise bootstrap secrets for templates](0044-bootstrap-secrets-templates.md): `[secrets]` schema + `[bootstrap.secrets]` emission, system-files templates only.
- [0045: Delete paru plugin once mise ships built-in aur](0045-delete-paru-plugin-after-aur-release.md): **blocked** — aur manager merged upstream after v2026.9.1; unknown managers are warn-and-ignore (fail-open), so no `aur:` emission until a release contains it.
- [0046: apply --force](0046-apply-force-flag.md): forwards `--force` to every `mise dotfiles apply`; default stays refusal.
- [0048: systemd user units and timers module section](0048-systemd-user-units-section.md): `[systemd.units.<name>]` passthrough, user-scope only, own apply step + section flag; mediamtx migration offered as a diff.

## Not yet specified

- Whether dotdrift's `status` should converge onto `mise bootstrap status
  --json` (per-resource drift with origin provenance) instead of its own
  probes. Large architectural question; parked as a standing consideration,
  not yet a sharp enough question to ticket.
- `brew:`/`brew-cask:` on Linux as a cross-distro package fallback (no
  Homebrew required upstream). No demand from the dogfood profile yet.

## Out of scope

- **Remote SSH orchestration** (`[bootstrap.remote]`, relay) — ruled out by
  the user; dotdrift's model is one profile, local execution, git distribution.
- **macOS sections** (launchd, macos-defaults, mas) — dotdrift is Linux-shaped.
