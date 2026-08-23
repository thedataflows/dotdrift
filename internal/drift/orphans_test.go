package drift_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Orphans: files inside a module's layer directories that no [dotfiles]
// entry references — neither explicitly (source = ...) nor implicitly (a
// direct child of a symlink-each source directory) — surface in an
// "orphans" section, attributed per module and layer.

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
}

func orphanLayers(root string) []drift.ModuleLayer {
	return []drift.ModuleLayer{
		{Dir: "shell", Layer: "base", Path: filepath.Join(root, "modules", "shell")},
		{Dir: "shell", Layer: "host", Owner: "h", Path: filepath.Join(root, "hosts", "h", "modules", "shell")},
		{Dir: "shell", Layer: "user", Owner: "cri", Path: filepath.Join(root, "users", "cri", "modules", "shell")},
	}
}

// User-layer orphans are attributed "[user:<username>]" — same as host
// layers name their host — so a multi-user profile's leftovers are
// distinguishable at a glance.
func TestCheckOrphans_userLayerAttribution(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml":       "",
		"users/cri/modules/shell/local.sh": "orphan in the user overlay",
	})
	plan := &resolve.Plan{}

	fs := drift.CheckOrphans(plan, orphanLayers(root))
	items := orphanItems(fs)
	require.Equal(t, []string{"local.sh"}, items["shell [user:cri]"])
}

func TestCheckOrphans_reportsUnreferencedFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml":     "",
		"modules/shell/bashrc":          "managed",
		"modules/shell/notes.md":        "orphan",
		"modules/shell/nested/deep.md":  "orphan",
		"hosts/h/modules/shell/hook.sh": "orphan",
	})
	plan := &resolve.Plan{}
	plan.Dotfiles.Entries = []resolve.DotfileEntry{
		{Module: "shell", Source: filepath.Join(root, "modules", "shell", "bashrc"), Mode: "symlink"},
	}

	fs := drift.CheckOrphans(plan, orphanLayers(root))
	items := orphanItems(fs)
	require.ElementsMatch(t, []string{
		"notes.md", "nested/deep.md",
	}, items["shell [base]"], "unreferenced base files are orphans")
	require.Equal(t, []string{"hook.sh"}, items["shell [host:h]"], "unreferenced overlay files are orphans per layer, naming the host")
	for _, f := range fs {
		require.Equal(t, drift.Drift, f.Status)
		require.NotEmpty(t, f.Detail)
	}
}

// module.toml is the manifest, never an orphan; symlink-each source-dir
// direct children are implicitly referenced; deeper files under the source
// dir are NOT deployed by symlink-each (only direct children) and are
// orphans.
func TestCheckOrphans_symlinkEachChildrenReferenced(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml":  "",
		"modules/shell/units/a.conf": "managed implicitly",
		"modules/shell/units/b.conf": "managed implicitly",
		"modules/shell/units/nested/c.conf": "NOT deployed by symlink-each",
	})
	plan := &resolve.Plan{}
	plan.Dotfiles.Entries = []resolve.DotfileEntry{
		{Module: "shell", Source: filepath.Join(root, "modules", "shell", "units"), Mode: "symlink-each"},
	}

	fs := drift.CheckOrphans(plan, orphanLayers(root))
	items := orphanItems(fs)
	require.Empty(t, items["shell [host:h]"], "no host layer dir exists")
	require.Equal(t, []string{"units/nested/c.conf"}, items["shell [base]"],
		"direct children referenced; nested files are orphans")
}

// Sources of other modules do not mark this module's files; template edit
// sources are referenced; mode="edit" sources were consumed at resolve
// time (Block content), leaving no reference to flag.
func TestCheckOrphans_attributionPerModule(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml":   "",
		"modules/shell/snippet.sh":    "referenced by edit template",
		"modules/other/module.toml":   "",
		"modules/other/other.conf":    "orphan of other",
	})
	plan := &resolve.Plan{}
	plan.Dotfiles.Entries = []resolve.DotfileEntry{
		{Module: "shell", Target: "~/.zshrc/snippet", Source: filepath.Join(root, "modules", "shell", "snippet.sh"), Template: "tera"},
	}
	layers := []drift.ModuleLayer{
		{Dir: "shell", Layer: "base", Path: filepath.Join(root, "modules", "shell")},
		{Dir: "other", Layer: "base", Path: filepath.Join(root, "modules", "other")},
	}

	fs := drift.CheckOrphans(plan, layers)
	items := orphanItems(fs)
	require.Empty(t, items["shell [base]"], "template edit source is referenced")
	require.Equal(t, []string{"other.conf"}, items["other [base]"])
}

// Nothing unreferenced → no findings, and the orphans section is omitted
// from the rendered report entirely.
func TestCheckOrphans_cleanModuleOmitsSection(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml": "",
		"modules/shell/bashrc":      "managed",
	})
	plan := &resolve.Plan{}
	plan.Dotfiles.Entries = []resolve.DotfileEntry{
		{Module: "shell", Source: filepath.Join(root, "modules", "shell", "bashrc"), Mode: "symlink"},
	}

	fs := drift.CheckOrphans(plan, orphanLayers(root))
	require.Empty(t, fs)

	var b strings.Builder
	drift.Render(&b, fs)
	require.NotContains(t, b.String(), "orphans")
}

// The orphans section renders after the built-in sections.
func TestRender_orphansSectionLast(t *testing.T) {
	findings := []drift.Finding{
		{Section: "packages", Item: "jq", Status: drift.OK, Module: "m"},
		{Section: "orphans", Item: "notes.md", Status: drift.Drift, Detail: "not referenced by [dotfiles]", Module: "m [base]"},
	}
	var b strings.Builder
	drift.Render(&b, findings)
	s := b.String()
	require.Contains(t, s, "orphans:")
	require.Greater(t, strings.Index(s, "orphans:"), strings.Index(s, "packages:"), "orphans renders after the built-in sections")
	require.Contains(t, s, "m [base]: notes.md")
}

// orphanItems groups findings by the rendered module (layer-attributed).
func orphanItems(fs []drift.Finding) map[string][]string {
	out := map[string][]string{}
	for _, f := range fs {
		out[f.Module] = append(out[f.Module], f.Item)
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// The real stack (profile.Load + resolve.Resolve + CheckOrphans): a
// symlink-each source's direct children must never be orphans, and neither
// must the source file of a mode = "edit" entry — resolve consumes it into
// an inline Block (Source=""), but the file is still the authored source.
// Only genuinely unreferenced files are reported.
func TestCheckOrphans_realStack(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml": `[dotfiles]
"~/.config/units" = { source = "units", mode = "symlink-each" }
"~/.profile/aliases" = { source = "home/aliases.sh", mode = "edit" }
"~/.zshrc/snippet" = { source = "snippets/snippet.tmpl", template = "tera" }
"~/.bashrc" = { source = "home/.bashrc", mode = "symlink" }
`,
		"modules/shell/units/a.conf":         "deployed implicitly",
		"modules/shell/units/b.conf":         "deployed implicitly",
		"modules/shell/home/aliases.sh":      "edit source, consumed at resolve",
		"modules/shell/snippets/snippet.tmpl": "template edit source",
		"modules/shell/home/.bashrc":         "whole-file source",
		"modules/shell/NOTES.md":             "true orphan",
	})

	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}
	p, err := profile.Load(root, f)
	require.NoError(t, err)
	plan, err := resolve.Resolve(p, f)
	require.NoError(t, err)

	fs := drift.CheckOrphans(plan, []drift.ModuleLayer{{Dir: "shell", Layer: "base", Path: filepath.Join(root, "modules", "shell")}})
	require.Equal(t, map[string][]string{
		"shell [base]": {"NOTES.md"},
	}, orphanItems(fs), "only the true orphan; edit/symlink-each/template sources are referenced")
}
