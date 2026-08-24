---
type: Issue
title: "apply: mise confirmation prompts can never be answered (stdin is the null device)"
description: "runOp streams mise output to the terminal but never wires stdin, so an interactive `files: apply <target>?` prompt resolves EOF as No and silently skips changes while the pipeline records success."
tags: [issue, product]
timestamp: 2026-08-24T00:00:00Z
---

# ISSUE 0028: apply: mise confirmation prompts can never be answered (stdin is the null device)

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [apply, mise, dotfiles, interactive]
- **Assignee**: none
- **Related code**: [`internal/mise/mise.go`](../../internal/mise/mise.go), [`internal/mise/stdin_internal_test.go`](../../internal/mise/stdin_internal_test.go)
- **Closing commits**: pending

## Summary

Field report: `status` correctly reported
`environment.d: ~/.config/environment.d/mozilla.conf - missing`, but
`apply` showed `files: apply ~/.config/environment.d/? [Y/n]` and
immediately `mise files: skipped`, then finished with `resume: clean` —
the drift could never be converged.

Root cause: every mise child (install, dotfiles apply, bootstrap, tasks)
is spawned through `runOp`/`runContextEnv` with `exec.CommandContext` and
no `Stdin`, so the child reads `/dev/null`. An interactive run streams
mise's stdout/stderr to the terminal, so the user SEES the prompt, but
the prompt's selector reads EOF from the null device and resolves as
"No"; mise exits 0 on skip, so the pipeline marks the step complete and
deletes the resume cursor.

## Details

- Fix: an `opStdin` seam wires `os.Stdin` into every mise child when
  stdin is a terminal (the same `executil.IsStdinTerminal` gate hook
  interactivity uses); non-terminal stdin keeps the null device so
  nothing blocks on input nobody will provide. Both spawn paths wired:
  `runContextEnv` (captured) and `runOp` (streamed).
- Reproduction (real mise 2026.8.11, sandbox HOME replicating the live
  11-link `environment.d` state): before the fix the prompt resolved to
  "No"/skipped; after, the pty-driven answer "Yes" creates the missing
  link (`mise files: created 11 symlink(s)`, 12 links after).
- Deliberately NOT fixed with `--yes`: answering every prompt yes would
  also approve clobbering files mise asks about; `--force`/`--yes`
  remain explicit user choices. With stdin wired, "No" is the user's
  conscious decision, not an artifact.
- The skip-then-succeed pipeline behavior is mise's semantics (exit 0
  on skip); dotdrift does not parse mise's streamed output.

## Verification

- `internal/mise`: `TestRunOp_childReceivesStdin` (captured + streamed
  spawn paths: a real child reads data from the wired stdin),
  `TestOpStdin_terminalGate` (terminal → os.Stdin, else nil).
- End-to-end against the reported scenario: `dotdrift apply --dotfiles
  --no-hooks environment.d` under a pty now applies the missing link.
