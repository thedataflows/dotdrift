---
type: Issue
title: Delete the `app` module.toml field; rename `--app` to `--module`
description: The `app` schema field is a display-only alias of `id` with no resolution, plan, or apply consumer — delete it. The `--app` flag (and every `App` name for "the module directory name") renames to `--module`/`Module` so the word "app" stops meaning two things.
tags: [chore, schema, cli, tui]
timestamp: 2026-09-21T20:13:51Z
---

# ISSUE 0090: Delete the `app` module.toml field; rename `--app` to `--module`

- **Type**: chore
- **Status**: open
- **Priority**: medium
- **Labels**: [schema, cli, tui, deletion]
- **Assignee**: none
- **Related**: [0055](0055-onboard-app-flag-mandatory.md) (the flag this renames), [0065](0065-full-schema-editor-suite-design.md) (the meta editor losing a row), [0079](0079-tui-undo-marks-onboard-cycle.md) (the prefill keeping its seam)
- **Related code**: [`internal/profile/`](../../internal/profile/), [`internal/onboard/onboard.go`](../../internal/onboard/onboard.go), [`cmd/onboard.go`](../../cmd/onboard.go), [`internal/tui/`](../../internal/tui/), [`internal/service/config.go`](../../internal/service/config.go)
- **Closing commits**: none yet

## Summary

`app` defaults to `id`, and its only consumers are cosmetic: the
`(app: <app>)` tag in `modules`, a meta row in the TUI workspace, and the
encoder round-trip. No resolution, plan, generate, or mise code reads it.
Delete the field. Separately, the word "app" also names the module
directory in `--app`, `onboard.Options.App`, `service.OnboardOpts.App`,
and two dialog field labels — rename that concept to `module` so the
name is consistent and says what it is.

## Details

Two distinct things share the name today:

1. **The schema field** `module.toml` `app` (`ModuleConfig.App`,
   `Module.App`): display alias of `id`, set by no writer except hand
   edits and the meta editor. Recorded as redundant in the log when
   onboard stopped writing it and `modules` dropped its default-equal
   column.
2. **The module directory name** spelled `app`: the mandatory `--app`
   flag (issue 0055), `onboard.Options.App`,
   `service.OnboardOpts.App`, the onboard and create dialogs' first
   field label, and `app` parameter names through
   `service/config.go`.

Strict schema rules do the migration: after the field leaves
`ModuleConfig`, any `module.toml` still carrying `app = ...` is a load
error (`unknown key "app"`), which is the documented strict behavior for
every undocumented key. Hand-authored modules must drop the line.

## Acceptance Criteria

- [ ] `ModuleConfig.App` and `Module.App` are gone; the encoder never
      writes `app`; a `module.toml` carrying `app = ...` fails to load
      with `unknown key "app"`
- [ ] `modules` no longer prints `(app: ...)`; the `appname` fixture is
      deleted
- [ ] The TUI workspace has no `app` meta row; the meta editor has no
      `app` case; the onboard and create dialogs' first field is labeled
      `module`
- [ ] `dotdrift onboard --module <name>` (aliases `add`, `adopt` keep
      working); help and the required-error text say `--module`
- [ ] `onboard.Options.App` → `Module`, `service.OnboardOpts.App` →
      `Module`, `service/config.go` parameter names → `name`
- [ ] Fixtures no longer declare `app`
- [ ] Docs updated: profile-layout (schema + example), cli-surface
      (modules tag, onboard row, flag table), merge-rules, README
- [ ] `go test ./...`, `go vet`, golangci-lint green

## Out of Scope

- Deleting `id` — it stays the module's canonical name and the only
  identity field.
- Any alias or fallback for old `app` lines (strict schema rejects them
  loudly; fix the file).

## Notes

The generate writer only round-trips the field (decode → re-encode), so
`internal/generate` needs no production change — only its seeded-fixture
test. `patchModule` in `service/config.go` collapses to `id` (with the
directory-name fallback) once the alias is gone.
