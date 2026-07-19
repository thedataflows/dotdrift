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
3. **Mise owns files.** Symlink, copy, template, and partial edits are performed only via mise. Dotdrift generates config and invokes mise. Mode vocabulary is translated at this boundary: dotdrift's `link` becomes mise's `symlink` in the generated config; `copy`, `template`, and `symlink-each` pass through unchanged.
4. **State is resume-only.** Persist completed steps, current step, status, error, and selection fingerprint. No content hashes of configs.
5. **Onboard materializes then applies.** Copy live paths into the module, write `module.toml`, then immediately run mise dotfiles apply (default mode: link).
6. **Mise is ensured before use.** Any command that needs mise runs [mise bootstrap](/product/mise-bootstrap.md) first.
7. **Exceptions are explicit.** `modules.disable`, `packages.absent`, `--mode copy|template`, host/user overlays. Defaults do the common case.
8. **Dotfile sources stay contained.** `dotfiles.<target>.source` is a relative path inside the module directory. A source that resolves outside the layer root (e.g. `../../outside`) is a resolve-time error naming the module and the source. A declared source file that does not exist in any layer is also a resolve-time error (chosen over a warning: fail fast with a clear message instead of dying later inside mise).
9. **Cross-module package conflicts are errors.** If a package is `present` in at least one selected module and `absent` in at least one other selected module, resolve fails with a conflict error naming the package and the modules on both sides. Within a single module, the layer rules still apply: a higher-layer `absent` cancels a lower-layer `present` without error.
10. **Fingerprint scope is broader than selection.** The stored "selection fingerprint" intentionally covers the selected module IDs, the `modules.disable` union, and the facts `hostname`, `username`, `os`, `gpu`, and `backend`. Any change to any of these — not just selection — produces a different fingerprint and resets resume state.
11. **Apply serializes on a sidecar lock.** `dotdrift apply` acquires `flock(LOCK_EX)` on `<state-path>.lock` before loading state and holds it until the pipeline's final save. The lock never targets the state file itself: `Save` replaces it via atomic tmp+rename, which would orphan an inode-held lock. `Load`/`Save` do no internal locking; callers needing a load→modify→save critical section must hold the sidecar lock. Lock-free readers (e.g. `status`) observe either the complete old or complete new state, and `Save` fsyncs the tmp file before rename for durability.
12. **Hooks are user-declared commands executed as mise tasks.** Modules may declare `[hooks]` `pre`/`post` shell commands in `module.toml`. Layers append (module → host → user) and modules aggregate in selection order — hooks are ordered sequences, not sets. In the apply pipeline `hooks-pre` runs before packages (a pre-hook failure aborts before any side effect) and `hooks-post` runs after dotfiles (a post-hook failure means everything else completed). Hook failure fails its step and resume re-runs it, so **post-hooks MUST be idempotent**. Hooks run from the profile root with `DOTDRIFT_PROFILE`/`DOTDRIFT_HOSTNAME`/`DOTDRIFT_USERNAME`/`DOTDRIFT_OS`/`DOTDRIFT_BACKEND` in the environment, are visible in `dotdrift plan`, and can be suppressed with `dotdrift apply --no-hooks` or `DOTDRIFT_NO_HOOKS=1`.

# Out of scope (v0.1)

- Custom file engine parallel to mise
- Multi-host remote agent
- Hardened apt/dnf production backends (v0.1 ships minimal functional install/remove/is-installed backends, auto-resolved from os-release)
- Interactive TUI