---
type: Issue
title: 0152 Invalid package name names its module and config
description: A package entry with spaces fails in mise with no pointer to the module or config that declared it.
tags: [issue, packages, apply]
timestamp: 2026-08-03T00:00:00Z
---

# ISSUE 0152: Invalid package name names its module and config

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [packages, apply]
- **Assignee**: none
- **Related**: none
- **Related code**: [`internal/resolve/`](../../internal/resolve/), [`internal/service/steps.go`](../../internal/service/steps.go)
- **Closing commits**: none

## Summary

On cri-laptop, `dotdrift apply` fails with `mise ERROR paru package
hook did not return state for 'A modern compatibility tools manager'`.
The text comes from mise, not dotdrift. It means the generated
`[bootstrap.packages]` config held a `paru:` entry whose name is that
whole phrase, so the paru plugin hook was asked for a package with
spaces in its name and could not return a state for it. The display
gives no pointer to the module or config file that declared the entry.

## Details

- A package name is one word. A multi-word string in a `present` or
  `absent` list is a config error, most likely a description pasted
  where a name belongs.
- Resolve must reject such an entry before mise runs, and the error
  must name the module, the layer (base, host, or user), and the
  `module.toml` path that holds the entry.
- When the packages step still fails inside mise (the hook can fail
  for other reasons), the step error must list each package in the run
  with the module or modules that declared it, using the attribution
  the plan already carries.

## Acceptance Criteria

- [x] Resolve rejects a `present` or `absent` package name that is empty or contains whitespace, naming the module, layer, and config path.
- [x] A failed packages bootstrap step reports each install package with its declaring module.
- [x] `go test ./...` and `go vet ./...` pass.

## Out of Scope

- Changing the mise or paru plugin hook protocol.
- Guessing the intended package from a description string.
