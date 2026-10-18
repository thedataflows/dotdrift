---
type: Issue
title: Resume cursor ignores changed dotfile sources
description: A leftover resume cursor skips the dotfiles steps even when a copy-mode source file changed after the interrupted apply, so the old copy stays in place.
tags: [issue, apply, state, dotfiles]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0106: Resume cursor ignores changed dotfile sources

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [apply, state, dogfooded]
- **Assignee**: agent
- **Related**: [contract invariant 2](../product/contract.md), [contract invariant 4](../product/contract.md), issue [0004](0004-resume-cursor-and-status-drift.md)
- **Related code**: [`internal/service/session.go`](../../internal/service/session.go), [`internal/state/state.go`](../../internal/state/state.go), [`internal/resolve/resolve.go`](../../internal/resolve/resolve.go)
- **Closing commits**: none

## Summary

`dotdrift apply` must not skip a step when the input of that step changed
after an interrupted apply. Today the state file stores only the last
completed step name, so a changed copy-mode source is not placed on resume.

## Details

Field report, confirmed by reproduction: an apply fails or is cancelled
after the `dotfiles` step (user scope) or the `dotfiles-system` step
(system scope) completes. The user then edits a copy-mode source file in
the profile and runs `apply` again. The run skips through the stored
cursor, so mise never copies the new content and the old copy stays at
the target. Symlink targets do not show the fault: their target is a link
to the source, so the edit is visible with no step run.

Reproduction with a test profile (copy entry + symlink entry, real mise):
run 1 places `copy-v1`/`link-v1`; run 2 with no leftover state places
`copy-v2`/`link-v2`; run 3 with state `{"lastCompleted":"dotfiles"}` and
both sources edited places nothing for the copy (`copy-v2` stays) while
the symlink target shows `link-v3`.

This fault class was fixed twice before with a plan hash (see log entries
"resume ignores module content edits" and "plan hash, source file
content"); issue 0004 removed the hash for a cursor-only state and the
fault came back. Contract invariant 4 ("No status/error/selection/
plan-hash fields") must change with this fix.

Fix shape: store one input hash next to the cursor. The hash covers the
selection fingerprint, the resolved plan, and the bytes of every dotfile
source file. On start, a stored cursor is used only when the stored hash
matches the current hash; a mismatch, or a state file with no hash (all
state files written before this issue), starts from the first step. A
full re-run is safe: every step converges. `status` shows the cursor as
pending only when the same test passes, so the two commands agree.

## Acceptance Criteria

- [x] Edit a copy-mode source after an interrupted apply: the next apply runs the dotfiles step and places the new content
- [x] No change after an interrupted apply: the next apply still resumes after the stored cursor
- [x] A state file with no input hash, or a hash that does not match, starts from the first step
- [x] `dotdrift status` and `dotdrift apply` agree on whether a stored cursor will be used
- [x] Contract invariants 2 and 4 and `docs/log.md` state the new rule
- [x] `go test ./...` and `go vet` green

## Out of Scope

- Per-step input hashes (one hash for the whole run is enough; a mismatch re-runs all steps)
- A `--force-resume` or `--no-resume` flag

## Notes

State files are short-lived (deleted on completion), so a state file with
no hash costs at most one full re-run after upgrade.
