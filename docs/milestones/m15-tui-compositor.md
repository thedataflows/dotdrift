---
type: Milestone
title: M15 TUI compositor
description: TUI presentation-layer redesign — one shell plus a modal compositor, sectioned workspace with inline editing, elevation modal, fuzzy palette, single keymap. Domain untouched; old view stack deleted last.
tags: [milestone, tui]
timestamp: 2026-09-13T00:00:00Z
order: 15
---

# Goal

**Status: complete (2026-09-15).** All eight tasks landed; the compositor
is the only `dotdrift tui`. Known follow-up: issue
[0074](/issues/0074-structural-section-editing.md) (structural-section
editing).

Rebuild `dotdrift tui`'s presentation layer per the accepted redesign
(issue [0073](../issues/0073-tui-compositor-redesign.md)): a compositor —
one base shell (header / nav / workspace / footer) plus a modal stack —
replacing the M14 view stack, the details text dump, the editor-form
pushed views, and the footer sudo prompt. Every domain package, the 0065
draft machinery, the apply session, and ADR-0008's service doorway survive
unchanged; this milestone is presentation-only, and the untouched domain
test suite is the proof. `docs/product/tui.md` is rewritten to match as
each task lands.

# Exit criteria

- **The shell.** Full-screen (altscreen) base layout: header (profile
  root, host/user context, dirty count, apply badge), nav left, workspace
  right, footer (spinner+operation name, transient message slot, keymap
  hints for the focused pane). Focus moves with `tab`/`shift+tab`; focus
  is border color; exactly one pane focused. Reflow behavior designed and
  golden-tested at fixed sizes (100x30 and 64x24).
- **The compositor.** Modals render over the dimmed base on a stack;
  while a modal is open it owns all input; `esc` pops the top layer —
  modal first, then edit mode, then focus to nav. One rule, no per-view
  surprises.
- **The nav.** Modules as rows with overlay layers (base / user / host)
  as expandable children; expansion state remembered per session; dirty
  `●` markers per file; greyed rows for modules that fail to load;
  placeholder rows while reads are in flight. Nav selection and workspace
  content never disagree.
- **The workspace.** `module.toml` rendered as a sectioned surface
  (meta, packages, links, writes, when, hooks, systemd.units, tools),
  each section with status glyphs (including `needs root` markers from
  the plan) and a designed empty state. Layer tabs cycle base → user →
  host, synced with nav's overlay children. Parse errors degrade to a
  raw-text mode with the error named; the workspace never goes blank.
- **Inline editing.** `enter`/`e` edits the field under the cursor in
  place over the 0065 file-scoped draft ledger: three-tier validation
  with inline error/warning, `ctrl+s` saves (disk-hash conflict refusal
  surfaces as a modal), `D` discards with confirm, drafts survive
  navigation and layer switches. Tables (packages, links) add rows with
  `a`, remove with `d` + confirm.
- **Elevation.** One modal per elevation, never per action: the plan's
  privileged operations are collected and listed as the reason before the
  password field; one prompt covers them all. Wrong password keeps the
  modal open with the failure shown; three failures abort; sudo timestamp
  expiry re-prompts in the same shape. Cancel aborts the whole operation
  before anything is touched. The password is a zeroed `[]byte`, never in
  the draft ledger, never logged. Plan/dry-run surfaces `needs root` in
  advance. Non-interactive contexts keep existing CLI behavior.
- **The palette.** `/` opens a fuzzy palette indexing (in fixed section
  order) modules+layers, contextually valid actions, and current-module
  fields; fzf-style subsequence ranking, recents (in-session, last 8) as
  tiebreak and empty-query content; selection jumps nav+workspace in sync
  or runs the action; `ctrl+n` from the no-results state opens
  create-module with the query prefilled. Closing restores prior focus
  exactly.
- **Dialogs.** One confirm component for every destructive action
  (discard draft, remove link/write/module, destructive apply, dirty
  quit): title is a question, body names the full target identity and
  consequence, `y` confirms and everything else cancels. The M14 writes
  dialogs and module-management dialogs become standard modals. Apply
  detail (output ring) is a modal; closing it never cancels the run
  (`ctrl+c` inside, with confirm). Success messages fade; failures persist
  until the next user action.
- **The keymap.** One binding table: shift is "the dangerous version"
  (`p`/`P`, `d`/`D`), `esc` has exactly one meaning, no undo (drafts +
  confirms are the safety net), `?` opens contextual help listing only
  currently valid bindings. Mouse mirrors everything; keyboard never less
  capable.
- **Non-goals hold.** No undo, no another-host preview, no when-tree
  builder, no shell-command palette (0065-D12's fog list stands). No
  domain or CLI output changes.
- `go test ./...`, `go vet ./...`, `golangci-lint run ./...` green;
  offline `GOFLAGS=-mod=vendor go test ./...` green. Goldens are
  fixed-size composited final frames (base + modal layers), not
  intermediate view output; a small set of message-driven interaction-flow
  tests covers what goldens cannot express.

# Tasks

1. [T-tui-compositor](/tasks/t-tui-compositor.md) — compositor + shell
2. [T-tui-nav](/tasks/t-tui-nav.md) — nav tree
3. [T-tui-workspace](/tasks/t-tui-workspace.md) — workspace read surface
4. [T-tui-editing](/tasks/t-tui-editing.md) — inline editing over drafts
5. [T-tui-modals](/tasks/t-tui-modals.md) — confirm, elevation, writes, apply detail
6. [T-tui-palette](/tasks/t-tui-palette.md) — fuzzy palette on `/`
7. [T-tui-keymap](/tasks/t-tui-keymap.md) — binding table + contextual help
8. [T-tui-cleanup](/tasks/t-tui-cleanup.md) — delete the old view layer, docs sweep

Sequencing: the compositor is the platform and lands first; nav and
workspace next (they are the product); modals before the palette (palette
actions reuse them); the old view layer is deleted **last** — both layers
coexist during the migration so every task ends green, and the final
deletion is the proof nothing depends on the old paths.

# Depends on

[M14](m14-tui.md) (shell, editors, writes, apply — all shipped). Builds
on the 0065 draft machinery and the apply session without modifying them.
