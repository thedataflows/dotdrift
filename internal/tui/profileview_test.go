package tui

// T-tui-views slice 1c (issue 0107): the Profile view body — a
// cursor-less read-only list of the detect facts, the profile paths, the
// resume line (Status's rule, the 0106 hash mask included), and the
// other-accounts note. The read is async (profileLoadedMsg, the
// navLoadedMsg pattern); r re-reads the view, R re-reads all views.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
	"github.com/thedataflows/dotdrift/internal/state"
)

// stubReads is a canned Reads seam: Modules returns an empty read,
// Profile returns the pinned model and counts its calls. The slice 2c
// fields (plan/status/diffs) serve the Status view tests; the methods
// live in statusview_test.go.
type stubReads struct {
	profile      *service.ProfileRead
	profileErr   error
	profileCalls int
	plan         *service.PlanRead
	planErr      error
	status       *service.StatusRead
	statusErr    error
	statusCalls  int
	lastOpts     service.StatusOpts
	diffs        []service.DiffEntry
	diffErr      error
}

func (s *stubReads) Modules(string, []string) (*service.ModulesRead, error) {
	return &service.ModulesRead{}, nil
}

func (s *stubReads) Profile(string, []string) (*service.ProfileRead, error) {
	s.profileCalls++
	return s.profile, s.profileErr
}

// pinnedProfileRead is the fixed Profile model the view tests render.
func pinnedProfileRead() *service.ProfileRead {
	return &service.ProfileRead{
		ProfilePath: "/home/cri/profiles/main",
		StatePath:   "/home/cri/.local/state/dotdrift/main.json",
		State:       &state.State{LastCompleted: "packages"},
		Facts: &facts.Facts{
			Hostname: "myhost", Username: "cri", OS: "linux",
			Kernel: "6.12.1-arch1-1", Distro: "arch", GPU: "amd", Backend: "paru",
		},
		Others: []profile.Account{
			{Name: "alice", Uid: "1000", Home: "/home/alice"},
			{Name: "root", Uid: "0", Home: "/root"},
		},
	}
}

// profileShell builds a compositor over the stub seam at 100x30.
func profileShell() (*stubReads, *Compositor) {
	stub := &stubReads{profile: pinnedProfileRead()}
	c := NewCompositor(stub, "/home/cri/profiles/main", nil)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	return stub, c
}

// loadProfileView enters the Profile view and settles the scheduled read.
func loadProfileView(t *testing.T, c *Compositor) *Compositor {
	t.Helper()
	var cmd tea.Cmd
	c, cmd = cstep(c, keyPress("4"))
	require.NotNil(t, cmd, "entering the Profile view schedules the read")
	c, _ = cstep(c, mustMsg(cmd))
	return c
}

func TestProfileView_pendingThenLoaded(t *testing.T) {
	stub, c := profileShell()

	var cmd tea.Cmd
	c, cmd = cstep(c, keyPress("4"))
	require.Equal(t, viewProfile, c.view)
	require.NotNil(t, cmd, "entering the Profile view schedules the read")
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.NotContains(t, frame, "myhost", "the facts render only after the read lands")

	c, _ = cstep(c, mustMsg(cmd))
	require.Equal(t, 1, stub.profileCalls)
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "myhost", "the landed read renders")
}

func TestProfileView_rendersFactsAndResume(t *testing.T) {
	stub, c := profileShell()
	c = loadProfileView(t, c)

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	for _, line := range []string{
		"host:    myhost",
		"user:    cri",
		"os:      linux",
		"kernel:  6.12.1-arch1-1",
		"distro:  arch",
		"gpu:     amd",
		"backend: paru",
		"profile: /home/cri/profiles/main",
		"state:   /home/cri/.local/state/dotdrift/main.json",
		`resume:  last completed "packages" - next apply resumes after it`,
		"OTHER ACCOUNTS",
		"configuration exists for other accounts on this machine:",
		"users/alice — apply with: sudo -iu alice dotdrift apply",
		"users/root — apply with: sudo dotdrift apply",
	} {
		require.Contains(t, frame, line)
	}
	subs := map[string]string{
		"/home/cri/profiles/main":                   "$PROFILE",
		"/home/cri/.local/state/dotdrift/main.json": "$STATE",
	}
	requireGolden(t, "views-profile.golden", c.View().Content, subs)

	// No pending cursor: the clean resume line (Status's rule).
	stub.profile = pinnedProfileRead()
	stub.profile.State = &state.State{}
	c, cmd := cstep(c, keyPress("r"))
	require.NotNil(t, cmd, "r re-reads the view")
	c, _ = cstep(c, mustMsg(cmd))
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "resume:  clean - next apply starts from the beginning")
	require.NotContains(t, frame, "last completed")
}

// 0144: no outer pane card around the group cards — the cards sit
// directly in the frame. 0145: they stretch the full frame width
// (no right padding), and the body fills the pane height so the
// footer stays at the bottom of the frame.
func TestProfileView_cardsStretchFullWidth(t *testing.T) {
	_, c := profileShell()
	c = loadProfileView(t, c)

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.NotContains(t, frame, "│╭", "no card nests inside a larger pane border")
	lines := strings.Split(frame, "\n")
	for _, line := range lines {
		if strings.Contains(line, "FACTS") {
			break
		}
		if strings.HasPrefix(line, "╭") {
			require.Equal(t, 100, len([]rune(strings.TrimRight(line, " "))),
				"the card spans the full 100-char frame")
		}
	}
	require.Contains(t, lines[len(lines)-1], "q quit", "the footer sits on the last line")
	require.Equal(t, "", strings.TrimSpace(lines[len(lines)-2]),
		"the body pads to the pane height, so the footer stays at the bottom")
}

func TestProfileView_resumeLineFollowsTheHash(t *testing.T) {
	// The 0106 rule through the real reads area: a cursor stored for a
	// different input is dead — the view must show the clean line, like
	// Status. A cursor whose hash matches the current input is pending.
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "profiles", "resolve"))
	require.NoError(t, err)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	statePath := state.ProfileStatePath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o755))

	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}
	area := service.NewReadsArea(service.ReadsDeps{
		Detect: func() (*facts.Facts, error) { return f, nil },
	})

	require.NoError(t, state.NewFileStore(statePath).Save(&state.State{
		LastCompleted: "packages",
		InputHash:     "stale-hash",
	}))
	c := NewCompositor(area, dir, nil)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	c = loadProfileView(t, c)
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "resume:  clean - next apply starts from the beginning",
		"a stale-hash cursor is dead, not pending (0106)")
	require.NotContains(t, frame, "last completed")

	// Store the cursor with the current input's hash: pending.
	r, err := area.Profile(dir, nil)
	require.NoError(t, err)
	require.NoError(t, state.NewFileStore(statePath).Save(&state.State{
		LastCompleted: "packages",
		InputHash:     resolve.InputHash(r.Profile, r.Facts, r.Plan),
	}))
	c, cmd := cstep(c, keyPress("r"))
	require.NotNil(t, cmd, "r re-reads the view")
	c, _ = cstep(c, mustMsg(cmd))
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, `resume:  last completed "packages" - next apply resumes after it`)
}

func TestProfileView_refreshKeys(t *testing.T) {
	stub, c := profileShell()
	c = loadProfileView(t, c)
	require.Equal(t, 1, stub.profileCalls)

	// r re-reads the active view; the landed read clears the stale mark.
	c.markStale(viewProfile)
	c, cmd := cstep(c, keyPress("r"))
	require.NotNil(t, cmd, "r re-reads the active view")
	c, _ = cstep(c, mustMsg(cmd))
	require.Equal(t, 2, stub.profileCalls)
	require.False(t, c.stale[viewProfile], "the landed read clears the stale mark")

	// R re-reads all views: the nav read and the profile read together.
	c, cmd = cstep(c, keyPress("R"))
	require.NotNil(t, cmd, "R re-reads all views")
	batch, ok := mustMsg(cmd).(tea.BatchMsg)
	require.True(t, ok, "R batches the per-view reads")
	for _, bc := range batch {
		c, _ = cstep(c, mustMsg(bc))
	}
	require.Equal(t, 3, stub.profileCalls)
}

func TestViews_smallFrame(t *testing.T) {
	_, c := profileShell()
	c = loadProfileView(t, c)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 64, Height: 24})
	subs := map[string]string{
		"/home/cri/profiles/main":                   "$PROFILE",
		"/home/cri/.local/state/dotdrift/main.json": "$STATE",
	}
	requireGolden(t, "views-profile-small.golden", c.View().Content, subs)

	// The too-small guard still fires under the 60x12 minimum.
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 59, Height: 11})
	require.Contains(t, c.View().Content, "too small")
}

func TestViews_footerFitsSmallFrame(t *testing.T) {
	// Fix round 1 (review finding): the off-Modules footer hints ran
	// ~150 cells with no help width set — past the 64-cell frame. Every
	// row of the pinned frame must fit the terminal.
	_, c := profileShell()
	c = loadProfileView(t, c)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 64, Height: 24})

	lines := strings.Split(strings.TrimRight(c.View().Content, "\n"), "\n")
	require.LessOrEqual(t, len(lines), 24, "the frame never exceeds the terminal height")
	for i, line := range lines {
		require.LessOrEqual(t, lipgloss.Width(line), 64, "row %d fits the frame width", i)
	}
}

// stubDialog is the minimal dialog for the write-finished path.
type stubDialog struct{}

func (stubDialog) HandleKey(string) tea.Cmd       { return nil }
func (stubDialog) View(theme) string              { return "" }
func (stubDialog) applyFinished(writeFinishedMsg) {}

func TestViews_writesMarkStale(t *testing.T) {
	// A saved draft outlives the read views' data (0107).
	c := newTestCompositor()
	c, _ = cstep(c, saveFinishedMsg{dir: "base/shell"})
	require.True(t, c.stale[viewStatus], "a write marks the Status view stale")
	require.True(t, c.stale[viewPlan], "a write marks the Plan view stale")
	require.True(t, c.stale[viewProfile], "a write marks the Profile view stale")
	require.False(t, c.stale[viewModules], "a write re-reads the nav itself")

	// A finished apply outlives them too, whatever the outcome — even a
	// failed or cancelled run moved the resume cursor.
	c = newTestCompositor()
	c.apply = &applyRunState{}
	c, _ = cstep(c, applyWaitedMsg{res: &service.SessionResult{Outcome: service.OutcomeCompleted}})
	require.True(t, c.stale[viewProfile], "a finished apply marks the read views stale")

	// A dialog write (manage / onboard / generate / restore) the same.
	c = newTestCompositor()
	c.modals = append(c.modals, &dialogModal{d: stubDialog{}, th: c.th, c: c})
	c, _ = cstep(c, writeFinishedMsg{})
	require.True(t, c.stale[viewProfile], "a dialog write marks the read views stale")
}
