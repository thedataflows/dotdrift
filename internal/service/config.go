package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/tomlsplice"
)

// The config area (0065-D7's layer 3): module.toml editing. It reads one
// layer's file as raw text plus a strict decode, runs the save pipeline
// (encode → splice → strict-decode → resolve-level checks → disk-hash
// check → atomic tmp+rename), and owns the module-management operations —
// create/move/delete per 0065-D5. Presentation-free: the compositor's
// workspace and dialogs drive it (M15).

// Section families, re-exported as the save pipeline's staging
// vocabulary: SaveRequest.Replacements is keyed by these.
const (
	FamilyKeys     = profile.FamilyKeys
	FamilyPackages = profile.FamilyPackages
	FamilyTools    = profile.FamilyTools
	FamilySecrets  = profile.FamilySecrets
	FamilyDotfiles = profile.FamilyDotfiles
	FamilyWhen     = profile.FamilyWhen
	FamilyHooks    = profile.FamilyHooks
	FamilyMounts   = profile.FamilyMounts
	FamilySmb      = profile.FamilySmb
	FamilySystemd  = profile.FamilySystemd
)

// ConfigDeps carries the config area's seams: the facts the resolve-level
// checks run against, and the loader/resolver the CLI already uses. The
// zero-value deps resolve to the real implementations.
type ConfigDeps struct {
	// Facts the save pipeline's resolve checks run against. Nil falls back
	// to an empty fact set (a profile whose modules carry no when-leaves
	// still resolves).
	Facts   *facts.Facts
	Resolve func(p *profile.Profile, f *facts.Facts) (*resolve.Plan, error)
}

func (d ConfigDeps) withDefaults() ConfigDeps {
	if d.Facts == nil {
		d.Facts = &facts.Facts{}
	}
	if d.Resolve == nil {
		d.Resolve = resolve.Resolve
	}
	return d
}

// ConfigEditor is the front ends' narrow surface over the config area
// (ADR-0008's doorway): what the TUI editor frame drives, plus the
// module-management ops the context-menu dialogs run (issue 0072).
// *ConfigArea satisfies it implicitly.
type ConfigEditor interface {
	ReadModuleLayer(dir string) (*ModuleLayerRead, error)
	WriteModuleLayer(req SaveRequest) (*SaveResult, error)
	CreateModule(name, layer string) error
	MoveModule(name, fromLayer, toLayer string) error
	// OverrideModule seeds an overlay for an existing module in a higher
	// layer (issue 0082).
	OverrideModule(name, fromLayer, toLayer string) error
	// DeletePreview is the orphan list the delete gate shows before
	// DeleteModule runs.
	DeletePreview(name, layer string) ([]string, error)
	DeleteModule(name, layer string) error
}

var _ ConfigEditor = (*ConfigArea)(nil)

// ConfigArea owns module.toml editing for one profile root.
type ConfigArea struct {
	root string
	deps ConfigDeps
}

// NewConfigArea builds the config area over a profile root.
func NewConfigArea(root string, deps ConfigDeps) *ConfigArea {
	return &ConfigArea{root: root, deps: deps.withDefaults()}
}

// Root is the profile root the area was built over.
func (a *ConfigArea) Root() string { return a.root }

// ModuleLayerRead is one layer's module.toml as the editor draft baselines
// it: the raw text (untouched bytes for the splice), its sha256, and the
// strict-decoded config. Exists is false for a not-yet-written file (the
// scaffold case) — Raw is empty and Hash baselines the empty text.
type ModuleLayerRead struct {
	Dir    string
	Path   string
	Exists bool
	Raw    string
	Hash   string
	Config *profile.ModuleConfig
}

// ReadModuleLayer loads one module layer's module.toml. A missing file is
// a scaffold start; a broken file surfaces as *SchemaError (the UI opens
// it read-only), never as a partial read.
func (a *ConfigArea) ReadModuleLayer(dir string) (*ModuleLayerRead, error) {
	path := filepath.Join(dir, "module.toml")
	r := &ModuleLayerRead{Dir: dir, Path: path, Config: &profile.ModuleConfig{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		r.Hash = RawHash(nil)
		return r, nil
	}
	r.Exists = true
	r.Raw = string(raw)
	r.Hash = RawHash(raw)
	cfg := &profile.ModuleConfig{}
	if err := profile.DecodeModuleTOML(path, raw, cfg); err != nil {
		return nil, schemaError(err)
	}
	// The same grammar check the loader applies: a broken when expression
	// is a load-time error even outside a full profile load.
	if err := profile.ValidateWhen(cfg.ID, cfg.When); err != nil {
		return nil, schemaError(err)
	}
	r.Config = cfg
	return r, nil
}

// DiskHashConflictError reports that the file changed on disk since the
// draft baselined it — the save is refused and the UI offers a reload
// (0065-D8: no merge, no clobber).
type DiskHashConflictError struct{ Path string }

func (e *DiskHashConflictError) Error() string {
	return e.Path + " changed on disk since it was opened — reload before saving"
}

// SaveRequest is one save: the draft's baseline hash and the staged
// family blocks (family → encoded TOML text) to splice into the file.
type SaveRequest struct {
	Dir      string
	BaseHash string
	// Replacements maps section families to their re-encoded blocks; see
	// tomlsplice.Splice and the profile family constants.
	Replacements map[string]string
	// Raw, when non-nil, is the whole-file candidate (the raw-text repair
	// path: a broken file cannot be family-spliced). Replacements is
	// ignored; every other pipeline stage guards the save unchanged.
	Raw *string
}

// SaveResult carries the post-save state the draft rebases onto.
type SaveResult struct {
	Hash string
	Raw  string
}

// WriteModuleLayer runs the save pipeline (0065-D8), in order: splice the
// staged blocks into the file's current raw text, strict-decode the
// result (contract 19 by construction — a failure here is an encoder
// bug), run the resolve-level checks with profile context on a patched
// in-memory profile, re-check the disk hash, then write atomically
// (tmp+rename). A refused save leaves the file untouched.
func (a *ConfigArea) WriteModuleLayer(req SaveRequest) (*SaveResult, error) {
	path := filepath.Join(req.Dir, "module.toml")
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if RawHash(current) != req.BaseHash {
		return nil, &DiskHashConflictError{Path: path}
	}

	spliced := tomlsplice.Splice(string(current), req.Replacements)
	if req.Raw != nil {
		spliced = *req.Raw
	}
	// 0086: a save always lands a file ending in a newline. The
	// splicer's verbatim contract preserves the baseline's final newline
	// state (or lack of it) and the raw candidate is the user's text
	// as-is — the write boundary is where the guarantee lives. Append
	// only: an existing trailing blank line is the author's, keep it.
	if spliced != "" && !strings.HasSuffix(spliced, "\n") {
		spliced += "\n"
	}

	// The round-trip proof: what the splice produced must be a valid
	// module.toml under the strict schema.
	cfg := &profile.ModuleConfig{}
	if err := profile.DecodeModuleTOML(path, []byte(spliced), cfg); err != nil {
		return nil, fmt.Errorf("save pipeline: strict decode: %w", schemaError(err))
	}
	if err := profile.ValidateWhen(cfg.ID, cfg.When); err != nil {
		return nil, fmt.Errorf("save pipeline: %w", err)
	}

	// The resolve-level checks with profile context: source containment
	// (contract 8), source existence, cross-module target/package
	// conflicts (contracts 16/9), scope constraints (contract 14). The
	// edited module's config is patched in memory — the checks see the
	// save's outcome without touching the disk.
	// The context load is tolerant: the edited module's own broken file
	// is exactly what a raw-repair save fixes (patchModule substitutes the
	// candidate), and a broken sibling must not block saving this file.
	p, err := profile.LoadTolerant(a.root, a.deps.Facts)
	if err != nil {
		return nil, fmt.Errorf("save pipeline: load profile: %w", schemaError(err))
	}
	if err := patchModule(p, req.Dir, cfg); err != nil {
		return nil, fmt.Errorf("save pipeline: %w", err)
	}
	if _, err := a.deps.Resolve(p, a.deps.Facts); err != nil {
		return nil, fmt.Errorf("save pipeline: %w", err)
	}

	// Disk-hash recheck: the file may have changed while the checks ran.
	now, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if RawHash(now) != req.BaseHash {
		return nil, &DiskHashConflictError{Path: path}
	}

	if err := atomicWrite(path, []byte(spliced)); err != nil {
		return nil, err
	}
	return &SaveResult{Hash: RawHash([]byte(spliced)), Raw: spliced}, nil
}

// patchModule replaces the in-memory config of the module living at dir —
// or appends it when the profile has no such module (a fresh scaffold the
// loader could not discover yet).
func patchModule(p *profile.Profile, dir string, cfg *profile.ModuleConfig) error {
	want, err := filepath.Abs(dir)
	if err != nil {
		want = filepath.Clean(dir)
	}
	for i := range p.Modules {
		have, err := filepath.Abs(p.Modules[i].Path)
		if err != nil {
			have = filepath.Clean(p.Modules[i].Path)
		}
		if have == want {
			p.Modules[i].Config = *cfg
			if p.Modules[i].ID == "" {
				p.Modules[i].ID = cfg.ID
			}
			return nil
		}
	}
	id := cfg.ID
	if id == "" {
		id = filepath.Base(dir)
	}
	p.Modules = append(p.Modules, profile.Module{ID: id, Path: dir, Config: *cfg})
	return nil
}

// atomicWrite writes data to path via a temp file in the same directory
// plus a rename — a crash or a refused rename never leaves a half file.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".module-*.toml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// RawHash is the disk-hash the draft ledger baselines against (0065-D8);
// exported so the TUI's raw-repair draft — forked from a SchemaError,
// which carries no ModuleLayerRead — can name its base.
func RawHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Layer names for the module operations: "" or "base" is the base layer;
// an overlay layer is "hosts/<hostname>" or "users/<username>" — the same
// labels the status report groups by (onboard's layerLabel).

// ModuleDir returns the module directory for (layer, name).
func ModuleDir(root, layer, name string) string {
	if layer == "" || layer == "base" {
		return filepath.Join(root, "modules", name)
	}
	return filepath.Join(root, filepath.FromSlash(layer), "modules", name)
}

// CreateModule scaffolds a minimal module (id, nothing else —
// sections are added through the editors themselves, 0065-D5) in the
// given layer. Refused when the layer already has that module.
func (a *ConfigArea) CreateModule(name, layer string) error {
	if name == "" {
		return errors.New("create module: a module name is required")
	}
	dir := ModuleDir(a.root, layer, name)
	if _, err := os.Stat(filepath.Join(dir, "module.toml")); err == nil {
		return fmt.Errorf("create module: %s already has module %q", layerLabelName(layer), name)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	scaffold := profile.EncodeKeysSection(profile.ModuleConfig{ID: name})
	if err := atomicWrite(filepath.Join(dir, "module.toml"), []byte(scaffold)); err != nil {
		return err
	}
	return nil
}

// MoveModule moves a module directory wholesale between layers (module
// dirs are self-contained per contract 8 — no content merging, ever).
// Refused when the target layer already has that module.
func (a *ConfigArea) MoveModule(name, fromLayer, toLayer string) error {
	src := ModuleDir(a.root, fromLayer, name)
	dst := ModuleDir(a.root, toLayer, name)
	if _, err := os.Stat(filepath.Join(dst, "module.toml")); err == nil {
		return fmt.Errorf("move module: %s already has module %q", layerLabelName(toLayer), name)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("move module: %s has no module %q", layerLabelName(fromLayer), name)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// OverrideModule seeds an overlay for an existing module in a higher
// layer (issue 0082): the new module.toml carries comments only — an
// empty overlay overrides nothing, and a copied base would pin its
// fields against later base edits. Refused when the source layer has no
// such module or the target layer already has one.
func (a *ConfigArea) OverrideModule(name, fromLayer, toLayer string) error {
	src := ModuleDir(a.root, fromLayer, name)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("override module: %s has no module %q", layerLabelName(fromLayer), name)
	}
	dst := ModuleDir(a.root, toLayer, name)
	if _, err := os.Stat(filepath.Join(dst, "module.toml")); err == nil {
		return fmt.Errorf("override module: %s already has module %q", layerLabelName(toLayer), name)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	seed := "# overlay of " + name + " (" + layerLabelName(fromLayer) + "): fields set here override it.\n" +
		"# merged: packages, tools, dotfiles, hooks, mounts, smb\n" +
		"# meta, scope, and when come from " + layerLabelName(fromLayer) + "\n"
	return atomicWrite(filepath.Join(dst, "module.toml"), []byte(seed))
}

// DeletePreview lists the module's own files that no declaration
// references — the same orphan definition as the status report, scoped to
// the dying directory: strays the module carries but never managed. The
// confirm dialog shows the set before the deletion.
func (a *ConfigArea) DeletePreview(name, layer string) ([]string, error) {
	dir := ModuleDir(a.root, layer, name)
	referenced := drift.ReferencedPaths(ModuleLayersAt(a.root))
	var orphans []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // absent/unreadable module dir: nothing to preview
		}
		if filepath.Base(path) == "module.toml" {
			return nil
		}
		if !referenced[path] {
			rel, rerr := filepath.Rel(dir, path)
			if rerr != nil {
				rel = path
			}
			orphans = append(orphans, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(orphans)
	return orphans, nil
}

// DeleteModule removes a module directory wholesale. The caller shows the
// orphan preview first (DeletePreview) — the service area does what it is
// told.
func (a *ConfigArea) DeleteModule(name, layer string) error {
	return os.RemoveAll(ModuleDir(a.root, layer, name))
}

// ModuleLayersAt lists EVERY module layer directory under a profile root:
// modules/*, hosts/*/modules/*, users/*/modules/*.
func ModuleLayersAt(root string) []drift.ModuleLayer {
	var layers []drift.ModuleLayer
	add := func(layer, owner, moduleDir string) {
		layers = append(layers, drift.ModuleLayer{
			Dir: filepath.Base(moduleDir), Layer: layer, Owner: owner, Path: moduleDir,
		})
	}
	entries := func(dir string) []string {
		list, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var names []string
		for _, e := range list {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		return names
	}
	for _, name := range entries(filepath.Join(root, "modules")) {
		add("base", "", filepath.Join(root, "modules", name))
	}
	for _, owner := range entries(filepath.Join(root, "hosts")) {
		for _, name := range entries(filepath.Join(root, "hosts", owner, "modules")) {
			add("host", owner, filepath.Join(root, "hosts", owner, "modules", name))
		}
	}
	for _, owner := range entries(filepath.Join(root, "users")) {
		for _, name := range entries(filepath.Join(root, "users", owner, "modules")) {
			add("user", owner, filepath.Join(root, "users", owner, "modules", name))
		}
	}
	return layers
}

// layerLabelName names a layer for error messages: "base",
// "hosts/<h>", "users/<u>".
func layerLabelName(layer string) string {
	if layer == "" {
		return strings.TrimSpace("base")
	}
	return layer
}
