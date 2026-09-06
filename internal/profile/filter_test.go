package profile_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

func TestParseModuleFilter(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"nil input is nil", nil, nil},
		{"empty input is nil", []string{}, nil},
		{"comma-separated single arg", []string{"vim,git"}, []string{"vim", "git"}},
		{"space-separated multi arg", []string{"vim", "git"}, []string{"vim", "git"}},
		{"mixed comma and space", []string{"vim,git", "ssh"}, []string{"vim", "git", "ssh"}},
		{"dupes collapsed first-seen order", []string{"git", "vim,git", "vim,ssh"}, []string{"git", "vim", "ssh"}},
		{"whitespace trimmed", []string{" vim , git ", "  ssh"}, []string{"vim", "git", "ssh"}},
		{"empties dropped", []string{"", " , ", ","}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, profile.ParseModuleFilter(tt.args))
		})
	}
}

// The filter keeps only the listed modules in Selected; excluded Selected
// modules move to Skipped with reason "module filter", preserving order.
func TestProfile_LimitTo_keepsOnlyListed(t *testing.T) {
	p, err := profile.Load(fixture(t, "scope"), &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"})
	require.NoError(t, err)
	require.Len(t, p.Selected, 2, "scope fixture must start with demo+shell selected")

	require.NoError(t, p.LimitTo([]string{"shell"}))

	require.Len(t, p.Selected, 1)
	require.Equal(t, "shell", p.Selected[0].ID)
	require.Len(t, p.Skipped, 1)
	require.Equal(t, "demo", p.Skipped[0].Module.ID)
	require.Equal(t, "module filter", p.Skipped[0].Reason)
}

// Listing every selected module is a shrink-to-same: nothing moves.
func TestProfile_LimitTo_allListedKeepsSelectionOrder(t *testing.T) {
	p, err := profile.Load(fixture(t, "scope"), &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"})
	require.NoError(t, err)

	require.NoError(t, p.LimitTo([]string{"shell", "demo"}))

	require.Len(t, p.Selected, 2)
	require.Equal(t, "demo", p.Selected[0].ID, "selection order is discovery order, not filter order")
	require.Equal(t, "shell", p.Selected[1].ID)
	require.Empty(t, p.Skipped)
}

// Unknown ids error loudly, naming the unknown ids and every valid module id.
func TestProfile_LimitTo_unknownIDErrors(t *testing.T) {
	p, err := profile.Load(fixture(t, "scope"), &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"})
	require.NoError(t, err)

	err = p.LimitTo([]string{"shell", "zzz"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "zzz")
	require.Contains(t, err.Error(), "demo")
	require.Contains(t, err.Error(), "shell")
	t.Logf("error: %v", err)
}

// Naming a disabled module errors, naming it and its existing skip reason —
// the filter never resurrects a disabled module.
func TestProfile_LimitTo_disabledModuleErrors(t *testing.T) {
	p, err := profile.Load(fixture(t, "disabled"), &facts.Facts{})
	require.NoError(t, err)

	err = p.LimitTo([]string{"a"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "a")
	require.Contains(t, err.Error(), "disabled")
	t.Logf("error: %v", err)
}

// Same for a when-filter-skipped module: the error carries its skip reason.
func TestProfile_LimitTo_whenFilteredModuleErrors(t *testing.T) {
	p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{Hostname: "otherhost", Username: "cri", OS: "linux"})
	require.NoError(t, err)

	err = p.LimitTo([]string{"hostonly"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hostonly")
	require.Contains(t, err.Error(), "when filter")
	t.Logf("error: %v", err)
}

// Empty filter is a no-op: selection and skips stay exactly as loaded.
func TestProfile_LimitTo_emptyNoop(t *testing.T) {
	p, err := profile.Load(fixture(t, "scope"), &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"})
	require.NoError(t, err)

	require.NoError(t, p.LimitTo(nil))
	require.NoError(t, p.LimitTo([]string{}))

	require.Len(t, p.Selected, 2)
	require.Empty(t, p.Skipped)
}

// A module that is selected AND superuser-skip-listed (its id collides with
// another account's overlay copy, issue 0032) passes the filter: the skip
// entry describes the overlay copy, not this view's selection.
func TestProfile_LimitTo_selectedPlusSuperuserSkippedPasses(t *testing.T) {
	m := profile.Module{ID: "micro"}
	p := &profile.Profile{
		Modules:  []profile.Module{m},
		Selected: []profile.Module{m},
		Skipped:  []profile.Skip{{Module: m, Reason: profile.ReasonSuperuserOverlay}},
	}

	require.NoError(t, p.LimitTo([]string{"micro"}))

	require.Len(t, p.Selected, 1)
	require.Equal(t, "micro", p.Selected[0].ID)
}

// A module that exists ONLY in another account's superuser overlay is not
// "unknown" — it errors with its skip reason, which is the actionable one.
func TestProfile_LimitTo_overlayOnlySuperuserModuleErrorsWithReason(t *testing.T) {
	p := &profile.Profile{
		Modules:  []profile.Module{{ID: "base"}},
		Selected: []profile.Module{{ID: "base"}},
		Skipped:  []profile.Skip{{Module: profile.Module{ID: "rootonly"}, Reason: profile.ReasonSuperuserOverlay}},
	}

	err := p.LimitTo([]string{"rootonly"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rootonly")
	require.Contains(t, err.Error(), "requires root")
	require.NotContains(t, err.Error(), "unknown module", "the module exists; the reason must be the skip, not unknown")
	t.Logf("error: %v", err)
}
