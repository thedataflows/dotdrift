package dotdrift

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/tui"
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

// printGenerateVolumes renders the --list-volumes table: local volumes
// the registry classifies as KindVolume, annotated MANAGED when the
// volume's UUID already appears as a source in the target module's
// [mounts] section (an absent module is tolerated: nothing managed).
func printGenerateVolumes(out io.Writer, root string, sel generate.Selection) error {
	sources, err := tui.ExistingMountSources(root, sel)
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
