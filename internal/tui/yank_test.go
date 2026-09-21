package tui

// 0093: nav yank — space/y toggle a module's yanked mark; p/P scope
// plan and apply to the yanked set (empty set = all modules, today's
// default). The set is session-only, pruned on reload, and rides the
// existing ApplyOpts.Modules filter.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/service"
)

// yankShell builds a loaded compositor over two plain modules, nav
// focused, cursor on the first (demo).
func yankShell(t *testing.T) *Compositor {
	t.Helper()
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":  "id = \"demo\"\n",
		"modules/other/module.toml": "id = \"other\"\n",
	})
	return c
}

func TestYank_toggleMarksRow(t *testing.T) {
	c := yankShell(t)
	require.Equal(t, "demo", c.nav.selected().moduleID)

	c = cpress(c, "space")
	require.True(t, c.nav.yanked["demo"], "space yanks the selected module")
	require.Contains(t, ansiRe.ReplaceAllString(c.nav.view(24, 10, c.th, nil), ""), "demo ✓",
		"a yanked module row carries the mark")

	c = cpress(c, "y")
	require.False(t, c.nav.yanked["demo"], "y on a yanked module unyanks it")
	require.NotContains(t, ansiRe.ReplaceAllString(c.nav.view(24, 10, c.th, nil), ""), "demo ✓")

	// A layer child row yanks its module, not the layer.
	c = cpress(c, "y")
	require.True(t, c.nav.yanked["demo"], "y yanks too")
}

func TestYank_planScopesToYanked(t *testing.T) {
	c := yankShell(t)
	fl := &fakeLauncher{}
	c.applyFor = func(*facts.Facts) ApplyLauncher { return fl }

	// No yanks: the plan passes no module filter (all modules).
	c = wsPress(t, c, "p")
	require.IsType(t, &planModel{}, c.modals[0])
	require.Nil(t, fl.previewOpts[0].Modules, "no yanks = the whole profile")
	c = cpress(c, "esc")

	// Yank demo: the plan is scoped and the modal names the scope.
	c = cpress(c, "space")
	c = wsPress(t, c, "p")
	pm := c.modals[0].(*planModel)
	require.Equal(t, []string{"demo"}, fl.previewOpts[1].Modules, "p previews the yanked set")
	require.Contains(t, ansiRe.ReplaceAllString(pm.view(60, 20), ""), "demo",
		"the plan modal names the yanked scope")
}

func TestYank_applyScopesToYanked(t *testing.T) {
	c := yankShell(t)
	fl := &fakeLauncher{run: closedRun(&service.SessionResult{Outcome: service.OutcomeCompleted})}
	c.applyFor = func(*facts.Facts) ApplyLauncher { return fl }
	c.pumpNoBlock = true

	// Yank demo and other: apply carries exactly those, in stable order.
	c = cpress(c, "space") // demo
	c = wsPress(t, c, "j") // other (a nav move schedules the layer read)
	c = cpress(c, "y")     // other
	c = wsPress(t, c, "P")
	require.NotNil(t, c.apply, "P started the run")
	require.Len(t, fl.startedOpts, 1)
	require.Equal(t, []string{"demo", "other"}, fl.startedOpts[0].Modules,
		"P applies the yanked set, snapshot at press")
}

func TestYank_reloadPrunesGoneModules(t *testing.T) {
	var dir string
	var c *Compositor
	subs, c2 := wsShell(t, map[string]string{
		"modules/demo/module.toml":  "id = \"demo\"\n",
		"modules/other/module.toml": "id = \"other\"\n",
	})
	for d := range subs {
		dir = d
	}
	c = c2

	c = cpress(c, "space") // yank demo
	require.True(t, c.nav.yanked["demo"])

	// demo disappears from the profile; the reload must drop its yank —
	// a stale id would surface as "unknown module(s)" from LimitTo.
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "modules", "demo")))
	c, cmd := cstep(c, mustMsg(c.reloadNav()))
	c = wsSettle(t, c, cmd)
	require.False(t, c.nav.yanked["demo"], "a reloaded-away module loses its yank")
	require.False(t, c.nav.loadErr != "" && c.nav.pending, "the reload landed")
}
