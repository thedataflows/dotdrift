---
type: Issue
title: Migrate cmd/apply onto the apply session
description: Rewire ApplyCmd.Run onto internal/service's session — flags to ApplyOpts, event rendering — and delete the duplicated orchestration.
tags: [implementation, apply, cli, migration]
timestamp: 2026-09-12T00:00:00Z
---

# ISSUE 0070: Migrate cmd/apply onto the apply session

- **Type**: task
- **Status**: open
- **Priority**: high
- **Labels**: [implementation]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [0069 apply-session service core](0069-implement-apply-session-service-core.md), [0061-D7 migration order](0061-service-layer-architecture-cli-migration.md)
- **Related code**: [`cmd/apply.go`](../../cmd/apply.go), `internal/service/`
- **Blocked by**: [0069 Implement apply-session service core](0069-implement-apply-session-service-core.md)
- **Closing commits**: none

## Question

Rewire `ApplyCmd.Run` as the 0061-D7 adapter: translate flags →
`ApplyOpts` (`Sections` from the section flags via `resolveSections`,
`Output` = the process streams for passthrough, `HandoverAvailable` =
its real stdin), drain the session's events, and render — `printPlan`
on `PlanResolved` (byte-identical), backup lines on `BackupTaken`,
`--diff` as a reads call before `Start` — then **delete** the duplicated
orchestration this migration orphans: the step types, `buildSteps`,
`backupCopyTargets`, `writeBootstrapConfig`, config-path wiring copied
into `internal/service` by 0069, **and the section-flag twins in
`cmd/sections.go`** (`sectionSet`, `newSectionSet`, `resolveSections`,
`isSectionName` — the flag layer reduces to kong parsing plus
`service.ResolveSections`). Nothing forks: after this ticket the
session is the only apply orchestration in the tree. The existing cmd
apply tests are the byte-identical gate; their seam setup moves from
package-level vars to `ApplyDeps` injection.

## Acceptance Criteria

- [ ] `cmd/apply.go` contains only flag translation, session drain, and
      event rendering; the duplicated step code is deleted.
- [ ] All existing cmd apply tests pass unchanged in their assertions
      (golden output parity), with seams injected via `ApplyDeps`.
- [ ] `--diff`, `--backup`, section flags, `DOTDRIFT_NO_HOOKS`,
      `--verbose` behave exactly as today.
