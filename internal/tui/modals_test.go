package tui

// T-tui-modals: the modal family on the compositor. One confirm
// component for every destructive action (dirty quit, module removal
// with the orphan preview, destructive apply); the elevation modal —
// one prompt per elevation, reasons listed from the plan's privileged
// steps, three failures abort, cancel aborts before anything is
// touched, the password a zeroed []byte never drafted; the apply detail
// modal as an inspector over a running session (closing never cancels,
// ctrl+c inside confirms). No real sudo anywhere near these tests: the
// checker is an injected seam.

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/service"
)

// applySettle runs compositor cmds until the chain ends. The fake run's
// channel is pre-fed and closed, so every step is immediate.
func applySettle(t *testing.T, c *Compositor, cmd tea.Cmd) *Compositor {
	t.Helper()
	for i := 0; i < 30 && cmd != nil; i++ {
		msg := mustMsg(cmd)
		if msg == nil {
			return c
		}
		c, cmd = cstep(c, msg)
	}
	return c
}

// cpressCmd is cpress but keeps the cmd (for submissions that start ops).
func cpressCmd(c *Compositor, s string) tea.Cmd {
	_, cmd := cstep(c, keyPress(s))
	return cmd
}

// applyShell builds a compositor with a fake apply launcher and a sudo
// checker over the demo profile.
func applyShell(t *testing.T, l *fakeLauncher) (map[string]string, *Compositor) {
	t.Helper()
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\napp = \"demo-app\"\n"})
	c.applyFor = func(*facts.Facts) ApplyLauncher { return l }
	c.sudoCheck = func(pw []byte) error { return nil }
	c.pumpNoBlock = true
	return subs, c
}

// closedRun is a fake run whose (buffered) event channel carries the
// canned events and is already closed: the drain settles immediately.
func closedRun(result *service.SessionResult, events ...service.Event) *testRun {
	run := newTestRun()
	for _, ev := range events {
		run.events <- ev
	}
	close(run.events)
	run.result = result
	return run
}

// sudoPreview is a privileged step's classification (the domain's own
// wording names sudo).
var sudoPreview = service.StepPreview{
	Name:     "dotfiles-system",
	NeedsTTY: true,
	Reason:   "elevated system edits: edit targets are not user-writable (sudo)",
}

func TestElevation_onePromptCoversAllPrivilegedOps(t *testing.T) {
	l := &fakeLauncher{previews: []service.StepPreview{
		{Name: "packages"},
		sudoPreview,
		{Name: "system-files", NeedsTTY: true, Reason: "elevated: system dotfiles (sudo)"},
	}}
	_, c := applyShell(t, l)
	l.run = closedRun(&service.SessionResult{Outcome: service.OutcomeCompleted})

	c = wsPress(t, c, "P")
	require.Len(t, c.modals, 1, "one elevation modal for the whole plan")
	el, ok := c.modals[0].(*elevationModel)
	require.True(t, ok, "the modal is the elevation prompt")
	require.Len(t, el.reasons, 2, "all privileged steps list as reasons — no per-action whack-a-mole")

	c = typeText(c, "hunter2")
	applySettle(t, c, cpressCmd(c, "enter"))
	require.Equal(t, 1, l.started, "one elevation covers every privileged step")
}

func TestElevation_states(t *testing.T) {
	subs, c := applyShell(t, &fakeLauncher{previews: []service.StepPreview{sudoPreview}})
	// The checker is captured at push time; the failure is wired before P.
	c.sudoCheck = func([]byte) error { return errors.New("nope") }
	c = wsPress(t, c, "P")
	requireGolden(t, "elevation-initial.golden", c.View().Content, subs)

	c = typeText(c, "sekrit")
	requireGolden(t, "elevation-typing.golden", c.View().Content, subs)

	c = cpress(c, "enter")
	requireGolden(t, "elevation-failed.golden", c.View().Content, subs)
}

func TestElevation_cancelAbortsBeforeTouching(t *testing.T) {
	l := &fakeLauncher{previews: []service.StepPreview{sudoPreview}}
	_, c := applyShell(t, l)
	c = wsPress(t, c, "P")
	require.Len(t, c.modals, 1)

	c = cpress(c, "esc")
	require.Empty(t, c.modals, "esc pops the prompt")
	require.Equal(t, 0, l.started, "cancel aborts before anything is touched")
	require.Contains(t, c.message, "cancelled", "the footer reports the abort")
}

func TestElevation_threeFailuresAbort(t *testing.T) {
	l := &fakeLauncher{previews: []service.StepPreview{sudoPreview}}
	_, c := applyShell(t, l)
	c.sudoCheck = func([]byte) error { return errors.New("nope") }
	c = wsPress(t, c, "P")

	for i := 0; i < 3; i++ {
		c = typeText(c, "x")
		c = cpress(c, "enter")
	}
	require.Empty(t, c.modals, "the third failure aborts the elevation")
	require.Equal(t, 0, l.started, "nothing ran")
	require.Contains(t, c.message, "cancelled")
	require.True(t, c.msgErr)
}

func TestElevation_passwordNeverLoggedNorDrafted(t *testing.T) {
	l := &fakeLauncher{previews: []service.StepPreview{sudoPreview}}
	_, c := applyShell(t, l)
	l.run = closedRun(&service.SessionResult{Outcome: service.OutcomeCompleted})
	var got []byte
	c.sudoCheck = func(pw []byte) error { got = append(got, pw...); return nil }
	c = wsPress(t, c, "P")
	el := c.modals[0].(*elevationModel)

	c = typeText(c, "hunter2")
	c = applySettle(t, c, cpressCmd(c, "enter"))

	require.Equal(t, "hunter2", string(got), "the checker receives the password")
	for _, b := range el.input {
		require.Zero(t, b, "the model's buffer is zeroed after submission")
	}
	require.Empty(t, c.store, "a password never enters the draft ledger")
	require.NotContains(t, c.View().Content, "hunter2", "the password never renders")
}

func TestConfirm_yConfirmsEverythingElseCancels(t *testing.T) {
	for _, key := range []string{"n", "enter", "x", "j"} {
		answered := -1
		m := &confirmModel{title: "sure?", onAnswer: func(ok bool) {
			if ok {
				answered = 1
			} else {
				answered = 0
			}
		}}
		m.update(keyPress(key))
		require.Equal(t, 0, answered, "%q cancels — y alone confirms", key)
		require.True(t, m.finished())
	}
	// esc cancels through the compositor's pop rule (tested elsewhere).
}

func TestConfirm_dirtyQuit(t *testing.T) {
	subs, _, c := editShell(t)
	c = cpress(c, "enter") // edit the description field → draft exists
	c = typeText(c, "-next")
	c = wsPress(t, c, "enter")

	c = cpress(c, "q")
	require.Len(t, c.modals, 1, "q with a dirty draft asks")
	requireGolden(t, "confirm-dirty-quit.golden", c.View().Content, subs)

	c = cpress(c, "n")
	require.Empty(t, c.modals, "n stays")

	c = cpress(c, "q")
	_, cmd := cstep(c, keyPress("y"))
	require.NotNil(t, cmd, "y quits")
	require.IsType(t, tea.QuitMsg{}, mustMsg(cmd))

	// Clean shell: q quits without asking.
	_, clean := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n"})
	_, cmd = cstep(clean, keyPress("q"))
	require.NotNil(t, cmd)
	require.IsType(t, tea.QuitMsg{}, mustMsg(cmd))
}

func TestConfirm_removeModuleWithOrphanPreview(t *testing.T) {
	subs, c := wsShell(t, map[string]string{
		"modules/demo/module.toml": "id = \"demo\"\napp = \"demo-app\"\n",
		"modules/demo/dotfilerc":   "# the managed file\n",
	})

	// m on the module row opens the manage menu as a modal.
	c = cpress(c, "m")
	require.Len(t, c.modals, 1, "the manage dialog is a modal now")
	requireGolden(t, "manage-menu.golden", c.View().Content, subs)

	// Down to delete, enter: the confirm names the module and lists the
	// orphan preview before anything is deleted. (The M14 manage dialog's
	// menu vocabulary is up/down.)
	c = cpress(c, "down")
	c = cpress(c, "down")
	c = cpress(c, "enter")
	frame := c.View().Content
	require.Contains(t, frame, "demo", "the confirm names the module")
	require.Contains(t, frame, "dotfilerc", "the orphan preview lists the module's files")
	requireGolden(t, "confirm-remove-module.golden", frame, subs)

	c = applySettle(t, c, cpressCmd(c, "y")) // delete for real (async op)
	require.Contains(t, c.View().Content, "deleted", "the report surfaces")
}

// T-0082-override: a manage write re-runs the startup profile read so
// the nav tree shows the new directory without a restart.
func TestManageWrite_reloadsNav(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml": "id = \"demo\"\napp = \"demo-app\"\n",
	})

	// The manage dialog creates a second module.
	c = cpress(c, "m")
	c = cpress(c, "enter") // create module (menu cursor 0)
	c = typeText(c, "extra")
	c = cpress(c, "enter") // confirm gate
	c = applySettle(t, c, cpressCmd(c, "y"))
	require.Contains(t, c.View().Content, "created", "the op ran")

	// The nav re-read landed: extra sits beside demo without a restart.
	found := false
	for _, mod := range c.nav.modules {
		if mod.id == "extra" {
			found = true
		}
	}
	require.True(t, found, "the nav re-read after the create")
}

func TestApply_destructiveConfirmBeforeElevation(t *testing.T) {
	l := &fakeLauncher{previews: []service.StepPreview{
		{Name: "dotfiles", Overwrites: []string{"~/.bashrc", "~/.zshrc"}},
		sudoPreview,
	}}
	_, c := applyShell(t, l)
	l.run = closedRun(&service.SessionResult{Outcome: service.OutcomeCompleted})

	c = wsPress(t, c, "P")
	require.Len(t, c.modals, 1)
	require.IsType(t, &confirmModel{}, c.modals[0], "the destructive confirm comes first")
	require.Contains(t, c.View().Content, "2 existing")
	require.Equal(t, 0, l.started)

	c = cpress(c, "y") // confirm destructive…
	require.Len(t, c.modals, 1)
	require.IsType(t, &elevationModel{}, c.modals[0], "…then the elevation gate")

	c = typeText(c, "pw")
	applySettle(t, c, cpressCmd(c, "enter"))
	require.Equal(t, 1, l.started, "both gates passed, the run starts")
}

// applyShellRun builds a compositor whose launcher starts the canned run.
func applyShellRun(t *testing.T, previews []service.StepPreview, run *testRun) (map[string]string, *fakeLauncher, *Compositor) {
	t.Helper()
	l := &fakeLauncher{previews: previews, run: run}
	run.previews = previews
	subs, c := applyShell(t, l)
	return subs, l, c
}

func TestApplyDetail_streaming(t *testing.T) {
	run := newTestRun()
	run.events <- service.StepStarted{Name: "packages", Index: 0, Total: 2}
	run.events <- service.StepOutput{Name: "packages", Chunk: []byte("installing ripgrep\n")}
	subs, _, c := applyShellRun(t, []service.StepPreview{{Name: "packages"}, {Name: "dotfiles"}}, run)

	c = wsPress(t, c, "P") // no privileged steps: the run starts directly
	require.True(t, c.applying, "the badge is live while the run lives")

	c = cpress(c, "a")
	require.Len(t, c.modals, 1, "a opens the apply detail modal")
	requireGolden(t, "apply-detail-streaming.golden", c.View().Content, subs)
}

func TestApplyDetail_failed(t *testing.T) {
	run := closedRun(&service.SessionResult{
		Outcome:   service.OutcomeFailed,
		StepError: &service.StepError{Step: "dotfiles", Err: errors.New("link failed")},
	},
		service.StepStarted{Name: "packages", Index: 0, Total: 2},
		service.StepFinished{Name: "packages"},
		service.StepStarted{Name: "dotfiles", Index: 1, Total: 2},
		service.StepFailed{Name: "dotfiles", Err: &service.StepError{Step: "dotfiles", Err: errors.New("link failed")}},
	)
	subs, _, c := applyShellRun(t, []service.StepPreview{{Name: "packages"}, {Name: "dotfiles"}}, run)

	c = wsPress(t, c, "P")
	require.False(t, c.applying, "a failed run is done")
	c = cpress(c, "a")
	requireGolden(t, "apply-detail-failed.golden", c.View().Content, subs)
}

func TestApplyDetail_closeNeverCancels_ctrlCInsideConfirms(t *testing.T) {
	run := newTestRun()
	run.events <- service.StepStarted{Name: "packages", Index: 0, Total: 1}
	_, _, c := applyShellRun(t, []service.StepPreview{{Name: "packages"}}, run)

	c = wsPress(t, c, "P")
	require.True(t, c.applying)

	c = cpress(c, "a")
	require.Len(t, c.modals, 1)
	c = cpress(c, "esc") // close the inspector…
	require.Empty(t, c.modals)
	require.True(t, c.applying, "…the run never cancels on close")

	c = cpress(c, "a")
	c = cpress(c, "ctrl+c")
	require.Len(t, c.modals, 2, "ctrl+c inside asks before cancelling")
	cpress(c, "y")
	require.Equal(t, 1, run.cancelled, "the confirm cancels the session")
}
