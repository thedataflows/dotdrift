---
type: Task
title: T-tui-writes
description: Onboard/restore/generate writes areas in the service layer; the TUI's Profile dialogs and generate context-menu actions run through them.
tags: [task, tdd, service, writes, tui]
timestamp: 2026-09-12T00:00:00Z
milestone: m14
---

# Goal

Move the remaining write orchestration out of `cmd/` (0061-D6/D7's writes
slice) and hang the TUI's Profile actions off it: onboard, restore, and
generate become service areas; the shell's dialogs (0062-D10) call them.
CLI output byte-identical per 0061-D7, as always.

# Tests first

- Service: `TestWrites_onboard_*` (adopt/update/orphan-adoption semantics
  unchanged from cmd's orchestration, `--dry-run` as an option field),
  `TestWrites_restore_*` (target resolution, generation pinning, same
  target-two-modules error, symlink removal, sudo install path as a
  `Handover`-style seam), `TestWrites_generate_*` (mounts/smb assembly
  through the shared builders + `WriteModule`; `--list-volumes` as a
  reads call).
- CLI parity: existing onboard/restore/generate cmd tests unchanged; the
  adapters translate flags to options structs only.
- TUI dialogs (state machines + golden `View()`):
  `TestOnboardDialog_confirmsAndDryRun`,
  `TestRestoreDialog_generationPicking`,
  `TestGenerateDialog_layerChoice` (module/layer context menus →
  mounts/smb flows with the editors' prefill seam, contract 15),
  `TestDialogs_writeOnlyThroughService` (no domain-package calls from
  the TUI).

# Implementation notes

- Restores that need elevation go through the same terminal-handover
  discipline as apply (session `Handover` contract, 0064-D9) — the TUI
  never runs sudo children itself.
- Onboard runs "mise apply immediately" as today; in the TUI the follow-up
  apply is announced and offered through the plan gate (the only write
  path into convergence), not silently spawned.
- Generate in the TUI reuses the editor suite's prefill seam (contract
  15): the dialog assembles `generate.Input` through the shared builders
  and writes through the service — no second assembly path.
- `cmd/onboard.go`, `cmd/restore.go`, `cmd/generate.go` thin to adapters;
  the init command's git orchestration stays command-local (0061's noted
  exception) until it earns a service area.

# Docs

- service-api.md area table (writes: designed → shipped); cli-surface
  rows only if flag behavior changed (it must not); log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
