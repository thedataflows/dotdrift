package drift

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// ModuleLayer names one layer directory of one module: the module's
// directory name (the layer-merge key), the layer it lives in, and the
// absolute path. For host/user layers Owner names the layer's hostname or
// username so orphan group headings read "hosts/<hostname>" /
// "users/<username>". Callers derive these from the profile layout
// (modules/<dir>, hosts/<hostname>/modules/<dir>, users/<username>/modules/<dir>).
type ModuleLayer struct {
	Dir   string // module directory name (dotfile-entry Module key)
	Layer string // "base" | "host" | "user"
	Owner string // hostname (host layer) or username (user layer); empty for base
	Path  string // absolute layer module directory
}

// groupLabel renders the layer-root group heading: "base",
// "hosts/<hostname>", or "users/<username>".
func (ml ModuleLayer) groupLabel() string {
	switch ml.Layer {
	case "host":
		return "hosts/" + ml.Owner
	case "user":
		return "users/" + ml.Owner
	default:
		return "base"
	}
}

// orphanDetail is the detail line for every orphan finding.
const orphanDetail = "not referenced by [dotfiles]"

// CheckOrphans reports files inside the given module layer directories that
// no [dotfiles] declaration of that module references. References come
// from the layer declarations themselves (see ReferencedPaths), not a
// resolved plan, so the scan checks every host/user layer of the profile
// from any machine: a leftover under hosts/<other> or users/<other> is
// reported here, now. module.toml is the manifest itself and never an
// orphan.
func CheckOrphans(layers []ModuleLayer) []Finding {
	referenced := ReferencedPaths(layers)
	var findings []Finding
	seen := map[string]bool{}
	for _, ml := range layers {
		if ml.Path == "" || seen[ml.Path] {
			continue
		}
		seen[ml.Path] = true
		err := filepath.WalkDir(ml.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil // absent layer
				}
				return err
			}
			if d.IsDir() {
				// A backups/ directory directly under the module layer root
				// is dotdrift's runtime output (apply --backup, issue 0025),
				// not profile content. Deeper backups dirs are still
				// content: only the module-root one is machine-written.
				if d.Name() == "backups" && filepath.Dir(path) == ml.Path {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() == "module.toml" {
				return nil
			}
			if referenced[path] {
				return nil
			}
			rel, relErr := filepath.Rel(ml.Path, path)
			if relErr != nil {
				return nil
			}
			findings = append(findings, Finding{
				Section: "orphans",
				Group:   ml.groupLabel(),
				Item:    rel,
				Status:  Drift,
				Detail:  orphanDetail,
				Module:  ml.Dir,
			})
			return nil
		})
		if err != nil {
			continue // unreadable layer dir: not drift, skip
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Group != findings[j].Group {
			return findings[i].Group < findings[j].Group
		}
		if findings[i].Module != findings[j].Module {
			return findings[i].Module < findings[j].Module
		}
		return findings[i].Item < findings[j].Item
	})
	return findings
}

// ReferencedPaths maps every profile-side file some [dotfiles] declaration
// uses to true, across every layer given. Orphans are profile-content
// drift, so references cannot come from a resolved plan (one plan covers
// one machine's view); they come from what the layers declare, evaluated
// per host/user view:
//
//   - A view's effective declarations merge its layers whole-entry by
//     precedence (user > host > base) — an overridden target's old source
//     is stranded in that view, exactly like resolve.
//   - A declaration whose source is a DIRECTORY at the declaring layer
//     references that whole subtree (mise links/copies directory trees
//     wholesale). The declaring layer is the anchor, not whichever layer
//     resolves first: an overlay holding a dir at the same rel-path wins
//     deployment, but the declaring layer's tree stays the authored
//     reference and extra overlay files remain orphans.
//   - A FILE source references the file the view resolves it to (user >
//     host > base, first existing). A base copy shadowed on every host is
//     dead content (orphan); one that still resolves for a host without
//     an overlay copy deploys there and stays referenced.
//
// Layers are grouped by module directory name; a layer without a readable
// module.toml declares nothing but still shapes every view's resolution
// order (its files shadow base copies). An invalid module.toml fails
// every command at load, so decode errors here mean "absent".
func ReferencedPaths(layers []ModuleLayer) map[string]bool {
	referenced := map[string]bool{}

	byModule := map[string][]ModuleLayer{}
	for _, ml := range layers {
		if ml.Path == "" {
			continue
		}
		if info, err := os.Stat(ml.Path); err != nil || !info.IsDir() {
			continue // absent layer dir: shapes no view and declares nothing
		}
		byModule[ml.Dir] = append(byModule[ml.Dir], ml)
	}
	for _, mods := range byModule {
		type declaration struct {
			layer ModuleLayer
			rel   string
		}
		layerDecls := map[string]map[string]declaration{} // layer path -> target -> declaration
		var bases []ModuleLayer
		hosts := map[string]ModuleLayer{}
		users := map[string]ModuleLayer{}
		for _, ml := range mods {
			// Classify the layer even when it declares nothing: an
			// overlay without its own module.toml still shapes every
			// view's resolution order (its files shadow base copies).
			switch ml.Layer {
			case "host":
				hosts[ml.Owner] = ml
			case "user":
				users[ml.Owner] = ml
			default:
				bases = append(bases, ml)
			}
			cfg, err := readLayerDeclarations(ml.Path)
			if err != nil {
				continue // no module.toml: overlay-only layer, nothing declared
			}
			decls := map[string]declaration{}
			for target, df := range cfg.Dotfiles {
				if df.Source != "" {
					decls[target] = declaration{layer: ml, rel: df.Source}
				}
			}
			layerDecls[ml.Path] = decls
		}
		for _, v := range layerViews(bases, hosts, users) {
			// Effective declarations: fold lowest precedence first so a
			// higher layer's whole-entry override replaces the target.
			effective := map[string]declaration{}
			for i := len(v.order) - 1; i >= 0; i-- {
				for target, d := range layerDecls[v.order[i].Path] {
					effective[target] = d
				}
			}
			for _, d := range effective {
				declared := filepath.Join(d.layer.Path, filepath.FromSlash(d.rel))
				if isDir(declared) {
					markTree(declared, referenced)
					continue
				}
				resolved := v.resolve(d.rel)
				if resolved == "" {
					continue
				}
				if isDir(resolved) {
					markTree(resolved, referenced)
				} else {
					referenced[resolved] = true
				}
			}
		}
	}
	return referenced
}

// layerView is one host/user overlay combination over the base layer —
// one machine's resolution order.
type layerView struct {
	order []ModuleLayer // user, host, base
}

// resolve returns the first existing path at rel along the view's layer
// order, or "".
func (v layerView) resolve(rel string) string {
	for _, ml := range v.order {
		p := filepath.Join(ml.Path, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// layerViews enumerates the host/user overlay combinations over base:
// every (host, user) pair among the module's layers. The bare-base view
// exists only when the module has no host and no user layers at all —
// with any overlay present, some real machine always sits above base, so
// a base copy nothing resolves stays unreferenced (shadowed = orphan).
func layerViews(bases []ModuleLayer, hosts, users map[string]ModuleLayer) []layerView {
	var views []layerView
	build := func(h, u ModuleLayer, hasH, hasU bool) {
		var v layerView
		if hasU {
			v.order = append(v.order, u)
		}
		if hasH {
			v.order = append(v.order, h)
		}
		v.order = append(v.order, bases...)
		views = append(views, v)
	}
	if len(hosts) == 0 && len(users) == 0 {
		build(ModuleLayer{}, ModuleLayer{}, false, false)
		return views
	}
	for _, h := range hosts {
		if len(users) == 0 {
			build(h, ModuleLayer{}, true, false)
			continue
		}
		for _, u := range users {
			build(h, u, true, true)
		}
	}
	for _, u := range users {
		build(ModuleLayer{}, u, false, true)
	}
	return views
}

// readLayerDeclarations decodes a layer directory's module.toml. A missing
// file is an error to the caller; callers treat it as "nothing declared".
func readLayerDeclarations(dir string) (*profile.ModuleConfig, error) {
	var cfg profile.ModuleConfig
	path := filepath.Join(dir, "module.toml")
	if err := profile.DecodeModuleTOMLFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// markTree marks root, or — when root is a directory — every file in its
// subtree, as referenced.
func markTree(root string, referenced map[string]bool) {
	if !isDir(root) {
		referenced[root] = true
		return
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			referenced[path] = true
		}
		return nil // unreadable/missing dirs already reported in dotfiles
	})
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
