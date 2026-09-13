---
type: Task
title: T-tui-editing
description: Per-field inline editing in the workspace over the 0065 draft ledger — validation in place, row add/remove with confirms, save/discard, degraded raw-text mode.
tags: [task, tdd, tui, editing]
timestamp: 2026-09-13T00:00:00Z
milestone: m15
---

# Goal

Replace the M14 editor-form pushed views with per-field inline editing in
the workspace, per [M15](/milestones/m15-tui-compositor.md). The 0065
machinery (file-scoped draft ledger, three-tier validation, disk-hash
conflict refusal, atomic writes via tomlsplice) is reused untouched —
only the editing shell changes.

# Tests first

- Golden `View()`: `TestEdit_fieldEditActive` (row in edit mode with the
  cursor in place, rest of the surface live),
  `TestEdit_inlineErrorAndWarning` (tiers 1–2 render at the field),
  `TestEdit_rawTextDegradedMode` (unparseable file edits as raw text with
  the parse error named), `TestEdit_dirtyRowMarker`.
- Message-driven: `TestEdit_enterStartsEdit_escExits`,
  `TestEdit_draftSurvivesNavigationAndLayerSwitch` (0065 ledger
  semantics — leaving and returning restores the draft, nav `●` agrees),
  `TestEdit_saveWritesAndClearsDraft` (`ctrl+s`; tomlsplice splice +
  atomic write through the service),
  `TestEdit_diskConflictRefusalSurfaces` (conflict → modal, draft kept),
  `TestEdit_discardRequiresConfirm` (`D`; confirm names module + layer +
  field count), `TestEdit_rowAddRemove` (`a` adds to the current table,
  `d` removes with confirm), `TestEdit_saveBlockedOnTier1Errors`.

# Implementation notes

- Editing a field never leaves the workspace; the row becomes an input
  with the section's validation applied live. Structural sections keep
  their 0065 custom models, rendered as workspace rows instead of pushed
  views.
- Drafts are file-scoped: switching modules or layers never prompts,
  because nothing is lost — the confirm burden sits on `D`, save, and
  dirty-quit, not on navigation.
- Parse-error files edit as raw text only; structured editing unlocks
  when the file parses again (state visible, never silent).
- The old editor-form views stay reachable behind the task flag until
  T-tui-cleanup; new editing must reach parity per section first.

# Docs

- tui.md editing section rewritten (editor-frame description deleted);
  log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
