---
type: Issue
title: "0136: mise subprocess always interactive"
description: Regression — mise children again run non-interactively, so an interactive mise prompt cannot be answered. Every mise subprocess must always be marked interactive.
tags: [issue, bug, mise, apply]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0136: mise subprocess always interactive

- **Type**: bug
- **Status**: open
- **Priority**: high
- **Labels**: [mise, apply]
- **Assignee**: none
- **Related**: [0028](0028-apply-mise-prompts-unanswerable-stdin.md)
- **Related code**: [`internal/mise/`](../../internal/mise/)
- **Closing commits**: none

## Summary

A mise subprocess again runs in non-interactive mode, so an interactive
mise prompt (noticed on `apply`, other areas may be affected) cannot be
answered. Rule: every subprocess mise must always be marked as
interactive.

## Details

Issue 0028 wired stdin into mise children only when dotdrift's own
stdin is a terminal (`opStdin` gates on `executil.IsStdinTerminal`).
Where that gate fails — a piped or redirected stdin in front of a user
who is present — mise sees a non-interactive child, its prompt resolves
as declined (or fails), and the step can still complete. The gate is
the regression surface: interactivity is a property of the mise child,
not of how dotdrift itself was launched.

Fix at the owner (`internal/mise`): every mise subprocess — operations
(`install`, `bootstrap`, `dotfiles apply`), the ensure/version runner,
and probes — always receives the real stdin. The mise installer child
(`defaultInstall`) gets stdin too. Handover children already run on the
consumer's real stdio and are unchanged.

## Acceptance Criteria

- [ ] `opStdin` always returns the real stdin; no terminal gate remains
  on any mise spawn path (`runContextEnv`, `runOp`, `runProbe`,
  `defaultInstall`, `internal/profile` mise probes).
- [ ] Regression test: a mise child receives stdin even when stdin is
  not a terminal, on the captured and streamed paths.
- [ ] `docs/product/cli-surface.md` states the unconditional rule.

## Out of Scope

- Adding `--yes` anywhere: prompts must stay answerable, not
  pre-answered.
- Package-manager children (paru/pacman): not mise subprocesses.

## Notes

- Probes keep captured output; only their stdin changes.
