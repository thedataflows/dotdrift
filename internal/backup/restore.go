package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Generations lists a module directory's backup generation names
// newest-first (generation names are timestamps, so lexical order is time
// order). A module without a backups/ directory lists empty.
func Generations(moduleDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(moduleDir, "backups"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var gens []string
	for _, e := range entries {
		if e.IsDir() {
			gens = append(gens, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(gens)))
	return gens, nil
}

// RestoreItem is Run's inverse for one target: the copy Run wrote to
// <genDir>/<target-without-leading-slash> lands back at the absolute live
// target. A symlink currently sitting at the target is removed, not
// followed — writing through it would clobber whatever it points at (e.g.
// the profile source after a mode switch). Missing parent directories are
// created; content and file mode come from the backup. A target with no
// backup in the generation is an error naming the target.
func RestoreItem(genDir, target string) error {
	if !filepath.IsAbs(target) {
		return fmt.Errorf("restore: target must be absolute, got %q", target)
	}
	src := filepath.Join(genDir, strings.TrimPrefix(target, string(filepath.Separator)))
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("restore: no backup of %s in %s", target, genDir)
		}
		return fmt.Errorf("restore %s: read backup %s: %w", target, src, err)
	}
	if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("restore %s: remove symlink: %w", target, err)
		}
	}
	if err := copyUnit(src, target, info); err != nil {
		return fmt.Errorf("restore %s: %w", target, err)
	}
	return nil
}
