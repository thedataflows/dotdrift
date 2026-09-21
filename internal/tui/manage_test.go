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
		[]string{"create module", "move module", "delete module", "override module"}, d.entries(),
		"the fixture module sits in base: host and user overlays can override it")
}

// T-0082-override: a user-layer module has nothing above it, so the
// override entry hides.
func TestManage_noOverrideEntryForUserModule(t *testing.T) {
	d, root := manageFixture(t)
	d.sel.dir = filepath.Join(root, "users", "cri", "modules", "demo")
	require.Equal(t, []string{"create module", "move module", "delete module"}, d.entries())
	require.Empty(t, d.overrideTargets())
}

func TestManage_overrideSeedsUserOverlay(t *testing.T) {
	d, root := manageFixture(t)
	d.HandleKey("down")
	d.HandleKey("down")
	d.HandleKey("down") // override module
	d.HandleKey("enter")
	require.Len(t, d.rows, 1, "the override target choice")
	require.Equal(t, []string{"hosts/myhost", "users/cri"}, d.rows[0].choice.opts)
	d.HandleKey("right") // users/cri
	require.Contains(t, d.confirmText(), "starts empty",
		"the gate says what an empty overlay means")
	d.HandleKey("enter") // confirm gate
	cmd := d.HandleKey("y")
	require.NotNil(t, cmd)
	d.applyFinished(cmd().(writeFinishedMsg))
	require.Empty(t, d.err)
	raw, err := os.ReadFile(filepath.Join(root, "users", "cri", "modules", "demo", "module.toml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "override", "the overlay is the comment seed, not a base copy")
	require.Contains(t, d.report, "overlay")
}

func TestManage_overrideCollisionRefuses(t *testing.T) {
	d, root := manageFixture(t)
	dir := filepath.Join(root, "users", "cri", "modules", "demo")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte("id = \"demo\"\n"), 0o644))

	d.HandleKey("down")
	d.HandleKey("down")
	d.HandleKey("down") // override module
	d.HandleKey("enter")
	d.HandleKey("right") // users/cri — taken
	d.HandleKey("enter") // confirm gate
	cmd := d.HandleKey("y")
	require.NotNil(t, cmd)
	d.applyFinished(cmd().(writeFinishedMsg))
	require.Error(t, d.err, "the typed collision refusal surfaces")
}

func TestManage_createScaffoldsModule(t *testing.T) {
	d, root := manageFixture(t)
	d.HandleKey("enter") // create module
	require.Len(t, d.rows, 2, "app field + layer choice")
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

func TestManage_backDiscipline(t *testing.T) {
	d, _ := manageFixture(t)
	d.HandleKey("enter") // into create
	require.True(t, d.back(), "a form steps back")
	require.False(t, d.confirm)
	d.HandleKey("enter")
	d.HandleKey("enter") // confirm gate
	require.True(t, d.back(), "a confirm gate consumes esc and stays")
	require.False(t, d.confirm)
	d.toMenu()
	require.False(t, d.back(), "the bare menu lets the shell pop")
}

func TestManage_cursorRowBar(t *testing.T) {
	// 0075 T-tui-selection: the manage menu and its form rows use the
	// bar cursor treatment.
	d, _ := manageFixture(t)
	th := newTheme(true)
	require.Contains(t, ansiRe.ReplaceAllString(d.View(th), ""), "│ create module",
		"the menu cursor row renders the bar")
	d.HandleKey("enter") // create mode: the app field row is focused
	require.Contains(t, ansiRe.ReplaceAllString(d.View(th), ""), "│ app",
		"the focused form row renders the bar")
}
