---
type: Issue
title: Delete paru plugin once mise ships built-in aur
description: The aur manager (mise PR #12718) merged after v2026.9.1; when a release contains it, aur/ markers map to aur: and the embedded paru plugin, its registry maintenance, and the dotdrift paru commands are deleted.
tags: [issue, mise, packages, deletion]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0045: Delete paru plugin once mise ships built-in aur

- **Type**: task
- **Status**: blocked
- **Priority**: medium
- **Labels**: [mise, packages, deletion]
- **Assignee**: none
- **Related**: [0043](0043-builtin-aur-pacman-managers.md), [0003](0003-paru-mise-package-plugin.md), [mise bootstrap alignment A4](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/paru/`](../../internal/paru/), [`cmd/paru.go`](../../cmd/paru.go), [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go)
- **Closing commits**: none

## Summary

0043 adopted mise's built-in `pacman` manager for bare Arch package names.
The built-in `aur` manager (yay preferred, paru fallback) merged upstream as
PR #12718 **after** the v2026.9.1 tag — no released mise contains it, and mise
ignores unknown managers with a warning + exit 0 (fail-open), so emitting
`aur:` today would silently skip every `aur/` package in a real profile (the
dogfood profile declares ~a dozen). When a mise release ≥ the one containing
#12718 is out, switch `aur/` → `aur:` and delete the plugin stack.

## Details

Verify the gate with the installed mise before starting:

```sh
mise bootstrap packages status   # with [bootstrap.packages] "aur:paru" = "latest"
# ready when it no longer warns "unknown bootstrap package manager 'aur'"
```

Then:

1. `PrefixedPackages`: `aur/<pkg>` → `aur:<pkg>` (drop the paru mapping);
   update `MinMiseVersion` to the release containing the aur manager.
2. Delete: `internal/paru/` (embedded plugin, `WritePlugin`/`EnsureInstalled`,
   `PluginVersion`), `cmd/paru.go` (the plugin's hooks shell out to
   `dotdrift paru installed|install` — nothing else consumes them),
   `ParuCmd` registration in `cmd/root.go`, the `misePluginsDir` maintenance
   block in `packagesStep.Run`, `MisePluginsDir`/`PluginsDirFromEnv` if
   otherwise unused, and the plugin tests.
3. Keep: `internal/packages` Paru backend (removal + `when.packages` probes).
4. Docs: profile-layout `packages.present` bullet, cli-surface paru row
   (delete), 0003 notes the supersession.

Behavioral deltas to document: built-in aur prefers **yay** over paru when
both exist; aur pins are status-only (dotdrift pins `"latest"` — no change).

## Acceptance Criteria

- [ ] Installed/required mise recognizes the `aur` manager (no unknown-manager warning)
- [ ] `aur/<pkg>` emits `aur:<pkg>`; no `paru:` keys remain in generated configs
- [ ] `internal/paru`, `cmd/paru.go`, plugin maintenance, and cli-surface row deleted
- [ ] `MinMiseVersion` bumped to the aur-containing release
- [ ] `go test ./...` and `go vet` green

## Out of Scope

- Package version pins (alignment A5).
- `state = "absent"` pacman entries (own-backend removal stays).

## Notes

Blocked on: an upstream mise release containing PR #12718
(`feat(bootstrap): add AUR package manager`), which merged 2026-09-05+ after
the v2026.9.1 tag. Check `mise version` ≥ the first release after that merge.
