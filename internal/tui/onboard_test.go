package tui

// T-tui-onboard-here: `o` on either pane opens the onboard dialog
// prefilled with the selection — the nav row's module and layer, or the
// workspace's active module and tab. The fields stay editable. The
// palette lists the same action for the selected module.

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// onboardModalOf pops the compositor's single dialog modal as an
// onboardDialog, failing the test otherwise.
func onboardModalOf(t *testing.T, c *Compositor) *onboardDialog {
	t.Helper()
	require.Len(t, c.modals, 1)
	dm, ok := c.modals[0].(*dialogModal)
	require.True(t, ok, "the modal is the onboard dialog")
	d, ok := dm.d.(*onboardDialog)
	require.True(t, ok)
	return d
}

// withWrites wires the test fake so the dialog can open.
func withWrites(c *Compositor) *Compositor {
	c.SetWrites(func(*facts.Facts) Writes { return newFakeWrites() })
	return c
}

func TestOnboardHere_prefillsFromNavSelection(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":           "id = \"demo\"\n",
		"users/cri/modules/demo/module.toml": "id = \"demo\"\ndescription = \"user layer\"\n",
	})
	withWrites(c)

	// The nav cursor rests on the module row: base layer, id demo.
	c.openOnboardHere()
	d := onboardModalOf(t, c)
	require.Equal(t, "demo", d.rows[0].field.String(), "the module field carries the module id")
	require.Equal(t, "base", d.rows[2].choice.String(), "a module row prefills the base layer")
}

func TestOnboardHere_prefillsLayerFromChildRow(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":           "id = \"demo\"\n",
		"users/cri/modules/demo/module.toml": "id = \"demo\"\ndescription = \"user layer\"\n",
	})
	withWrites(c)
	c = wsPress(t, c, "l") // expand demo
	c = wsPress(t, c, "j") // onto the base child
	c = wsPress(t, c, "j") // onto the user child
	c.openOnboardHere()
	d := onboardModalOf(t, c)
	require.Equal(t, "demo", d.rows[0].field.String())
	require.Equal(t, "user", d.rows[2].choice.String(), "a layer child prefills its layer")
}

func TestOnboardHere_prefillsFromWorkspaceTab(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":           "id = \"demo\"\n",
		"users/cri/modules/demo/module.toml": "id = \"demo\"\ndescription = \"user layer\"\n",
	})
	withWrites(c)
	c = wsPress(t, c, "enter") // into the workspace: the base tab is active
	c.openOnboardHere()
	d := onboardModalOf(t, c)
	require.Equal(t, "demo", d.rows[0].field.String())
	require.Equal(t, "base", d.rows[2].choice.String())
}

func TestOnboardHere_staysEditableAndRefusesWithoutModule(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n"})
	withWrites(c)
	c.openOnboardHere()
	d := onboardModalOf(t, c)
	d.HandleKey("backspace") // the prefilled module field edits like any other
	require.Equal(t, "dem", d.rows[0].field.String())

	// A shell with no selection refuses loudly.
	_, empty := wsShell(t, map[string]string{})
	empty.nav.modules = nil
	empty.openOnboardHere()
	require.Empty(t, empty.modals, "no dialog without a module")
	require.Equal(t, "select a module first", empty.message)
}

func TestOnboardHere_paletteListsTheAction(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n"})
	c.openPalette()
	p := c.modals[len(c.modals)-1].(*paletteModel)
	require.Contains(t, p.actionLabels(), "onboard into demo")
}
