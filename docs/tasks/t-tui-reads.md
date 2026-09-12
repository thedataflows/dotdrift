---
type: Task
title: T-tui-reads
description: Migrate the reads areas (modules, plan, status, diff, detect) into internal/service with canonical renderers; cmd adapters stay byte-identical.
tags: [task, tdd, service, reads, tui]
timestamp: 2026-09-12T00:00:00Z
milestone: m14
---

# Goal

Implement the reads half of [M14](/milestones/m14-tui.md): the reads
areas of the service layer (0061-D2/D6) with the canonical renderers, and
`cmd/` thinned onto them — the first migration slice, done to 0061-D7's
gate: every command's output byte-identical, kong flags unchanged, suite
green per slice.

# Tests first

- Service: `TestService_modules_*` / `_plan_*` / `_status_*` / `_diff_*`
  (each read returns the typed model the domain packages already produce,
  translated errors included); `TestRender_planReport_golden` /
  `_diff_golden` / `_statusSummary_golden` (canonical renderers moved
  from cmd, goldens re-pinned byte-for-byte);
  `TestSchemaError_fromStrictLoad` (wrapped-string load errors translated
  to `SchemaError{Path, Line}` at the boundary); `TestReads_lockFree`
  (no state lock taken on any read path).
- CLI parity (the gate): the existing cmd tests for `modules`, `plan`
  (`--json` included), `status`, and `detect` pass **unchanged** — the
  adapters swap internals, not output.

# Implementation notes

- One area struct per concern (0061-D2), composed into `service.Service`;
  consumers (cmd today, the TUI in T-tui-shell) define narrow interfaces
  they fake — no shipped hierarchy.
- Canonical renderers move from `cmd/` into the service layer; `--json`
  stays a dumb marshal in cmd over the typed plan (0061-D5). TUI-styled
  rendering never touches these.
- `SchemaError` enters the taxonomy here (strict-schema loads through the
  reads paths); `cmd` error printing switches to `errors.As` rendering.
- No behavior change is in scope — any output diff is a bug in the slice.

# Docs

- service-api.md area table (reads: designed → shipped); package-layout
  service line; log entry per slice.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
