---
type: Issue
title: Refresh the mise alignment map to mise 2026.11.9
description: Update docs/product/mise-bootstrap-alignment.md with the adopted, partial, and rejected mise capabilities as of 2026-11-17.
tags: [docs, mise]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0161: Refresh the mise alignment map

- **Type**: chore
- **Status**: done
- **Priority**: low
- **Labels**: [docs, mise]
- **Assignee**: none
- **Related**: [0158](0158-adopt-mise-dotfiles-depends.md), [0159](0159-declare-services-instead-of-systemctl-hooks.md), [0160](0160-services-entry-scope.md)
- **Related code**: [docs/product/mise-bootstrap-alignment.md](../product/mise-bootstrap-alignment.md)
- **Closing commits**: none

## Summary

Update the mise alignment map to the state after issues 0158 to 0160:
what dotdrift adopts from mise, what it adopts in part, and what it
does not adopt, with reasons a reader can check.

## Details

The map dates from 2026-09-07 and mise 2026.8.2. Since then: dotfiles
depends (0158) replaced the per-level apply passes, generated configs
carry min_version, [services] replaced systemctl hooks (0159, 0160),
and the A3 recommendation (system files through [bootstrap.files])
was reversed by 0156 because [bootstrap.files] has no symlink form.
The September text still recommends A3 without that note, and knows
nothing of the November mise features reviewed on 2026-11-17.

## Acceptance Criteria

- [x] The alignment map lists every mise bootstrap section and the main dotfiles and task additions with an adopted, partial, or not adopted status and a reason
- [x] The stale A3 recommendation carries the 0156 reversal
- [x] Remaining candidates are named with their blockers

## Out of Scope

- Any code change. Adoption stays one issue per feature.
