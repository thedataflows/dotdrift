---
type: Specification
title: Product contract
description: Non-negotiable invariants for dotdrift behavior.
tags: [product, invariants]
timestamp: 2026-07-14T00:00:00Z
---

# Invariants

1. **Presence = managed.** Every directory under `modules/` with a valid `module.toml` is selected unless listed in `disable` or its `when` filter fails. There is no enable list and no `--enable`.
2. **Apply always resumes.** `dotdrift apply` continues from the first incomplete step. No `--resume` flag.
3. **Mise owns files.** Symlink, copy, template, and partial edits are performed only via mise. Dotdrift generates config and invokes mise.
4. **State is resume-only.** Persist completed steps, current step, status, error, and selection fingerprint. No content hashes of configs.
5. **Onboard materializes then applies.** Copy live paths into the module, write `module.toml`, then immediately run mise dotfiles apply (default mode: link).
6. **Mise is ensured before use.** Any command that needs mise runs [mise bootstrap](/product/mise-bootstrap.md) first.
7. **Exceptions are explicit.** `modules.disable`, `packages.absent`, `--mode copy|template`, host/user overlays. Defaults do the common case.

# Out of scope (v0.1)

- Custom file engine parallel to mise
- Multi-host remote agent
- Full apt/dnf production backends (interface only if time)
- Interactive TUI