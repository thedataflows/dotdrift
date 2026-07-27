package dotdrift

import (
	"context"
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
	"strings"
	"text/tabwriter"

	"github.com/BurntSushi/toml"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Plumbing shared by the generate subcommands: the TTY probe, mode
// selection, target/facts/user resolution, module-dir location, and the
// --list-volumes and summary renderings.

// isTTY reports whether stdin is a terminal: stdlib-only char-device
// check (golang.org/x/term is not vendored). A deliberate small
// duplication of internal/smb's isTTY; a package-level var so tests can
// substitute it (same pattern as detectFacts in apply.go).
var isTTY = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// generateMode is how a generate subcommand proceeds after flag parsing.
type generateMode int

const (
	// generateModeCLI assembles generate.Input from the input flags and
	// calls generate.WriteModule directly.
	generateModeCLI generateMode = iota
	// generateModeWizard hands control to the interactive TUI wizard.
	generateModeWizard
)

// generateModeQuery carries the mode-selection inputs shared by both
// generate subcommands.
type generateModeQuery struct {
	// Sub is the subcommand name for error messages.
	Sub string
	// TUI is the --tui/--no-tui flag (nil: auto).
	TUI *bool
	// HasInput reports whether any subcommand input flag was given.
	HasInput bool
	// Required names the CLI-mode required flags for the no-TTY error.
	Required string
}

// selectGenerateMode applies the strict mode-selection rules:
//   - --tui forces the wizard (requires a terminal), even with input
//     flags — they pass through the wizard seam for pre-fill;
//   - --no-tui or any input flag selects CLI mode;
//   - with no input flags a terminal selects the wizard, and no terminal
//     is an actionable error naming the flags CLI mode needs.
func selectGenerateMode(q generateModeQuery) (generateMode, error) {
	switch {
	case q.TUI != nil && *q.TUI:
		if !isTTY() {
			return generateModeCLI, fmt.Errorf("generate %s: --tui requires an interactive terminal", q.Sub)
		}
		return generateModeWizard, nil
	case q.TUI != nil:
		return generateModeCLI, nil
	case q.HasInput:
		return generateModeCLI, nil
	case isTTY():
		return generateModeWizard, nil
	default:
		return generateModeCLI, fmt.Errorf("generate %s: no input flags and no terminal for the interactive wizard; CLI mode requires: %s", q.Sub, q.Required)
	}
}

// generateSelection resolves the target selection, filling hostname/
// username from detected facts when the layer needs them and the flag
// was not given (the same detectFacts seam onboard uses).
func generateSelection(sel generate.Selection) (generate.Selection, error) {
	wantHost := sel.Layer == generate.LayerHost && sel.Hostname == ""
	wantUser := sel.Layer == generate.LayerUser && sel.Username == ""
	if !wantHost && !wantUser {
		return sel, nil
	}
	f, err := detectFacts()
	if err != nil {
		return sel, fmt.Errorf("detect: %w", err)
	}
	if wantHost {
		sel.Hostname = f.Hostname
	}
	if wantUser {
		sel.Username = f.Username
	}
	return sel, nil
}

// invokingUser resolves the invoking OS user: numeric uid/gid for
// mount-option token expansion and the username for smb's default user
// list. Unparseable numeric ids are an error.
func invokingUser() (uid int, gid int, username string, err error) {
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

// generateModuleDir mirrors generate's (unexported) layer placement so
// the command can locate the module dir without writing: base →
// <root>/modules/<id>, host → <root>/hosts/<hostname>/modules/<id>,
// user → <root>/users/<username>/modules/<id>.
func generateModuleDir(root string, sel generate.Selection) (string, error) {
	if sel.ModuleID == "" {
		return "", fmt.Errorf("empty module id")
	}
	switch sel.Layer {
	case generate.LayerBase:
		return filepath.Join(root, "modules", sel.ModuleID), nil
	case generate.LayerHost:
		if sel.Hostname == "" {
			return "", fmt.Errorf("host layer for module %q requires a hostname", sel.ModuleID)
		}
		return filepath.Join(root, "hosts", sel.Hostname, "modules", sel.ModuleID), nil
	case generate.LayerUser:
		if sel.Username == "" {
			return "", fmt.Errorf("user layer for module %q requires a username", sel.ModuleID)
		}
		return filepath.Join(root, "users", sel.Username, "modules", sel.ModuleID), nil
	default:
		return "", fmt.Errorf("unknown layer %q for module %q", sel.Layer, sel.ModuleID)
	}
}

// existingMountSources decodes the target module's module.toml (when
// present) and returns its mounts' sources in name order; an absent
// file yields nil (nothing managed).
func existingMountSources(dir string) ([]string, error) {
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

// printGenerateVolumes renders the --list-volumes table: local volumes
// the registry classifies as KindVolume, annotated MANAGED when the
// volume's UUID already appears as a source in the target module's
// [mounts] section (an absent module is tolerated: nothing managed).
func printGenerateVolumes(out io.Writer, root string, sel generate.Selection) error {
	dir, err := generateModuleDir(root, sel)
	if err != nil {
		return err
	}
	sources, err := existingMountSources(dir)
	if err != nil {
		return err
	}
	reg, err := generate.Load()
	if err != nil {
		return err
	}
	vols, err := generate.Volumes(context.Background(), reg, sources)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "UUID\tFSTYPE\tLABEL\tSIZE\tMOUNTPOINTS\tMANAGED"); err != nil {
		return err
	}
	for _, v := range vols {
		mountpoints := strings.Join(v.Mountpoints, ",")
		if mountpoints == "" {
			mountpoints = "-"
		}
		managed := "-"
		if v.Managed {
			managed = "yes"
		}
		label := v.Label
		if label == "" {
			label = "-"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			v.UUID, v.FSType, label, v.Size, mountpoints, managed); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// printGenerateSummary lists what WriteModule materialized: the module
// dir and its files (os.ReadDir is name-sorted).
func printGenerateSummary(out io.Writer, root string, sel generate.Selection) error {
	dir, err := generateModuleDir(root, sel)
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
