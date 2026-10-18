---
type: Issue
title: "0138: apply must never delete files in mise conf.d"
description: Bug — the per-module conf.d stale-snippet sweep from 0137 treats a module-filtered plan as the whole profile and deletes every other module's snippets.
tags: [issue, bug, mise, tools]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0138: apply must never delete files in mise conf.d

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [mise, tools]
- **Assignee**: none
- **Related**: [0137](0137-tools-per-module-confd.md)
- **Related code**: [`internal/mise/step.go`](../../internal/mise/step.go), [`internal/service/steps.go`](../../internal/service/steps.go)
- **Closing commits**: `6ad636c`

## Summary

A module-filtered apply (`dotdrift apply dsh`, or a single-module
apply from the TUI) empties `~/.config/mise/conf.d/` of dotdrift's
per-module tool snippets. The stale-snippet sweep added in 0137 uses
the current plan as the keep set; a filtered plan does not cover the
whole profile, so every unselected module looks stale.

## Details

Verified on the user's machine (2026-10-18): after a filtered apply,
`conf.d/` held no snippets. The generated tools config for that run
contained only the filtered module's tools, which proves the plan was
partial when the sweep ran. Two shapes:

- Filtered to modules with tools: `writeSnippets` keeps only the
  selected modules' snippets and deletes all other marked snippets.
- Filtered to a module with no tools: the empty-plan branch sweeps
  with an empty keep set and deletes every marked snippet.

Mise is not involved: mise never deletes `conf.d/*.toml`. The only
deleter is `removeStaleSnippets` in `internal/mise/step.go`. Foreign
files without the generated-by marker are not touched; only
dotdrift's own snippets are lost.

Fix (user decision, 2026-10-18): no cleanup sweep of conf.d at all.
The directory may hold user files, and any deletion logic there —
even marker-gated — is a risk with no need behind it: dotdrift
writes and regenerates its own per-module snippets and deletes
nothing. A module removed from the profile leaves its snippet
behind; the user deletes it by hand if they want the tools gone.

## Acceptance Criteria

- [x] The tools step never deletes any file in conf.d, on any apply
  (whole-profile, module-filtered, or tool-less plan).
- [x] Per-module snippet writing is unchanged: one marked
  `<module>.toml` per module with tools, after a successful install.
- [x] Regression tests cover the filtered shapes and the tool-less
  plan.

## Out of Scope

- Section-filtered applies: when the tools section is deselected the
  tools step is not built, so nothing is swept — already safe.
- Changing which snippets a whole-profile apply keeps.
