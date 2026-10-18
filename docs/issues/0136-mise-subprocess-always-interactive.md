---
type: Issue
title: "0136: mise subprocess always interactive"
description: Regression — sudo's password prompt inside mise fails (EIO, clear-text echo) on apply. Interactivity is marked in the generated mise config; dotdrift must not defeat it by backgrounding terminal children.
tags: [issue, bug, mise, apply]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0136: mise subprocess always interactive

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [mise, apply]
- **Assignee**: none
- **Related**: [0028](0028-apply-mise-prompts-unanswerable-stdin.md)
- **Related code**: [`internal/mise/`](../../internal/mise/)
- **Closing commits**: `84f8b01`, corrected by `2fd5fac`

## Summary

A mise subprocess again runs in non-interactive mode, so an interactive
mise prompt (noticed on `apply`, other areas may be affected) cannot be
answered. Rule: every subprocess mise must always be marked as
interactive.

## Details

First fix (`84f8b01`) was wrong: it forced stdin into every mise
subprocess. The user reported the actual symptom on `apply`
(mountpoints): sudo is invoked, the typed password appears in clear
text under the prompt, then `sudo: unable to read password:
Input/output error` and `sudo: a password is required`.

Root cause (verified against mise 2026.10.12 and a fake-sudo harness):
interactivity is marked in the mise config dotdrift generates —
hook tasks already carry mise's `interactive = true` (issue 0088) —
but dotdrift spawned every terminal-bound mise child with
`SetGroupKill` (Setpgid), putting it in a background process group.
A background group cannot own the terminal: sudo's /dev/tty password
read fails with EIO and echo is never disabled, so the password
shows in clear. A plain spawn in the caller's (foreground) group
reads the password with echo off, as before. Stdio wiring was never
the problem, so the 0136 stdin forcing is reverted in full (0028's
terminal gate restored; probes and the installer back to no stdin).

Correct fix (`2fd5fac`): terminal-bound children keep the caller's
process group — the session's handover twin gets no Setpgid, and
`runOp` children streamed to real terminals skip `SetGroupKill`.
Piped children (captured runs, the session collector) keep their
own group so cancel still kills the whole tree. Known trade-off: a
programmatic (non-tty) cancel now kills the handover child itself,
not its whole tree; tty Ctrl-C still reaches the whole foreground
group. One cancel test's sleeper now `exec`s so no orphan holds the
test pipe open (the suite had gained a 30 s tail from the leak).

## Acceptance Criteria

- [x] No stdio forcing: 0028's terminal gate on `opStdin` restored;
  probes and the mise installer spawn without stdin again.
- [x] Terminal-bound mise children (handover twin, terminal-streamed
  `runOp`) stay in the caller's process group; piped children keep
  their own group for cancel.
- [x] Regression tests pin the process group on both paths and the
  handover twin's missing Setpgid.
- [x] `docs/product/cli-surface.md` states the rule.

## Out of Scope

- Adding `--yes` anywhere: prompts must stay answerable, not
  pre-answered.
- Package-manager children (paru/pacman): not mise subprocesses.

## Notes

- Probes keep captured output and no stdin, as before 0136.
