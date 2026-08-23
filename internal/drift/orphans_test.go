package drift_test

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/palette"
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
	groups := orphanGroups(fs)
	require.Equal(t, []string{"local.sh"}, groups["users/cri"]["shell"])
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
	groups := orphanGroups(fs)
	require.ElementsMatch(t, []string{"notes.md", "nested/deep.md"},
		groups["base"]["shell"], "unreferenced base files are orphans")
	require.Equal(t, []string{"hook.sh"}, groups["hosts/h"]["shell"],
		"unreferenced overlay files are orphans, grouped under the host layer root")
	for _, f := range fs {
		require.Equal(t, drift.Drift, f.Status)
		require.NotEmpty(t, f.Detail)
	}
}

// module.toml is the manifest, never an orphan; the WHOLE subtree of a
// symlink-each source directory is referenced (mise links dir children,
// so nested files deploy too) — only files outside any reference are
// orphans.
func TestCheckOrphans_symlinkEachChildrenReferenced(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml":       "",
		"modules/shell/units/a.conf":      "direct child",
		"modules/shell/units/db/b.conf":   "nested under a dir child",
		"modules/shell/units/db/sub/c.conf": "nested deeper",
	})
	plan := &resolve.Plan{}
	plan.Dotfiles.Entries = []resolve.DotfileEntry{
		{Module: "shell", Source: filepath.Join(root, "modules", "shell", "units"), Mode: "symlink-each"},
	}

	fs := drift.CheckOrphans(plan, orphanLayers(root))
	require.Empty(t, orphanGroups(fs), "the whole symlink-each source subtree is referenced")
}

// A whole-file entry whose source is a DIRECTORY (symlink/copy mode,
// the shape onboard produces) deploys the whole subtree; none of its
// files is an orphan, at any depth. Same declaring-layer anchor as
// symlink-each: the subtree counts where the entry is declared.
func TestCheckOrphans_dirSourceSubtreeReferenced(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/shell/module.toml":          "",
		"modules/shell/home/app/a.conf":      "direct child",
		"modules/shell/home/app/db/b.conf":   "nested",
		"hosts/h/modules/shell/home/app/db/c.conf": "overlay file nothing references",
	})
	plan := &resolve.Plan{}
	plan.Dotfiles.Entries = []resolve.DotfileEntry{
		{Module: "shell", Layer: "base", Source: filepath.Join(root, "modules", "shell", "home", "app"), Mode: "symlink"},
	}

	fs := drift.CheckOrphans(plan, orphanLayers(root))
	groups := orphanGroups(fs)
	require.Empty(t, groups["base"], "the whole dir-source subtree is referenced")
	require.Equal(t, []string{"home/app/db/c.conf"}, groups["hosts/h"]["shell"],
		"overlay files at the same rel-path that nothing references stay orphans")
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
	groups := orphanGroups(fs)
	require.Empty(t, groups["base"]["shell"], "template edit source is referenced")
	require.Equal(t, []string{"other.conf"}, groups["base"]["other"])
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
// The orphans section renders last, grouped under layer-root headings
// (base, hosts/<hostname>, users/<username>), one `module: file` line per
// orphan. No em-dashes anywhere in the output - plain ASCII only.
func TestRender_orphansGroupedUnderLayerRoots(t *testing.T) {
	findings := []drift.Finding{
		{Section: "packages", Item: "jq", Status: drift.OK, Module: "m"},
		{Section: "orphans", Group: "base", Item: "home/etc/samba/smb.conf.d/shares.conf",
			Status: drift.Drift, Detail: "not referenced by [dotfiles]", Module: "system-samba"},
		{Section: "orphans", Group: "hosts/cri-pc", Item: "file1",
			Status: drift.Drift, Detail: "not referenced by [dotfiles]", Module: "module"},
		{Section: "orphans", Group: "users/cri", Item: "file3",
			Status: drift.Drift, Detail: "not referenced by [dotfiles]", Module: "module2"},
	}
	var b strings.Builder
	drift.Render(&b, findings)
	s := b.String()
	t.Log(s)
	require.Greater(t, strings.Index(s, "orphans:"), strings.Index(s, "packages:"), "orphans renders last")
	require.Contains(t, s, "  base:\n    system-samba: home/etc/samba/smb.conf.d/shares.conf - not referenced by [dotfiles]")
	require.Contains(t, s, "  hosts/cri-pc:\n    module: file1 - not referenced by [dotfiles]")
	require.Contains(t, s, "  users/cri:\n    module2: file3 - not referenced by [dotfiles]")
	require.NotContains(t, s, "\u2014", "no em-dashes in output")
}

// On a TTY, orphan findings use a distinct shade (magenta) so they stand
// apart from regular drift hues (orange/red/yellow); plain output is
// unchanged.
func TestRender_orphansDistinctColor(t *testing.T) {
	origTerminal, origNoColor := executil.IsTerminal, executil.NoColor
	t.Cleanup(func() { executil.IsTerminal, executil.NoColor = origTerminal, origNoColor })
	executil.IsTerminal = func(io.Writer) bool { return true }
	executil.NoColor = false

	var b strings.Builder
	drift.Render(&b, []drift.Finding{
		{Section: "packages", Item: "jq", Status: drift.Drift, Detail: "missing", Module: "m"},
		{Section: "orphans", Group: "base", Item: "notes.md", Status: drift.Drift, Detail: "not referenced by [dotfiles]", Module: "m"},
	})
	s := b.String()
	require.Contains(t, s, "\033[35m", "orphan line carries the magenta hue")
	require.NotContains(t, strings.Split(s, "orphans:")[0], "\033[35m", "non-orphan sections keep their hues")
}

// A palette override re-hues the orphans section (and missing findings).
func TestRender_paletteOverrides(t *testing.T) {
	origTerminal, origNoColor := executil.IsTerminal, executil.NoColor
	t.Cleanup(func() { executil.IsTerminal, executil.NoColor = origTerminal, origNoColor })
	executil.IsTerminal = func(io.Writer) bool { return true }
	executil.NoColor = false

	p, err := palette.FromConfig(map[string]string{"orphan": "94", "missing": "38;5;75"})
	require.NoError(t, err)

	var b strings.Builder
	drift.Render(&b, []drift.Finding{
		{Section: "packages", Item: "jq", Status: drift.Drift, Detail: "missing", Module: "m"},
		{Section: "orphans", Group: "base", Item: "notes.md", Status: drift.Drift, Detail: "not referenced by [dotfiles]", Module: "m"},
	}, drift.WithPalette(p))
	s := b.String()
	require.Contains(t, s, "\033[94m", "orphan lines use the overridden hue")
	require.Contains(t, s, "\033[38;5;75m", "missing finding uses the overridden hue")
	require.NotContains(t, s, "\033[35m", "default magenta replaced")
}

// orphanGroups groups findings by (group, module): group is the layer-root
// label ("base", "hosts:<h>", "users:<u>"), module the bare module dir.
func orphanGroups(fs []drift.Finding) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for _, f := range fs {
		if out[f.Group] == nil {
			out[f.Group] = map[string][]string{}
		}
		out[f.Group][f.Module] = append(out[f.Group][f.Module], f.Item)
	}
	for _, mods := range out {
		for k := range mods {
			sort.Strings(mods[k])
		}
	}
	return out
}

// The real stack (profile.Load + resolve.Resolve + CheckOrphans): a
// symlink-each source's children must never be orphans, and neither must
// the source file of a mode = "edit" entry — resolve consumes it into
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
	groups := orphanGroups(fs)
	require.Equal(t, []string{"NOTES.md"}, groups["base"]["shell"],
		"only the true orphan; edit/symlink-each/template sources are referenced")
}

// The real stack (profile.Load + resolve.Resolve + CheckOrphans), shaped
// like the easyeffects case from the field report: a symlink-each source
// dir with nested subdirectories (db/, input/) plus a same-path file in
// the host overlay. Everything under the base symlink-each source is
// referenced at ANY depth; the host overlay's unreferenced file is the
// only orphan.
func TestCheckOrphans_realStackSymlinkEachSubtree(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"modules/easyeffects/module.toml": `description = "PipeWire audio effects"
[dotfiles]
"~/.config/easyeffects" = { source = "home/.config/easyeffects", mode = "symlink-each" }
`,
		"modules/easyeffects/home/.config/easyeffects/db/bassEnhancerrc": "x",
		"modules/easyeffects/home/.config/easyeffects/db/deesserrc":      "x",
		"modules/easyeffects/home/.config/easyeffects/input/Noise Suppression.json": "x",
		"hosts/cri-pc/modules/easyeffects/home/.config/easyeffects/db/easyeffectsrc": "overlay",
	})

	f := &facts.Facts{Hostname: "cri-pc", Username: "cri", OS: "linux"}
	p, err := profile.Load(root, f)
	require.NoError(t, err)
	plan, err := resolve.Resolve(p, f)
	require.NoError(t, err)

	fs := drift.CheckOrphans(plan, []drift.ModuleLayer{
		{Dir: "easyeffects", Layer: "base", Path: filepath.Join(root, "modules", "easyeffects")},
		{Dir: "easyeffects", Layer: "host", Owner: "cri-pc", Path: filepath.Join(root, "hosts", "cri-pc", "modules", "easyeffects")},
	})
	groups := orphanGroups(fs)
	require.Equal(t, []string{"home/.config/easyeffects/db/easyeffectsrc"}, groups["hosts/cri-pc"]["easyeffects"],
		"the whole base symlink-each subtree is referenced; only the host-overlay file nothing references is an orphan")
	require.Empty(t, groups["base"], "the whole base symlink-each subtree is referenced")
}
