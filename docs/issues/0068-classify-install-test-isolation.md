---
type: Issue
title: TestClassifyInstall writes into the real user home
description: The test creates a fake mise binary in the real ~/.local/bin instead of an isolated temp HOME; it fails outright when HOME is read-only.
tags: [bug, testing]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0068: TestClassifyInstall writes into the real user home

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [testing]
- **Assignee**: cri (main session)
- **Related**: sibling test `TestClassifyInstall_systemEnvVariants` (correct pattern)
- **Related code**: [`internal/mise/mise_test.go`](../../internal/mise/mise_test.go)
- **Closing commits**: pending

## Summary

`TestClassifyInstall` resolves the *real* user home via
`os.UserHomeDir()` and writes a fake `mise` binary into
`~/.local/bin/mise` (cleaning up afterwards). The suite must never touch
anything outside its own temp dirs; in any environment where `$HOME` is
read-only (sandboxes, some CI) the test fails outright.

## Details

`internal/mise/mise_test.go` contains the correct isolation pattern one
function below: `TestClassifyInstall_systemEnvVariants` uses
`t.TempDir()` + `t.Setenv("HOME", home)`. `TestClassifyInstall` predates
it and still writes into the live home directory — it "works" on a
developer machine by littering `~/.local/bin` with a fake binary for the
duration of the run. Classification itself only inspects path strings,
so an isolated home changes nothing about what is exercised.

## Acceptance Criteria

- [x] `TestClassifyInstall` uses `t.TempDir()` + `t.Setenv("HOME", …)`
      like its sibling; no filesystem writes outside the test's temp dir.
- [x] `go test ./...` passes in a sandbox where `$HOME` is read-only.

## Out of Scope

- Any change to `ClassifyInstall` or mise bootstrap behavior — this is
  test hygiene only.

## Notes

The stray `mise … upgrading user install` zerolog lines seen in sandbox
runs come from the shell's mise hook, not from this suite; the tests
assert those messages against an injected buffer.
