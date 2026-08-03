---
type: Issue
title: Cross-module dotfile target conflict emits unparseable mise.toml
description: Two modules claiming the same dotfile target produce a mise.toml with duplicate keys that mise refuses to parse.
tags: [issue, bug, resolve]
timestamp: 2026-08-03T00:00:00Z
---

# ISSUE 0001: Cross-module dotfile target conflict emits unparseable mise.toml

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [resolve, dogfooded]
- **Assignee**: sisyphus
- **Related**: [contract invariant 9](../product/contract.md) (the package-conflict analogue), [contract invariant 16](../product/contract.md) (added by this issue)
- **Related code**: [`internal/resolve/resolve.go`](../../internal/resolve/resolve.go), [`internal/mise/mise.go`](../../internal/mise/mise.go)
- **Closing commits**: 13c51b1, e280f0d

## Summary

`dotdrift apply` produced a `mise.toml` that mise rejected as corrupted (`TOML parse error … duplicate key`) whenever two selected modules claimed the same dotfile target path. `Resolve` had no cross-module dotfile-target uniqueness check — only packages had one.

## Details

Dogfooded against a real profile. The `mediamtx` and `systemd` modules both targeted `~/.config/systemd`:

```toml
# modules/mediamtx/module.toml (excerpt)
[dotfiles]
"~/.config/systemd" = { source = "home/.config/systemd", mode = "copy" }

# modules/systemd/module.toml (excerpt)
[dotfiles]
"~/.config/systemd" = { source = "home/.config/systemd", mode = "symlink-each" }
```

`dotdrift apply` generated `<state>/profiles/<hash>/mise/mise.toml` with both entries:

```
"~/.config/systemd" = { source = ".../mediamtx/.../systemd", mode = "copy" }
…
"~/.config/systemd" = { source = ".../systemd/.../systemd", mode = "symlink-each" }
```

`mise` then failed:

```
TOML parse error at line 139, column 1
    |
139 | "~/.config/systemd" = { source = "...", mode = "symlink-each" }
    | ^^^^^^^^^^^^^^^^^^^
duplicate key
```

### Root cause

`mergeDotfiles` in `internal/resolve/resolve.go` deduplicated dotfile targets only *within* a single module (across its base/host/user overlays). Across modules, entries were appended to `plan.Dotfiles.Entries` with no uniqueness check, and `mise.GenerateDotfiles` emitted one line per entry — producing duplicate `[dotfiles]` keys that mise refused to parse.

This is structurally identical to the cross-module package-conflict case, which was already covered by `checkPackageConflicts` (contract invariant 9). Dotfiles had no analogous check.

### Fix

Add `checkDotfileConflicts` in `internal/resolve/resolve.go`, symmetric with `checkPackageConflicts`. It runs after the per-module merge over the flattened entries and fails `Resolve` naming the target and every claiming module — fail-fast at plan time instead of an opaque mise parse error at apply time. Within a single module the existing layer rules (user > host > base) still apply; no error.

```
resolve plan: dotfile target conflict across modules: "~/.config/systemd" claimed by modules [mediamtx, systemd]
```

## Acceptance Criteria

- [x] Failing test `TestResolve_crossModuleDotfileTargetConflict` written first (TDD), asserting the error names the target and every claiming module.
- [x] `checkDotfileConflicts` implemented in `internal/resolve/resolve.go`, wired into `Resolve` after the per-module loop, before `sortEntries`.
- [x] New product contract invariant #16 covering cross-module dotfile target conflicts.
- [x] `docs/product/merge-rules.md` resolution table updated with the cross-module rule.
- [x] `docs/product/profile-layout.md` notes the cross-module rule next to the existing in-module override rule.
- [x] `go test ./...` green; `go vet ./...` clean.
- [x] End-to-end check against the reproducer profile (`/home/cri/dotfiles`) emits the new error instead of writing a broken `mise.toml`.
- [x] `docs/log.md` `Fixed (resolve, dogfooded)` entry is present (already written).
- [x] Closing commit landed and is recorded in **Closing commits** above.

## Out of Scope

- Cross-module mounts-target conflicts (`plan.Mounts.Entries`) — different namespace, not encountered in the wild. YAGNI.
- An auto-resolution policy (last-module-wins, scope-aware precedence, etc.) — explicit error is the contract; users fix their profile. The library is not in the business of picking winners between equally-valid declarations.
- Updating the reproducer profile (`/home/cri/dotfiles`) — the user owns that repo; dotdrift's job is to surface the conflict loudly, which it now does.

## Notes

The fix landed as `13c51b1` (code + test + log entry), with the codifying
product-spec updates in `e280f0d`. Created retrospectively as `done`
because the bug was discovered and fixed in a single dogfooding session
before the local issues convention existed.

The reproducer profile (`mediamtx` copying into `~/.config/systemd` vs
`systemd` `symlink-each`-ing the whole dir) is a genuine semantic clash
for the user to resolve separately; likely fixes are scoping the mediamtx
unit to a more specific path (e.g. `~/.config/systemd/user/mediamtx.service`)
or narrowing the `systemd` module's broad `symlink-each`.
