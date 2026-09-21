package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// T-tui-nav: the M15 nav pane on the compositor — module rows with
// overlay layers as expandable children, dirty markers per file,
// expansion memory, placeholder rows while reads are in flight. Goldens
// pin the composited final frame; message-driven tests pin behavior.

// navShell builds a compositor over the real reads area and the named
// checked-in profile, fully loaded (the navLoadedMsg already applied).
func navShell(t *testing.T, profileRel string) (map[string]string, *Compositor) {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "profiles", profileRel))
	require.NoError(t, err)

	area := service.NewReadsArea(service.ReadsDeps{
		Detect: func() (*facts.Facts, error) {
			return &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Kernel: "6.12.1-arch1-1", Distro: "arch", GPU: " amd", Backend: "pacman"}, nil
		},
	})
	c := NewCompositor(area, dir, nil)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	load := mustMsg(c.Init())
	require.NotNil(t, load, "Init schedules the modules read")
	c, _ = cstep(c, load)
	require.False(t, c.nav.pending, "the read has landed")
	return map[string]string{dir: "$PROFILE"}, c
}

func TestNav_flatModuleList(t *testing.T) {
	subs, c := navShell(t, "resolve")
	requireGolden(t, "nav-flat.golden", c.View().Content, subs)
}

func TestNav_expandedModuleShowsOverlayChildren(t *testing.T) {
	subs, c := navShell(t, "resolve")
	c = cpress(c, "l")
	requireGolden(t, "nav-expanded.golden", c.View().Content, subs)
}

func TestNav_dirtyMarkersPerFile(t *testing.T) {
	subs, c := navShell(t, "resolve")
	c = cpress(c, "l")
	user := c.nav.modules[0].layers[1]
	require.Equal(t, "user", user.layer)
	c.store = map[string]*wsDraft{user.dir: {changes: 1}} // a staged draft marks the file
	requireGolden(t, "nav-dirty.golden", c.View().Content, subs)
}

func TestNav_emptyProfile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "modules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))
	area := service.NewReadsArea(service.ReadsDeps{
		Detect: func() (*facts.Facts, error) {
			return &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}, nil
		},
	})
	c := NewCompositor(area, dir, nil)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 64, Height: 24})
	c, _ = cstep(c, mustMsg(c.Init()))
	requireGolden(t, "nav-empty.golden", c.View().Content, map[string]string{dir: "$PROFILE"})
}

func TestNav_failedModuleGreyedOut(t *testing.T) {
	// tui-superuser: vault's only layer is users/root — skipped for cri
	// (issue 0029). The row stays visible, greyed, naming the reason.
	subs, c := navShell(t, "tui-superuser")
	requireGolden(t, "nav-skipped.golden", c.View().Content, subs)
}

func TestNav_loadPlaceholderRows(t *testing.T) {
	area := service.NewReadsArea(service.ReadsDeps{
		Detect: func() (*facts.Facts, error) { return &facts.Facts{}, nil },
	})
	c := NewCompositor(area, ".", nil)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 64, Height: 24})
	requireGolden(t, "nav-loading.golden", c.View().Content, nil)
}

func TestNav_loadErrorNamed(t *testing.T) {
	area := service.NewReadsArea(service.ReadsDeps{
		Detect: func() (*facts.Facts, error) { return &facts.Facts{}, nil },
		LoadProfile: func(string, *facts.Facts) (*profile.Profile, error) {
			return nil, errors.New("boom")
		},
	})
	c := NewCompositor(area, ".", nil)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	c, _ = cstep(c, mustMsg(c.Init()))
	require.Contains(t, c.View().Content, "boom", "a failed read names the error in the nav")
	require.False(t, c.nav.pending)
}

func TestNav_expandCollapseWithHL(t *testing.T) {
	_, c := navShell(t, "resolve")
	require.Len(t, c.nav.rows(), 1, "the module starts collapsed")

	c = cpress(c, "l")
	require.Len(t, c.nav.rows(), 4, "l expands: module + base/user/host children")

	c = cpress(c, "h")
	require.Len(t, c.nav.rows(), 1, "h collapses")
}

func TestNav_expandCollapseWithArrows(t *testing.T) {
	_, c := navShell(t, "resolve")
	c = cpress(c, "right")
	require.Len(t, c.nav.rows(), 4)
	c = cpress(c, "left")
	require.Len(t, c.nav.rows(), 1)
}

func TestNav_childHumpsToParent(t *testing.T) {
	_, c := navShell(t, "resolve")
	c = cpress(c, "l")
	c = cpress(c, "j") // onto the base child
	require.Equal(t, "base", c.nav.rows()[c.nav.cursor].layer)
	c = cpress(c, "h")
	require.Equal(t, 0, c.nav.cursor, "h on a child returns to the module row")
	require.Len(t, c.nav.rows(), 4, "the module stays expanded")
}

func TestNav_expansionRememberedPerSession(t *testing.T) {
	_, c := navShell(t, "resolve")
	c = cpress(c, "l")
	c, _ = cstep(c, mustMsg(c.Init())) // a reload lands
	require.Len(t, c.nav.rows(), 4, "reload keeps the expansion")
}

func TestNav_selectionSyncsWorkspace(t *testing.T) {
	_, c := navShell(t, "resolve")
	require.Contains(t, c.ws.placeholder, "shell", "the workspace opens on the first module")

	c = cpress(c, "l")
	c = cpress(c, "j")
	require.Contains(t, c.ws.placeholder, "shell")
	require.Contains(t, c.ws.placeholder, "base", "the workspace follows the selected layer")

	c = cpress(c, "j")
	require.Contains(t, c.ws.placeholder, "user", "the workspace never disagrees with the nav")
}

func TestNav_cursorSurvivesReload(t *testing.T) {
	_, c := navShell(t, "resolve")
	c = cpress(c, "l")
	c = cpress(c, "j")
	c = cpress(c, "j") // the user child
	want := c.nav.rows()[c.nav.cursor]

	c, _ = cstep(c, mustMsg(c.Init()))
	got := c.nav.rows()[c.nav.cursor]
	require.Equal(t, want.moduleID, got.moduleID)
	require.Equal(t, want.layer, got.layer, "the reload keeps the selected row selected")
}

func TestNav_cursorClampsWhenSelectionVanishes(t *testing.T) {
	_, c := navShell(t, "tui-superuser")
	last := len(c.nav.rows()) - 1
	for i := 0; i < last; i++ {
		c = cpress(c, "j")
	}
	// Reload against a profile where the module is gone.
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "profiles", "resolve"))
	require.NoError(t, err)
	c.root = dir
	c, _ = cstep(c, mustMsg(c.Init()))
	require.Less(t, c.nav.cursor, len(c.nav.rows()), "the cursor stays in range")
}

func TestNav_pageKeys(t *testing.T) { // 0075 T-tui-page: pgup/pgdown move a visible page, home/end jump to
	// the ends — the nav is longer than one screen at 100x30.
	files := map[string]string{}
	for i := 0; i < 30; i++ {
		files[fmt.Sprintf("modules/m%02d/module.toml", i)] = fmt.Sprintf("id = \"m%02d\"\n", i)
	}
	_, c := wsShell(t, files)
	require.Len(t, c.nav.rows(), 30, "the profile builds a long nav")
	require.Equal(t, 0, c.nav.cursor)

	page := c.nav.bodyH - 1 // the group-title line takes one body row
	require.Greater(t, page, 5, "the test window steps more than a screenful")

	c = cpress(c, "end")
	require.Equal(t, 29, c.nav.cursor, "end jumps to the last row")
	c = cpress(c, "home")
	require.Equal(t, 0, c.nav.cursor, "home jumps to the first row")
	c = cpress(c, "pgdown")
	require.Equal(t, page, c.nav.cursor, "pgdown moves a page")
	c = cpress(c, "pgup")
	require.Equal(t, 0, c.nav.cursor, "pgup moves back a page")
	c = cpress(c, "end")
	c = cpress(c, "pgdown")
	require.Equal(t, 29, c.nav.cursor, "pgdown clamps at the last row")
	c = cpress(c, "home")
	c = cpress(c, "pgup")
	require.Equal(t, 0, c.nav.cursor, "pgup clamps at the first row")
}

func TestNav_cursorRowBar(t *testing.T) {
	// 0075 T-tui-selection: the nav's cursor row renders the bar and its
	// label column aligns with the plain rows (marker column, then text).
	_, c := navShell(t, "resolve")
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "│ ▸ shell", "the cursor row renders bar + marker + label")

	c = cpress(c, "l")
	c = cpress(c, "j") // onto the base child
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "│     base", "the cursor child row keeps the bar and the child indent")
	require.Contains(t, frame, "  ▾ shell", "plain rows keep the lead and marker")
}
