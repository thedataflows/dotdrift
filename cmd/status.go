package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/palette"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/state"
)

// StatusCmd reports drift between the resolved profile and the live system,
// plus the apply resume cursor. Read-only; exits 0 even when drift is found.
type StatusCmd struct {
	Profile string   `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string   `help:"Path to state file" type:"path" default:""`
	Verbose bool     `help:"Show each probe as it starts ('checking <section>: <item>') on stderr" short:"v" default:"false"`
	Jobs    int      `help:"Concurrent probe workers (0 = number of CPUs)" short:"j" default:"0"`
	Diff    string   `help:"Show diff for files whose content differs; bare = internal diff, --diff=tool uses the named tool" default:""`
	Modules []string `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
	out     io.Writer
	err     io.Writer
}

// Run loads state, resolves the plan, probes the live system for drift, and
// prints the report. Only real errors (detect/load/resolve/corrupt state)
// return non-nil; drift is reported, never an error.
func (c *StatusCmd) Run() error {
	statePath := c.State
	if statePath == "" {
		statePath = state.ProfileStatePath(c.Profile)
	}
	store := state.NewFileStore(statePath)
	s, err := store.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	f, p, plan, err := loadAndResolve(c.Profile, c.Modules)
	if err != nil {
		return err
	}
	pr := drift.DefaultProbes()
	pr.IsInstalled = packagesFor(f.Backend).IsInstalled
	pr.ToolCurrent = mise.NewExecMise(defaultMise()).Current
	pr = elevateProbes(pr) // retry elevated on permission denied (system files)
	profileRoot, _ := filepath.Abs(p.Root)
	opts := drift.CheckOptions{Jobs: c.Jobs}
	if c.Verbose {
		errW := c.err
		if errW == nil {
			errW = os.Stderr
		}
		opts.Verbose = errW
	}
	findings := drift.Check(context.Background(), plan, profileRoot, pr, opts)
	findings = append(findings, drift.CheckOrphans(statusModuleLayers(p))...)

	out := c.out
	if out == nil {
		out = os.Stdout
	}
	pal, err := palette.FromConfig(p.Config.Colors)
	if err != nil {
		return err // already validated at load; unreachable double-check
	}
	fmt.Fprintf(out, "profile: %s\n", c.Profile)
	fmt.Fprintf(out, "state: %s\n", statePath)
	// The resume line's MESSAGE carries a role hue on a TTY (the literal
	// `resume: ` prefix stays plain): ok when clean, warn when a cursor
	// is pending (an interrupted apply awaits resuming).
	resumeMsg := "clean - next apply starts from the beginning"
	resumeRole := palette.OK
	if s.LastCompleted != "" {
		resumeMsg = fmt.Sprintf("last completed %q - next apply resumes after it", s.LastCompleted)
		resumeRole = palette.Warn
	}
	if executil.ColorEnabled(out) {
		resumeMsg = pal.Wrap(resumeRole, resumeMsg)
	}
	fmt.Fprintf(out, "resume: %s\n", resumeMsg)
	drift.Render(out, findings, drift.WithPalette(pal))

	if c.Diff != "" {
		if err := showDotfileDiffs(plan, profileRoot, c.Diff, out); err != nil {
			return err
		}
	}
	return nil
}

// sudoRead executes `sudo <name> <args>` and returns stdout. A test seam so
// status tests can assert elevation behavior without running real sudo.
var sudoRead = func(name string, args ...string) ([]byte, error) {
	return exec.Command("sudo", append([]string{name}, args...)...).Output()
}

// elevateProbes wraps the file-access probes (Readlink, ReadFile, StatDir) so
// each retries elevated via sudo when the OS denies access. Only permission
// denied triggers the retry — other errors (not-exist, invalid path) propagate
// as-is. Already-root processes skip the wrapping entirely. Run (external
// commands like systemctl/getent) is left untouched; those commands work for
// normal users in the read-only status path.
func elevateProbes(pr drift.Probes) drift.Probes {
	if os.Geteuid() == 0 {
		return pr
	}
	if pr.Readlink != nil {
		pr.Readlink = elevateReadlink(pr.Readlink)
	}
	if pr.ReadFile != nil {
		pr.ReadFile = elevateReadFile(pr.ReadFile)
	}
	if pr.StatDir != nil {
		pr.StatDir = elevateStatDir(pr.StatDir)
	}
	if pr.ListDir != nil {
		pr.ListDir = elevateListDir(pr.ListDir)
	}
	return pr
}

// statusModuleLayers lists EVERY module layer directory in the profile:
// modules/*, hosts/*/modules/*, users/*/modules/*. Orphans are
// profile-content drift, not live-system drift — a leftover under
// another host's or user's overlay is visible from any machine, and a
// module not selected here (when-filter) is still scanned. Reference
// semantics live in drift.ReferencedPaths (layer declarations, all
// views), so no plan or facts are needed here.
func statusModuleLayers(p *profile.Profile) []drift.ModuleLayer {
	if p == nil || p.Root == "" {
		return nil
	}
	var layers []drift.ModuleLayer
	add := func(layer, owner, moduleDir string) {
		layers = append(layers, drift.ModuleLayer{
			Dir: filepath.Base(moduleDir), Layer: layer, Owner: owner, Path: moduleDir,
		})
	}
	// moduleDirs lists the subdirectories of a modules/ root.
	moduleDirs := func(root string) []string {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil
		}
		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
		return dirs
	}
	for _, d := range moduleDirs(filepath.Join(p.Root, "modules")) {
		add("base", "", d)
	}
	owners := func(kind string) []string {
		entries, err := os.ReadDir(filepath.Join(p.Root, kind))
		if err != nil {
			return nil
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		return names
	}
	for _, h := range owners("hosts") {
		for _, d := range moduleDirs(filepath.Join(p.Root, "hosts", h, "modules")) {
			add("host", h, d)
		}
	}
	for _, u := range owners("users") {
		for _, d := range moduleDirs(filepath.Join(p.Root, "users", u, "modules")) {
			add("user", u, d)
		}
	}
	return layers
}

// elevate wraps a probe function so it retries elevated via sudo when the OS
// denies access. The inner function runs first; on permission denied, sudo
// runs and its result is returned. When sudo itself fails, the original
// permission error is returned.
func elevate[T any](inner func(string) (T, error), sudo func(string) (T, error)) func(string) (T, error) {
	return func(path string) (T, error) {
		result, err := inner(path)
		if err == nil || !os.IsPermission(err) {
			return result, err
		}
		sudoResult, sudoErr := sudo(path)
		if sudoErr != nil {
			return result, err
		}
		return sudoResult, nil
	}
}

func elevateReadlink(inner func(string) (string, error)) func(string) (string, error) {
	return elevate(inner, func(path string) (string, error) {
		out, err := sudoRead("readlink", path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	})
}

func elevateReadFile(inner func(string) ([]byte, error)) func(string) ([]byte, error) {
	return elevate(inner, func(path string) ([]byte, error) {
		return sudoRead("cat", path)
	})
}

func elevateStatDir(inner func(string) (bool, error)) func(string) (bool, error) {
	return elevate(inner, func(path string) (bool, error) {
		out, err := sudoRead("stat", "-c", "%F", path)
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(string(out)) == "directory", nil
	})
}

func elevateListDir(inner func(string) ([]string, error)) func(string) ([]string, error) {
	return elevateSlice(inner, func(path string) ([]string, error) {
		out, err := sudoRead("ls", "-1", path)
		if err != nil {
			return nil, err
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			return nil, nil
		}
		return lines, nil
	})
}

// elevateSlice is elevate for slice-returning probes (ListDir): retry
// elevated via sudo when the OS denies access.
func elevateSlice[T any](inner func(string) ([]T, error), sudo func(string) ([]T, error)) func(string) ([]T, error) {
	return func(path string) ([]T, error) {
		result, err := inner(path)
		if err == nil || !os.IsPermission(err) {
			return result, err
		}
		sudoResult, sudoErr := sudo(path)
		if sudoErr != nil {
			return result, err
		}
		return sudoResult, nil
	}
}
