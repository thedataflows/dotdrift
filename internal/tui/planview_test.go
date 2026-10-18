package tui

// T-tui-views slice 3 (issue 0109): the Plan view — the merged plan's
// sections with counts on the left, the selected section's entries
// exactly as RenderPlanReport renders them on the right, d toggling
// the copy-mode diff list (ReadsArea.Diff data) in the detail pane.
// Yank-scoped: the title names the scope; the 0108 stale-seq landing
// rule holds (a mark set after the read started survives its landing).

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
)

// planFixtureRead is the fixed Plan model the view tests render: every
// section kind with entries.
func planFixtureRead() *service.PlanRead {
	return &service.PlanRead{
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Profile: &profile.Profile{Root: "/home/cri/profiles/main"},
		Plan: &resolve.Plan{
			Packages: resolve.PackagesStep{Install: []string{"firefox", "ripgrep"}, Remove: []string{"nano"}},
			Tools:    resolve.ToolsStep{Versions: map[string]string{"go": "1.25.0", "node": "22.11.0"}},
			Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
				{Target: "/home/cri/.zshrc", Source: "modules/shell/zshrc", Mode: "symlink", Module: "shell", Layer: "base"},
			}},
			Hooks: resolve.HooksStep{
				Pre:  []profile.HookCommand{{Command: "echo pre"}},
				Post: []profile.HookCommand{{Command: "echo post", Optional: true}},
			},
			Systemd: resolve.SystemdStep{Units: []resolve.SystemdUnitEntry{
				{Name: "backup", Kind: "timer", Module: "backup", Layer: "base"},
			}},
			Mounts: resolve.MountsStep{Entries: []resolve.MountEntry{
				{Name: "data", Spec: profile.MountSpec{Type: "bind", Source: "/srv/data", Destination: "/home/cri/data"}, Module: "base", Layer: "base", Scope: "user"},
			}},
			Smb: resolve.SmbStep{Modules: []resolve.SmbModuleSpec{
				{Module: "share", Spec: profile.SmbSpec{Users: []string{"cri"}}},
			}},
		},
	}
}

// planShell builds a compositor at 100x30 over the stub seam with the
// given plan read.
func planShell(read *service.PlanRead) (*stubReads, *Compositor) {
	stub := &stubReads{profile: pinnedProfileRead(), plan: read}
	c := NewCompositor(stub, "/home/cri/profiles/main", nil)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	return stub, c
}

// loadPlanView enters the Plan view and settles the scheduled read.
func loadPlanView(t *testing.T, c *Compositor) *Compositor {
	t.Helper()
	var cmd tea.Cmd
	c, cmd = cstep(c, keyPress("3"))
	require.NotNil(t, cmd, "entering the Plan view schedules the read")
	c, _ = cstep(c, mustMsg(cmd))
	return c
}

func TestPlanView_rendersMergedSections(t *testing.T) {
	_, c := planShell(planFixtureRead())
	c = loadPlanView(t, c)
	// A wide frame: no row truncates, so the wording pins are exact.
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 140, Height: 30})

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	// Left: one row per non-empty section, in the report's order, with
	// the merged counts (packages names install and remove, hooks name
	// pre and post).
	require.Contains(t, frame, "packages: 2 install, 1 remove")
	require.Contains(t, frame, "tools: 2")
	require.Contains(t, frame, "dotfiles: 1")
	require.Contains(t, frame, "hooks: 1 pre, 1 post")
	require.Contains(t, frame, "systemd: 1")
	require.Contains(t, frame, "mounts: 1")
	require.Contains(t, frame, "smb: 1")

	// Right: the selected (first) section's entries exactly as
	// RenderPlanReport renders them — the install and remove blocks.
	require.Contains(t, frame, "  - firefox")
	require.Contains(t, frame, "  - ripgrep")
	require.Contains(t, frame, "  - nano")
	require.NotContains(t, frame, "go: 1.25.0", "other sections stay off the detail pane")

	// Moving the cursor swaps the detail to the selected section.
	c = cpress(c, "j")
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "go: 1.25.0")
	require.Contains(t, frame, "node: 22.11.0")
	require.NotContains(t, frame, "  - firefox", "the detail follows the cursor")
}

// 0140: the detail pane's header strip already names the section, so
// the report block's own "<section>:" header line must not repeat in
// the body. Sub-block headers (packages' "remove:") stay.
func TestPlanSectionLines_dropsDuplicatedHeader(t *testing.T) {
	c := &Compositor{}
	c.planv.report = "packages:\n  - firefox\nremove:\n  - nano\ntools:\n  go: 1.25.0"

	require.Equal(t, []string{"  - firefox", "remove:", "  - nano"},
		c.planSectionLines("packages", 80),
		"the packages header drops, the remove sub-header stays")
	require.Equal(t, []string{"  go: 1.25.0"},
		c.planSectionLines("tools", 80),
		"a single-block section drops its header line")
}

func TestPlanView_diffToggle(t *testing.T) {
	stub, c := planShell(planFixtureRead())
	stub.diffs = []service.DiffEntry{{
		Module:        "shell",
		Target:        "/home/cri/.zshrc",
		Source:        "/home/cri/profiles/main/modules/shell/zshrc",
		TargetContent: "a\n",
		SourceContent: "b\n",
	}}
	c = loadPlanView(t, c)

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.NotContains(t, frame, "[shell] /home/cri/.zshrc", "the diff list is hidden by default")

	c = cpress(c, "d")
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "[shell] /home/cri/.zshrc", "d shows the copy-mode diff list")
	require.NotContains(t, frame, "  - firefox", "the diff list replaces the section entries")

	c = cpress(c, "d")
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.NotContains(t, frame, "[shell] /home/cri/.zshrc")
	require.Contains(t, frame, "  - firefox", "d again restores the section entries")
}

func TestPlanView_yankScopeNamed(t *testing.T) {
	_, c := planShell(planFixtureRead())
	c.nav = navModel{modules: []navModule{{id: "shell"}, {id: "apps"}}, yanked: map[string]bool{"shell": true}}
	c = loadPlanView(t, c)

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "Plan [shell]", "the title names the yank scope the read ran under")
	require.Contains(t, frame, "scope: shell")
}

func TestPlanView_midReadStaleMarkSurvivesLanding(t *testing.T) {
	// The 0108 landing rule on the Plan view: a stale mark set while
	// the read runs (a yank change, a write, a finished apply) survives
	// the landing — the landed data is the old scope's.
	_, c := planShell(planFixtureRead())
	c.nav = navModel{modules: []navModule{{id: "shell"}, {id: "apps"}}}

	var cmd tea.Cmd
	c, cmd = cstep(c, keyPress("3"))
	require.NotNil(t, cmd, "entering the Plan view schedules the read")

	// Mid-read: the user yanks a module on the Modules view.
	c = cpress(c, "1")
	c = cpress(c, "y")
	require.True(t, c.stale[viewPlan], "the yank marks the Plan view stale")

	c, _ = cstep(c, mustMsg(cmd)) // the old-scope read lands
	require.True(t, c.stale[viewPlan],
		"a mark set after the read started survives its landing (old-scope data is not current)")
	require.Contains(t, ansiRe.ReplaceAllString(c.View().Content, ""), "Plan (11) ●")

	// The other leg: no mid-read mark, a landed refresh clears the mark.
	c, _ = cstep(c, keyPress("3")) // back to the Plan view (already loaded: no new read)
	c, cmd = cstep(c, keyPress("r"))
	require.NotNil(t, cmd, "r re-resolves the plan")
	c, _ = cstep(c, mustMsg(cmd))
	require.False(t, c.stale[viewPlan], "an uninterrupted refresh clears the stale mark")
}

// 0111: the detail pane scrolls — a long section never clips silently.
// tab focuses the detail pane, the movement keys scroll it, a clipped
// side carries an overflow mark, and moving the section cursor resets
// the scroll.
func TestPlanView_detailScrolls(t *testing.T) {
	read := planFixtureRead()
	pkgs := make([]string, 40)
	for i := range pkgs {
		pkgs[i] = fmt.Sprintf("pkg-%02d", i)
	}
	read.Plan.Packages.Install = pkgs
	_, c := planShell(read)
	c = loadPlanView(t, c)

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "pkg-00", "the section's head renders")
	require.NotContains(t, frame, "pkg-39", "the tail starts out of the window")
	require.Contains(t, frame, "more below", "a clipped detail marks the hidden tail")

	c = cpress(c, "right") // focus the detail pane
	c = cpress(c, "end")
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "pkg-39", "end reaches the section's tail")
	require.Contains(t, frame, "more above", "a scrolled detail marks the hidden head")
	require.NotContains(t, frame, "more below", "the tail is the end")

	// Moving the section cursor resets the detail scroll.
	c = cpress(c, "left") // back to the section list
	c = cpress(c, "j")    // tools
	c = cpress(c, "k")    // packages again
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "pkg-00", "a cursor move resets the detail scroll to the head")
	require.NotContains(t, frame, "pkg-39")
	require.NotContains(t, frame, "more above")
}

func TestPlanView_golden(t *testing.T) {
	stub, c := planShell(planFixtureRead())
	stub.diffs = []service.DiffEntry{{
		Module:        "shell",
		Target:        "/home/cri/.zshrc",
		Source:        "/home/cri/profiles/main/modules/shell/zshrc",
		TargetContent: "a\n",
		SourceContent: "b\n",
	}}
	c = loadPlanView(t, c)
	subs := map[string]string{"/home/cri/profiles/main": "$PROFILE"}
	requireGolden(t, "views-plan.golden", c.View().Content, subs)

	c, _ = cstep(c, tea.WindowSizeMsg{Width: 64, Height: 24})
	requireGolden(t, "views-plan-small.golden", c.View().Content, subs)
}
