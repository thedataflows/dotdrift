package tui

// The manage dialog's domain flows (0072), tested directly against the
// model — the M14 shell that used to carry these is gone (T-tui-cleanup);
// the compositor drives the same dialog as a modal (modals_test.go).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/service"
)

// manageFixture builds a profile root with one base module and returns
// the dialog over the real config area.
func manageFixture(t *testing.T) (*manageDialog, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))
	dir := filepath.Join(root, "modules", "demo")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte("id = \"demo\"\napp = \"demo\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotfilerc"), []byte("# managed\n"), 0o644))
	area := service.NewConfigArea(root, service.ConfigDeps{Facts: &facts.Facts{Hostname: "myhost", Username: "cri"}})
	return newManageDialog(area, root, &facts.Facts{Hostname: "myhost", Username: "cri"}, manageSel{moduleID: "demo", dir: dir}), root
}

func TestManage_menuEntriesForModule(t *testing.T) {
	d, _ := manageFixture(t)
	require.Equal(t,
		[]string{"create module", "move module", "delete module"}, d.entries(),
		"override lives on its own key (O) now, not in the menu")
}

// T-0082-override, simplified: O replaces the manage-dialog flow — one
// key creates the user-layer overlay, the nav reload lands the workspace
// on the new file. The seed is comments only, so there is nothing to
// confirm; delete module undoes it.
func TestOverride_oneKeyCreatesAndLands(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml": "id = \"demo\"\napp = \"demo\"\n",
	})
	c, cmd := cstep(c, keyPress("O"))
	require.Contains(t, c.message, "created overlay of demo in users/cri",
		"the footer names what happened")
	require.FileExists(t, filepath.Join(c.root, "users", "cri", "modules", "demo", "module.toml"))
	raw, err := os.ReadFile(filepath.Join(c.root, "users", "cri", "modules", "demo", "module.toml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "# overlay of demo", "the seed is comments, not a base copy")

	c = wsSettle(t, c, cmd)
	require.Equal(t, "user", c.ws.tabs[c.ws.active].layer, "the workspace lands on the new overlay")
	require.Equal(t, "user", c.nav.selected().layer, "the nav cursor follows onto the child row")
	require.Contains(t, ansiRe.ReplaceAllString(c.View().Content, ""),
		"hooks, mounts, smb merge", "the overlay tab states the merge rule")
}

func TestOverride_refusals(t *testing.T) {
	t.Run("no selection", func(t *testing.T) {
		_, c := wsShell(t, nil)
		c = cpress(c, "O")
		require.Contains(t, c.message, "select a module first")
		require.Empty(t, c.nav.modules, "the shell is empty, nothing was created")
	})
	t.Run("already user", func(t *testing.T) {
		_, c := wsShell(t, map[string]string{
			"modules/demo/module.toml":           "id = \"demo\"\napp = \"demo\"\n",
			"users/cri/modules/demo/module.toml": "id = \"demo\"\napp = \"demo\"\n",
		})
		c = wsPress(t, c, "l") // expand demo
		c = wsPress(t, c, "j")
		c = wsPress(t, c, "j") // the user child row
		require.Equal(t, "user", c.nav.selected().layer)
		c = cpress(c, "O")
		require.Contains(t, c.message, "already lives in your user layer")
	})
	t.Run("target taken", func(t *testing.T) {
		_, c := wsShell(t, map[string]string{
			"modules/demo/module.toml":           "id = \"demo\"\napp = \"demo\"\n",
			"users/cri/modules/demo/module.toml": "id = \"demo\"\napp = \"demo\"\n",
		})
		c = cpress(c, "O")
		require.Contains(t, c.message, "already has module", "the op's refusal surfaces")
	})
}

func TestManage_createScaffoldsModule(t *testing.T) {
	d, root := manageFixture(t)
	d.HandleKey("enter") // create module
	require.Len(t, d.rows, 2, "module field + layer choice")
	for _, r := range "newapp" {
		d.HandleKey(string(r))
	}
	d.HandleKey("enter") // confirm gate
	require.True(t, d.confirm)
	cmd := d.HandleKey("y")
	require.NotNil(t, cmd, "the create op runs async")
	msg := cmd()
	require.NotNil(t, msg)
	d.applyFinished(msg.(writeFinishedMsg))
	require.Empty(t, d.err)
	require.FileExists(t, filepath.Join(root, "modules", "newapp", "module.toml"))
	require.Contains(t, d.report, "created")
}

func TestManage_moveCollisionRefuses(t *testing.T) {
	d, root := manageFixture(t)
	// A host overlay with the same module exists → moving there collides.
	hostDir := filepath.Join(root, "hosts", "myhost", "modules", "demo")
	require.NoError(t, os.MkdirAll(hostDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hostDir, "module.toml"), []byte("id = \"demo\"\n"), 0o644))

	d.HandleKey("down") // move module
	d.HandleKey("enter")
	require.Len(t, d.rows, 1, "the move target choice")
	d.HandleKey("enter") // confirm gate
	cmd := d.HandleKey("y")
	require.NotNil(t, cmd)
	d.applyFinished(cmd().(writeFinishedMsg))
	require.Error(t, d.err, "the typed collision refusal surfaces")
}

func TestManage_deletePreviewThenDelete(t *testing.T) {
	d, root := manageFixture(t)
	d.HandleKey("down")
	d.HandleKey("down") // delete module
	d.HandleKey("enter")
	require.True(t, d.confirm, "delete sits at its gate")
	require.Contains(t, d.orphans, "dotfilerc", "the orphan preview lists the module's files")
	cmd := d.HandleKey("y")
	require.NotNil(t, cmd)
	d.applyFinished(cmd().(writeFinishedMsg))
	require.NoDirExists(t, filepath.Join(root, "modules", "demo"))
	require.Contains(t, d.report, "deleted")
}

func TestManage_cursorRowBar(t *testing.T) {
	// 0075 T-tui-selection: the manage menu and its form rows use the
	// bar cursor treatment.
	d, _ := manageFixture(t)
	th := newTheme(true)
	require.Contains(t, ansiRe.ReplaceAllString(d.View(th), ""), "│ create module",
		"the menu cursor row renders the bar")
	d.HandleKey("enter") // create mode: the module field row is focused
	require.Contains(t, ansiRe.ReplaceAllString(d.View(th), ""), "│ module",
		"the focused form row renders the bar")
}

// 0089: the create/move confirm prompt renders only while the gate is
// armed — enter shows it, n hides it; before the gate the form has no
// y/n tail.
func TestManage_createGatePromptOnlyWhenArmed(t *testing.T) {
	d, _ := manageFixture(t)
	d.HandleKey("enter") // create module
	for _, r := range "newapp" {
		d.HandleKey(string(r))
	}
	th := newTheme(true)
	require.NotContains(t, d.View(th), "y/n", "no confirm tail before the gate arms")
	d.HandleKey("enter")
	require.Contains(t, d.View(th), `create module "newapp"`, "enter arms the gate and shows its prompt")
	require.Contains(t, d.View(th), "y runs", "the armed footer names the gate's keys")
	d.HandleKey("n")
	require.NotContains(t, d.View(th), "y/n", "n disarms the gate and hides the prompt")
}

// 0089: delete's n steps back to the menu — outside the gate the
// manageDelete mode has no key handling, so disarming in place would
// strand the dialog.
func TestManage_deleteRefusedGateReturnsToMenu(t *testing.T) {
	d, _ := manageFixture(t)
	d.HandleKey("down")
	d.HandleKey("down") // delete module
	d.HandleKey("enter")
	require.True(t, d.confirm, "delete sits at its gate")
	d.HandleKey("n")
	require.Equal(t, manageMenu, d.mode, "n backs out of delete to the menu")
}
