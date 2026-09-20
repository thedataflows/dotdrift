---
type: Task
title: T-tui-structural-cleanup
description: Close issue 0074 — keymap/help copy for container rows and the new sections, palette field sweep, ponytail audit, verification-before-completion, docs, final gates.
tags: [task, tdd, tui, issue-0074]
timestamp: 2026-09-15T00:00:00Z
issue: "0074"
---

# Goal

Fourth 0074 slice: fold the structural editing into the shell's chrome
and close the issue.

# Tests first

- Keymap table/help: the workspace help names container rows (enter
  edits fields, a adds beneath, d removes the entry).
- Palette: field deep links reach the new sections' rows (the palette
  indexes wsRows; pin at least one secrets/mounts/systemd link).
- No new production behavior without a failing test.

# Implementation notes

- ponytail-audit over the 0074 diff (the structural switch grows per
  family — check it has not become a second schema), then
  verification-before-completion: fresh `go test ./... -count=1`, vet,
  gofmt, golangci-lint.

# Docs

- tui.md (the structural editing section), issue 0074 closed with the
  accepted-grammar notes, tasks index, log entry.

# Acceptance

- [ ] Landed.
- [Definition of done](/engineering/definition-of-done.md) checklist complete.
