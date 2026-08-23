package drift

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/thedataflows/dotdrift/internal/resolve"
)

// ModuleLayer names one layer directory of one module: the module's
// directory name (the layer-merge key), the layer it lives in, and the
// absolute path. Callers derive these from the profile layout
// (modules/<dir>, hosts/<hostname>/modules/<dir>, users/<username>/modules/<dir>).
type ModuleLayer struct {
	Dir   string // module directory name (dotfile-entry Module key)
	Layer string // "base" | "host" | "user"
	Path  string // absolute layer module directory
}

// orphanDetail is the detail line for every orphan finding.
const orphanDetail = "not referenced by [dotfiles]"

// CheckOrphans reports files inside the given module layer directories that
// no [dotfiles] entry of that module references — neither explicitly
// (source = "...") nor implicitly (a direct child of a symlink-each source
// directory). Findings land in the "orphans" section, attributed
// "<dir> [<layer>]", so stale module content is visible per host/user/module.
// module.toml is the manifest itself and never an orphan. Files shadowed by
// a higher layer's source of the same name are orphans too — apply resolves
// sources top-down and never reads them.
func CheckOrphans(plan *resolve.Plan, layers []ModuleLayer) []Finding {
	referenced := referencedSources(plan)
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
				Item:    rel,
				Status:  Drift,
				Detail:  orphanDetail,
				Module:  fmt.Sprintf("%s [%s]", ml.Dir, ml.Layer),
			})
			return nil
		})
		if err != nil {
			continue // unreadable layer dir: not drift, skip
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Module != findings[j].Module {
			return findings[i].Module < findings[j].Module
		}
		return findings[i].Item < findings[j].Item
	})
	return findings
}

// referencedSources maps every absolute profile-side file the plan uses to
// true: whole-file entry sources, template-edit sources, and the direct
// file children of symlink-each source directories (what apply deploys).
// Nested files under a symlink-each source are NOT deployed (only direct
// children) and stay unreferenced.
func referencedSources(plan *resolve.Plan) map[string]bool {
	referenced := map[string]bool{}
	if plan == nil {
		return referenced
	}
	for _, e := range plan.Dotfiles.Entries {
		if e.Source == "" {
			continue // inline line/block edit — no on-disk source
		}
		if e.Mode == "symlink-each" {
			children, err := os.ReadDir(e.Source)
			if err != nil {
				continue // missing dir already reported in the dotfiles section
			}
			for _, c := range children {
				if !c.IsDir() {
					referenced[filepath.Join(e.Source, c.Name())] = true
				}
			}
			continue
		}
		referenced[e.Source] = true
	}
	return referenced
}
