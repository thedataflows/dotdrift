package generate

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// gcStaleUnits removes previously generated unit files that are not in the
// freshly rendered set. "Generated" is defined precisely: the file name
// matches the unit naming scheme AND module.toml tracks it via a dotfile
// entry whose target is under unitTargetDir. Anything else in the module
// dir — smb.conf, untracked files, user dotfiles — is never touched. The
// stale dotfile entries are dropped from cfg in place.
func gcStaleUnits(dir string, dotfiles map[string]profile.Dotfile, rendered map[string]string) {
	for _, target := range slices.Sorted(maps.Keys(dotfiles)) {
		df := dotfiles[target]
		if !strings.HasPrefix(target, unitTargetDir+"/") || !isUnitFile(df.Source) {
			continue
		}
		if _, ok := rendered[filepath.Base(df.Source)]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, df.Source)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			// Removal failed: keep the entry so file and tracking stay consistent.
			continue
		}
		delete(dotfiles, target)
		log.Warn().Msgf("removed generated unit %s from the module; the placed unit may still be live — run: sudo systemctl disable --now %s && sudo rm %s/%s", df.Source, df.Source, unitTargetDir, df.Source)
	}
}

// isUnitFile reports whether name matches the generated unit naming scheme.
func isUnitFile(name string) bool {
	for _, suffix := range unitSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
