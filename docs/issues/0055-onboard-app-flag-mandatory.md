---
type: Issue
title: Make onboard --app mandatory
description: dotdrift onboard must name its module explicitly — the --app flag becomes required and the first-path name inference is deleted.
tags: [issue, cli, onboard]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0055: Make onboard --app mandatory

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [cli, onboard]
- **Assignee**: agent
- **Related**: [0017](0017-onboard-adoption-units-and-profile-paths.md) (directed module-file adoption still overrides the value)
- **Related code**: [`cmd/onboard.go`](../../cmd/onboard.go), [`internal/onboard/onboard.go`](../../internal/onboard/onboard.go)
- **Closing commits**: none (recorded at done)

## Summary

`onboard`'s `--app` flag was optional, defaulting to an inference from the
first path (`~/.config/nvim/init.lua` → module `nvim`). User decision: the
module name must always be explicit — inference is a silent guess at profile
structure, and a wrong guess silently creates or updates the wrong module.
`--app` becomes a required kong flag and `inferApp` is deleted.

## Details

- `OnboardCmd.App` gains `required:""`; help text drops the inference note.
  Kong rejects `onboard <path>` without `--app` before `Run` executes.
- `internal/onboard`: the `opts.App == ""` inference branch and `inferApp`
  are deleted; a loud guard (`onboard: --app is required`) stays for direct
  `Options` callers.
- Unchanged: a path **inside** a module layer directory is a directed
  adoption — its own layer names the module and overrides the `--app` value
  (0017). The flag is still required on the command line even then.
- Docs: README onboard examples gain `--app`, cli-surface rows updated,
  t8-onboard note corrected.

## Acceptance Criteria

- [x] `onboard <path>` without `--app` fails at parse time naming the flag
- [x] `inferApp` and the inference branch deleted; internal guard errors on empty App
- [x] Directed module-file adoption still overrides the `--app` value
- [x] Docs updated (README examples, cli-surface, t8-onboard)
- [x] `go test ./...` and `go vet` green

## Out of Scope

- Changing any other onboard flag's optionality.
- Renaming `--app` (it names the module directory; the schema's optional
  `app` field is untouched).
