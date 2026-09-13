package cmd

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/thedataflows/dotdrift/internal/generate"
)

// Plumbing shared by the generate subcommands: the --list-volumes table
// rendering. Target/facts resolution and the module write run through the
// service writes area (T-tui-writes). Mode selection lived here until
// generate went CLI-only (issue 0066); the required-flag validation in
// each subcommand is the whole no-input story now.

// printGenerateVolumes renders the --list-volumes table over the
// service-classified volumes (already annotated MANAGED when the volume's
// UUID already appears as a source in the target module's [mounts]
// section; an absent module is tolerated: nothing managed).
func printGenerateVolumes(out io.Writer, vols []generate.Volume) error {
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
