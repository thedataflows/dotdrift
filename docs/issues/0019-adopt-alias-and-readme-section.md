---
type: Issue
title: "adopt alias for onboard; README onboarding/adopting section"
description: Third command alias adopt, and a README section walking the onboarding/adopting scenarios.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0019: adopt alias for onboard; README onboarding/adopting section

- **Type**: task
- **Status**: done
- **Priority**: low
- **Labels**: [onboard, docs]
- **Assignee**: none
- **Related**: [issue 0015](0015-onboard-adopts-orphans.md), [issue 0017](0017-onboard-adoption-units-and-profile-paths.md)
- **Related code**: [`cmd/root.go`](../../cmd/root.go), [`README.md`](../../README.md)
- **Closing commits**: pending

## Summary

`onboard` gains `adopt` as a second alias (besides `add`), and README
gains an "Onboarding and adopting" section walking the scenarios:
onboarding a live path, directed adoption of a module file, dry-run
preview, and orphan adoption riding any onboard.

## Details

The alias rides kong's `aliases` tag. The README section sits between
"Edit entries" and "Generate"; the Commands table row and
cli-surface.md list both aliases.

## Acceptance Criteria

- [x] `dotdrift adopt ...` parses onto OnboardCmd with the same flags.
- [x] README section covers the four scenarios with runnable examples
      and layer-labeled notices.

## Out of Scope

- None.
