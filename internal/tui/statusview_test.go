package tui

// T-tui-views slice 2c (issue 0108): the Status view — findings rows
// (drift/unknown/unauthorized only, all-OK section row, counts), the
// detail pane with the copy-mode dotfile diff, the sudo elevation gate
// (a system-scope dotfile/mount/smb entry in the scoped plan validates
// sudo through the elevation modal BEFORE the read starts — no prompt
// ever reaches the altscreen), tolerant loads (a broken module surfaces,
// never reads as "no drift"), and the probing state that keeps the shell
// responsive while the read runs as a tea.Cmd.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
)

// --- stubReads gains the slice 2c seam methods ---

func (s *stubReads) Plan(string, []string, *facts.Facts) (*service.PlanRead, error) {
	if s.plan == nil && s.planErr == nil {
		// A clean plan: no elevation gate. The Profile is set because
		// the Plan view renders the report off it (0109).
		return &service.PlanRead{Profile: &profile.Profile{}, Plan: &resolve.Plan{}}, nil
	}
	return s.plan, s.planErr
}

func (s *stubReads) Status(_ context.Context, opts service.StatusOpts) (*service.StatusRead, error) {
	s.statusCalls++
	s.lastOpts = opts
	return s.status, s.statusErr
}

func (s *stubReads) Diff(*resolve.Plan, string) ([]service.DiffEntry, error) {
	return s.diffs, s.diffErr
}

// statusFixtureRead is the fixed Status model the view tests render: one
// drift, one unknown, one unauthorized, one ok — every row kind.
func statusFixtureRead() *service.StatusRead {
	return &service.StatusRead{
		ProfilePath: "/home/cri/profiles/main",
		ProfileRoot: "/home/cri/profiles/main",
		Profile:     &profile.Profile{},
		Plan:        &resolve.Plan{},
		Findings: []drift.Finding{
			{Section: "packages", Item: "firefox", Status: drift.Drift, Detail: "not installed", Module: "apps"},
			{Section: "packages", Item: "ripgrep", Status: drift.Unknown, Detail: "probe failed", Module: "shell"},
			{Section: "tools", Item: "go", Status: drift.OK},
			{Section: "dotfiles", Item: "/etc/hosts", Status: drift.Unauthorized, Detail: "read failed: permission denied", Module: "base"},
		},
	}
}

// statusShell builds a compositor at 100x30 over the stub seam: the plan
// check finds no system-scope entries (no elevation gate) and Status
// returns the given read.
func statusShell(read *service.StatusRead) (*stubReads, *Compositor) {
	stub := &stubReads{
		profile: pinnedProfileRead(),
		plan:    &service.PlanRead{Plan: &resolve.Plan{}},
		status:  read,
	}
	c := NewCompositor(stub, "/home/cri/profiles/main", nil)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	return stub, c
}

// loadStatusView enters the Status view and settles the plan check and
// the status read.
func loadStatusView(t *testing.T, c *Compositor) *Compositor {
	t.Helper()
	var cmd tea.Cmd
	c, cmd = cstep(c, keyPress("2"))
	require.NotNil(t, cmd, "entering the Status view schedules the plan check")
	c, cmd = cstep(c, mustMsg(cmd))
	require.NotNil(t, cmd, "a clean plan check starts the status read")
	c, _ = cstep(c, mustMsg(cmd))
	return c
}

func TestStatusView_listsFindings(t *testing.T) {
	stub, c := statusShell(statusFixtureRead())
	c = loadStatusView(t, c)
	require.Equal(t, 1, stub.statusCalls)
	// A wide frame: no row truncates, so the wording pins are exact. The
	// pinned-size layouts live in the goldens.
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 140, Height: 30})

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "packages: 1 drift, 1 unknown", "the section header names the counts")
	require.Contains(t, frame, "apps: firefox - not installed")
	require.Contains(t, frame, "shell: ripgrep - probe failed (?)", "unknown findings keep the (?) mark")
	require.Contains(t, frame, "base: /etc/hosts - read failed: permission denied (unauthorized)")
	require.Contains(t, frame, "drift: 1 item, 1 unknown, 1 unauthorized", "the summary names each count")
	require.NotContains(t, frame, "go - ", "ok findings never render as rows")

	// The detail pane shows the selected (first) finding.
	require.Contains(t, frame, "module:  apps")
	require.Contains(t, frame, "item:    firefox")
	require.Contains(t, frame, "state:   drift")
	require.Contains(t, frame, "detail:  not installed")
}

func TestStatusView_allOKSectionRow(t *testing.T) {
	read := statusFixtureRead()
	read.Findings = []drift.Finding{
		{Section: "dotfiles", Item: "/home/cri/.zshrc", Status: drift.OK, Module: "shell"},
		{Section: "dotfiles", Item: "/home/cri/.bashrc", Status: drift.OK, Module: "shell"},
	}
	_, c := statusShell(read)
	c = loadStatusView(t, c)

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "dotfiles:")
	require.Contains(t, frame, "ok: all 2 checks passed", "an all-OK section shows the one ok row")
	require.NotContains(t, frame, ".zshrc - ", "ok findings never render as rows")
	require.Contains(t, frame, "no drift")
}

func TestStatusView_sudoGate(t *testing.T) {
	// Review Focus 1: a system-scope entry in the scoped plan gates the
	// read behind the elevation modal; declined or failed validation runs
	// the read with Elevated:false — no sudo prompt ever reaches the
	// altscreen body.
	sysPlan := &service.PlanRead{Plan: &resolve.Plan{
		Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
			{Target: "/etc/foo.conf", Source: "modules/base/foo.conf", Mode: "copy", Module: "base", Scope: profile.ScopeSystem},
		}},
	}}

	// Declined (esc): the gate opens before the read starts; the read
	// then runs unelevated.
	stub, c := statusShell(statusFixtureRead())
	stub.plan = sysPlan
	c.sudoCheck = func([]byte) error { return nil }
	var cmd tea.Cmd
	c, cmd = cstep(c, keyPress("2"))
	c, _ = cstep(c, mustMsg(cmd)) // the plan check landed
	require.NotEmpty(t, c.modals, "a system-scope entry gates the read on sudo")
	require.IsType(t, &elevationModel{}, c.modals[len(c.modals)-1])
	require.Equal(t, 0, stub.statusCalls, "the read never starts before the gate answers")
	require.Contains(t, ansiRe.ReplaceAllString(c.View().Content, ""), "sudo elevation required")

	c, cmd = cstep(c, keyPress("esc"))
	require.Empty(t, c.modals, "esc closes the gate")
	require.NotNil(t, cmd, "a declined gate still runs the read")
	c, _ = cstep(c, mustMsg(cmd))
	require.Equal(t, 1, stub.statusCalls)
	require.False(t, stub.lastOpts.Elevated, "declined elevation reads unelevated (0108)")

	// Failed validation: one failure keeps the gate open (no read); the
	// third failure aborts and the read runs unelevated.
	stub, c = statusShell(statusFixtureRead())
	stub.plan = sysPlan
	c.sudoCheck = func([]byte) error { return errors.New("bad password") }
	c, cmd = cstep(c, keyPress("2"))
	c, _ = cstep(c, mustMsg(cmd))
	require.IsType(t, &elevationModel{}, c.modals[len(c.modals)-1])
	for range 2 {
		c = typeText(c, "pw")
		c = cpress(c, "enter")
		require.Equal(t, 0, stub.statusCalls, "a failed validation holds the gate")
	}
	require.Contains(t, ansiRe.ReplaceAllString(c.View().Content, ""), "authentication failed")
	c = typeText(c, "pw")
	c, cmd = cstep(c, keyPress("enter")) // the third failure aborts
	require.Empty(t, c.modals)
	require.NotNil(t, cmd, "an aborted gate still runs the read")
	c, _ = cstep(c, mustMsg(cmd))
	require.False(t, stub.lastOpts.Elevated, "failed validation reads unelevated (0108)")

	// Validated sudo: the read runs elevated.
	stub, c = statusShell(statusFixtureRead())
	stub.plan = sysPlan
	c.sudoCheck = func([]byte) error { return nil }
	c, cmd = cstep(c, keyPress("2"))
	c, _ = cstep(c, mustMsg(cmd))
	c = typeText(c, "pw")
	c, cmd = cstep(c, keyPress("enter"))
	require.Empty(t, c.modals)
	require.NotNil(t, cmd, "a validated gate runs the read")
	c, _ = cstep(c, mustMsg(cmd))
	require.True(t, stub.lastOpts.Elevated, "validated sudo reads elevated")
}

func TestStatusView_tolerantBrokenModule(t *testing.T) {
	// Review Focus 2: loads stay tolerant — a broken module surfaces in
	// the view and the summary never claims "no drift".
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "modules", "good"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "modules", "good", "module.toml"), []byte("id = \"good\"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "modules", "broken"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "modules", "broken", "module.toml"), []byte("id = [broken\n"), 0o644))

	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}
	area := service.NewReadsArea(service.ReadsDeps{
		Detect:      func() (*facts.Facts, error) { return f, nil },
		LoadProfile: profile.LoadTolerant,
	})
	c := NewCompositor(area, dir, nil)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	c = loadStatusView(t, c)

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "load problems", "the tolerant load's broken module surfaces")
	require.Contains(t, frame, "broken: invalid module.toml")
	require.NotContains(t, frame, "no drift", "a broken module never reads as clean")
}

func TestStatusView_probePendingKeepsShell(t *testing.T) {
	// Review Focus 4: the read is a tea.Cmd — while it runs (a blocked
	// probe), the view shows the probing state and the shell keeps
	// handling keys. The probe cmd is never executed here: it stands in
	// for a probe that has not returned.
	stub, c := statusShell(statusFixtureRead())
	var cmd tea.Cmd
	c, cmd = cstep(c, keyPress("2"))
	c, cmd = cstep(c, mustMsg(cmd))
	require.NotNil(t, cmd, "the plan check schedules the status read")
	require.True(t, c.status.pending, "the read is in flight")
	require.Contains(t, ansiRe.ReplaceAllString(c.View().Content, ""), "probing")

	// Other keys are handled while the probe runs.
	c = cpress(c, "1")
	require.Equal(t, viewModules, c.view)
	c = cpress(c, "3")
	require.Equal(t, viewPlan, c.view)
	c = cpress(c, "?")
	require.IsType(t, &helpModel{}, c.modals[len(c.modals)-1], "help opens during a probe")
	c = cpress(c, "esc")
	require.Empty(t, c.modals)

	// Back on Status: the pending read is not rescheduled, and r during
	// a probe is a no-op (builder's call, named in reloadStatus).
	c, cmd = cstep(c, keyPress("2"))
	require.Nil(t, cmd, "re-entering a probing view schedules nothing")
	c, cmd = cstep(c, keyPress("r"))
	require.Nil(t, cmd, "a second r during a probe is a no-op")
	require.Equal(t, 0, stub.statusCalls, "the read runs only through the scheduled cmd")
}

func TestStatusView_sectionFilter(t *testing.T) {
	// The section filter (f, the one mechanism): cycle all → each
	// present section → all.
	_, c := statusShell(statusFixtureRead())
	c = loadStatusView(t, c)

	c = cpress(c, "f")
	require.Equal(t, "packages", c.status.filter)
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "ripgrep")
	require.NotContains(t, frame, "/etc/hosts", "a filter narrows the list to one section")

	c = cpress(c, "f")
	require.Equal(t, "tools", c.status.filter)
	c = cpress(c, "f")
	require.Equal(t, "dotfiles", c.status.filter)
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "/etc/hosts")
	require.NotContains(t, frame, "ripgrep")

	c = cpress(c, "f")
	require.Equal(t, "", c.status.filter, "the cycle wraps back to all sections")
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "ripgrep")
	require.Contains(t, frame, "/etc/hosts")

	// f is inert off the Status view.
	c = cpress(c, "1")
	c = cpress(c, "f")
	require.Equal(t, "", c.status.filter, "f does nothing on the Modules view")
}

func TestStatusView_midProbeStaleMarkSurvivesLanding(t *testing.T) {
	// Fix round 1: a stale mark set during the probe (a yank change, a
	// write, a finished apply) must survive the landing of the read that
	// started before it — the landed data is the OLD scope's.
	_, c := statusShell(statusFixtureRead())
	c.nav = navModel{modules: []navModule{{id: "shell"}, {id: "firefox"}}}

	var cmd tea.Cmd
	c, cmd = cstep(c, keyPress("2"))
	c, cmd = cstep(c, mustMsg(cmd)) // the plan check landed; the probe is in flight

	// Mid-probe: the user yanks a module on the Modules view.
	c = cpress(c, "1")
	c = cpress(c, "y")
	require.True(t, c.stale[viewStatus], "the yank marks the Status view stale")

	c, _ = cstep(c, mustMsg(cmd)) // the old-scope read lands
	require.True(t, c.stale[viewStatus],
		"a mark set after the read started survives its landing (old-scope data is not current)")
	require.Contains(t, ansiRe.ReplaceAllString(c.View().Content, ""), "Status (stale)")

	// The other leg: no mid-probe mark, a landed refresh clears the mark.
	_, c = statusShell(statusFixtureRead())
	c = loadStatusView(t, c)
	c.markStale(viewStatus)
	c, cmd = cstep(c, keyPress("r"))
	c, cmd = cstep(c, mustMsg(cmd))
	c, _ = cstep(c, mustMsg(cmd))
	require.False(t, c.stale[viewStatus], "an uninterrupted refresh clears the stale mark")
}

func TestStatusView_failedRefreshKeepsStaleList(t *testing.T) {
	// The anti-goal: a failed re-run keeps the old list only with the
	// stale mark, never silently.
	stub, c := statusShell(statusFixtureRead())
	c = loadStatusView(t, c)
	require.False(t, c.stale[viewStatus])

	stub.status, stub.statusErr = nil, errors.New("probe blew up")
	var cmd tea.Cmd
	c, cmd = cstep(c, keyPress("r"))
	c, cmd = cstep(c, mustMsg(cmd))
	c, _ = cstep(c, mustMsg(cmd))

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "firefox", "the old list survives a failed refresh")
	require.True(t, c.stale[viewStatus], "the surviving list carries the stale mark")
	require.Contains(t, frame, "Status (stale)")
	require.Equal(t, "probe blew up", c.message, "the failure lands in the footer")
}

func TestStatusView_yankMarksStale(t *testing.T) {
	// A yank change scopes the Status/Plan reads: their data is stale
	// from that moment (the design's settled rule).
	c := newTestCompositor()
	c = cpress(c, "y")
	require.True(t, c.stale[viewStatus], "a yank change marks the Status view stale")
	require.True(t, c.stale[viewPlan], "a yank change marks the Plan view stale")
}

func TestStatusView_enterJumpsToModule(t *testing.T) {
	// Slice 2d: enter on a finding opens the Modules view with the nav
	// cursor on the finding's first owning module (the palette jump
	// idiom: focus to the workspace, the workspace follows).
	read := statusFixtureRead()
	read.Findings = []drift.Finding{
		{Section: "dotfiles", Item: "/home/cri/.vimrc", Status: drift.Drift, Detail: "content differs", Module: "editors, shell"},
	}
	_, c := statusShell(read)
	c.nav = navModel{modules: []navModule{{id: "editors"}, {id: "shell"}}}
	c = loadStatusView(t, c)

	c = cpress(c, "enter")
	require.Equal(t, viewModules, c.view, "enter jumps to the Modules view")
	require.Equal(t, "editors", c.nav.selected().moduleID,
		"the cursor lands on the first owner of a comma-joined multi-owner finding")
	require.Equal(t, focusWork, c.focus, "the jump hands focus to the workspace")
	require.Equal(t, "editors", c.ws.moduleID, "the workspace follows the jump")

	// An orphan finding's owner is its layer-dir basename; the jump
	// resolves it to the owning module (module.toml's id can differ
	// from the dir name).
	orphanRead := statusFixtureRead()
	orphanRead.Findings = []drift.Finding{
		{Section: "orphans", Item: "stray.conf", Status: drift.Drift, Detail: "not referenced by any declaration", Module: "dotdir"},
	}
	_, c = statusShell(orphanRead)
	c.nav = navModel{modules: []navModule{{
		id:     "dots",
		layers: []navLayer{{layer: "base", dir: "/home/cri/profiles/main/modules/dotdir"}},
	}}}
	c = loadStatusView(t, c)
	c = cpress(c, "enter")
	require.Equal(t, viewModules, c.view)
	require.Equal(t, "dots", c.nav.selected().moduleID,
		"an orphan jumps to the module owning its layer dir")

	// The all-OK row owns no finding: enter is inert.
	okRead := statusFixtureRead()
	okRead.Findings = []drift.Finding{
		{Section: "tools", Item: "go", Status: drift.OK, Module: "shell"},
	}
	_, c = statusShell(okRead)
	c = loadStatusView(t, c)
	c = cpress(c, "enter")
	require.Equal(t, viewStatus, c.view, "enter on the all-OK row stays put")
}

func TestStatusView_applyScopesAllOwners(t *testing.T) {
	// Slice 2d: P on a finding runs the one apply flow with
	// ApplyOpts.Modules = every owner of the finding — the finding's
	// owners beat the yank set.
	read := statusFixtureRead()
	read.Findings = []drift.Finding{
		{Section: "dotfiles", Item: "/home/cri/.vimrc", Status: drift.Drift, Detail: "content differs", Module: "editors, shell"},
	}
	_, c := statusShell(read)
	c.nav = navModel{modules: []navModule{{id: "editors"}, {id: "shell"}, {id: "other"}}}
	c.nav.yanked = map[string]bool{"other": true}
	fl := &fakeLauncher{run: closedRun(&service.SessionResult{Outcome: service.OutcomeCompleted})}
	c.applyFor = func(*facts.Facts) ApplyLauncher { return fl }
	c.pumpNoBlock = true
	c = loadStatusView(t, c)

	c = wsPress(t, c, "P") // opens the options gate
	c = applyGateY(t, c)   // confirm: settles preview → gates → run
	require.Equal(t, []string{"editors", "shell"}, c.applyModules,
		"P snapshots the finding's owners at the press")
	require.Len(t, fl.startedOpts, 1)
	require.Equal(t, []string{"editors", "shell"}, fl.startedOpts[0].Modules,
		"the owners ride the gates into Start, beating the yank set")

	// The all-OK row owns no finding: P keeps the yank scope.
	okRead := statusFixtureRead()
	okRead.Findings = []drift.Finding{
		{Section: "tools", Item: "go", Status: drift.OK, Module: "shell"},
	}
	_, c = statusShell(okRead)
	c.nav = navModel{modules: []navModule{{id: "shell"}, {id: "other"}}}
	c.nav.yanked = map[string]bool{"other": true}
	fl = &fakeLauncher{run: closedRun(&service.SessionResult{Outcome: service.OutcomeCompleted})}
	c.applyFor = func(*facts.Facts) ApplyLauncher { return fl }
	c.pumpNoBlock = true
	c = loadStatusView(t, c)
	c = wsPress(t, c, "P")
	c = applyGateY(t, c)
	require.Equal(t, []string{"other"}, fl.startedOpts[0].Modules,
		"with no finding under the cursor the yank snapshot stands")
}

func TestStatusView_yankChangeMarksStale(t *testing.T) {
	// Review Focus 3: a yank change marks the Status and Plan views
	// stale from that moment — their reads ran under the old scope.
	_, c := statusShell(statusFixtureRead())
	c.nav = navModel{modules: []navModule{{id: "shell"}, {id: "apps"}}}
	c = loadStatusView(t, c)
	require.False(t, c.stale[viewStatus], "the landed read is fresh")

	c = cpress(c, "1") // the Modules view owns the yank keys
	c = cpress(c, "y")
	require.True(t, c.stale[viewStatus], "a yank change marks Status stale")
	require.True(t, c.stale[viewPlan], "a yank change marks Plan stale")
	require.Contains(t, ansiRe.ReplaceAllString(c.View().Content, ""), "Status (stale)",
		"the row title carries the mark")
}

func TestStatusView_applyFinishMarksStale(t *testing.T) {
	// A finished apply outlives the Status/Plan data: the views are
	// marked stale, never auto re-probed — r refreshes.
	stub, c := statusShell(statusFixtureRead())
	c = loadStatusView(t, c)
	require.False(t, c.stale[viewStatus])

	c.apply = &applyRunState{}
	c, _ = cstep(c, applyWaitedMsg{res: &service.SessionResult{Outcome: service.OutcomeCompleted}})
	require.True(t, c.stale[viewStatus], "a finished apply marks Status stale")
	require.True(t, c.stale[viewPlan], "a finished apply marks Plan stale")
	require.Equal(t, 1, stub.statusCalls, "the finish does not re-probe — r refreshes")
}

func TestStatusView_golden(t *testing.T) {
	stub, c := statusShell(statusFixtureRead())
	stub.diffs = []service.DiffEntry{{
		Module:        "base",
		Target:        "/etc/hosts",
		Source:        "/home/cri/profiles/main/modules/base/hosts",
		TargetContent: "127.0.0.1 localhost\n",
		SourceContent: "127.0.0.1 localhost\n127.0.1.1 myhost\n",
	}}
	c = loadStatusView(t, c)
	c = cpress(c, "end") // the dotfiles finding: the detail pane shows the live diff
	subs := map[string]string{"/home/cri/profiles/main": "$PROFILE"}
	requireGolden(t, "views-status.golden", c.View().Content, subs)

	c, _ = cstep(c, tea.WindowSizeMsg{Width: 64, Height: 24})
	requireGolden(t, "views-status-small.golden", c.View().Content, subs)
}
