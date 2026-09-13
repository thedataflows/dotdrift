package tui

import (
	"fmt"
	"io"
	"testing"

	"charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The Profile dialogs (T-tui-writes): onboard, restore, and generate run
// their writes through the Writes interface only. The tests drive the
// dialog models directly, then one routing test proves the shell opens
// them and hands keys over.

// fakeWrites records every write the dialogs attempt; it is the only
// writer the TUI can reach. Its methods write a short report through the
// caller's Out, the way the real service reports to the CLI.
type fakeWrites struct {
	onboardCalls []service.OnboardOpts
	onboardErr   error

	planCalls    []service.RestoreOpts
	plan         []service.RestorePlanItem
	planErr      error
	restoreCalls []service.RestoreOpts
	restoreErr   error

	selCalls  []generate.Selection
	selOut    generate.Selection
	selErr    error
	genCalls  []generate.Input
	genSels   []generate.Selection
	genRoots  []string
	genReport string
	genErr    error
}

func newFakeWrites() *fakeWrites {
	return &fakeWrites{
		selOut:    generate.Selection{Layer: "host", Hostname: "filled", ModuleID: "mounts"},
		genReport: "generated modules/mounts",
	}
}

func (f *fakeWrites) Onboard(opts service.OnboardOpts) error {
	f.onboardCalls = append(f.onboardCalls, opts)
	if opts.Out != nil {
		if opts.DryRun {
			fmt.Fprint(opts.Out, "would onboard (dry run)")
		} else {
			fmt.Fprint(opts.Out, "onboarded")
		}
	}
	return f.onboardErr
}

func (f *fakeWrites) RestorePlan(opts service.RestoreOpts) ([]service.RestorePlanItem, error) {
	f.planCalls = append(f.planCalls, opts)
	return f.plan, f.planErr
}

func (f *fakeWrites) Restore(opts service.RestoreOpts) error {
	f.restoreCalls = append(f.restoreCalls, opts)
	if opts.Out != nil {
		fmt.Fprint(opts.Out, "restored")
	}
	return f.restoreErr
}

func (f *fakeWrites) GenerateSelection(sel generate.Selection) (generate.Selection, error) {
	f.selCalls = append(f.selCalls, sel)
	return f.selOut, f.selErr
}

func (f *fakeWrites) WriteGenerate(root string, sel generate.Selection, input generate.Input, out io.Writer) error {
	f.genCalls = append(f.genCalls, input)
	f.genSels = append(f.genSels, sel)
	f.genRoots = append(f.genRoots, root)
	fmt.Fprint(out, f.genReport)
	return f.genErr
}

// dialogKey is the part of a dialog the tests type at.
type dialogKey interface {
	HandleKey(string) tea.Cmd
}

// typeInto types a string into the dialog's focused row.
func typeInto(t *testing.T, d dialogKey, s string) {
	t.Helper()
	for _, r := range s {
		d.HandleKey(string(r))
	}
}

// confirmRun answers the confirm gate, executes the returned command,
// and feeds the finishing message back into the dialog.
func confirmRun(t *testing.T, d dialog, name string) {
	t.Helper()
	cmd := d.HandleKey("y")
	require.NotNil(t, cmd, "confirming %s must return the run command", name)
	msg := cmd()
	fm, ok := msg.(writeFinishedMsg)
	require.True(t, ok, "%s run must finish with a writeFinishedMsg, got %T", name, msg)
	require.Equal(t, name, fm.key)
	d.applyFinished(fm)
}

// The onboard dialog gates its run behind a confirm, carries the form and
// the dry-run choice onto the service call, and shows the report.
func TestOnboardDialog_confirmsAndDryRun(t *testing.T) {
	fake := newFakeWrites()
	d := newOnboardDialog(fake, "/profile")

	// Form: app, paths, layer choice moved to host, dry-run to yes.
	typeInto(t, d, "myapp")
	d.HandleKey("down")
	typeInto(t, d, "~/.config/app /etc/app.conf")
	d.HandleKey("down")
	d.HandleKey("right") // base -> host
	for range 6 {
		d.HandleKey("down")
	}
	d.HandleKey("right")

	// Enter opens the confirm gate; n backs out without running.
	d.HandleKey("enter")
	require.Contains(t, d.View(newTheme(true)), "y/n", "the confirm gate shows")
	d.HandleKey("n")
	require.Empty(t, fake.onboardCalls, "a refused confirm runs nothing")

	// Confirming runs the write with everything the form holds.
	d.HandleKey("enter")
	confirmRun(t, d, "onboard")
	require.Len(t, fake.onboardCalls, 1)
	o := fake.onboardCalls[0]
	require.Equal(t, "myapp", o.App)
	require.Equal(t, []string{"~/.config/app", "/etc/app.conf"}, o.Paths)
	require.True(t, o.HostSet, "the host layer choice maps onto the overlay flag")
	require.True(t, o.DryRun, "the dry-run choice flows onto the call")
	require.Contains(t, d.View(newTheme(true)), "would onboard", "the report renders")
}

// The restore dialog resolves targets to generations, pins an older one,
// restores through the service, and skips a target that needs elevation.
func TestRestoreDialog_generationPicking(t *testing.T) {
	fake := newFakeWrites()
	target := "/home/cri/.config/app/config.toml"
	sys := "/etc/stiff.conf"
	fake.plan = []service.RestorePlanItem{
		{Target: target, Label: "modules/app/backups/g2", Gen: "g2"},
		{Target: sys, Label: "modules/app/backups/g1", Gen: "g1", Elevated: true},
	}
	index := map[string]map[string][]service.RestoreHit{
		target: {
			"/profile/modules/app": {
				{ModuleDir: "/profile/modules/app", Gen: "g2", Path: "/b/g2"},
				{ModuleDir: "/profile/modules/app", Gen: "g1", Path: "/b/g1"},
			},
		},
	}
	d := newRestoreDialog(fake, "/profile", func() (map[string]map[string][]service.RestoreHit, error) {
		return index, nil
	})

	typeInto(t, d, target+" "+sys)
	cmd := d.HandleKey("enter") // resolve
	require.NotNil(t, cmd, "resolving returns the plan command")
	msg := cmd()
	pm, ok := msg.(restorePlanMsg)
	require.True(t, ok, "resolve must finish with a restorePlanMsg, got %T", msg)
	require.Equal(t, fake.plan, pm.plan)
	d.applyPlan(pm)

	// The plan landed: both rows show, the elevated one names what it needs.
	view := d.View(newTheme(true))
	require.Contains(t, view, target)
	require.Contains(t, view, "terminal", "an elevated target names the handover it needs")

	// Generation picking: the newest generation is the default (no Gen,
	// the service picks it); left selects the older one.
	d.HandleKey("left")
	d.HandleKey("enter")
	run := d.HandleKey("y")
	require.NotNil(t, run, "confirming restore returns the run command")
	fm, ok := run().(writeFinishedMsg)
	require.True(t, ok, "the restore run finishes with a writeFinishedMsg")
	d.applyFinished(fm)
	require.Len(t, fake.restoreCalls, 1, "only the user-writable target restores")
	o := fake.restoreCalls[0]
	require.Equal(t, []string{target}, o.Targets)
	require.Equal(t, "g1", o.Gen, "the picked generation pins the restore")
	require.NotNil(t, o.Handover, "the handover seam rides the call")
	require.Contains(t, d.View(newTheme(true)), "restored", "the report renders")
}

// The generate dialog picks the layer, assembles the shared generate
// input, and writes through the service — the same assembly path as the
// CLI (contract 15).
func TestGenerateDialog_layerChoice(t *testing.T) {
	fake := newFakeWrites()
	d := newGenerateDialog(fake, "/profile")

	// Layer: base -> host (the row under the kind row).
	d.HandleKey("down")
	d.HandleKey("right")
	// Mount fields: name, source, destination, type.
	d.HandleKey("down")
	typeInto(t, d, "data")
	d.HandleKey("down")
	typeInto(t, d, "UUID=abc")
	d.HandleKey("down")
	typeInto(t, d, "/mnt/data")
	d.HandleKey("down")
	typeInto(t, d, "vfat")

	d.HandleKey("enter")
	confirmRun(t, d, "generate")
	require.Len(t, fake.selCalls, 1)
	require.Equal(t, "host", fake.selCalls[0].Layer, "the layer choice flows onto the selection")
	require.Len(t, fake.genCalls, 1, "the module is written through the service")
	require.NotNil(t, fake.genCalls[0].Mounts, "the mounts builder assembled the input")
	require.Contains(t, fake.genCalls[0].Mounts, "data")
	require.Equal(t, "/profile", fake.genRoots[0])
	require.Contains(t, d.View(newTheme(true)), fake.genReport, "the summary renders")
}

// The dialogs open from the tree's action nodes, the stub text is gone,
// and typed keys reach the focused dialog.
func TestDialogs_writeOnlyThroughService(t *testing.T) {
	fake := newFakeWrites()
	area := &fakeReads{read: editorFixture(t)}
	m := New(area, Options{
		ProfilePath: area.read.Profile.Root,
		WritesFor:   func(*facts.Facts) Writes { return fake },
	})
	cmd := m.Init()
	if cmd != nil {
		cmd()
	}
	m.setSize(120, 40)
	m.Update(modulesLoadedMsg{read: area.read})

	item := m.actionItem("onboard")
	require.NotNil(t, item, "the tree carries an onboard action")
	m.walkTo(item)
	m.syncSelection()
	top := m.stack.top()
	require.Equal(t, viewAction, top.id.kind)
	require.NotContains(t, top.content, "disabled until then", "the stub is gone once the dialog opens")

	// Keys route to the dialog while it is on top with main focus.
	m.focus = focusMain
	m.handleKey(keyPress("down"))
	typeInto(t, keyRouter{m}, "myapp")
	require.Contains(t, m.stack.top().content, "myapp", "typed keys reach the dialog")
}

// actionItem finds the tree's action node for an action (zero value when
// the tree carries none).
func (m *Shell) actionItem(action string) treeItem {
	for _, g := range m.roots {
		for _, k := range g.kids {
			if k.item.kind == kindAction && k.item.action == action {
				return k.item
			}
		}
	}
	return treeItem{}
}

// keyRouter types into whatever dialog is on top of the shell.
type keyRouter struct{ m *Shell }

func (r keyRouter) HandleKey(s string) tea.Cmd {
	_, cmd := r.m.handleKey(keyPress(s))
	return cmd
}
