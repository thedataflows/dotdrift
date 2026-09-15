package mise

// OverwriteTargets (M15, T-tui-modals): the destructive-apply signal —
// copy-mode destinations that exist on disk, listed in their declared
// (tilde) form for the confirm modal.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func TestDotfilesStep_OverwriteTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "copyrc"), []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(home, "linkrc"), []byte("old"), 0o644))

	st := &DotfilesStep{Plan: &resolve.Plan{Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
		{Target: "~/copyrc", Mode: "copy"},    // exists → overwrite
		{Target: "~/linkrc", Mode: "symlink"}, // symlink replacement is not content overwrite
		{Target: "~/missing", Mode: "copy"},   // first apply → nothing to overwrite
		{Target: "~/editrc/line", Line: "x"},  // edits are marker-scoped
	}}}}
	require.Equal(t, []string{"~/copyrc"}, st.OverwriteTargets())

	empty := &DotfilesStep{}
	require.Empty(t, empty.OverwriteTargets(), "a nil plan overwrites nothing")
}
