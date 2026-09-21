package tui

// T-tui-editing: per-field inline editing in the workspace over the 0065
// file-scoped draft ledger. Editing never leaves the workspace: the row
// becomes an input, tier-1 validation renders at the field, commits
// splice through the profile family encoders into the draft's working
// raw, and ctrl+s runs the untouched save pipeline (tomlsplice + atomic
// write, disk-hash conflict as a modal). Drafts survive navigation and
// layer switches; D discards with a confirm naming module, layer, and
// change count; a adds rows to the current table, d removes with
// confirm. Broken files edit as raw text lines and save through the raw
// candidate path.

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// typeText sends each rune as a key with Text set — what a real terminal
// delivers while a field input is active.
func typeText(c *Compositor, s string) *Compositor {
	for _, r := range s {
		c, _ = cstep(c, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return c
}

func keyCtrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

const editFixture = `id = "demo"
description = "the demo module"

[packages]
present = ["neovim", "ripgrep"]

[tools]
node = "20"
`

// editShell loads the edit fixture, focuses the workspace, and parks the
// cursor on the description row (the first editable field).
func editShell(t *testing.T) (map[string]string, string, *Compositor) {
	t.Helper()
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": editFixture})
	dir := c.ws.activeDir()
	c = cpress(c, "tab")
	return subs, dir, cpress(c, "j") // id row → description row
}

func TestEdit_enterStartsEdit_escExits(t *testing.T) {
	_, _, c := editShell(t)

	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing, "enter on an entry row starts the field edit")
	require.Equal(t, "the demo module", c.ws.editing.inputString(), "the input seeds from the field's value")

	c = typeText(c, " x")
	require.Equal(t, "the demo module x", c.ws.editing.inputString())

	c = cpress(c, "esc")
	require.Nil(t, c.ws.editing, "esc cancels the field edit")
	require.Empty(t, c.ws.draftEdits(), "a cancelled edit stages nothing")
}

func TestEdit_commitSplicesIntoDraft(t *testing.T) {
	_, dir, c := editShell(t)

	c = cpress(c, "enter") // meta app row
	c = typeText(c, "-next")
	c = wsPress(t, c, "enter")
	require.Nil(t, c.ws.editing, "enter commits the field")
	require.Contains(t, c.ws.placeholderOrBody(), "the demo module-next", "the row re-renders from the draft")
	require.NotNil(t, c.wsDraftFor(dir), "a commit forks the draft")
}

func TestEdit_fieldEditActive(t *testing.T) {
	subs, _, c := editShell(t)
	c = cpress(c, "enter")
	c = typeText(c, "-next")
	requireGolden(t, "edit-field-active.golden", c.View().Content, subs)
}

func TestEdit_inlineErrorAndWarning(t *testing.T) {
	subs, _, c := editShell(t)
	// A free-text field's tier-1 grammar renders live at the field.
	// (0076: closed-set fields open the choice picker instead — their
	// invalid values are unreachable by construction.)
	c = cursorTo(t, c, "neovim")
	c = cpress(c, "enter")
	for i := 0; i < len("neovim"); i++ {
		c, _ = cstep(c, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	require.Contains(t, c.ws.editing.err, "package name", "tier-1 renders live at the field")
	requireGolden(t, "edit-inline-error.golden", c.View().Content, subs)
}

func TestEdit_dirtyRowMarker(t *testing.T) {
	subs, _, c := editShell(t)
	c = cpress(c, "enter")
	c = typeText(c, "-next")
	c = wsPress(t, c, "enter")
	requireGolden(t, "edit-dirty-row.golden", c.View().Content, subs)
}

func TestEdit_draftSurvivesNavigationAndLayerSwitch(t *testing.T) {
	subs, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":           editFixture,
		"users/cri/modules/demo/module.toml": "id = \"demo\"\ndescription = \"user layer\"\n",
		"modules/other/module.toml":          "id = \"other\"\n",
	})
	active := c.ws.activeDir()

	c = cpress(c, "tab")
	c = cpress(c, "j") // id row → app row
	c = cpress(c, "enter")
	c = typeText(c, "-next")
	c = wsPress(t, c, "enter")
	require.NotNil(t, c.wsDraftFor(active))

	// Away to another module and back: the draft is file-scoped. Module
	// navigation is the nav's vocabulary — tab back first.
	c = cpress(c, "tab")
	c = wsPress(t, c, "j") // demo → other
	require.Nil(t, c.ws.draft, "the other module has no draft")
	c = wsPress(t, c, "k") // back to demo
	require.NotNil(t, c.wsDraftFor(active), "the draft waits where it was left")
	require.Contains(t, c.ws.placeholderOrBody(), "the demo module-next")

	// Layer switch: the user layer has no draft; back to base it returns.
	c = cpress(c, "tab")
	c = wsPress(t, c, "L")
	require.Nil(t, c.ws.draft)
	require.Contains(t, c.ws.placeholderOrBody(), "user layer")
	c = wsPress(t, c, "L")
	require.Contains(t, c.ws.placeholderOrBody(), "the demo module-next", "the draft re-renders on return")

	// The nav agrees: demo's base layer row carries the dirty marker.
	require.True(t, c.draftMarks()[active])
	requireGolden(t, "edit-dirty-nav.golden", c.View().Content, subs)
}

func TestEdit_saveWritesAndClearsDraft(t *testing.T) {
	_, dir, c := editShell(t)

	c = cpress(c, "enter")
	c = typeText(c, "-next")
	c = wsPress(t, c, "enter")
	c, cmd := cstep(c, keyCtrl('s'))
	for i := 0; i < 10 && cmd != nil; i++ { // settle the save op
		msg := mustMsg(cmd)
		if msg == nil {
			break
		}
		c, cmd = cstep(c, msg)
	}
	require.Nil(t, c.wsDraftFor(dir), "a successful save clears the draft")
	raw, err := os.ReadFile(filepath.Join(dir, "module.toml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "description = \"the demo module-next\"", "tomlsplice wrote the field")
	require.Contains(t, string(raw), "id = \"demo\"", "untouched sections keep their bytes")
	require.Equal(t, "saved demo", c.message, "the footer reports the save")
}

func TestEdit_saveBlockedOnTier1Errors(t *testing.T) {
	_, _, c := editShell(t)

	// Commit an emptied package: staged, marked, and blocking the save.
	// (0076: closed-set fields can no longer hold invalid values — the
	// free-text grammar still can.)
	c = cursorTo(t, c, "neovim")
	c = cpress(c, "enter") // the neovim package row
	for i := 0; i < len("neovim"); i++ {
		c, _ = cstep(c, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	c = wsPress(t, c, "enter")
	require.Nil(t, c.ws.editing)
	require.NotEmpty(t, c.ws.draftErrs(), "the invalid value is staged with its error")

	c, cmd := cstep(c, keyCtrl('s'))
	for i := 0; i < 10 && cmd != nil; i++ {
		msg := mustMsg(cmd)
		if msg == nil {
			break
		}
		c, cmd = cstep(c, msg)
	}
	require.True(t, c.msgErr, "the save is blocked while tier-1 errors stand")
	require.Contains(t, c.message, "package name")
	require.NotNil(t, c.ws.draft, "the blocked save keeps the draft")
}

func TestEdit_diskConflictRefusalSurfaces(t *testing.T) {
	_, dir, c := editShell(t)
	c = cpress(c, "enter")
	c = typeText(c, "-next")
	c = wsPress(t, c, "enter")

	// The file changes on disk behind the draft's back.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte("\n"), 0o644))

	c, cmd := cstep(c, keyCtrl('s'))
	for i := 0; i < 10 && cmd != nil; i++ {
		msg := mustMsg(cmd)
		if msg == nil {
			break
		}
		c, cmd = cstep(c, msg)
	}
	require.Len(t, c.modals, 1, "the conflict refusal surfaces as a modal")
	require.Contains(t, c.View().Content, "changed on disk")
	require.NotNil(t, c.wsDraftFor(dir), "the draft survives the refusal")

	c = cpress(c, "n") // keep editing
	require.Empty(t, c.modals)
	require.NotNil(t, c.wsDraftFor(dir))
}

func TestEdit_discardRequiresConfirm(t *testing.T) {
	_, dir, c := editShell(t)
	c = cpress(c, "enter")
	c = typeText(c, "-next")
	c = wsPress(t, c, "enter")

	c = cpress(c, "D")
	require.Len(t, c.modals, 1, "D asks before discarding")
	frame := c.View().Content
	require.Contains(t, frame, "discard draft")
	require.Contains(t, frame, "demo", "the confirm names the module")
	require.Contains(t, frame, "base", "the confirm names the layer")
	require.Contains(t, frame, "1 change", "the confirm counts the staged changes")

	c = cpress(c, "n")
	require.NotNil(t, c.wsDraftFor(dir), "n keeps the draft")
	c = cpress(c, "D")
	c = cpress(c, "y")
	require.Nil(t, c.wsDraftFor(dir), "y discards the draft")
	require.Contains(t, c.ws.placeholderOrBody(), "id demo", "the surface reverts to disk state")
	require.NotContains(t, c.ws.placeholderOrBody(), "demo-app-next")
}

func TestEdit_rowAddRemove(t *testing.T) {
	_, _, c := editShell(t)

	// a on the packages section opens the add form; the name commits.
	for !c.ws.atSection("packages") {
		c = cpress(c, "j")
	}
	c = cpress(c, "a")
	addFormTop(t, c)
	c = typeText(c, "fd")
	c = wsPress(t, c, "enter")
	require.Contains(t, c.ws.placeholderOrBody(), "+ fd", "the committed row joins the table")

	// d on that row asks, then removes.
	for c.ws.rows[c.ws.cursor].value != "fd" {
		c = cpress(c, "j")
	}
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before removing")
	require.Contains(t, c.View().Content, "fd")
	c = cpress(c, "y")
	require.NotContains(t, c.ws.placeholderOrBody(), "+ fd", "y removes the row")
	require.Contains(t, c.ws.placeholderOrBody(), "+ neovim", "siblings stand")
}

func TestEdit_rawTextDegradedMode(t *testing.T) {
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n[broken\n"})
	c = cpress(c, "tab")

	// Raw rows are selectable and line-editable.
	c = cpress(c, "j") // line 2: [broken
	c = cpress(c, "e")
	require.NotNil(t, c.ws.editing, "a broken file edits as raw text lines")
	require.Equal(t, "[broken", c.ws.editing.inputString())
	c = typeText(c, "]")
	c = wsPress(t, c, "enter")
	require.Contains(t, c.ws.placeholderOrBody(), "[broken]", "the line splice lands in the draft")
	requireGolden(t, "edit-raw-degraded.golden", c.View().Content, subs)

	// Repair the line into a real table header: the structured surface
	// unlocks (state visible, never silent).
	c = cpress(c, "e")
	c.ws.editing.input = []rune("[packages]") // the line editor seeds from the row; set the repair directly
	c = wsPress(t, c, "enter")
	require.Contains(t, c.ws.placeholderOrBody(), "meta", "a file that parses again renders structured")
	require.Contains(t, c.ws.placeholderOrBody(), "packages")

	// A raw-mode save writes the whole repaired file (the disk original is
	// broken — family splice has nothing sound to splice into).
	dir := c.ws.activeDir()
	c, cmd := cstep(c, keyCtrl('s'))
	for i := 0; i < 10 && cmd != nil; i++ {
		msg := mustMsg(cmd)
		if msg == nil {
			break
		}
		c, cmd = cstep(c, msg)
	}
	require.False(t, c.msgErr, "the repair saves: %s", c.message)
	raw, err := os.ReadFile(filepath.Join(dir, "module.toml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "[packages]")
	require.Nil(t, c.wsDraftFor(dir), "the save clears the draft")
}

// 0078: enter is the primary action in context. A row that owns no
// field (a section header, a structural container) grows rows instead:
// enter opens the same add form `a` opens, so an empty section is
// reachable without knowing a second key.

func TestEdit_enterOnEmptyHeaderOpensAddForm(t *testing.T) {
	_, dir, c := editShell(t)
	wsToHeader(t, c, "hooks") // an empty section: no field to edit
	c = wsPress(t, c, "enter")
	f := addFormTop(t, c)
	require.Nil(t, c.ws.editing, "no invisible inline input behind the form")
	require.Contains(t, f.view(100, 30), "add hook · demo")
	require.Nil(t, c.wsDraftFor(dir), "opening a form stages nothing")
}

func TestEdit_enterOnContainerAddsIntoEntry(t *testing.T) {
	_, c := systemdShell(t)
	wsToKey(t, c, "demo.service") // the unit container row
	c = wsPress(t, c, "enter")
	f := addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "add directive · demo.service")
}
