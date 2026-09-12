package service

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/state"
)

// Canonical renderer goldens (T-tui-reads): the bytes the cmd renderers
// produced before the move, pinned under testdata/golden and reproduced
// byte for byte by the service renderers (0061-D5/D7).

func TestRender_planReport_golden(t *testing.T) {
	profileDir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "profiles", "resolve"))
	require.NoError(t, err)
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	r, err := area.Plan(profileDir, nil, f)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, RenderPlanReport(&buf, r, nil))
	requireGoldenSub(t, "plan-report.golden", buf.String(), map[string]string{profileDir: "$PROFILE"})
}

func TestRender_diff_golden(t *testing.T) {
	dir, plan := diffFixture(t)
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	entries, err := area.Diff(plan, dir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	var buf bytes.Buffer
	require.NoError(t, RenderDiff(&buf, entries, "internal", false))
	requireGoldenSub(t, "diff.golden", buf.String(), map[string]string{dir: "$PROFILE"})
}

func TestRender_statusSummary_golden(t *testing.T) {
	dir := statusFixture(t)
	statePath := filepath.Join(dir, "state.json")
	require.NoError(t, state.NewFileStore(statePath).Save(&state.State{LastCompleted: "packages"}))

	probesFor := func(*facts.Facts) drift.Probes {
		pr := drift.DefaultProbes()
		pr.IsInstalled = func(ctx context.Context, pkg string) (bool, error) { return false, nil }
		pr.ToolCurrent = func(ctx context.Context, tool string) (string, error) { return "", errors.New("no mise") }
		return pr
	}

	area := NewReadsArea(ReadsDeps{
		OtherAccounts: func(string, *facts.Facts) ([]profile.Account, error) {
			return []profile.Account{
				{Name: "alice", Uid: "1000", Home: "/home/alice"},
				{Name: "root", Uid: "0", Home: "/root"},
			}, nil
		},
	}.WithDefaults())
	r, err := area.Status(t.Context(), StatusOpts{ProfilePath: dir, StatePath: statePath, ProbesFor: probesFor})
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, RenderStatusSummary(&buf, r))
	requireGoldenSub(t, "status-summary.golden", buf.String(), map[string]string{dir: "$PROFILE"})
}
