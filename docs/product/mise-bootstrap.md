---
type: Specification
title: Mise bootstrap
description: Automatic mise detection, minimum version, and upgrade policy.
tags: [product, mise, bootstrap]
timestamp: 2026-07-14T00:00:00Z
---

# Goal

Before any operation that invokes mise (`apply` tools/dotfiles steps, `onboard` post-apply, optional `plan` dry-run), dotdrift **ensures** a usable mise binary.

# Hardcoded minimum version

- `const MinMiseVersion = "2025.1.0"` in `internal/mise`.
- Mise uses calendar versions like `2026.6.6`. Compare by splitting on `.` and comparing each numeric component left-to-right.

# Algorithm (`EnsureMise`)

1. Look for `mise` on PATH (and well-known user locations: `~/.local/bin/mise`, `~/.local/share/mise/bin/mise`).
2. If found, run `mise --version` and parse the version string.
   - If version ≥ `MinMiseVersion`, return the absolute path.
3. If missing or too old, classify the installation:
   - **System-wide** (under `/usr/bin`, `/usr/local/bin`, or `DOTDRIFT_MISE_SYSTEM=1`): do not auto-upgrade; tell the user to upgrade via the package manager.
   - **User-managed** (under `$HOME`, writable, installed via `https://mise.run`): run the official installer or `mise self-update`.
4. After install/upgrade, verify the binary reports version ≥ `MinMiseVersion`.
5. Return the path or an error.

# Official install source

- Primary: `https://mise.run` pipe to `sh` as documented by mise.
- Network required only for install/upgrade; unit tests mock the installer.

# When EnsureMise runs

| Entry | Ensure? |
|-------|---------|
| `apply` | Yes |
| `onboard` | Yes |
| `plan` if it shells to mise | Yes |
| `modules`, `detect`, `status` | No |

# User-visible messages

- Missing: “mise not found; installing via https://mise.run …”
- Too old + system: “mise X < required Y; system install at /usr/bin/mise — upgrade with your package manager”
- Too old + user: “mise X < required Y; upgrading user install…”

# Testing

See [Ensure mise task](/tasks/t-mise-ensure.md). All branches use fakes for PATH, version output, and installer.
