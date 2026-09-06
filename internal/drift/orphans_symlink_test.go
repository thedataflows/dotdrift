package drift_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
)

// A symlinked directory that declarations reference THROUGH (the link's
// subtree is the entry's source) is a container for referenced content, not
// content: WalkDir never descends into it, so without special-casing the link
// itself is falsely flagged (issue 0034 — field report: an overlay's
// home -> base symlink flagged while the copy entry resolves through it).
// Dangling links and links nothing references through stay flagged.
func TestCheckOrphans_referencedThroughSymlinkedDir(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml": `[dotfiles]
"~/.config/app" = { source = "home/app", mode = "symlink-each" }
`,
		"shared/home/app/a.conf": "referenced through the link",
		"shared/other/b.conf":    "nothing references through unref-link",
	})
	shellDir := filepath.Join(root, "modules", "shell")
	require.NoError(t, os.Symlink(filepath.Join(root, "shared", "home"), filepath.Join(shellDir, "home")))
	require.NoError(t, os.Symlink(filepath.Join(root, "shared", "other"), filepath.Join(shellDir, "unref-link")))
	require.NoError(t, os.Symlink(filepath.Join(root, "shared", "gone"), filepath.Join(shellDir, "dangling")))

	fs := drift.CheckOrphans(orphanLayers(root))
	groups := orphanGroups(fs)
	require.Equal(t, []string{"dangling", "unref-link"}, groupsForModule(groups, "base", "shell"),
		"the referenced-through home link is not flagged; dangling and unreferenced-through links stay flagged")
}

// groupsForModule flattens groups[group][module] for one group/module pair.
func groupsForModule(groups map[string]map[string][]string, group, module string) []string {
	if groups[group] == nil {
		return nil
	}
	return groups[group][module]
}
