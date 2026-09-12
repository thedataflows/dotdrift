package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/thedataflows/dotdrift/internal/generate"
)

// Plumbing shared by the generate subcommands: target/facts/user
// resolution, module-dir location, and the --list-volumes and summary
// renderings. Mode selection lived here until generate went CLI-only
// (issue 0066); the required-flag validation in each subcommand is the
// whole no-input story now.

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
	sources, err := generate.ExistingMountSources(root, sel)
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
