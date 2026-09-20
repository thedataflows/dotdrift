package tui

// T-tui-palette: the `/` fuzzy palette — the fast path to a known
// target. Indexes modules+layers, contextually valid actions, and the
// current module's fields in fixed section order; sahilm/fuzzy ranks
// within a section, in-session recents break ties. Selecting a module
// jumps nav and workspace in sync; esc restores the exact prior state.
// A navigator, not a command line.

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// paletteShell builds a loaded compositor over three modules (demo has
// user+host overlays) and opens the palette.
func paletteShell(t *testing.T) (map[string]string, *Compositor) {
	t.Helper()
	subs, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":              editFixture,
		"users/cri/modules/demo/module.toml":    "id = \"demo\"\ndescription = \"user layer\"\n",
		"hosts/myhost/modules/demo/module.toml": "id = \"demo\"\n",
		"modules/plain/module.toml":             "id = \"plain\"\napp = \"plain\"\n",
		"modules/other/module.toml":             "id = \"other\"\napp = \"other\"\n",
	})
	c.applyFor = func(*facts.Facts) ApplyLauncher { return &fakeLauncher{} }
	c = cpress(c, "/")
	require.Len(t, c.modals, 1, "/ opens the palette")
	require.IsType(t, &paletteModel{}, c.modals[0])
	return subs, c
}

func pal(t *testing.T, c *Compositor) *paletteModel {
	t.Helper()
	return c.modals[len(c.modals)-1].(*paletteModel)
}

func TestPalette_emptyQueryShowsRecents(t *testing.T) {
	subs, c := paletteShell(t)
	requireGolden(t, "palette-empty.golden", c.View().Content, subs)
	p := pal(t, c)
	labels := p.visibleLabels()
	require.Contains(t, labels, "demo · user cri", "layer children are separate entries")
	require.Contains(t, labels, "plain")
}

func TestPalette_midQueryRanked(t *testing.T) {
	subs, c := paletteShell(t)
	c = typeText(c, "dem")
	p := pal(t, c)
	labels := p.visibleLabels()
	require.Equal(t, "demo", labels[0], "exact-prefix module ranks first")
	// Sections keep fixed order: any action/field match trails modules.
	subs2 := subs
	requireGolden(t, "palette-midquery.golden", c.View().Content, subs2)
}

func TestPalette_rowAnatomy(t *testing.T) {
	subs, c := paletteShell(t)
	c = typeText(c, "demo · host")
	requireGolden(t, "palette-row-anatomy.golden", c.View().Content, subs)
}

func TestPalette_noResults(t *testing.T) {
	subs, c := paletteShell(t)
	c = typeText(c, "zzzznothing")
	frame := c.View().Content
	require.Contains(t, frame, "no matches")
	require.Contains(t, frame, "ctrl+n", "the dimmed hint names the create escape")
	requireGolden(t, "palette-no-results.golden", frame, subs)
}

func TestPalette_selectModuleJumpsNavAndWorkspaceInSync(t *testing.T) {
	_, c := paletteShell(t)
	c = typeText(c, "plain")
	c = wsPress(t, c, "enter")
	require.Empty(t, c.modals, "enter chooses and closes")
	require.Equal(t, "plain", c.nav.selected().moduleID, "nav jumped")
	require.Contains(t, c.ws.placeholderOrBody(), "plain", "workspace follows")
	require.Equal(t, focusWork, c.focus, "focus moves to the workspace")
}

func TestPalette_layerEntryJumpsToThatLayer(t *testing.T) {
	_, c := paletteShell(t)
	c = typeText(c, "demo · user")
	c = wsPress(t, c, "enter")
	sel := c.nav.selected()
	require.Equal(t, "user", sel.layer, "the layer child is the nav selection")
	require.Equal(t, "user", c.ws.tabs[c.ws.active].layer, "the workspace tab agrees")
	require.Contains(t, c.ws.placeholderOrBody(), "user layer")
}

func TestPalette_actionsContextual(t *testing.T) {
	_, c := paletteShell(t)
	p := pal(t, c)
	actions := p.actionLabels()
	require.Contains(t, actions, "apply")
	require.Contains(t, actions, "new module")
	require.NotContains(t, actions, "save draft", "nothing dirty → no save action")

	// Dirty a draft: save/discard appear.
	c = cpress(c, "esc")
	c = cpress(c, "tab")
	c = cpress(c, "j")
	c = cpress(c, "enter")
	c = typeText(c, "-x")
	c = wsPress(t, c, "enter")
	c = cpress(c, "/")
	actions = pal(t, c).actionLabels()
	require.Contains(t, actions, "save draft")
	require.Contains(t, actions, "discard draft")

	// Selecting save draft runs the save (same as ctrl+s). Actions rank
	// under a query (the empty query shows recents + modules only).
	c = typeText(c, "save d")
	p = pal(t, c)
	require.Equal(t, "save draft", p.rows[p.sel].label)
	c = applySettle(t, c, cpressCmd(c, "enter"))
	require.Empty(t, c.store, "the palette action saved the draft")
}

func TestPalette_selectFieldDeepLinks(t *testing.T) {
	_, c := paletteShell(t)
	c = typeText(c, "packages")
	p := pal(t, c)
	for i, l := range p.visibleLabels() {
		if l == "packages" && p.rows[i].section == sectionFields {
			p.sel = i
		}
	}
	c = cpress(c, "enter")
	require.Equal(t, focusWork, c.focus)
	require.True(t, c.ws.atSection("packages"), "the cursor deep-links to the section")
	require.False(t, c.ws.rows[c.ws.cursor].header, "on the first entry, not the header")
}

func TestPalette_closeRestoresFocusExactly(t *testing.T) {
	_, c := paletteShell(t)
	c = cpress(c, "esc") // close the opener
	// Disturb state: move the nav cursor.
	c = cpress(c, "j")
	wantNav := c.nav.cursor
	wantFocus := c.focus
	c = cpress(c, "/")
	c = typeText(c, "demo")
	c = cpress(c, "esc")
	require.Equal(t, wantNav, c.nav.cursor, "nav cursor restored")
	require.Equal(t, wantFocus, c.focus, "focus restored")
	require.Equal(t, 0, c.ws.offset, "workspace scroll untouched")
}

func TestPalette_ctrlNFromNoResultsPrefillsCreate(t *testing.T) {
	_, c := paletteShell(t)
	c = typeText(c, "zzzznothing")
	c = cpress(c, "ctrl+n")
	// The manage dialog opens in create mode with the query prefilled.
	require.Len(t, c.modals, 1)
	dm, ok := c.modals[0].(*dialogModal)
	require.True(t, ok, "ctrl+n opens module creation")
	md := dm.d.(*manageDialog)
	require.Contains(t, md.View(c.th), "zzzznothing", "the query prefills the app field")
}

func TestPalette_recentsInSessionOnly(t *testing.T) {
	_, c := paletteShell(t)
	c = typeText(c, "plain")
	c = wsPress(t, c, "enter")
	require.Equal(t, []string{"mod:plain/"}, c.paletteRecents[:1], "the jump is remembered")

	// Reopen: the recent leads the empty query.
	c = cpress(c, "/")
	p := pal(t, c)
	require.Equal(t, "plain", p.rows[0].label, "recents lead the empty query")

	// A second jump pushes it down; cap is 8.
	c = typeText(c, "other")
	c = wsPress(t, c, "enter")
	require.Equal(t, "mod:other/", c.paletteRecents[0])
	require.LessOrEqual(t, len(c.paletteRecents), 8)
}

func TestPalette_dirtyDraftNeverWarnsOnJump(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":  editFixture,
		"modules/plain/module.toml": "id = \"plain\"\napp = \"plain\"\n",
	})
	c = cpress(c, "tab")
	c = cpress(c, "j") // id row → app row
	c = cpress(c, "enter")
	c = typeText(c, "-next")
	c = wsPress(t, c, "enter")
	dirty := c.ws.activeDir()

	c = cpress(c, "/")
	c = typeText(c, "plain")
	c = wsPress(t, c, "enter")
	require.Empty(t, c.modals, "jumping away from a dirty draft asks nothing")
	require.NotNil(t, c.wsDraftFor(dirty), "the draft waits where it was left")
	require.Equal(t, "plain", c.nav.selected().moduleID)
}

func TestPalette_cursorRowBar(t *testing.T) {
	// 0075 T-tui-selection: the palette's selected row renders the bar.
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n"})
	c = cpress(c, "/")
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "│ ◆", "the palette's selected row renders the bar")
}
