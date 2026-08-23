package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/backup"
	"github.com/thedataflows/dotdrift/internal/drift"
)

// elevatedRestore copies one backed-up file to a target the current user
// cannot write (system files): `sudo install -D -m <mode>` creates leading
// directories and lands the file root-owned — cp -p would stamp the
// backup's user owner onto /etc. A symlink at the target is removed first
// (install would follow it like cp). A test seam; sudo's timestamp cache
// means at most one password prompt per run.
var elevatedRestore = defaultElevatedRestore

func defaultElevatedRestore(src, dst string, mode os.FileMode) error {
	if fi, err := os.Lstat(dst); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if err := sudoRun("rm", "-f", dst); err != nil {
			return fmt.Errorf("remove symlink (elevated): %w", err)
		}
	}
	return sudoRun("install", "-D", "-m", fmt.Sprintf("%04o", mode.Perm()), src, dst)
}

func sudoRun(args ...string) error {
	cmd := exec.Command("sudo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// restoreHit is one backed-up file: generation gen of moduleDir holds the
// copy at path (absolute) for the target it mirrors.
type restoreHit struct {
	moduleDir string
	gen       string
	path      string
}

// RestoreCmd copies backed-up copy-mode destinations back to their live
// targets (the inverse of apply --backup).
type RestoreCmd struct {
	Targets []string  `arg:"" optional:"" name:"targets" help:"Live target paths to restore (absolute or ~/), as they were when backed up"`
	Profile string    `help:"Path to profile directory" type:"existingdir" default:"."`
	Gen     string    `help:"Restore from this backup generation (timestamp, as shown by --list); default: the newest generation holding each target"`
	DryRun  bool      `help:"List what would be restored without touching anything"`
	List    bool      `help:"List backup generations (per module, or the generations holding the given targets) and exit"`
	Out     io.Writer `kong:"-"`
}

// Run implements the restore command.
func (c *RestoreCmd) Run() error {
	_, p, err := loadProfile(c.Profile, nil)
	if err != nil {
		return err
	}
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	hits := indexBackups(statusModuleLayers(p))
	home, _ := os.UserHomeDir()

	if c.List {
		return c.list(hits, home, p.Root, out)
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("restore: specify at least one target path (--list browses generations)")
	}

	restored := 0
	for _, spec := range c.Targets {
		target, err := normalizeRestoreTarget(spec, home)
		if err != nil {
			return err
		}
		byModule := hits[target]
		if len(byModule) == 0 {
			return fmt.Errorf("restore: no backup of %s (try --list)", target)
		}
		if c.Gen != "" {
			filtered := map[string][]restoreHit{}
			for dir, list := range byModule {
				for _, h := range list {
					if h.gen == c.Gen {
						filtered[dir] = append(filtered[dir], h)
					}
				}
			}
			if len(filtered) == 0 {
				var have []string
				for _, list := range byModule {
					for _, h := range list {
						have = append(have, h.gen)
					}
				}
				sort.Strings(have)
				return fmt.Errorf("restore: generation %s holds no backup of %s (have: %s)",
					c.Gen, target, strings.Join(have, ", "))
			}
			byModule = filtered
		}
		if len(byModule) > 1 {
			var dirs []string
			for dir := range byModule {
				dirs = append(dirs, dir)
			}
			sort.Strings(dirs)
			return fmt.Errorf("restore: %s is backed up by multiple modules (%s); remove the stale backup",
				target, strings.Join(dirs, ", "))
		}
		// One module: hits are newest-first (per module).
		hit := byModule[moduleDirOf(byModule)][0]
		label := moduleRel(p.Root, hit.moduleDir)
		rel := fmt.Sprintf("%s/backups/%s", label, hit.gen)
		if c.DryRun {
			fmt.Fprintf(out, "would restore: %s (%s)\n", target, rel)
			restored++
			continue
		}
		if pathUserWritable(target) {
			if err := backup.RestoreItem(filepath.Join(hit.moduleDir, "backups", hit.gen), target); err != nil {
				return err
			}
		} else {
			info, err := os.Stat(hit.path)
			if err != nil {
				return fmt.Errorf("restore %s: read backup: %w", target, err)
			}
			if err := elevatedRestore(hit.path, target, info.Mode().Perm()); err != nil {
				return fmt.Errorf("restore %s (elevated): %w", target, err)
			}
		}
		fmt.Fprintf(out, "restored: %s (%s)\n", target, rel)
		restored++
	}
	if restored > 0 {
		fmt.Fprintln(out, "note: restored targets may drift from the profile; dotdrift apply overwrites them again - re-onboard a path to keep its restored content")
	}
	return nil
}

// list prints the browse view: every module layer's generations with file
// counts, or — when targets are given — the generations holding each.
func (c *RestoreCmd) list(hits map[string]map[string][]restoreHit, home, profileRoot string, out io.Writer) error {
	if len(c.Targets) == 0 {
		counts := map[string]map[string]int{}
		for _, byModule := range hits {
			for dir, list := range byModule {
				if counts[dir] == nil {
					counts[dir] = map[string]int{}
				}
				for _, h := range list {
					counts[dir][h.gen]++
				}
			}
		}
		var dirs []string
		for dir := range counts {
			dirs = append(dirs, dir)
		}
		sort.Strings(dirs)
		for _, dir := range dirs {
			fmt.Fprintf(out, "%s:\n", dir)
			var gens []string
			for gen := range counts[dir] {
				gens = append(gens, gen)
			}
			sort.Sort(sort.Reverse(sort.StringSlice(gens)))
			for _, gen := range gens {
				fmt.Fprintf(out, "  %s  %d file(s)\n", gen, counts[dir][gen])
			}
		}
		return nil
	}
	for _, spec := range c.Targets {
		target, err := normalizeRestoreTarget(spec, home)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s:\n", spec)
		byModule := hits[target]
		var lines []string
		for dir, list := range byModule {
			for _, h := range list {
				lines = append(lines, fmt.Sprintf("  %s %s", moduleRel(profileRoot, dir), h.gen))
			}
		}
		if len(lines) == 0 {
			fmt.Fprintf(out, "  (no backup)\n")
			continue
		}
		sort.Sort(sort.Reverse(sort.StringSlice(lines)))
		for _, line := range lines {
			fmt.Fprintln(out, line)
		}
	}
	return nil
}

// indexBackups walks every module layer's backup generations and maps each
// backed-up file to its live target: the mirrored path under a generation
// directory IS the absolute target (minus the leading separator). Hits per
// module are newest-first (generations iterate newest-first).
func indexBackups(layers []drift.ModuleLayer) map[string]map[string][]restoreHit {
	hits := map[string]map[string][]restoreHit{}
	for _, ml := range layers {
		if ml.Path == "" {
			continue
		}
		gens, err := backup.Generations(ml.Path)
		if err != nil {
			continue // unreadable backups dir: nothing to restore from
		}
		for _, gen := range gens { // newest-first
			genDir := filepath.Join(ml.Path, "backups", gen)
			_ = filepath.WalkDir(genDir, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				rel, relErr := filepath.Rel(genDir, path)
				if relErr != nil {
					return nil
				}
				target := filepath.Clean(string(filepath.Separator) + filepath.ToSlash(rel))
				if hits[target] == nil {
					hits[target] = map[string][]restoreHit{}
				}
				hits[target][ml.Path] = append(hits[target][ml.Path], restoreHit{
					moduleDir: ml.Path, gen: gen, path: path,
				})
				return nil
			})
		}
	}
	return hits
}

// normalizeRestoreTarget expands ~ and requires an absolute path: the
// mirrored backup layout keys on absolute targets.
func normalizeRestoreTarget(spec, home string) (string, error) {
	t := spec
	if t == "~" || strings.HasPrefix(t, "~/") {
		t = filepath.Join(home, strings.TrimPrefix(t, "~"))
	}
	if !filepath.IsAbs(t) {
		return "", fmt.Errorf("restore: target %q must be absolute or start with ~", spec)
	}
	return filepath.Clean(t), nil
}

// moduleDirOf returns the single key of a one-entry map (the caller has
// already established there is exactly one module).
func moduleDirOf(m map[string][]restoreHit) string {
	for k := range m {
		return k
	}
	return ""
}

// moduleRel renders a module directory as a profile-relative label
// (modules/<m>, hosts/<h>/modules/<m>, ...).
func moduleRel(root, moduleDir string) string {
	rel, err := filepath.Rel(root, moduleDir)
	if err != nil {
		return moduleDir
	}
	return rel
}
