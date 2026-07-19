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
- A version with a non-numeric suffix on a segment (e.g. `2025.1.0-rc1`, `2025.1.0-dev.1`, `2025.1.0+build.5`) is a pre-release and compares **below** the plain release. Unparseable versions (`""`, `abc`, …) are explicit errors, never a silent less-than.
- `mise --version` output is scanned for the first token that starts with a digit, so a leading program-name token does not break parsing.

# Algorithm (`EnsureMise`)

1. Look for `mise` on PATH (and well-known user locations: `~/.local/bin/mise`, `~/.local/share/mise/bin/mise`).
2. If found, run `mise --version` and parse the version string.
   - If version ≥ `MinMiseVersion`, return the absolute path.
3. If missing or too old, classify the installation:
   - **System-wide** (under `/usr/bin`, `/usr/local/bin`, `/bin`, `/sbin`, `/usr/sbin`, `/opt`, or `DOTDRIFT_MISE_SYSTEM=1`): do not auto-upgrade; tell the user to upgrade via the package manager.
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

Emitted via zerolog (`log.Info` / `log.Warn`) from `Ensure`:

- Missing (info, before install): “mise not found; installing via https://mise.run …”
- Too old + system (warn): “mise X < required Y; system install at /usr/bin/mise — upgrade with your package manager”
- Too old + user (info, before `self-update`): “mise X < required Y; upgrading user install…”

Errors from a failing `mise` subprocess include the captured (trimmed) combined output. A failed `https://mise.run` install additionally hints to check network connectivity or pre-seed `~/.local/bin/mise`.

The `Ensure` result (success or failure) is memoized per `Mise` instance, so the bootstrap runs at most once per process no matter how many steps invoke it. `EnsureContext` propagates a `context.Context` to every subprocess (`exec.CommandContext`); `Ensure` uses `context.Background()`.

# Trusting generated configs

Generated `mise.toml` files live under the XDG state dir (`$XDG_STATE_HOME/dotdrift/profiles/<hash>/…`), which is outside mise's default trusted paths — unmodified mise refuses to parse them (`Config files in … are not trusted`). Whenever `ExecMise` invokes mise against a generated config (`EnsureAndInstall`, `DotfilesApply`, `RunTask`), the subprocess environment includes `MISE_TRUSTED_CONFIG_PATHS` covering the config's directory, merged with (never clobbering) any pre-existing user value of that variable, colon-separated. The env is derived per call and never mutates shared `Mise` state, so concurrent steps are race-free.

# Testing

See [Ensure mise task](/tasks/t-mise-ensure.md). All branches use fakes for PATH, version output, and installer.
