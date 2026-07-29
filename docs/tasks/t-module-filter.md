---
type: Task
title: T-module-filter
description: Optional positional module filter limiting modules/plan/apply scope to the listed modules.
tags: [task, tdd, cli, profile]
timestamp: 2026-07-29T00:00:00Z
---

# Goal

`dotdrift modules`, `dotdrift plan`, and `dotdrift apply` accept optional
positional module ids (`dotdrift apply vim git` ≡ `dotdrift apply vim,git`)
limiting the command's scope to those modules. No ids → behavior unchanged.
The filter never resurrects modules skipped for their own reason (disabled
via `[modules] disable` or excluded by a `when` filter).

# Tests first

- `TestParseModuleFilter` — comma-separated single arg, space-separated
  multi-arg, mixed forms, duplicates collapsed first-seen, whitespace
  trimmed, empty input → nil.
- `TestProfile_LimitTo_keepsOnlyListed` / `_allListedKeepsSelectionOrder` —
  only the listed modules stay in `Selected`; excluded Selected modules move
  to `Skipped` with reason `module filter`, order preserved.
- `TestProfile_LimitTo_unknownIDErrors` — error names the unknown ids and
  every valid module id.
- `TestProfile_LimitTo_disabledModuleErrors` / `_whenFilteredModuleErrors` —
  naming an already-skipped module errors, naming it and its existing skip
  reason.
- `TestProfile_LimitTo_emptyNoop` — nil/empty filter leaves selection and
  skips untouched.
- `TestCLI_modules_filterKeepsListed` / `_filterExcludesSelected` — filtered
  listing: listed module stays selected, already-disabled module keeps its
  `disabled` reason, selected-but-excluded module shows `module filter`.
- `TestCLI_modules_filterUnknownErrors` / `_filterDisabledErrors` — CLI-level
  loud errors.
- `TestCLI_plan_filterLimitsScope` — `--json` `modules` array holds only the
  filtered ids; the excluded module's dotfile entries are absent.
- `TestApply_moduleFilterLimitsSelection` — filtered apply reaches
  `resolvePlan` with a limited `Selected` (observed via the `resolvePlan`
  seam wrapper; call order load → resolve).
- `TestApply_moduleFilterUnknownErrors` — unknown id: `Run` errors and
  `resolvePlan` is never called.
- `TestKong_applyPositionalModules` — kong populates the positional `Modules`
  slice with the raw argv tokens (`["vim,git", "extra"]`).

# Implementation notes

- `internal/profile/filter.go`: `ParseModuleFilter(args []string) []string`
  (split each arg on commas, trim, drop empties, dedupe first-seen; empty →
  nil) and `(*Profile).LimitTo(ids []string) error` (nil ids → no-op; unknown
  → error naming unknown + valid ids; skipped → error naming id + existing
  skip reason; otherwise move non-listed Selected modules to `Skipped` as
  `Skip{Module: m, Reason: "module filter"}` preserving order).
- Wiring: `ModulesCmd`, `PlanCmd`, `ApplyCmd` gain
  `Modules []string `arg:"" optional:"" name:"modules"`` (kong, not cobra);
  `p.LimitTo(profile.ParseModuleFilter(c.Modules))` runs right after profile
  load — in apply between `profileLoad` and `resolvePlan`.
- `resolve.Fingerprint` is untouched: a filtered apply naturally produces a
  different fingerprint, so the existing selection-change path resets resume
  state with its standard warning (documented caveat; alternating scoped and
  unscoped applies resets the cursor each time).
- The stale "optional `--only` later as power-user only" note in
  cli-surface's apply row is removed — this feature supersedes it.
