package tui

// Shared model bits that survived the M14 tree's deletion (T-tui-cleanup):
// the superuser-overlay classification feeding the nav's 0029 labels, and
// the minimal selection the manage dialog hangs off.

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
)

// superuserOwners extracts the uid-0 overlay owners the profile loader
// marked (issue 0029): users/<owner>/modules/… paths on skip entries with
// the superuser reason. The uid lookup itself stays in profile — the nav
// only reads the verdict.
func superuserOwners(skips []profile.Skip) map[string]bool {
	owners := map[string]bool{}
	for _, s := range skips {
		if s.Reason != profile.ReasonSuperuserOverlay {
			continue
		}
		if owner, ok := userOverlayOwner(s.Module.Path); ok {
			owners[owner] = true
		}
	}
	return owners
}

// userOverlayOwner pulls the users/<owner> segment out of a module path.
func userOverlayOwner(path string) (string, bool) {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range parts {
		if part == "users" && i+1 < len(parts) {
			return parts[i+1], true
		}
	}
	return "", false
}

// manageSel is the selection the manage dialog hangs off: a module id and
// the layer dir (currentLayer derives the layer from it).
type manageSel struct {
	moduleID string
	dir      string
}

// Reads is the compositor's narrow read seam (ADR-0008's doorway): the
// modules listing, the profile read, and the Status view's reads (0108
// slice 2c), facts included.
// *service.ReadsArea satisfies it.
type Reads interface {
	Modules(profilePath string, modules []string) (*service.ModulesRead, error)
	// Profile is the light no-probes read behind the Profile view
	// (0107); the view itself lands in slice 1c.
	Profile(profilePath string, modules []string) (*service.ProfileRead, error)
	// Plan is the probe-free plan read behind the Status view's
	// elevation gate (it decides whether the probing read needs sudo
	// before the read starts) and the Plan view's merged-sections read
	// (0109). f nil = detect fresh facts.
	Plan(profilePath string, modules []string, f *facts.Facts) (*service.PlanRead, error)
	// Status is the probing read; StatusOpts.Elevated says the sudo
	// gate passed. Diff feeds the copy-mode dotfile detail pane.
	Status(ctx context.Context, opts service.StatusOpts) (*service.StatusRead, error)
	Diff(plan *resolve.Plan, profileRoot string) ([]service.DiffEntry, error)
}

// minWidth/minHeight: below this the shell shows the too-small guard.
const (
	minWidth  = 60
	minHeight = 12
)
