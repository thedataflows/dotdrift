package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
	"github.com/thedataflows/dotdrift/internal/state"
)

func TestFinalGateAllOffRejectsAtServiceBoundary(t *testing.T) {
	g := newApplyGateModel(newTheme(true))
	for i := 2; i < len(g.rows); i++ {
		g.rows[i].on = false
	}
	_, _, sections := g.snapshot()
	_, err := service.ResolveSections(sections)
	require.ErrorContains(t, err, "no sections selected")
}

func TestFinalApplyPendingKeepsFirstSnapshot(t *testing.T) {
	for _, answer := range []string{"y", "esc"} {
		t.Run(answer, func(t *testing.T) {
			l := &fakeLauncher{startErr: errors.New("stop after start")}
			_, c := applyShell(t, l)
			c.nav.yanked = map[string]bool{"demo": true}
			c = cpress(c, "P")
			c = cpress(c, "space")
			c, preview := cstep(c, keyPress("y"))
			require.NotNil(t, preview)
			c.nav.yanked = nil
			c = cpress(c, "P")
			require.Empty(t, c.modals, "P must not open another gate during Preview")
			c = cpress(c, answer)
			msg := preview()
			require.Len(t, l.previewOpts, 1)
			require.True(t, l.previewOpts[0].Force)
			require.Equal(t, []string{"demo"}, l.previewOpts[0].Modules)
			c, start := cstep(c, msg)
			require.NotNil(t, start)
			c, _ = cstep(c, start())
			require.True(t, l.startedOpts[0].Force)
			require.Equal(t, []string{"demo"}, l.startedOpts[0].Modules)
			c = cpress(c, "P")
			require.Len(t, c.modals, 1, "a failed start releases the chain")
		})
	}
}

func TestFinalProfilePendingKeepsStaleMark(t *testing.T) {
	for _, kind := range []string{"write", "apply"} {
		t.Run(kind, func(t *testing.T) {
			_, c := profileShell()
			cmd := c.reloadProfile()
			require.Nil(t, c.reloadProfile(), "only one Profile read can run")
			if kind == "write" {
				c, _ = cstep(c, writeFinishedMsg{})
			} else {
				c.apply = &applyRunState{}
				c, _ = cstep(c, applyWaitedMsg{res: &service.SessionResult{Outcome: service.OutcomeCompleted}})
			}
			c, _ = cstep(c, cmd())
			require.True(t, c.stale[viewProfile], "a mark after read start must survive landing")
			cmd = c.reloadProfile()
			c, _ = cstep(c, cmd())
			require.False(t, c.stale[viewProfile])
		})
	}
}

func TestFinalWriteCompletionInvalidatesWithoutDialog(t *testing.T) {
	for _, covered := range []bool{false, true} {
		t.Run(fmt.Sprint(covered), func(t *testing.T) {
			_, c := profileShell()
			c.modals = append(c.modals, &dialogModal{c: c, d: newManageDialog(nil, c.root, nil, manageSel{})})
			if covered {
				c.openHelp()
			} else {
				c = cpress(c, "esc")
			}
			c, _ = cstep(c, writeFinishedMsg{})
			for _, v := range []viewKind{viewStatus, viewPlan, viewProfile} {
				require.True(t, c.stale[v])
			}
		})
	}
}

func TestFinalOverrideInvalidatesReadViews(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n"})
	c = cpress(c, "O")
	for _, v := range []viewKind{viewStatus, viewPlan, viewProfile} {
		require.True(t, c.stale[v])
	}
}

func TestFinalChromeFitsLoadedStaleScopedViews(t *testing.T) {
	for _, size := range [][2]int{{64, 24}, {100, 30}} {
		for _, v := range []viewKind{viewModules, viewStatus, viewPlan, viewProfile} {
			t.Run(fmt.Sprint(size, v), func(t *testing.T) {
				_, c := statusShell(statusFixtureRead())
				c = loadStatusView(t, c)
				c.planv.read = planFixtureRead()
				c.status.scope = []string{"a-very-long-module-name", "another-module"}
				c.planv.scope = c.status.scope
				c.profile.read = pinnedProfileRead()
				c.markReadViewsStale()
				c.view = v
				c, _ = cstep(c, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				for _, task := range []bool{false, true} {
					c.message, c.msgErr = strings.Repeat("long error ", 20), true
					if task {
						c.op = strings.Repeat("long task ", 20)
					}
					frame := c.View().Content
					require.LessOrEqual(t, lipgloss.Width(frame), size[0])
					require.LessOrEqual(t, lipgloss.Height(frame), size[1])
					row := ansiRe.ReplaceAllString(c.viewRow(), "")
					require.Contains(t, row, "stale")
					require.Contains(t, row, "[")
				}
			})
		}
	}
}

func TestFinalFailedRefreshKeepsDisplayedScope(t *testing.T) {
	t.Run("Status", func(t *testing.T) {
		stub, c := statusShell(statusFixtureRead())
		c = loadStatusView(t, c)
		c.status.scope = []string{"old"}
		c.nav.yanked = map[string]bool{"new": true}
		stub.planErr = errors.New("read failed")
		cmd := c.reloadStatus()
		c, _ = cstep(c, cmd())
		require.Equal(t, []string{"old"}, c.status.scope)
	})
	t.Run("Plan", func(t *testing.T) {
		stub, c := profileShell()
		c.planv.read = &service.PlanRead{}
		c.planv.scope = []string{"old"}
		c.nav.yanked = map[string]bool{"new": true}
		stub.planErr = errors.New("read failed")
		cmd := c.reloadPlan()
		c, _ = cstep(c, cmd())
		require.Equal(t, []string{"old"}, c.planv.scope)
	})
}

func TestFinalStatusJumpClearsNavFilter(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/owner/module.toml":           "id = \"owner\"\n",
		"users/cri/modules/owner/module.toml": "id = \"owner\"\n",
		"modules/other/module.toml":           "id = \"other\"\n",
	})
	c.nav.query = []rune("other")
	c.view = viewStatus
	c.status.read = statusFixtureRead()
	c.status.read.Plan.Dotfiles.Entries = append(c.status.read.Plan.Dotfiles.Entries, resolve.DotfileEntry{Target: "/target", Module: "owner", Layer: "user"})
	c.status.read.Findings = []drift.Finding{{Section: "dotfiles", Module: "owner", Item: "/target", Status: drift.Drift}}
	c.status.clamp(c.statusRows())
	c.focus = focusNav
	c = wsPress(t, c, "enter")
	require.Equal(t, "owner", c.nav.selected().moduleID)
	require.Equal(t, "user", c.nav.selected().layer)
	require.Empty(t, c.nav.query)
}

func TestFinalVolumePickerFitsAndKeepsSelection(t *testing.T) {
	for _, w := range []int{64, 100} {
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			p := &volumePickerModel{th: newTheme(true)}
			for i := 0; i < 30; i++ {
				p.vols = append(p.vols, generate.Volume{UUID: fmt.Sprintf("12345678-1234-1234-1234-%012d", i), FSType: "ext4", Label: "data", Size: "1T", Mountpoints: []string{"/mnt/data"}, Managed: true})
			}
			p.cur = 29
			text := p.view(w, 24)
			require.LessOrEqual(t, lipgloss.Width(text), w)
			require.LessOrEqual(t, lipgloss.Height(text), 24)
			plain := ansiRe.ReplaceAllString(text, "")
			for _, s := range []string{p.vols[29].UUID, "ext4", "data", "1T", "/mnt/data", "MANAGED"} {
				require.Contains(t, plain, s)
			}
		})
	}
}

func TestFinalStatusResume(t *testing.T) {
	read := statusFixtureRead()
	read.State = &state.State{LastCompleted: "packages"}
	_, c := statusShell(read)
	c = loadStatusView(t, c)
	require.Contains(t, ansiRe.ReplaceAllString(c.statusList(120, 30), ""), `last completed "packages" - next apply resumes after it`)
	read.State.LastCompleted = ""
	require.Contains(t, ansiRe.ReplaceAllString(c.statusList(120, 30), ""), "clean - next apply starts from the beginning")
}

func TestFinalRefreshKeepsListWithPendingNote(t *testing.T) {
	_, c := statusShell(statusFixtureRead())
	c = loadStatusView(t, c)
	c.reloadStatus()
	text := ansiRe.ReplaceAllString(c.statusList(100, 30), "")
	require.Contains(t, text, "probing")
	require.Contains(t, text, "firefox")
	_, c = profileShell()
	c.planv.read = &service.PlanRead{}
	c.reloadPlan()
	text = ansiRe.ReplaceAllString(c.planList(100, 30), "")
	require.Contains(t, text, "resolving")
	require.Contains(t, text, "plan is empty")
}

func TestFinalReadViewsDoNotEditHiddenModule(t *testing.T) {
	for _, v := range []viewKind{viewStatus, viewPlan, viewProfile} {
		for _, k := range []string{"a", "o"} {
			t.Run(fmt.Sprint(v, k), func(t *testing.T) {
				_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n[packages]\npresent = [\"git\"]\n"})
				c.writesFor = func(*facts.Facts) Writes { return service.NewWritesArea(service.WritesDeps{}) }
				for i, row := range c.ws.rows {
					if row.section == "packages" {
						c.ws.cursor = i
						break
					}
				}
				c.view, c.focus = v, focusWork
				c = cpress(c, k)
				require.Empty(t, c.modals)
				c.apply = &applyRunState{}
				c = cpress(c, "a")
				require.Len(t, c.modals, 1)
				require.IsType(t, &applyDetailModel{}, c.modals[0])
			})
		}
	}
}

func TestFinalHelpMatchesReadViewKeys(t *testing.T) {
	_, c := profileShell()
	c.view = viewStatus
	c.status.read = statusFixtureRead()
	c.status.clamp(c.statusRows())
	rows := fmt.Sprint(c.helpRows())
	require.Contains(t, rows, "P apply owners")
	require.Contains(t, ansiRe.ReplaceAllString(c.footerView(), ""), "P apply owners")
	for _, s := range []string{"enter", "owner", "P", "pgup", "pgdown", "home", "end"} {
		require.Contains(t, rows, s)
	}
	require.NotContains(t, rows, "onboard into module")
	require.NotContains(t, rows, "add entry")
	c.view, c.focus = viewPlan, focusNav
	require.Contains(t, fmt.Sprint(c.helpRows()), "P apply yanked/all")
	require.Contains(t, ansiRe.ReplaceAllString(c.footerView(), ""), "P apply yanked/all")
	require.Contains(t, fmt.Sprint(c.helpRows()), "toggle diff list")
	c = cpress(c, "d")
	require.True(t, c.planv.showDiff)
	c.focus = focusWork
	require.NotContains(t, fmt.Sprint(c.helpRows()), "toggle diff list")
	c = cpress(c, "d")
	require.True(t, c.planv.showDiff, "d is not accepted by the detail pane")
	c.view = viewProfile
	require.Contains(t, fmt.Sprint(c.helpRows()), "P apply yanked/all")
	require.Contains(t, ansiRe.ReplaceAllString(c.footerView(), ""), "P apply yanked/all")
	require.NotContains(t, fmt.Sprint(c.helpRows()), "onboard into module")
}

func TestFinalStatusJumpUsesKnownLayer(t *testing.T) {
	for _, kind := range []string{"mount", "expanded dotfile", "orphan", "overlay only"} {
		t.Run(kind, func(t *testing.T) {
			files := map[string]string{"modules/other/module.toml": "id = \"other\"\n", "users/cri/modules/owner/module.toml": "id = \"owner\"\n"}
			if kind != "overlay only" {
				files["modules/owner/module.toml"] = "id = \"owner\"\n"
			}
			_, c := wsShell(t, files)
			home := t.TempDir()
			t.Setenv("HOME", home)
			f := drift.Finding{Section: "packages", Module: "owner", Item: "git", Status: drift.Drift}
			c.status.read = statusFixtureRead()
			switch kind {
			case "mount":
				f.Section, f.Item = "mounts", "/mnt/data"
				c.status.read.Plan.Mounts.Entries = []resolve.MountEntry{{Module: "owner", Layer: "user", Spec: profile.MountSpec{Destination: f.Item}}}
			case "expanded dotfile":
				f.Section, f.Item = "dotfiles", filepath.Join(home, ".config")
				c.status.read.Plan.Dotfiles.Entries = []resolve.DotfileEntry{{Module: "owner", Layer: "user", Target: "~/.config"}}
			case "orphan":
				f.Section, f.Group = "orphans", "users/cri"
			}
			c.status.read.Findings = []drift.Finding{f}
			c.status.clamp(c.statusRows())
			c.view, c.focus = viewStatus, focusNav
			c.nav.query = []rune("other")
			c = wsPress(t, c, "enter")
			require.Equal(t, "owner", c.nav.selected().moduleID)
			if kind != "overlay only" {
				require.Equal(t, "user", c.nav.selected().layer)
			}
		})
	}
}

func TestFinalApplyChainReleasesAfterCancelOrPreviewFailure(t *testing.T) {
	for _, kind := range []string{"preview failure", "overwrite escape", "overwrite decline", "elevation escape"} {
		t.Run(kind, func(t *testing.T) {
			l := &fakeLauncher{}
			switch kind {
			case "preview failure":
				l.previewErr = errors.New("cannot preview")
			case "overwrite escape", "overwrite decline":
				l.previews = []service.StepPreview{{Name: "dotfiles", Overwrites: []string{"/target"}}}
			case "elevation escape":
				l.previews = []service.StepPreview{{Name: "dotfiles", NeedsTTY: true, Reason: "sudo"}}
			}
			_, c := applyShell(t, l)
			c.sudoCheck = func([]byte) error { return nil }
			c = cpress(c, "P")
			c = applyGateY(t, c)
			if kind != "preview failure" {
				answer := "esc"
				if kind == "overwrite decline" {
					answer = "n"
				}
				c = cpress(c, answer)
			}
			require.Empty(t, c.modals)
			c = cpress(c, "P")
			require.Len(t, c.modals, 1)
			require.IsType(t, &applyGateModel{}, c.modals[0])
		})
	}
}
