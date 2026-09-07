---
type: Issue
title: Adopt mise bootstrap secrets for templates
description: Modules can declare env-provided secret inputs and reference them from file templates via mise's secret() function, closing the passwords-in-git hole.
tags: [issue, mise, bootstrap, secrets, templates]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0044: Adopt mise bootstrap secrets for templates

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [mise, secrets, templates]
- **Assignee**: agent
- **Related**: [mise bootstrap alignment B-secrets](../product/mise-bootstrap-alignment.md), [0042](0042-system-files-bootstrap-files.md)
- **Related code**: [`internal/profile/profile.go`](../../internal/profile/profile.go), [`internal/resolve/`](../../internal/resolve/), [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go)
- **Closing commits**: TBD

## Summary

Adopt mise's `[bootstrap.secrets]`: a module declares the sensitive inputs
its templates need (logical name → environment variable), and file templates
reference them with `{{ secret(name="...") }}`. Values come from the
environment (fnox, CI, systemd, shell) — never from git. mise resolves and
redacts; dotdrift only declares and emits.

## Details

Design (minimal, aligned with mise's spec):

- `module.toml` gains `[secrets]`: `name = "ENV_VAR"` (short form) or
  `name = { env = "ENV_VAR", description = "...", allow_empty = true }`.
- Layers merge secrets like other scalar maps (base → host → user, nearer
  wins per name).
- The resolved secrets are emitted as `[bootstrap.secrets]` into every
  generated config that contains template files (system bootstrap.files
  after 0042; user `[dotfiles]` templates if mise resolves secrets there —
  verify against upstream behavior during implementation and document the
  boundary).
- Validation: a template referencing `secret(name=...)` without a matching
  declaration is mise's own fail-fast domain; dotdrift stays dumb — declare,
  emit, let mise resolve and redact.

## Acceptance Criteria

- [x] `[secrets]` parses in both short and table form, strict-mode clean (unknown keys rejected)
- [x] Layered merge: nearer layer replaces same-name entries
- [x] `[bootstrap.secrets]` emitted alongside template-carrying configs; absent when no secrets declared
- [x] Docs: profile-layout secrets section, contract touchpoint if user-facing behavior changes
- [x] `go test ./...` and `go vet` green

## Resolution notes

- Empirically verified against mise 2026.9.1: `secret()` renders in
  `[bootstrap.files]` templates; a missing env var fails loud naming the
  secret, the variable, and remediation; `[dotfiles]` templates have no
  `secret` function — emission therefore targets the system files config
  only, and the boundary is documented in profile-layout.
- `dotdrift plan` does not list secrets (names are visible in module.toml
  and the generated config; values never appear anywhere).

## Out of Scope

- `--prompt-secrets` plumbing (mise's flag; dotdrift apply can document passing values via the environment).
- `dotdrift status` secret availability reporting (`mise bootstrap secrets status` exists upstream).
- Remote/SSH secret transport (out of scope per project decision).
