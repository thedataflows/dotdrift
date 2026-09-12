---
type: Task
title: T-tui-editors
description: The full-schema editor suite — tomlsplice, per-section encoders, the service config area, the editor frame with adapters and custom models, module management.
tags: [task, tdd, tui, editors, toml]
timestamp: 2026-09-12T00:00:00Z
milestone: m14
---

# Goal

Implement the editor suite (issue 0065's four layers) for
[M14](/milestones/m14-tui.md): `internal/tomlsplice`, per-section
encoders in `internal/profile`, the service config area, and the
`internal/tui/editor` frame with its adapters and custom models — plus
the module-management dialogs. Every `module.toml` section becomes
first-class editable in the TUI.

# Tests first

- `internal/tomlsplice` property tests (0065-D11):
  `TestSplice_untouchedSectionsByteIdentical` (comments, position,
  formatting), `TestSplice_deterministic`, `TestSplice_multilineStringImmunity`
  (header grammar tracks multi-line strings), `TestSplice_outputStrictDecodes`.
- Encoders: golden per section; `TestEncode_canonicalFixedPoint`
  (encode∘decode∘encode = encode); `TestEncode_keyedTablesSorted`
  / `_orderedArraysPreserveOrder`.
- Service config area: `TestReadModuleLayer_strictDecodeAndRawText` /
  `_brokenFileCarriesSchemaError`; `TestWriteModuleLayer_savePipeline`
  (encode → splice → strict-decode → resolve checks → disk-hash → atomic
  rename), `TestWriteModuleLayer_diskHashConflictRefuses`,
  `TestWriteModuleLayer_brokenFileReadOnly`,
  `TestModuleOps_createScaffold` / `_moveRefusedOnCollision` /
  `_deletePreviewsOrphans` (orphan set from `ReferencedPaths`).
- Frame + adapters (pure state machines, golden `View()`):
  `TestFrame_draftIsFileScoped` (two dirty sections of one file → one
  splice), `TestFrame_dirtyDeepCompare` / `_dirtyConfirmGuardsEscQ`,
  `TestFrame_validationTiers` (live field, debounced cross-checks,
  authoritative save), per-adapter field/validator vectors **reusing
  profile's own checks**; custom models: `TestDotfilesEditor_kindBadges`
  / `_variantSwitch` / `_contract18Exclusivity`,
  `TestWhenEditor_grammarValidationPositions`,
  `TestHooksEditor_orderedRows` / `_spellingPerRow`,
  `TestSystemdEditor_directivePassthrough` / `_kindBadge`.
- Contract 15 at the seam: `TestGenerate_cliTuiEquivalence_*` retarget —
  for any flag set `ParseShareFlags` accepts, the mounts/smb adapters'
  prefilled input equals `generate`'s assembled input.

# Implementation notes

- Layer order per 0065-D7: tomlsplice (text mechanics only) → encoders
  (typed values → canonical TOML text, CLI-reachable) → config area
  (strict decode + raw text; the save pipeline owns the disk-hash check)
  → `internal/tui/editor` (frame: draft ledger, dirty tracking,
  validation tiers, save gating, dirty-confirm chrome, help bindings).
- Onboard refactors onto tomlsplice with behavior unchanged (its tests
  pass untouched) — the splicer leaves onboard, the behavior does not.
- Editors open only from the raw overlay-stack node; the editor chrome
  names the exact file (0065-D2). `smb.users` annotated "becomes OS
  accounts at apply"; `bootstrap.users` is not editable schema (0065-D3).
- Edited sections re-encode canonically — interior comments of a saved
  section go; untouched sections never lose a byte. A broken file opens
  read-only with its `SchemaError`.
- The mounts/smb adapters prefill from `internal/generate`'s machines
  (`KindForDefaults`/`PrefillForVolume`/`ShareLoopDone`, `MountsInput`/
  `SmbInput`) — the write-once accumulate loop and wholesale
  `[mounts]`/`[smb]` replace semantics carry over from the machines.

# Docs

- tui.md editors section (drift fixes only); profile-layout if an encoder
  canonicalizes differently than documented; log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
