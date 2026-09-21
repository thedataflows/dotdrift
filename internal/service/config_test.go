package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// The config area (0065-D7 layer 3): read one layer's module.toml as raw
// text plus a strict decode; save through the splice pipeline; manage
// modules. The save pipeline owns the authoritative validation tiers
// (0065-D8): strict decode of the spliced text (contract 19 by
// construction), the resolve-level checks with profile context, the
// disk-hash conflict check, then the atomic write.

func configFixture(t *testing.T) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, "modules", "editor")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(
		"# hand-written: editor\nid = \"editor\"\napp = \"editor\"\n\n[packages]\npresent = [\n  \"ripgrep\",\n]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zshrc"), []byte("# the zshrc\n"), 0o644))
	return root, dir
}

func configArea(root string) *ConfigArea {
	return NewConfigArea(root, ConfigDeps{Facts: &facts.Facts{Hostname: "box", Username: "kim"}})
}

func TestReadModuleLayer_strictDecodeAndRawText(t *testing.T) {
	root, dir := configFixture(t)
	r, err := configArea(root).ReadModuleLayer(dir)
	require.NoError(t, err)
	require.True(t, r.Exists)
	require.Equal(t, filepath.Join(dir, "module.toml"), r.Path)
	require.Contains(t, r.Raw, "# hand-written: editor", "raw text passes through verbatim")
	require.Equal(t, "editor", r.Config.ID)
	require.Equal(t, []string{"ripgrep"}, r.Config.Packages.Present)
	require.NotEmpty(t, r.Hash, "the read carries the disk hash the draft baselines against")
}

func TestReadModuleLayer_brokenFileCarriesSchemaError(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "modules", "broken")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(
		"id = \"broken\"\nbogus = 1\n"), 0o644))
	_, err := configArea(root).ReadModuleLayer(dir)
	require.Error(t, err)
	var schema *SchemaError
	require.True(t, errors.As(err, &schema), "broken files surface as *SchemaError, got: %v", err)
	require.NotZero(t, schema.Line, "the strict decoder localizes the offending line")

	// A missing module.toml is a scaffold start, not an error.
	empty := filepath.Join(root, "modules", "fresh")
	require.NoError(t, os.MkdirAll(empty, 0o755))
	r, err := configArea(root).ReadModuleLayer(empty)
	require.NoError(t, err)
	require.False(t, r.Exists)
	require.NotNil(t, r.Config)
	require.NotEmpty(t, r.Hash, "the empty file's hash baselines the first save")
}

func TestWriteModuleLayer_savePipeline(t *testing.T) {
	root, dir := configFixture(t)
	area := configArea(root)

	r, err := area.ReadModuleLayer(dir)
	require.NoError(t, err)

	// Two dirty sections of one file (0065-D8's file-scoped draft): the
	// packages block is edited AND the module keys gain a description. One
	// save splices both families and leaves the rest untouched.
	res, err := area.WriteModuleLayer(SaveRequest{
		Dir:      dir,
		BaseHash: r.Hash,
		Replacements: map[string]string{
			FamilyKeys:     profile.EncodeKeysSection(profile.ModuleConfig{ID: "editor", App: "editor", Description: "edited"}),
			FamilyPackages: profile.EncodePackagesSection(profile.PackageEntries([]string{"ripgrep", "bat"}), nil),
		},
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dir, "module.toml"))
	require.NoError(t, err)
	require.NotContains(t, string(raw), "# hand-written", "the keys family re-encoded (interior comments go)")
	require.Contains(t, string(raw), "description = \"edited\"")
	require.Contains(t, string(raw), "\"bat\"", "the new package is there")
	require.NotContains(t, string(raw), "[tools]", "untouched families appear nowhere")
	require.NotEqual(t, r.Hash, res.Hash, "the save returns the new hash for the draft to rebase")

	// The saved file still strict-decodes with the new values in place.
	r2, err := area.ReadModuleLayer(dir)
	require.NoError(t, err)
	require.Equal(t, "edited", r2.Config.Description)
	require.Equal(t, []string{"ripgrep", "bat"}, r2.Config.Packages.Present)
}

func TestWriteModuleLayer_brokenFileReadOnly(t *testing.T) {
	root, dir := configFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(
		"id = \"editor\"\nbogus = true\n"), 0o644))
	// Reading the broken file yields the schema error (the frame opens it
	// read-only and never stages a save). The write tier holds the same
	// guarantee on its own: a correctly-baselined save touching an
	// UNRELATED family is still refused by the pipeline's strict decode.
	raw, err := os.ReadFile(filepath.Join(dir, "module.toml"))
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	_, err = configArea(root).WriteModuleLayer(SaveRequest{
		Dir:          dir,
		BaseHash:     hex.EncodeToString(sum[:]),
		Replacements: map[string]string{FamilyTools: "[tools]\nfd = \"9\"\n"},
	})
	require.Error(t, err, "a broken file cannot be saved (read-only in the UI)")
	var schema *SchemaError
	require.True(t, errors.As(err, &schema), "the failure carries the schema error, got: %v", err)
}

func TestWriteModuleLayer_diskHashConflictRefuses(t *testing.T) {
	root, dir := configFixture(t)
	area := configArea(root)
	r, err := area.ReadModuleLayer(dir)
	require.NoError(t, err)

	// An external edit lands after the read.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(
		"id = \"editor\"\napp = \"editor\"\n"), 0o644))

	_, err = area.WriteModuleLayer(SaveRequest{
		Dir:          dir,
		BaseHash:     r.Hash,
		Replacements: map[string]string{FamilyTools: "[tools]\nfd = \"9\"\n"},
	})
	require.Error(t, err, "an external change refuses the save")
	var conflict *DiskHashConflictError
	require.True(t, errors.As(err, &conflict), "the conflict error is typed, got: %v", err)
	require.Equal(t, filepath.Join(dir, "module.toml"), conflict.Path)

	// The external bytes stand — no merge, no clobber.
	raw, err := os.ReadFile(filepath.Join(dir, "module.toml"))
	require.NoError(t, err)
	require.Equal(t, "id = \"editor\"\napp = \"editor\"\n", string(raw))
}

func TestWriteModuleLayer_resolveChecksRefuse(t *testing.T) {
	root := t.TempDir()
	dirA := filepath.Join(root, "modules", "a")
	dirB := filepath.Join(root, "modules", "b")
	for dir, target := range map[string]string{dirA: "~/.zshrc", dirB: "~/.zshrc"} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(
			"id = \""+filepath.Base(dir)+"\"\napp = \""+filepath.Base(dir)+"\"\n\n[dotfiles]\n\""+target+"\" = { source = \"zshrc\", mode = \"symlink\" }\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "zshrc"), []byte("x"), 0o644))
	}
	area := configArea(root)
	r, err := area.ReadModuleLayer(dirA)
	require.NoError(t, err)

	// Editing module A keeps its target claim — module B already claims the
	// same target, so the resolve-level cross-module check refuses the save.
	_, err = area.WriteModuleLayer(SaveRequest{
		Dir:      dirA,
		BaseHash: r.Hash,
		Replacements: map[string]string{
			FamilyDotfiles: "[dotfiles]\n\"~/.zshrc\" = { source = \"zshrc\", mode = \"symlink\" }\n",
		},
	})
	require.Error(t, err, "cross-module target conflicts (contract 16) refuse the save")
	require.Contains(t, err.Error(), "b", "the error names the conflicting module")

	// And the file is untouched by the refused save.
	raw, err := os.ReadFile(filepath.Join(dirA, "module.toml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "id = \"a\"")

	// A broken when expression is refused at the grammar tier.
	_, err = area.WriteModuleLayer(SaveRequest{
		Dir:      dirA,
		BaseHash: r.Hash,
		Replacements: map[string]string{
			FamilyWhen: "[when]\nor = []\n",
		},
	})
	require.Error(t, err, "an empty or-list fails the when grammar")
	require.Contains(t, err.Error(), "when.or")
}

func TestModuleOps_createScaffold(t *testing.T) {
	root := t.TempDir()
	area := configArea(root)

	require.NoError(t, area.CreateModule("notes", ""))
	dir := filepath.Join(root, "modules", "notes")
	raw, err := os.ReadFile(filepath.Join(dir, "module.toml"))
	require.NoError(t, err)
	require.Equal(t, "id = \"notes\"\napp = \"notes\"\n", string(raw),
		"the scaffold is minimal: id and app, nothing else")

	// Collision refused.
	err = area.CreateModule("notes", "")
	require.Error(t, err, "the target layer already has that module")

	// Overlays land in their layer.
	require.NoError(t, area.CreateModule("notes", "hosts/box"))
	_, err = os.Stat(filepath.Join(root, "hosts", "box", "modules", "notes", "module.toml"))
	require.NoError(t, err)
}

// T-0082-override: an overlay for an existing module seeds a comment-only
// module.toml — an empty overlay overrides nothing, and a copied base
// would pin its fields against later base edits.
func TestModuleOps_overrideSeedsEmptyOverlay(t *testing.T) {
	root := t.TempDir()
	area := configArea(root)
	require.NoError(t, area.CreateModule("m", ""))

	require.NoError(t, area.OverrideModule("m", "", "users/kim"))
	raw, err := os.ReadFile(filepath.Join(root, "users", "kim", "modules", "m", "module.toml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "override", "the seed names its purpose")
	require.Contains(t, string(raw), "packages, tools, dotfiles, hooks, mounts, smb",
		"the seed names the merged families")
	require.NotContains(t, string(raw), "id =", "the overlay stays empty: meta comes from the base")
	require.FileExists(t, filepath.Join(root, "modules", "m", "module.toml"),
		"the source module stays put")

	// The target layer already having the module refuses.
	err = area.OverrideModule("m", "", "users/kim")
	require.Error(t, err, "users/kim already has module m")

	// The source layer lacking the module refuses and creates nothing.
	err = area.OverrideModule("nope", "", "hosts/box")
	require.Error(t, err, "base has no module nope")
	require.NoDirExists(t, filepath.Join(root, "hosts", "box", "modules", "nope"))
}

func TestModuleOps_moveRefusedOnCollision(t *testing.T) {
	root := t.TempDir()
	area := configArea(root)
	require.NoError(t, area.CreateModule("m", ""))
	require.NoError(t, area.CreateModule("m", "users/kim"))

	err := area.MoveModule("m", "", "users/kim")
	require.Error(t, err, "the target layer already has that module")
	_, err = os.Stat(filepath.Join(root, "modules", "m"))
	require.NoError(t, err, "the source stays put on refusal")

	require.NoError(t, area.MoveModule("m", "", "hosts/box"))
	_, err = os.Stat(filepath.Join(root, "modules", "m"))
	require.True(t, os.IsNotExist(err), "the module moved wholesale (contract 8: dirs are self-contained)")
	_, err = os.Stat(filepath.Join(root, "hosts", "box", "modules", "m", "module.toml"))
	require.NoError(t, err)
}

func TestModuleOps_deletePreviewsOrphans(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "modules", "dele")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(
		"id = \"dele\"\napp = \"dele\"\n\n[dotfiles]\n\"~/.zshrc\" = { source = \"zshrc\", mode = \"symlink\" }\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zshrc"), []byte("# referenced\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stray.sh"), []byte("# not referenced\n"), 0o644))

	preview, err := configArea(root).DeletePreview("dele", "")
	require.NoError(t, err)
	require.Equal(t, []string{"stray.sh"}, preview,
		"the orphan set comes from ReferencedPaths: unreferenced sources only")

	// Deleting removes the directory wholesale.
	require.NoError(t, configArea(root).DeleteModule("dele", ""))
	_, err = os.Stat(dir)
	require.True(t, os.IsNotExist(err))
}

func TestModuleLayersAt_listsAllLayers(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, configArea(root).CreateModule("m", ""))
	require.NoError(t, configArea(root).CreateModule("m", "hosts/box"))
	require.NoError(t, configArea(root).CreateModule("m", "users/kim"))

	layers := ModuleLayersAt(root)
	require.Len(t, layers, 3)
	var names []string
	for _, l := range layers {
		names = append(names, l.Layer+"/"+l.Owner)
	}
	require.Contains(t, strings.Join(names, " "), "base/")
	require.Contains(t, strings.Join(names, " "), "host/box")
	require.Contains(t, strings.Join(names, " "), "user/kim")
}
