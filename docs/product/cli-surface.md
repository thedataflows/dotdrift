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
| `dotdrift plan [--json]` | Print effective plan; side-effect-free, never touches mise. `--json` emits a single JSON object (`fingerprint`, `modules`, `packages.install`/`remove`, `tools`, `dotfiles[]` with target/source/mode/module/layer) instead of the text rendering; the no-modules warning is suppressed so stdout stays parseable |
| `dotdrift apply [--yes]` | Full pipeline; always resumes; optional `--only` later as power-user only |
| `dotdrift status` | Resume cursor, selection, last error |
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

Forbidden: `--enable`, `--resume`.

# Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime failure needing user action (e.g. mise conflict, package error) |
| 2 | Usage / invalid args |