---
type: Task
title: T-tui-modals
description: The modal family — one confirm component for every destructive action, the reason-listed elevation modal, writes/module dialogs absorbed, apply detail as a modal.
tags: [task, tdd, tui, modals]
timestamp: 2026-09-13T00:00:00Z
milestone: m15
---

# Goal

Build the M15 modal family on the compositor, per
[M15](/milestones/m15-tui-compositor.md): one confirm component for all
destructive actions, the elevation modal that fixes the footer-sudo
interruption, the M14 writes/module-management dialogs absorbed into the
modal layer, and the apply output ring as an inspect modal.

# Tests first

- Golden `View()`: `TestConfirm_variants` — discard draft, remove link,
  remove write, remove module (with the orphan preview), destructive
  apply, dirty quit — each naming the full target identity (module +
  layer) and consequence; `TestElevation_states` (initial with reason
  list, typing masked dots, `✗ authentication failed`, credentials
  expired re-prompt); `TestApplyDetail_streaming` / `_failed` /
  `_complete`; writes and module-management dialog goldens re-shot as
  modals.
- Message-driven: `TestConfirm_yConfirmsEverythingElseCancels` (`n`,
  `esc`, click-outside all cancel; `enter` alone never confirms),
  `TestElevation_onePromptCoversAllPrivilegedOps` (plan's privileged set
  collected once — no per-action whack-a-mole),
  `TestElevation_cancelAbortsBeforeTouching` (apply cancelled, nothing
  executed, footer reports it),
  `TestElevation_threeFailuresAbort`, `TestElevation_passwordNeverLogged
  NorDrafted` (zeroed `[]byte`; zerolog fields excluded),
  `TestApplyDetail_closeNeverCancels_ctrlCInsideConfirms`.

# Implementation notes

- Confirm anatomy is fixed: title is a question, body is the consequence
  with full target identity, irreversibility line when applicable, `y` to
  confirm. The test for needing a confirm is "loses user work or mutates
  the filesystem destructively", not "feels important".
- Elevation is requested exactly when the computed plan contains
  privileged operations — never pre-emptively at startup or on module
  open. The modal takes the password submission as a callback and owns no
  policy; the existing exec/sudo path does the work. No real sudo
  anywhere near tests.
- The M14 writes dialogs and 0072 module-management dialogs keep their
  domain logic; only the shell changes (pushed view → modal). The
  `dialog.back()` esc semantics fold into the compositor's pop rule.
- Apply detail: full-width-ish modal over the progress view-model's
  output ring (viewport, wheel + `j`/`k`); header line with run status
  and counts. Ambient status stays in the footer spinner + header badge —
  the modal is for inspecting, not monitoring.
- Every overlay in the app is now the same kind of thing, rendered by
  the same compositor, dismissed by the same `esc` pop.

# Docs

- tui.md dialogs/elevation sections rewritten (the footer-sudo design is
  deleted); log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
