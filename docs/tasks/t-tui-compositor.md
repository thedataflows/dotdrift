---
type: Task
title: T-tui-compositor
description: The compositor platform — base shell layout (header/nav/workspace/footer), modal stack, altscreen, focus model, and the single esc rule.
tags: [task, tdd, tui, compositor]
timestamp: 2026-09-13T00:00:00Z
milestone: m15
---

# Goal

Build the platform every later M15 task builds on, per
[M15](/milestones/m15-tui-compositor.md): the full-screen base shell and
the modal compositor. No real nav/workspace content yet — placeholder
panes prove the layout, focus, and overlay mechanics.

# Tests first

- Pure state machines + golden `View()` on **composited final frames** at
  fixed sizes 100x30 and 64x24 (never intermediate view output):
  `TestShell_layoutBase` (header/nav/workspace/footer regions),
  `TestShell_reflowNarrow` / `_veryNarrow` (64x24 and below),
  `TestCompositor_modalOverDimmedBase` (base rendered, dimmed, modal
  centered), `TestCompositor_modalStackRendersTopOwnsInput`.
- Message-driven: `TestFocus_tabCyclesPanes` / `_shiftTabReverses` /
  `_exactlyOnePaneFocused`, `TestEsc_popsTopLayer` (modal → edit mode →
  focus-to-nav ordering), `TestModal_openModalCapturesAllInput` (base
  receives nothing while open),
  `TestFooter_spinnerShowsOperationName` / `_messageSlotSuccessFades` /
  `_failurePersistsUntilNextAction`,
  `TestHeader_dirtyCountAndApplyBadge`.
- `TestPaletteRegistry_noInlineStyles` stays green — every style resolved
  through the ADR-0003 registry, adaptive `LightDark`.

# Implementation notes

- Altscreen on: goldens are width/height-sensitive by design; the test
  harness always renders at the two fixed sizes.
- Header: profile root, host/user context (read-only), dirty count
  (`● N` when drafts exist), apply badge while a session runs. Footer:
  spinner + operation name for any >200ms operation, transient message
  slot (success fades, failure in Error color persists until the next
  user action), keymap hints following the focused pane.
- The compositor is a stack of layers over the base; only the top layer
  receives messages; the base keeps rendering underneath (dimmed).
  `esc` is owned by the compositor, not individual views — this kills the
  M14 per-view fallthrough class of bug by construction.
- The old view stack stays alive alongside (deleted in T-tui-cleanup);
  the new shell mounts behind a task flag until it reaches parity.

# Docs

- `docs/product/tui.md` gains the compositor/shell section (the M15 spec
  starts replacing M14-era descriptions as tasks land); log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
