package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// discover scans the module directories of every active layer — base, then
// host, then user — so presence = managed holds per layer: a module existing
// only in a host or user layer is selected like a base module. Layers are
// keyed by directory name: a host/user directory with the same name as an
// already-discovered module is its overlay (merged later by resolve), not a
// second module. The first layer containing a directory name wins, so the
// representative Module path/config is base-preferred. Declared-id collisions
// across different directory names remain fatal. Empty hostname/username
// facts skip that layer's scan; missing layer module directories are absent
// layers, not errors (only the base modules/ directory is required).
func (p *Profile) discover(root string, f *facts.Facts) error {
	layers := []string{filepath.Join(root, "modules")}
	if f.Hostname != "" {
		layers = append(layers, filepath.Join(root, "hosts", f.Hostname, "modules"))
	}
	if f.Username != "" {
		layers = append(layers, filepath.Join(root, "users", f.Username, "modules"))
	}
	byDir := make(map[string]int) // dirName → index in p.Modules
	seenIDs := make(map[string]string)
	for i, layerDir := range layers {
		entries, err := os.ReadDir(layerDir)
		if err != nil {
			if os.IsNotExist(err) {
				if i == 0 {
					return fmt.Errorf("not a dotdrift profile: %s missing modules/ directory", root)
				}
				continue
			}
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			modPath := filepath.Join(layerDir, entry.Name())
			mod, err := loadModule(modPath, entry.Name())
			if err != nil {
				return err
			}
			if mod == nil {
				continue
			}
			if prevIdx, ok := byDir[entry.Name()]; ok {
				// Overlay: a higher layer's module.toml may set disabled = true
				// to disable the module the base layer discovered. Any disable
				// sticks (union semantics, same as dotdrift.toml's disable list).
				if mod.Config.Disabled {
					p.Modules[prevIdx].Config.Disabled = true
				}
				continue
			}
			if prev, dup := seenIDs[mod.ID]; dup {
				return fmt.Errorf("duplicate module id %q: %s and %s", mod.ID, prev, modPath)
			}
			seenIDs[mod.ID] = modPath
			byDir[entry.Name()] = len(p.Modules)
			p.Modules = append(p.Modules, *mod)
		}
	}
	sort.Slice(p.Modules, func(i, j int) bool { return p.Modules[i].ID < p.Modules[j].ID })
	return nil
}

func loadModule(path, dirName string) (*Module, error) {
	modToml := filepath.Join(path, "module.toml")
	if _, err := os.Stat(modToml); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg ModuleConfig
	if _, err := toml.DecodeFile(modToml, &cfg); err != nil {
		return nil, fmt.Errorf("decode %s: %w", modToml, err)
	}
	if cfg.ID == "" {
		cfg.ID = dirName
	}
	if err := validateWhenKernel(cfg.ID, cfg.When.Kernel); err != nil {
		return nil, err
	}
	if cfg.App == "" {
		cfg.App = cfg.ID
	}
	return &Module{ID: cfg.ID, App: cfg.App, Path: path, Config: cfg}, nil
}
