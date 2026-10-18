---
type: Issue
title: "0139: status lists unmanaged files in mise conf.d"
description: Status must list files in ~/.config/mise/conf.d/ that the plan does not manage — user files and stale dotdrift snippets — for information only, never as drift.
tags: [issue, feature, status, mise, tools]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0139: status lists unmanaged files in mise conf.d

- **Type**: feature
- **Status**: open
- **Priority**: medium
- **Labels**: [status, mise, tools]
- **Assignee**: none
- **Related**: [0137](0137-tools-per-module-confd.md), [0138](0138-confd-sweep-partial-apply.md)
- **Related code**: [`internal/drift/`](../../internal/drift/), [`internal/service/reads.go`](../../internal/service/reads.go), [`internal/tui/statusview.go`](../../internal/tui/statusview.go)
- **Closing commits**: none

## Summary

Since 0138, apply never deletes files in `~/.config/mise/conf.d/`.
The user needs to see what lives there that dotdrift does not
manage: `dotdrift status` must list unmanaged conf.d files, for
information only.

## Details

A conf.d file is managed when it is the snippet the current plan
would write: `<module>.toml` for a module whose resolved tools are
non-empty. Every other regular file in the directory is unmanaged:

- No generated-by marker: a user file — "not written by dotdrift".
- Marker present but no matching module in the plan: a stale
  dotdrift snippet (its module left the profile, or a module-filtered
  status run) — the note says the file can be deleted by hand to
  stop its tools from resolving.

Findings carry a new `Unmanaged` status in the `tools` section:
they render in both the CLI report and the TUI Status view, and
they never count as drift, unknown, or unauthorized — the summary,
the section counts, and the exit behavior are unchanged. A missing
conf.d directory lists nothing. Subdirectories are skipped.

## Acceptance Criteria

- [ ] Status lists each unmanaged conf.d file with its reason
  (user file vs stale dotdrift snippet).
- [ ] Managed snippets (one per module with tools in the plan) are
  not listed.
- [ ] Unmanaged findings never change drift counts, the "no drift"
  summary, or status exit behavior.
- [ ] Both the CLI report and the TUI Status view show the lines.

## Out of Scope

- Deleting or editing the listed files from status.
- Listing managed snippets as checked items.
