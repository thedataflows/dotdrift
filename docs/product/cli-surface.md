---
type: Specification
title: CLI surface
description: Commands and minimal flags for dotdrift.
tags: [product, cli]
timestamp: 2026-07-14T00:00:00Z
---

# Commands

| Command | Behavior |
|---------|----------|
| `dotdrift init [path-or-git-url]` | Local path: create the profile and git-initialize it. Git URL: clone into a dir named from the URL minus any trailing `.git`, relative to the given path; the clone must be a dotdrift profile (`dotdrift.toml` present) or init errors |
| `dotdrift detect` | Print facts (host, user, os, distro, gpu, package backend) |
| `dotdrift modules` | List modules with selected/skipped + reason (no `modules ls` form; bare `dotdrift` prints help and exits with a usage error) |
| `dotdrift plan [--json]` | Print effective plan; side-effect-free, never touches mise. `--json` emits a single JSON object (`fingerprint`, `modules`, `packages.install`/`remove`, `tools`, `dotfiles[]` with target/source/mode/module/layer/scope, `hooks.pre`/`post`, `mounts[]` with module/name/source/destination/type/options/startat/state/layer/scope, `smb[]` with module plus the merged spec — group/users/avahi (`null` when unset)/shares) instead of the text rendering; the no-modules warning is suppressed so stdout stays parseable. System-scope dotfiles are marked `[system]` in the text rendering. The text rendering ends with conditional sections when the profile declares them: `mounts:` lists each entry as `<module>: <name> <type> <source> -> <destination> [layer][scope]` plus `startat=`/`state=` markers when set; `smb:` lists per module the effective group (unset → `smb`), users, avahi (unset → `true`), and each share as `name -> path`. Both sections are omitted entirely when their aggregates are empty |
| `dotdrift apply [--yes] [--no-hooks]` | Full pipeline; always resumes; optional `--only` later as power-user only. Step order: `hooks-pre` → `packages` → `tools` → `dotfiles` → `dotfiles-system` → `mounts` → `smb` → `hooks-post`. `--no-hooks` (or `DOTDRIFT_NO_HOOKS=1`) skips the `hooks-pre`/`hooks-post` steps. When the plan contains `scope = "system"` modules, a `dotfiles-system` step applies their dotfiles via `sudo` (skipped when already root); sudo's timestamp cache means at most one password prompt per apply. `mounts` (mkdir destinations, `systemctl daemon-reload`, enable/disable units + startat timers) and `smb` (group/user membership, avahi, testparm gate, service enablement, samba accounts) are conditional activation steps constructed only when the plan declares mounts/smb; they never write config files — placement is mise's job |
| `dotdrift status` | Resume cursor, selection, last error. Progress prints `completed/8 steps` against the static pipeline superset; the conditional steps (`hooks-pre`/`hooks-post`, `dotfiles-system`, `mounts`, `smb`) legitimately leave a completed apply below 8/8 — status never resolves the plan, it counts completed steps against the static list |
| `dotdrift onboard <path>...` | Create/update module; mise apply immediately |

# Onboard flags (minimal)

| Flag | Default | When to use |
|------|---------|-------------|
| (none) | mode=link, module inferred, enabled by presence | Happy path |
| `--app ID` | inferred | Inference wrong |
| `--mode link\|copy\|template` | link | Apps that rewrite configs → copy |
| `--packages P` | none | Declare distro package in module.toml (comma-separated or repeated for several) |
| `--tools T` | none | Declare mise tool (comma-separated or repeated for several) |
| `--host` | base module files | Host overlay only |
| `--dry-run` | false | Preview only |
| `--force` | false | Re-onboard a conflicting path: replace the module's copy with the live file/dir instead of erroring |

Forbidden: `--enable`, `--resume`.

# Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime failure needing user action (e.g. mise conflict, package error) |
| 2 | Usage / invalid args |