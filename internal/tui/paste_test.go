package tui

// T-0091-paste: bracketed paste arrives as tea.PasteMsg, not as key
// presses — every text input in the shell consumes it. Insertion
// respects the caret where one exists (workspace editor, dialog
// fields); append-only inputs (nav filter, palette, password) extend
// the buffer. Control runes (newlines, CR, tab) drop out — every field
// is single-line.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestPaste_workspaceFieldEdit(t *testing.T) {
	_, _, c := editShell(t)
	c = cpress(c, "enter") // edit the field under the cursor; caret at the end
	seed := c.ws.editing.inputString()
	c = cpress(c, "left")
	c = cpress(c, "left")
	c, _ = cstep(c, tea.PasteMsg{Content: "XX"})
	want := seed[:len(seed)-2] + "XX" + seed[len(seed)-2:]
	require.Equal(t, want, c.ws.editing.inputString(), "paste inserts at the caret")
}

func TestPaste_stripsControlRunes(t *testing.T) {
	_, _, c := editShell(t)
	c = cpress(c, "enter")
	seed := c.ws.editing.inputString()
	c, _ = cstep(c, tea.PasteMsg{Content: " one\ntwo\r\nthree\t"})
	require.Equal(t, seed+" onetwothree", c.ws.editing.inputString(),
		"newlines, CR, and tab drop out; spaces stay")
}

func TestPaste_onboardDialog(t *testing.T) {
	d := newOnboardDialog(newFakeWrites(), "/profile")
	dm := &dialogModal{d: d, th: newTheme(true)}
	dm.update(tea.PasteMsg{Content: "myapp"})
	require.Equal(t, "myapp", d.rows[0].field.String(), "the focused field takes the paste")

	d.HandleKey("down") // the paths row
	dm.update(tea.PasteMsg{Content: "~/.config/nvim\n"})
	require.Equal(t, "~/.config/nvim", d.rows[1].field.String())
}

func TestPaste_dialogGateSwallowsPaste(t *testing.T) {
	d := newOnboardDialog(newFakeWrites(), "/profile")
	dm := &dialogModal{d: d, th: newTheme(true)}
	dm.update(tea.PasteMsg{Content: "myapp"})
	d.HandleKey("down") // the paths row
	dm.update(tea.PasteMsg{Content: "~/.config/nvim"})
	d.HandleKey("enter") // arm the gate
	dm.update(tea.PasteMsg{Content: "LATE"})
	require.Equal(t, "myapp", d.rows[0].field.String(), "an armed gate swallows paste like any key")
	require.Equal(t, "~/.config/nvim", d.rows[1].field.String())
}

func TestPaste_restoreTargetsDialog(t *testing.T) {
	d := newRestoreDialog(newFakeWrites(), "/profile", nil)
	dm := &dialogModal{d: d, th: newTheme(true)}
	dm.update(tea.PasteMsg{Content: "~/.zshrc ~/.bashrc"})
	require.Equal(t, "~/.zshrc ~/.bashrc", d.targets.String())
}

func TestPaste_generateDialog(t *testing.T) {
	d := newGenerateDialog(newFakeWrites(), "/profile")
	dm := &dialogModal{d: d, th: newTheme(true)}
	dm.update(tea.PasteMsg{Content: "nas"}) // the kind row is a choice; paste lands nowhere
	require.Equal(t, "mounts", d.module.String(), "a choice row ignores paste")
	d.HandleKey("down")
	d.HandleKey("down") // the name field
	dm.update(tea.PasteMsg{Content: "media"})
	require.Equal(t, "media", d.mounts[0].field.String())
}

func TestPaste_manageCreateDialog(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml": "id = \"demo\"\napp = \"demo-app\"\n",
	})
	c = cpress(c, "m")
	c = cpress(c, "enter") // create module
	c, _ = cstep(c, tea.PasteMsg{Content: "newmod"})
	md := c.modals[0].(*dialogModal).d.(*manageDialog)
	require.Equal(t, "newmod", md.rows[0].field.String(), "the app field takes the paste")
}

func TestPaste_addForm(t *testing.T) {
	_, rows, build, _ := addFormSpec("demo", "packages", "", "")
	f := newAddForm(newTheme(true), "add package · demo", rows, build, func(string) string { return "" })
	f.update(tea.PasteMsg{Content: "ripgrep\n"})
	require.Equal(t, "ripgrep", f.rows[0].field.String())
}

func TestPaste_navFilter(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	c, _ = cstep(c, tea.PasteMsg{Content: "oth"})
	require.Equal(t, "oth", string(c.nav.query), "the query extends")
	require.Equal(t, "other", c.nav.selected().moduleID, "the list refilters")
}

func TestPaste_palette(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "tab") // workspace focus: / opens the palette there
	c = cpress(c, "/")
	require.IsType(t, &paletteModel{}, c.modals[0])
	c, _ = cstep(c, tea.PasteMsg{Content: "demo"})
	p := c.modals[0].(*paletteModel)
	require.Equal(t, "demo", string(p.query), "the palette query extends")
}

func TestPaste_elevationPassword(t *testing.T) {
	e := &elevationModel{th: newTheme(true)}
	e.update(tea.PasteMsg{Content: "s3cr et\n"})
	require.Equal(t, []byte("s3cr et"), e.input, "spaces survive; the newline drops")
}
