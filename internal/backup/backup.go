// Package backup snapshots live copy-mode destinations into the profile
// before apply overwrites them. Backups are copy-mode only: symlink
// destinations are recreated as links, and edit entries are marker-scoped,
// so a copy overwrite is the one apply operation that loses destination
// content.
package backup

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// File is one copy-mode destination to snapshot. Target is the live absolute
// path (file or directory); ModuleDir is the module layer directory whose
// module.toml declared the entry — the backup lands in its backups/ subtree,
// next to the profile content that replaces it.
type File struct {
	Target    string
	ModuleDir string
}

// Run copies every existing Target to <ModuleDir>/backups/<gen>/<Target>:
// absolute target paths mirror under the generation directory, so a restore
// is a plain copy back to the mirrored path. gen identifies the run (one
// apply = one generation shared by every module). Missing targets are
// skipped — nothing exists to lose. Returns the number of top-level units
// (files or directories) written. Any unreadable target is an error, not a
// skip: the caller asked for a backup before overwrite, so losing the file
// silently must never happen.
func Run(files []File, gen string) (int, error) {
	if gen == "" {
		return 0, fmt.Errorf("backup: generation is empty")
	}
	written := 0
	for _, f := range files {
		if f.Target == "" || f.ModuleDir == "" {
			return 0, fmt.Errorf("backup: target and module dir are required")
		}
		info, err := os.Stat(f.Target)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return written, fmt.Errorf("backup %s: %w", f.Target, err)
		}
		// os.Stat follows symlinks: a link left by a mode switch is backed up
		// as the content it resolves to — the data the overwrite destroys.
		// ponytail: the link itself is not preserved; upgrade path is
		// Readlink+symlink recreation if a profile ever needs it.
		// Mirror the absolute target path: strip the leading separator so
		// Join nests /home/cri/x as backups/<gen>/home/cri/x (Join would
		// otherwise nest the full absolute path one level deeper).
		dst := filepath.Join(f.ModuleDir, "backups", gen,
			strings.TrimPrefix(f.Target, string(filepath.Separator)))
		if err := copyUnit(f.Target, dst, info); err != nil {
			return written, fmt.Errorf("backup %s: %w", f.Target, err)
		}
		written++
	}
	return written, nil
}

// copyUnit copies one file or directory tree. Permissions are preserved;
// ownership is not (the profile is user-owned).
func copyUnit(src, dst string, info fs.FileInfo) error {
	if info.IsDir() {
		return copyDir(src, dst, info.Mode().Perm())
	}
	return copyFile(src, dst, info.Mode().Perm())
}

func copyDir(src, dst string, perm fs.FileMode) error {
	if err := os.MkdirAll(dst, perm); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		info, err := os.Stat(s)
		if err != nil {
			return err
		}
		if err := copyUnit(s, d, info); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".dotdrift-tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
