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
| `dotdrift init [git-url]` | Create or clone profile; set local path |
| `dotdrift detect` | Print facts (host, user, gpu, os, package backend) |
| `dotdrift modules` | List modules (default action; no `ls` subcommand) with selected/skipped + reason |
| `dotdrift plan` | Print effective plan; no side effects (still may ensure mise if plan includes mise dry-run) |
| `dotdrift apply [--yes]` | Full pipeline; always resumes; optional `--only` later as power-user only |
| `dotdrift status` | Resume cursor, selection, last error |
| `dotdrift onboard <path>...` | Create/update module; mise apply immediately |

# Onboard flags (minimal)

| Flag | Default | When to use |
|------|---------|-------------|
| (none) | mode=link, module inferred, enabled by presence | Happy path |
| `--app ID` | inferred | Inference wrong |
| `--mode link\|copy\|template` | link | Apps that rewrite configs → copy |
| `--package P` | none | Declare distro package in module.toml |
| `--tool T` | none | Declare mise tool |
| `--host` | base module files | Host overlay only |
| `--dry-run` | false | Preview only |

Forbidden: `--enable`, `--resume`.

# Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime failure needing user action (e.g. mise conflict, package error) |
| 2 | Usage / invalid args |