---
type: Issue
title: "0137: tools as per-module global conf.d snippets"
description: Design change — each module's tools section must land as its own global mise conf.d snippet (~/.config/mise/conf.d/<module>.toml), not in one combined fragment.
tags: [issue, feature, mise, tools]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0137: tools as per-module global conf.d snippets

- **Type**: feature
- **Status**: open
- **Priority**: medium
- **Labels**: [mise, tools]
- **Assignee**: none
- **Related**: [0037](0037-tools-global-activation.md)
- **Related code**: [`internal/mise/`](../../internal/mise/), [`internal/resolve/`](../../internal/resolve/)
- **Closing commits**: none

## Summary

The tools sections of the various modules are not permanent with mise
as one combined fragment. New design: every module's resolved tools
are written as a global conf.d snippet of their own —
`~/.config/mise/conf.d/<module>.toml` — so each module's tools stay
active globally, per module.

## Details

Today the tools step (issue 0037) writes one dotdrift-owned fragment,
`conf.d/dotdrift.toml`, holding the whole merged `[tools]` table, and
removes it wholesale when the plan has no tools. Replace it with one
snippet per module:

- The resolved plan must carry tools per module (layers merged
  base → host → user for that module), not only the global winner map.
- After a successful install, the tools step writes
  `<conf.d>/<module>.toml` for every module that declares tools, each
  with the generated-by marker and that module's `[tools]` table.
- Snippets of modules that no longer declare tools (or are no longer
  selected) are removed; a file is removed only when it carries the
  dotdrift generated-by marker — foreign conf.d files are never
  touched. The legacy combined `dotdrift.toml` fragment is removed on
  the same condition (migration).
- A plan with no tools removes every dotdrift-marked snippet.
- File extension is `.toml`: verified with mise 2026.10.12 that
  `conf.d/*.toml` loads as global config and `*.conf` is ignored.

## Acceptance Criteria

- [ ] `mise.ToolsStep` writes one `conf.d/<module>.toml` per module
  with tools, containing exactly that module's merged tools.
- [ ] Stale marked snippets and the legacy `dotdrift.toml` fragment
  are removed; unmarked files in conf.d are untouched.
- [ ] The install itself still runs against the per-step tools config;
  snippets are written only after a successful install.
- [ ] Product docs describe the per-module snippets.

## Out of Scope

- Changing how tools install (the state-dir per-step config stays).
- Per-layer snippets: one snippet per module, layers merged.

## Notes

- `resolve.Plan.Tools` keeps `Versions`/`ToolModules` for status and
  plan output; the per-module map is additive.
