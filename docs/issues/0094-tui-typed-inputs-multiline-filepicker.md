---
type: Issue
title: TUI typed inputs — multi-line editor, file picker, and path-aware forms
description: Fields now edit through the input their value's shape demands — a writes block or hook opens a multi-line modal editor (optional source coloring), path fields open a desktop-style file picker on enter, and form fields that take paths browse with ctrl+o. Every commit rides the existing validate/splice/undo pipeline.
tags: [feat, tui, dogfooded]
timestamp: 2026-09-23T00:00:00Z
---

# ISSUE 0094: TUI typed inputs — multi-line editor, file picker, and path-aware forms

- **Type**: feat
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0095](0095-tui-links-edit-opens-link-modal.md) (the link modal — the pair input this issue keeps), [0092](0092-tui-field-caret-glyph-shifts-text.md) (the caret convention the editor reuses), [0091](0091-tui-text-inputs-drop-paste.md) (paste as a message; the editor keeps its newlines), [0077](0077-tui-add-forms.md) (the forms ctrl+o extends)
- **Related code**: [`internal/tui/filepicker.go`](../../internal/tui/filepicker.go), [`internal/tui/multiline.go`](../../internal/tui/multiline.go), [`internal/tui/editing.go`](../../internal/tui/editing.go), [`internal/tui/addform.go`](../../internal/tui/addform.go)

## Summary

User request: different types of inputs — one line of text, multi-line
text (optional source coloring), paths (file/directory) opening a
dialog on enter "close to actual desktop GUI dialogs, while keeping it
simple for TUI usage", and special ones like symlinks'
`target ← source` pair. The workspace editor previously had exactly one
input: the inline single-line field. A block write was `noEdit` (dead
to enter), a multi-line hook edited as one line, and a path was typed
blind.

The row now carries an **edit kind** and `startEdit` dispatches on it:
`kindText` (the inline input, the default), `kindMulti` (the new
multi-line editor modal), and the path kinds (the new file picker:
`kindDirs`, `kindFiles`, `kindEither`). Symlinks needed no new input:
the pair input is 0095's link modal, unchanged — its `source` field
gained browsing.

## Details

**Multi-line editor** ([`multiline.go`](../../internal/tui/multiline.go))
— a modal seeded with the row's full value; enter splits, backspace
joins at column 0, arrows/home/end/pgup/pgdn walk across line
boundaries, paste keeps its newlines (CR-normalized; the single-line
sanitizer handles the rest), `ctrl+enter` commits, `esc` cancels
through the compositor's pop. Rows: writes **block** entries (no longer
`noEdit` — `value` carries the block, `mutateField` gained the block
branch, emptying refuses: `d` removes the entry) and pre/post **hook**
commands. The caret is the 0092 reverse-video block. `ctrl+g` toggles
[chroma](https://github.com/alecthomas/chroma) coloring guessed from
the target's extension — a bare letter can never be a binding in a
text editor (every printable key is content), and the caret's own line
renders uncolored so the reversed caret cell survives.

**File picker** ([`filepicker.go`](../../internal/tui/filepicker.go)) —
a custom modal, not the bubbles filepicker, per the request. Dirs-first
case-insensitive listing, symlinks resolved (broken ones are files),
dotfiles behind `.`; starts at the field's current value (file → its
dir, missing path → nearest existing ancestor, empty → `$HOME`);
up/down/pgup/pgdn/home/end, `→`/`l` descend, `←`/`h`/backspace parent,
`~` home; enter picks per mode (dirs mode picks directories; files mode
descends into them; either does both), `ctrl+enter` picks the shown
directory; `/` filters the current listing with the nav filter's
semantics; `ctrl+l` opens the location bar — type or paste any path
(tilde expands; a directory navigates; a not-yet-existing leaf picks
while its parent exists — the escape hatch for new mountpoints, share
dirs, and device paths); an unreadable directory is never entered (the
failure names itself inline); esc backs out of filter/location first,
then closes. Workspace rows: smb share `path` (dirs), mounts
`destination` (dirs), mounts `source` (either).

**Forms browse with ctrl+o.** In a form, enter already means *commit
the form*, so path-kind form fields open the picker with `ctrl+o` and
the pick fills the field text (typing still works): onboard `paths`
(either; the pick appends to the space-separated list), the add/edit
link forms' `source` (either), the writes add form's `target` (files),
and the mounts/smb `field = value` forms, where the browse mode follows
the chosen field (`source`→either, `destination`/`path`→dirs). The
footers name the gesture only on rows that have it.

**The commits are the pipeline's.** Every pick and editor commit rides
`commitFieldAt` → `validateField` → `applyEdit` (splice round-trip,
ledger, undo) — a refusal renders inside the modal and it stays open.
Two compositor hooks generalize the modal contract: `onEsc() bool`
(a modal backs out of its own inner state before the pop) and
`ownsText() bool` (`?` is content in the editor and in the picker's
text inputs, not the help gesture).

**Encoding note**: committed multi-line content lands in module.toml as
one escaped string line (`tomlBasicString` escapes newlines) — valid
and round-trip-proven on every commit; emitting pretty `"""` literal
blocks is encoder cosmetics, a separate issue.

## Testing

TDD red→green in three batches. Picker unit tests against `t.TempDir`
fixtures (ordering, dirs-only mode, hidden toggle, filter + esc clamp,
per-mode enter, navigation, location bar pick/cd/new-leaf/refusals,
seed resolution, commit refusal, unreadable dir, wheel, view) —
compile-red first; GREEN fixed one real defect the unreadable-dir test
demanded (a failed read navigated anyway) and three test bugs of mine
(selection index, case-insensitive sortedness asserted byte-wise,
`keyPress` lacking `backspace`). Multi-line unit tests (seed/round-trip,
split/join, caret movement, paste lines, refusal, tab, color toggle,
caret-line-plain view) — compile-red; GREEN fixed a real paste index
panic (`slices.Replace` bounds computed after advancing the cursor) and
two test bugs (caret splitting the searched word; the border's
truecolor caught by the no-token-colors assertion). Compositor wiring
tests (block/hook open+commit+refusal, smb/mounts picker rows, esc
semantics, `?` types in the editor, all five ctrl+o sites, onboard
append) — 14 behavior-red first. Three existing tests updated to the
new UX deliberately: block rows show `✎` (the mark cannot lie), the
mounts required-field refusal pins on text-kind `type` (the picker
cannot produce an empty path), the smb path edit drives the picker.
`workspace-all.golden` regenerated (exactly the block row's `✎`).
Gates: `go test ./...` 20 packages ok, `go vet ./...` exit 0.

## Resolution

Done. Deviations from the first design sketch, all forced by reality:
the color toggle is `ctrl+g`, not `c` (a bare letter is content in a
text editor); the location bar accepts a non-existent directory whose
parent exists (refusing made not-yet-created mountpoints and
non-local share paths uneditable — a typed path is an explicit
request); the esc/`?` handling is two named modal hooks on the
compositor, not per-modal hacks.
