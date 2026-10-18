---
type: Issue
title: Addable section headers are selectable
description: Navigation skips non-empty addable headers such as packages; every header except meta must be selectable when its section supports adding.
tags: [issue, tui]
timestamp: 2026-10-18T19:20:00Z
---

# ISSUE 0128: Addable section headers are selectable

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux, keys]
- **Assignee**: agent
- **Related**: [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `a289b6e`

## Summary

Workspace navigation skips the `packages` header (and every other
non-empty, non-structural addable header): `wsRows` marks a header
selectable only when the section is empty or its family is
structural. The rule must be the user's: every section header except
meta is selectable as long as the section supports adding items —
which is the existing `addable` predicate, covering empty and
non-empty sections alike. Meta stays non-selectable (it has no add).
