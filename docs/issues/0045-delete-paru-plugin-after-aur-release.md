---
type: Issue
title: Switch aur/ markers to built-in aur once mise ships it
description: The aur manager (mise PR #12718) merged after v2026.9.1; when a release contains it, aur/ markers map to aur:. The embedded paru plugin is NO LONGER deleted — bare Arch names route through paru: permanently (0054).
tags: [issue, mise, packages]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0045: Switch aur/ markers to built-in aur once mise ships it

- **Type**: task
- **Status**: blocked
- **Priority**: medium
- **Labels**: [mise, packages]
- **Assignee**: none
- **Related**: [0043](0043-builtin-aur-pacman-managers.md), [0054](0054-bare-arch-packages-via-paru.md) (keeps the plugin alive), [0003](0003-paru-mise-package-plugin.md), [mise bootstrap alignment A4](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go)
- **Closing commits**: none

## Summary

**Scope amended by [0054](0054-bare-arch-packages-via-paru.md)**: this issue
originally also deleted the embedded paru plugin, its registry maintenance,
and the `dotdrift paru` commands. 0054 routed bare Arch names back to `paru:`,
so the plugin stack is permanent and deletion is off the table. What remains
here is only the marker switch.

The built-in `aur` manager (yay preferred, paru fallback) merged upstream as
PR #12718 **after** the v2026.9.1 tag — no released mise contains it, and mise
ignores unknown managers with a warning + exit 0 (fail-open), so emitting
`aur:` today would silently skip every `aur/` package in a real profile (the
dogfood profile declares ~a dozen). When a mise release ≥ the one containing
#12718 is out, switch `aur/` → `aur:` and bump `MinMiseVersion`.

## Details

Verify the gate with the installed mise before starting:

```sh
mise bootstrap packages status   # with [bootstrap.packages] "aur:paru" = "latest"
# ready when it no longer warns "unknown bootstrap package manager 'aur'"
```

Then:

1. `PrefixedPackages`: `aur/<pkg>` → `aur:<pkg>` (drop that one mapping; bare
   names stay `paru:` per 0054); update `MinMiseVersion` to the release
   containing the aur manager.
2. Keep everything else: `internal/paru/`, `cmd/paru.go`, and the plugin
   registry maintenance in `packagesStep.Run` (bare Arch names need them —
   0054), plus `internal/packages` Paru backend (removal + `when.packages`
   probes).
3. Docs: profile-layout `packages.present` bullet, 0003 notes the marker
   supersession.

Behavioral deltas to document: built-in aur prefers **yay** over paru when
both exist; aur pins are status-only (dotdrift pins `"latest"` — no change).

## Acceptance Criteria

- [ ] Installed/required mise recognizes the `aur` manager (no unknown-manager warning)
- [ ] `aur/<pkg>` emits `aur:<pkg>`; bare Arch names still emit `paru:`
- [ ] `MinMiseVersion` bumped to the aur-containing release
- [ ] Plugin stack (`internal/paru`, `cmd/paru.go`, registry maintenance) untouched
- [ ] `go test ./...` and `go vet` green

## Out of Scope

- Deleting the paru plugin stack (permanent per 0054).
- Package version pins (alignment A5).
- `state = "absent"` pacman entries (own-backend removal stays).

## Notes

Blocked on: an upstream mise release containing PR #12718
(`feat(bootstrap): add AUR package manager`), which merged 2026-09-05+ after
the v2026.9.1 tag. Check `mise version` ≥ the first release after that merge.
