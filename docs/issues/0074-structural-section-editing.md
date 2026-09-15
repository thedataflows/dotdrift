---
type: Issue
title: Structural-section inline editing in the M15 workspace
description: systemd units, secrets, mounts, smb, and nested when trees are read-only rows in the compositor workspace — port the editing capability the deleted M14 editor frames carried.
tags: [issue, tui, editing, follow-up]
timestamp: 2026-09-15T00:00:00Z
---

# ISSUE 0074: Structural-section inline editing in the M15 workspace

- **Type**: task
- **Status**: open

## Context

M15 (issue [0073](0073-tui-compositor-redesign.md)) replaced the M14
editor-frame views with per-field inline editing in the workspace
(T-tui-editing). The inline editor covers the scalar and table sections:
meta fields, packages, tools, links, writes (line entries), when leaves,
hooks. The structural families — `systemd.units` (unit tables with
directive passthrough), `secrets`, `mounts`, `smb` shares, and nested
`when.and/or/not` trees — render as read-only rows; their 0065 custom
editors (`internal/tui/editor` adapters) were deleted with the M14 view
layer in T-tui-cleanup.

The save machinery they need is intact: the family encoders
(`profile.EncodeSystemdSection` et al.), the draft ledger, and the 0065
pipeline. What is missing is the per-family row grammar: how a systemd
unit's directives become editable rows (add/remove directive, edit
value), how a mount's fields edit, how nested when trees expand.

## Acceptance Criteria

- [ ] `systemd.units` rows edit directives in place; units add/remove with
      confirm.
- [ ] `secrets`, `mounts`, `smb` rows edit their fields; entries
      add/remove.
- [ ] Nested `when` groups (`and`/`or`/`not`) are editable as an
      expandable tree (0065-D12's builder was fog; this is its landing).
- [ ] Each family round-trips through the encoders with the tomlsplice
      byte-preservation guarantees (contract 19) and the draft ledger.
- [ ] Golden + message-driven tests per family; the "other" read-only
      grouping in the workspace shrinks to nothing.

## Notes

The M14 adapters (`editor/adapters.go`, `dotfiles.go`, `hooks.go`,
`mounts.go`) carried per-family state machines with forms; the M15 shape
should stay row-native (no pushed forms) per the redesign's inline
principle. git history has the deleted adapters for reference.
