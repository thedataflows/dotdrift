package tui

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// InvokingUser resolves the invoking OS user: numeric uid/gid for
// mount-option token expansion and the username for smb's default user
// list. Unparseable numeric ids are an error.
func InvokingUser() (uid int, gid int, username string, err error) {
	u, err := user.Current()
	if err != nil {
		return 0, 0, "", fmt.Errorf("resolve current user: %w", err)
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, "", fmt.Errorf("parse current user uid %q: %w", u.Uid, err)
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, "", fmt.Errorf("parse current user gid %q: %w", u.Gid, err)
	}
	return uid, gid, u.Username, nil
}

// ExistingMountSources returns the mount sources the target module
// already manages (for MANAGED volume marking), sorted by mount name. An
// absent module yields nil, nil; a malformed module.toml is a loud error.
func ExistingMountSources(root string, sel generate.Selection) ([]string, error) {
	dir, err := generate.ModuleDir(root, sel)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "module.toml")
	var cfg profile.ModuleConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	sources := make([]string, 0, len(cfg.Mounts))
	for _, name := range slices.Sorted(maps.Keys(cfg.Mounts)) {
		sources = append(sources, cfg.Mounts[name].Source)
	}
	return sources, nil
}

// PrintSummary lists what a generate run materialized: the module dir
// and its files, name-sorted, written to the caller's writer (stdout for
// the CLI, stderr for the wizard).
func PrintSummary(out io.Writer, root string, sel generate.Selection) error {
	dir, err := generate.ModuleDir(root, sel)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read module dir %s: %w", dir, err)
	}
	if _, err := fmt.Fprintf(out, "wrote %s:\n", dir); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, err := fmt.Fprintf(out, "  %s\n", e.Name()); err != nil {
			return err
		}
	}
	return nil
}
