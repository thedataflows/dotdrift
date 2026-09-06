package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ReasonMisplacedModule prefixes the Skip.Reason of a module directory that
// sits directly under users/<name>/ or hosts/<host>/ — missing the modules/
// level every code path scans (issue 0033). The full reason names the
// expected path.
const ReasonMisplacedModule = "misplaced"

// markMisplacedModules appends a skip entry for every module.toml-bearing
// directory directly under users/<name>/ or hosts/<host>/ (i.e. missing the
// modules/ level). Without an entry the directory is silently invisible to
// discovery, selection, multi-account status, and the orphan scan alike.
// Every host/account dir is scanned regardless of the current machine — a
// misplaced module is profile-content drift, visible from anywhere (the same
// rule the orphan scan follows).
func (p *Profile) markMisplacedModules(root string) error {
	var skips []Skip
	for _, kind := range []string{"hosts", "users"} {
		owners, err := os.ReadDir(filepath.Join(root, kind))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		for _, owner := range owners {
			if !owner.IsDir() {
				continue
			}
			ownerDir := filepath.Join(root, kind, owner.Name())
			children, err := os.ReadDir(ownerDir)
			if err != nil {
				return err
			}
			for _, child := range children {
				if !child.IsDir() || child.Name() == "modules" {
					continue
				}
				mod, err := loadModule(filepath.Join(ownerDir, child.Name()), child.Name())
				if err != nil {
					return err
				}
				if mod == nil {
					continue
				}
				skips = append(skips, Skip{Module: *mod, Reason: fmt.Sprintf("%s: module belongs at %s",
					ReasonMisplacedModule,
					filepath.Join(kind, owner.Name(), "modules", child.Name()))})
			}
		}
	}
	sort.Slice(skips, func(i, j int) bool { return skips[i].Module.ID < skips[j].Module.ID })
	p.Skipped = append(p.Skipped, skips...)
	return nil
}
