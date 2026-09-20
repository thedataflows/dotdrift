---
type: Issue
title: TUI ergonomics round — paging, visible selection, render-what-is disclosure
description: The compositor needs page keys, a selection you can see, and surfaces that show what is set instead of enumerating what could be set.
tags: [issue, tui, ergonomics, follow-up]
timestamp: 2026-09-16T00:00:00Z
---

# ISSUE 0075: TUI ergonomics round — paging, visible selection, render-what-is disclosure

- **Type**: task
- **Status**: done
- **Closing commits**: 4bf07a6, 43f87b4, a8050e3

## Context

Dogfooding the compositor ([0073](0073-tui-compositor-redesign.md)) and the
structural editors ([0074](0074-structural-section-editing.md)) surfaced
three usability gaps:

1. **No page keys.** pgup/pgdown/home/end move nothing in either pane; a
   long profile or a long module surface is reachable only by held `j`.
   The workspace's scroll window also never follows the cursor after a
   load (the nav has window logic, the workspace does not).
2. **Invisible selection.** The cursor row is a bare bold attribute —
   next to section labels (also bold) it does not read as a selection.
   bubbles' list delegate, the ecosystem's reference treatment, renders
   the selected row with a left border bar in an accent hue plus bold
   accent-colored text, and pads normal rows so text stays
   column-aligned.
3. **Enumeration instead of state.** The when tree renders all seven
   leaf types in every group whether set or not; secrets, mounts, and
   smb render every field of every entry; empty sections fill with
   "(none)". A surface should show what IS. The palette (`/`) stays the
   place to ask for what is not visible.

## Details

Three tasks, TDD-first, one commit each:

- **T-tui-page** — pgup/pgdown move the cursor by the visible body
  height, home/end jump to the first/last row, in both panes.
  Binding-table entries only, so the footer hints and `?` help follow.
  The workspace gains the cursor-following scroll window (the nav rule:
  offset clamps to the cursor in view).
- **T-tui-selection** — a `cursorRow` registry entry: left bar + bold
  accent text (fuchsia, the registry's highlight hue — the same
  treatment family as bubbles' selected-row delegate). Normal rows keep
  their two-column lead, so text stays column-aligned with the cursor
  row. Applied to every cursor: nav rows, workspace rows, the active
  field input, palette rows, writes-menu rows, manage/dialog rows. The
  active layer tab keeps plain bold — a different concept.
- **T-tui-disclosure** — a row renders only when it differs from the
  zero value: unset when leaves, unset secrets/mounts/smb fields, unset
  smb scalars render nothing; empty sections render the header line
  only, no "(none)". meta keeps description/scope (identity rows, not
  options). Hiding creates gaps, so the add grammar closes them: `a` on
  a section header adds the section's entry (headers of empty sections
  are selectable rest points; the when/smb headers always are — they are
  the section-level scope), `a` on a container adds `field = value`
  INTO it (secrets/mounts/smb gain what systemd already had — the
  fresh-entry dead end), `a` on when accepts `field = value` leaves
  beside and/or/not, `a` on the smb header accepts `field = value` for
  its scalars beside bare share names. A container with nothing inside
  shows one dim hint row naming the gesture. Editing a field to its
  zero value clears it where the mutator permits (required fields still
  refuse).

## Acceptance Criteria

- [x] pgup/pgdown/home/end work in the nav and the workspace; the
      workspace's visible window follows the cursor. (`TestNav_pageKeys`,
      `TestWorkspace_pageKeysFollowTheCursor` — the window assertions
      failed against the pre-fix offset behavior.)
- [x] The cursor row carries a bar + accent text everywhere a cursor
      exists; the new keys appear in the footer and `?` help from the
      one binding table. (`TestTheme_cursorRow` pins the treatment and
      the column alignment; `TestNav_cursorRowBar`,
      `TestWorkspace_cursorRowBar`, `TestPalette_cursorRowBar`,
      `TestWritesMenu_cursorRowBar`, `TestManage_cursorRowBar`;
      `TestKeys_noOrphanedBindings` pins the eight new entries.)
- [x] A module surface shows only set values plus section headers; the
      when tree shows set leaves and existing groups only; empty
      sections render the header alone. (`TestWorkspace_disclosureRendersOnlySet`,
      `TestWhen_treeRenders`, `TestEntries_sectionsReplaceOther`'s
      disclosed counts.)
- [x] Everything hidden stays reachable through the add grammar; adds
      and clears round-trip through the encoders with the splice
      guarantees (contract 19); saves refuse structural gaps as before.
      (`TestWhen_headerAddsRootLeaf`, `TestWhen_groupAddNested`,
      `TestWhen_notAddAndDuplicateRefuse`, `TestWhen_emptyGroupBlocksSave`,
      `TestSecrets_containerFieldAdd`, `TestSmb_containerFieldAdd`,
      `TestSystemd_containerFieldAdd`, `TestSmb_scalarAndShareEdits`,
      `TestMounts_addBlocksSaveUntilFilled`, `TestSecrets_envEditAndBool`'s
      cleared-bool row, `TestWhen_groupLeafEdit`'s cleared leaf.)
- [x] Goldens regenerated and eyeballed; `go test ./...` green per task.
      (Final tree: 20 packages ok / 0 FAIL with `-count=1`, `go vet`
      clean, gofmt clean on touched files, golangci-lint 0 issues, 132
      tui tests passing.)

## Out of Scope

Undo; the deleted M14 status screen; structural dialogs/forms; palette
re-ranking.
