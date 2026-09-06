package drift_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
)

// A user overlay that redeclares a symlink-each dir target (mode=copy) wins
// deployment in its own views, but the declaring layer's subtree stays the
// authored reference: an account without a user layer resolves the bare-base
// view the enumeration cannot see and deploys that tree (issue 0031 — field
// report: base mise files flagged while the invoking account's live symlinks
// point into them).
func TestCheckOrphans_dirSourceDeclaringLayerSurvivesOverride(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml": `[dotfiles]
"~/.config/app" = { source = "home/app", mode = "symlink-each" }
`,
		"modules/shell/home/app/a.conf": "base tree, deployed by bare-base accounts",
		"users/root/modules/shell/module.toml": `[dotfiles]
"~/.config/app" = { source = "home/app", mode = "copy" }
`,
		"users/root/modules/shell/home/app/a.conf": "root overlay copy tree",
	})

	fs := drift.CheckOrphans([]drift.ModuleLayer{
		{Dir: "shell", Layer: "base", Path: filepath.Join(root, "modules", "shell")},
		{Dir: "shell", Layer: "user", Owner: "root", Path: filepath.Join(root, "users", "root", "modules", "shell")},
	})
	groups := orphanGroups(fs)
	require.Empty(t, groups["base"],
		"the declaring layer's dir subtree stays referenced when an overlay redeclares the target")
	require.Empty(t, groups["users/root"],
		"the overriding layer's own declared tree is referenced too")
}
