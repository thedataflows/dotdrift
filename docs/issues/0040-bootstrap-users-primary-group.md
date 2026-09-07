---
type: Issue
title: Bootstrap users missing required primary group
description: GenerateBootstrapAccounts emits supplementary groups only; mise rejects present users without an explicit primary group, so any smb profile with users fails at apply.
tags: [issue, mise, bootstrap, smb]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0040: Bootstrap users missing required primary group

- **Type**: bug
- **Status**: done
- **Priority**: critical
- **Labels**: [mise, bootstrap, smb]
- **Assignee**: agent
- **Related**: [mise bootstrap alignment A1](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go)
- **Closing commits**: 83fee17

## Summary

`GenerateBootstrapAccounts` emits `[bootstrap.users]` entries with only
supplementary `groups`; upstream mise bails with `present bootstrap user
'<name>' requires a primary group` (`src/system/accounts.rs:333`). Any
profile declaring `[smb] users = [...]` fails at apply.

## Details

Found by the mise bootstrap alignment research (A1). The mise spec
(`docs/bootstrap/accounts.md`): "Present users require an explicit primary
`group`." dotdrift's smb translation creates one group and adds each user to
it — the natural primary group for these accounts is that same smb group.

Fix: emit `group = "<smb group>"` (primary) alongside `groups = [...]`
(supplementary) in `GenerateBootstrapAccounts`.

## Acceptance Criteria

- [x] Emitted `[bootstrap.users]` entries carry both `group` (primary, the smb group) and `groups` (supplementary)
- [x] Existing accounts tests updated; new test locks the primary-group field
- [x] `go test ./...` and `go vet` green

## Out of Scope

- Other account fields mise supports (`system`, `home`, `shell`, `comment`, uid/gid pins, `state = "absent"` removal) — adopt when a profile needs them.
