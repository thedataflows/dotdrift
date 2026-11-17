---
type: Issue
title: "0150: Hook tasks include the module name"
description: Generated hook tasks are named hooks-pre-0, hooks-pre-1, and so on. The name does not say which module declared the hook.
tags: [issue, feature, hooks]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0150: Hook tasks include the module name

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [hooks, mise]
- **Assignee**: none
- **Related**: [contract invariant 12](../product/contract.md)
- **Related code**: [`internal/resolve/`](../../internal/resolve/), [`internal/mise/`](../../internal/mise/)
- **Closing commits**: none

## Summary

Hook tasks must state the module that declared the hook. A reader of the
generated mise config or of a `mise run` command must see the module
without a second lookup.

## Details

Tasks were named `hooks-<phase>-<index>`, for example `hooks-pre-0`.
Resolve aggregates hooks from all selected modules into one list and kept
no module name, so the generated name could not state the module.

The new name is `hooks-<module>-<pre|post>-<NN>`, for example
`hooks-shell-pre-01` and `hooks-shell-pre-02`. NN starts at 01 and
counts per module and phase. The pipeline step names `hooks-pre` and
`hooks-post` do not change. Resolve sets the module directory name on
each hook command. Task generation and task execution use the same name
rule. A hook command built without a module name keeps the short form,
for example `hooks-pre-01`.

## Acceptance Criteria

- [x] Resolve sets the declaring module directory name on each pre and post hook command.
- [x] Generated tasks use `hooks-<module>-<pre|post>-<NN>`, with NN counted per module and phase from 01.
- [x] The apply pipeline runs the same task names that generation writes.
- [x] `go test ./...` and `go vet ./...` pass.

## Out of Scope

- The pipeline step names `hooks-pre` and `hooks-post` do not change.
- The `dotdrift plan` output does not change.
