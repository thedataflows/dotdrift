---
type: Task
title: T-tui-structural-entries
description: secrets, mounts, and smb stop being the read-only "other" group — each becomes a first-class section with container rows, always-rendered field rows, entry add/remove, and required-field cross-checks at save.
tags: [task, tdd, tui, editing, issue-0074]
timestamp: 2026-09-15T00:00:00Z
issue: "0074"
---

# Goal

Second 0074 slice: the "other" grouping dissolves into three sections —
secrets, mounts, smb — with the container+field row grammar. Entries add
and remove; every field edits in place; save refuses entries that could
never resolve (missing env/path/source-destination-type).

# Tests first

- Rows: each section renders one container per entry plus fixed field
  rows (secrets: env/description/allow_empty; mounts:
  source/destination/type/options/startat/state; smb: group/users/avahi
  at top, path/comment/valid_users/writable/public per share).
- Edits: env non-empty tier-1; bool fields accept true/false/"" (avahi ""
  unsets to nil); options/users comma lists; mount state restricted to
  ""/enabled/disabled.
- Adds: secrets `name = ENV`; mounts and shares bare name (required
  fields enforced by crossCheck at save, not at add).
- Removes: container d + confirm; crossCheck pins for the required
  fields; goldens per section.

# Implementation notes

- Field rows are `pathKey(entry, field)`; scalar/list/bool mutation
  helpers per family in structural.go; `encodeFamily` grows the three
  cases.
- crossCheck (tier-2, save gate) grows: mount source/destination/type
  required, secret env required, share path required — mirroring
  resolve's structural contract, before the pipeline runs.

# Docs

- tui.md editing bullet, log entry, issue 0074 checkbox.

# Acceptance

- [x] Landed — commit `feat(tui): 0074 secrets/mounts/smb editing`.
- [Definition of done](/engineering/definition-of-done.md) checklist complete.
