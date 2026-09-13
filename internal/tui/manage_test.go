package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/facts"
)

// The module-management dialogs (0072): `m` on a module selection opens
// the context menu; its entries open in place as forms over the config
// area's module ops. esc steps back through the menu, then out.

// selectModule walks the cursor to the first module node and syncs.
func selectModule(t *testing.T, m *Shell) treeItem {
	t.Helper()
	for _, it := range m.allItems() {
		if it.kind == kindModule {
			m.walkTo(it)
			m.syncSelection()
			return it
		}
	}
	t.Fatal("no module item in the tree")
	return treeItem{}
}

// selectModuleNamed walks the cursor to a module by id and syncs.
func selectModuleNamed(t *testing.T, m *Shell, id string) {
	t.Helper()
	for _, it := range m.allItems() {
		if it.kind == kindModule && it.moduleID == id {
			m.walkTo(it)
			m.syncSelection()
			return
		}
	}
	t.Fatalf("module %q is not in the tree", id)
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	require.NotNil(t, cmd, "expected a command")
	return cmd()
}

func TestShell_mOpensModuleMenu(t *testing.T) {
	m := editorShell(t)
	selectModule(t, m)

	press(m, "m")

	top := m.stack.top()
	require.Equal(t, viewAction, top.id.kind, "the menu replaces the main pane")
	require.Equal(t, "modules", top.id.key)
	require.Contains(t, top.content, "create module")
	require.Contains(t, top.content, "move module")
	require.Contains(t, top.content, "delete module")
	require.Equal(t, focusMain, m.focus, "the menu takes focus to receive keys")
}

func TestShell_mIgnoresNonModuleSelection(t *testing.T) {
	m := editorShell(t)
	m.walkTo(m.roots[0].item) // a group node carries no module context
	before := m.stack.top()

	press(m, "m")

	require.Equal(t, before.id, m.stack.top().id, "no menu opens without a module selection")
}

func TestManage_createScaffoldsModuleAndReloads(t *testing.T) {
	m := editorShell(t)
	selectModule(t, m)

	press(m, "m")     // menu
	press(m, "enter") // create is the first entry
	typeInto(t, keyRouter{m}, "notes")
	press(m, "enter") // confirm gate

	gate := m.stack.top().content
	require.Contains(t, gate, "notes", "the gate names the module")

	_, ycmd := step(m, keyPress("y"))
	msg := runCmd(t, ycmd)
	fin, ok := msg.(writeFinishedMsg)
	require.True(t, ok, "the run reports through writeFinishedMsg")
	require.NoError(t, fin.err)

	scaffold := filepath.Join(m.opts.ProfilePath, "modules", "notes", "module.toml")
	require.FileExists(t, scaffold, "the scaffold lands in the chosen layer")

	_, reload := m.Update(msg)
	require.NotNil(t, reload, "a successful op reloads the modules read")
	require.IsType(t, runCmd(t, reload), modulesLoadedMsg{})
	require.Contains(t, m.stack.top().content, "notes", "the report names the module")
}

func TestManage_createRefusesDuplicate(t *testing.T) {
	m := editorShell(t)
	selectModule(t, m)

	press(m, "m")
	press(m, "enter")
	typeInto(t, keyRouter{m}, "shell") // already in the base layer
	press(m, "enter")

	_, ycmd := step(m, keyPress("y"))
	m.Update(runCmd(t, ycmd))

	require.Contains(t, m.stack.top().content, "already has module", "the typed refusal renders")
	require.FileExists(t, filepath.Join(m.opts.ProfilePath, "modules", "shell", "module.toml"))
}

func TestManage_moveMovesWholesale(t *testing.T) {
	m := editorShell(t)
	selectModuleNamed(t, m, "shell")

	press(m, "m")
	press(m, "down")  // move module
	press(m, "enter") // form: target layer choice
	press(m, "right") // users/cri
	press(m, "enter") // confirm gate

	_, ycmd := step(m, keyPress("y"))
	msg := runCmd(t, ycmd)
	require.NoError(t, msg.(writeFinishedMsg).err)

	root := m.opts.ProfilePath
	require.NoDirExists(t, filepath.Join(root, "modules", "shell"))
	require.FileExists(t, filepath.Join(root, "users", "cri", "modules", "shell", "module.toml"))
}

func TestManage_moveRefusesCollision(t *testing.T) {
	m := editorShell(t)
	root := m.opts.ProfilePath
	dst := filepath.Join(root, "users", "cri", "modules", "shell")
	require.NoError(t, os.MkdirAll(dst, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "module.toml"),
		[]byte("id = \"shell\"\napp = \"shell\"\n"), 0o644))

	selectModuleNamed(t, m, "shell")
	press(m, "m")
	press(m, "down")
	press(m, "enter")
	press(m, "right") // users/cri — occupied
	press(m, "enter")

	_, ycmd := step(m, keyPress("y"))
	m.Update(runCmd(t, ycmd))

	require.Contains(t, m.stack.top().content, "already has module", "the typed refusal renders")
	require.FileExists(t, filepath.Join(root, "modules", "shell", "module.toml"),
		"the source stays put on refusal")
}

func TestManage_deletePreviewsOrphansThenConfirms(t *testing.T) {
	m := editorShell(t)
	selectModuleNamed(t, m, "shell")
	stray := filepath.Join(m.opts.ProfilePath, "modules", "shell", "stray.txt")
	require.NoError(t, os.WriteFile(stray, []byte("x"), 0o644))

	press(m, "m")
	press(m, "down")
	press(m, "down")  // delete module
	press(m, "enter") // the preview IS the gate

	preview := m.stack.top().content
	require.Contains(t, preview, "stray.txt", "the orphan preview lists the unreferenced file")
	require.FileExists(t, filepath.Join(m.opts.ProfilePath, "modules", "shell", "module.toml"),
		"nothing is deleted at preview time")

	_, ycmd := step(m, keyPress("y"))
	msg := runCmd(t, ycmd)
	require.NoError(t, msg.(writeFinishedMsg).err)
	require.NoDirExists(t, filepath.Join(m.opts.ProfilePath, "modules", "shell"))
}

func TestManage_escStepsBackThroughMenuThenOut(t *testing.T) {
	m := editorShell(t)
	selectModule(t, m)

	press(m, "m")
	press(m, "enter") // create form
	press(m, "esc")   // back to the menu, not out
	require.Equal(t, "modules", m.stack.top().id.key)
	require.Contains(t, m.stack.top().content, "move module", "the menu is back")

	press(m, "esc") // the bare menu pops
	require.NotEqual(t, viewAction, m.stack.top().id.kind, "esc pops the menu")
}

func TestManage_reservedGlobalsStayShell(t *testing.T) {
	m := editorShell(t)
	selectModule(t, m)
	press(m, "m")
	before := m.focus

	press(m, "tab")
	require.NotEqual(t, before, m.focus, "tab stays a shell global over the menu")
	press(m, "tab")
	require.Equal(t, before, m.focus)

	press(m, "?")
	require.True(t, m.help.ShowAll, "? stays a shell global over the menu")
}

// The esc discipline is shared: the writes dialogs never had a way out —
// their own hint says "esc back" but the shell never routed esc to a pop.
func TestDialog_escPopsWritesDialog(t *testing.T) {
	fake := newFakeWrites()
	area := &fakeReads{read: editorFixture(t)}
	m := New(area, Options{
		ProfilePath: area.read.Profile.Root,
		WritesFor:   func(*facts.Facts) Writes { return fake },
	})
	if cmd := m.Init(); cmd != nil {
		cmd()
	}
	m.setSize(120, 40)
	m.Update(modulesLoadedMsg{read: area.read})
	selectModule(t, m)
	underneath := m.stack.top().id

	m.walkTo(m.actionItem("onboard"))
	m.syncSelection()
	m.focus = focusMain
	require.Equal(t, "onboard", m.stack.top().id.key)

	typeInto(t, keyRouter{m}, "myapp") // row 0: app
	press(m, "down")
	typeInto(t, keyRouter{m}, "/tmp/a") // row 1: paths
	press(m, "enter")                   // confirm gate
	require.Contains(t, m.stack.top().content, "run onboard?", "at the gate")

	press(m, "esc") // consumes the gate, stays
	require.Equal(t, "onboard", m.stack.top().id.key, "esc at the gate clears it, no pop")

	press(m, "esc") // pops back to the view beneath
	require.Equal(t, underneath, m.stack.top().id, "esc leaves the dialog")
}
