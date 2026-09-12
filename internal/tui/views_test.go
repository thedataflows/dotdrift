package tui

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
	"github.com/thedataflows/dotdrift/internal/state"
)

// Golden View() tests for the read views (T-tui-shell): the module
// resolved stack, the raw toggle, the canonical status report, the plan
// with diffs, and the stubs. Views are exercised through the real
// service reads area over the checked-in resolve profile — the same
// narrow interface the cmd adapter satisfies — so the goldens pin what
// the TUI actually renders, layout included. ANSI styling is stripped:
// goldens pin layout and text; the palette has its own tests.

var updateGoldens = flag.Bool("update", false, "rewrite golden files with actual render output")

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;:?]*[a-zA-Z]`)

func requireGolden(t *testing.T, name, got string, subs map[string]string) {
	t.Helper()
	got = ansiRe.ReplaceAllString(got, "")
	for from, to := range subs {
		got = strings.ReplaceAll(got, from, to)
	}
	// Viewport padding trails every line; its width depends on substituted
	// path lengths, so trailing spaces are normalized away.
	var lines []string
	for _, line := range strings.Split(got, "\n") {
		lines = append(lines, strings.TrimRight(line, " "))
	}
	got = strings.Join(lines, "\n")
	path := filepath.Join("testdata", "golden", name)
	if *updateGoldens {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s", name)
	require.Equal(t, string(want), got, "golden %s", name)
}

// goldenShell builds a fully loaded shell over the real reads area and
// the named checked-in profile. The startup selection is the profile's
// first module with its resolved view already filled in.
func goldenShell(t *testing.T, profileRel string) (map[string]string, *Shell) {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "profiles", profileRel))
	require.NoError(t, err)

	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "state.json")
	require.NoError(t, state.NewFileStore(statePath).Save(&state.State{LastCompleted: "packages:demo"}))

	area := service.NewReadsArea(service.ReadsDeps{
		Detect: func() (*facts.Facts, error) {
			return &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Kernel: "6.12.1-arch1-1", Distro: "arch", GPU: " amd", Backend: "pacman"}, nil
		},
		OtherAccounts: func(string, *facts.Facts) ([]profile.Account, error) {
			return []profile.Account{{Name: "root", Uid: "0"}}, nil
		},
	})
	m := New(area, Options{
		ProfilePath: dir,
		StatePath:   statePath,
		ProbesFor: func(*facts.Facts) drift.Probes {
			pr := drift.DefaultProbes()
			pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }
			pr.ToolCurrent = func(context.Context, string) (string, error) { return "", errors.New("no mise") }
			return pr
		},
	})
	m, _ = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m, loadCmd := step(m, mustMsg(m.Init()))
	require.NotNil(t, loadCmd)
	m, _ = step(m, loadCmd())
	require.False(t, m.stack.top().loading)
	return map[string]string{dir: "$PROFILE", tmp: "$STATE"}, m
}

// screen is the active view's rendered body, golden-ready: the read view
// itself (title + body). The surrounding chrome — header, panes, status
// bar — is pinned by the shell's own tests; a full-screen golden would
// bake viewport padding in, which shifts with substituted path lengths.
func screen(m *Shell) string { return m.stack.top().content }

// advanceToNoCmd is advanceTo for synchronous views (no scheduled load).
func advanceToNoCmd(t *testing.T, m *Shell, label string) *Shell {
	t.Helper()
	m, cmd := advanceTo(t, m, label)
	require.Nil(t, cmd, "a synchronous view schedules no load")
	return m
}

func TestGolden_moduleResolvedView(t *testing.T) {
	subs, m := goldenShell(t, "resolve")
	require.Equal(t, "shell ·3", selectedItemLabel(m), "the shell opens on the profile's module")
	requireGolden(t, "module-resolved.golden", screen(m), subs)
}

func TestGolden_moduleRawToggleView(t *testing.T) {
	subs, m := goldenShell(t, "resolve")
	m = press(m, "r")
	requireGolden(t, "module-raw.golden", screen(m), subs)
}

func TestGolden_originView(t *testing.T) {
	subs, m := goldenShell(t, "resolve")
	m = advanceToNoCmd(t, m, "base")
	requireGolden(t, "origin.golden", screen(m), subs)
}

func TestGolden_statusView(t *testing.T) {
	subs, m := goldenShell(t, "resolve")
	m, cmd := advanceTo(t, m, "status")
	require.NotNil(t, cmd, "status loads async")
	m, _ = step(m, cmd())
	require.False(t, m.stack.top().loading)
	requireGolden(t, "status.golden", screen(m), subs)
}

func TestGolden_planView(t *testing.T) {
	subs, m := goldenShell(t, "resolve")
	m, cmd := advanceTo(t, m, "plan")
	require.NotNil(t, cmd, "plan loads async")
	m, _ = step(m, cmd())
	require.False(t, m.stack.top().loading)
	requireGolden(t, "plan.golden", screen(m), subs)
}

func TestGolden_accountView(t *testing.T) {
	// The superuser-visibility profile (issue 0029): a users/root overlay
	// surfaces as a skipped module and a superuser account node.
	subs, m := goldenShell(t, "tui-superuser")
	m = advanceToNoCmd(t, m, "user root (superuser)")
	requireGolden(t, "account-superuser.golden", screen(m), subs)
}

func TestGolden_actionStubView(t *testing.T) {
	subs, m := goldenShell(t, "resolve")
	m = advanceToNoCmd(t, m, "generate")
	requireGolden(t, "action-stub.golden", screen(m), subs)
}
