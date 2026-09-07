---
type: Issue
title: E2E suite predates the bootstrap-driven pipeline
description: tests/e2e has never run against the mise-bootstrap-driven apply pipeline (0042/0043 emission shapes); it may fail today and is the only end-to-end proof against real mise.
tags: [issue, testing, e2e]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0052: E2E suite predates the bootstrap-driven pipeline

- **Type**: chore
- **Status**: open
- **Priority**: high
- **Labels**: [testing, e2e, debt]
- **Assignee**: none
- **Related**: [0002](0002-delegate-convergence-to-mise-bootstrap.md), [0042](0042-system-files-bootstrap-files.md), [0043](0043-builtin-aur-pacman-managers.md)
- **Related code**: [`tests/e2e/`](../../tests/e2e/)
- **Closing commits**: none

## Summary

The Docker e2e suite (debian + ubuntu) was written for the pre-ADR-0004
pipeline: it asserts the old emission and invocation shapes. Since then the
packages step routes through `mise bootstrap --only packages` with
manager-prefixed keys, system files converge via `[bootstrap.files]`, and
mount dirs are `[bootstrap.directories]` — none of it verified end-to-end
against real mise. This is the only integration coverage that catches
fake-blind bugs (trust, mode vocabulary, config clobbering were all found
this way).

## Details

- The scenario pins or installs a specific mise; it must be a current
  release (≥ the one dotdrift's `MinMiseVersion` requires) for the
  bootstrap phases to exist.
- Assertions to revisit: per-step config paths (`mise/system`,
  `mise/system-edits`), bootstrap argv shapes, package manager prefixes,
  and any system-file convergence expectations (the e2e images run as
  root, so the privileged-batch path is exercised differently than on a
  desktop).
- 0002's closure recorded this as test-infrastructure debt, not a
  convergence gap.

## Acceptance Criteria

- [ ] `tests/e2e/run.sh` passes on debian and ubuntu against the current pipeline
- [ ] The scenario exercises at least one bootstrap phase explicitly (e.g. a system-scope file through `[bootstrap.files]`)
- [ ] The mise version in the images is pinned to a release dotdrift supports

## Out of Scope

- New feature coverage (secrets, systemd units) — add scenarios as those land.
- CI matrix expansion beyond the existing debian/ubuntu pair.
