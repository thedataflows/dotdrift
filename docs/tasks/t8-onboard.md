---
type: Task
title: T8 Onboard
description: Module factory; copy; EnsureMise; mise apply; no enable.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m8
---

# Goal

Implement `dotdrift onboard` per [CLI surface](/product/cli-surface.md).

# Tests first

- `TestPathMap_homeAndSystem` — map absolute paths to module-relative source paths and target paths.
- `TestInferApp_configDir` — infer app name from `~/.config/<app>` paths.
- `TestOnboard_copiesTree` — live paths are copied into the module directory.
- `TestOnboard_writesToml` — `module.toml` is written with discovered metadata.
- `TestOnboard_noEnableFlagNeeded` — created module is selected by presence on next `apply`.
- `TestOnboard_orderCopyThenEnsureThenMiseApply` — records the mise call sequence and asserts copy → ensure mise → dotfiles apply order.
- `TestOnboard_defaultModeLink` — default mode is `link`.
- `TestOnboard_conflictKeepsModule` — if target exists, keep module files and fail the command.
- `TestOnboard_forceReplacesExistingFile` / `TestOnboard_forceReplacesExistingDir` — with `--force`, a conflicting destination is removed and re-copied from the live path (file refresh; dir add/modify/delete refresh).
- `TestOnboard_dryRun_noSideEffects` — `--dry-run` writes nothing and invokes nothing.
- `TestOnboard_miseConfigInStateDir_absoluteSources` — the generated mise config lives under the XDG state dir (`$XDG_STATE_HOME/dotdrift/profiles/<hash>/onboard/mise.toml`), its dotfile sources are absolute and exist, and the module dir contains no runtime files.
- `TestOnboard_preservesDirTreeModes` — materializing a directory tree preserves per-file and per-directory modes (ownership is not preserved).
- `TestOnboard_yesPropagatesToDotfilesApply` — `--yes` reaches `mise dotfiles apply`.

# Implementation notes

- `internal/onboard` implements the onboard logic.
- Copies live paths into `modules/<app>/` under the profile, writes a `module.toml`, then runs `EnsureMise` and `mise dotfiles apply`.
- The generated mise config is written to the profile's state directory (`onboard/mise.toml` next to apply's `mise/mise.toml`), never inside the profile; dotfile sources in it are absolute so mise's `--cd` resolution finds the materialized files.
- `--yes` is plumbed from the CLI through `onboard.Options` to `DotfilesApply` for non-interactive runs.
- `--force` turns a destination conflict into a refresh: the existing module destination is removed (`os.RemoveAll`, covering both file and directory destinations) and re-copied from the live path. Without `--force`, a conflict errors and leaves the module untouched.
- File modes are preserved when copying files and directory trees; ownership is not (copies belong to the current user).
- Default mode is `link`; `--mode copy|template` overrides.
- `--app` overrides inferred app name.
- `--package` and `--tool` add entries to `module.toml`.
- `--host` writes files only to the host overlay directory.
- No `--enable` flag; presence selects the module automatically.

# Acceptance

- Defaults only for happy path; presence enables module.
- Copy → ensure mise → mise apply order is enforced by an order-recording test.
- Onboard leaves no runtime files in the profile; the generated mise config and resume state live under the XDG state directory.
- Dry run has no side effects.
