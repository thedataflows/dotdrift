package drift

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sort"

	"github.com/thedataflows/dotdrift/internal/resolve"
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
// no [dotfiles] entry of that module references — neither explicitly
// (source = "...") nor implicitly (the source subtree of a symlink-each
// entry, attributed to the layer whose module.toml DECLARES the entry).
// Findings land in the "orphans" section grouped by layer root (Group =
// "base" / "hosts/<hostname>" / "users/<username>", Module = the bare
// module dir), so stale content is visible per host/user/module.
// module.toml is the manifest itself and never an orphan.
func CheckOrphans(plan *resolve.Plan, layers []ModuleLayer) []Finding {
	byKey := make(map[string]ModuleLayer, len(layers))
	for _, ml := range layers {
		byKey[ml.Dir+"/"+ml.Layer] = ml
	}
	referenced := referencedSources(plan, byKey)
	var findings []Finding
	for _, ml := range layers {
		err := filepath.WalkDir(ml.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil // absent layer
				}
				return err
			}
			if d.IsDir() {
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

// referencedSources maps every absolute profile-side file the plan uses to
// true. An entry's reference set is its source file, or — when the source
// is a directory — the WHOLE SUBTREE in the layer whose module.toml
// DECLARES the entry (e.Layer): mise links/copies directory trees
// wholesale, so every nested file deploys with the entry and none of them
// is an orphan. The declaring layer is the anchor, not the resolved source
// dir: resolve picks the highest-precedence layer holding the source path,
// so an overlay that happens to contain a dir at the same rel-path wins
// resolution — its tree is what apply deploys — while the declaring
// layer's tree stays the authored reference and any extra overlay files
// remain orphans.
func referencedSources(plan *resolve.Plan, byKey map[string]ModuleLayer) map[string]bool {
	referenced := map[string]bool{}
	if plan == nil {
		return referenced
	}
	for _, e := range plan.Dotfiles.Entries {
		// A mode = "edit" entry's source was consumed at resolve time
		// (contents inlined into Block, Source cleared) — the file is still
		// the authored source and must not be an orphan.
		if e.EditSource != "" {
			referenced[e.EditSource] = true
		}
		if e.Source == "" {
			continue // inline line/block edit — no on-disk source
		}
		src := declaringSourceTree(e, byKey)
		info, err := os.Stat(src)
		if err != nil || !info.IsDir() {
			// Missing or a plain file: the exact path is the reference.
			// (Missing sources are already reported in the dotfiles
			// section; an unreadable anchored dir resolves here too.)
			referenced[src] = true
			continue
		}
		_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				referenced[path] = true
			}
			return nil // unreadable/missing dirs already reported in dotfiles
		})
	}
	return referenced
}

// declaringSourceTree returns the source path whose subtree counts as
// referenced: the declaring layer's tree at the entry's rel-path when it
// can be derived, falling back to the resolved source dir. rel is derived
// by locating the layer whose path prefixes the resolved Source.
func declaringSourceTree(e resolve.DotfileEntry, byKey map[string]ModuleLayer) string {
	for _, ml := range byKey {
		if ml.Path == "" || !strings.HasPrefix(e.Source, ml.Path+string(filepath.Separator)) {
			continue
		}
		rel, err := filepath.Rel(ml.Path, e.Source)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			break
		}
		if declaring, ok := byKey[e.Module+"/"+e.Layer]; ok && declaring.Path != "" {
			return filepath.Join(declaring.Path, rel)
		}
		break
	}
	return e.Source
}
