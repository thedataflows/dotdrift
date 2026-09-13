package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"

	"github.com/thedataflows/dotdrift/internal/service"
)

// restoreHandover runs restore's privileged children (sudo install / sudo
// rm) on the CLI's real terminal: stdout/stderr passthrough, output live.
// The adapter side of the session Handover contract (0064-D9); the
// service builds the commands, the consumer runs them. A test seam;
// sudo's timestamp cache means at most one password prompt per run.
var restoreHandover = func(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RestoreCmd copies backed-up copy-mode destinations back to their live
// targets (the inverse of apply --backup). Resolution and copying run
// through the service writes area (T-tui-writes); this adapter keeps the
// --list browse rendering, which only the CLI shows.
type RestoreCmd struct {
	Targets []string  `arg:"" optional:"" name:"targets" help:"Live target paths to restore (absolute or ~/), as they were when backed up"`
	Profile string    `help:"Path to profile directory" type:"existingdir" default:"."`
	Gen     string    `help:"Restore from this backup generation (timestamp, as shown by --list); default: the newest generation holding each target"`
	DryRun  bool      `help:"List what would be restored without touching anything"`
	List    bool      `help:"List backup generations (per module, or the generations holding the given targets) and exit"`
	Out     io.Writer `kong:"-"`
}

// Run translates flags onto the service writes area.
func (c *RestoreCmd) Run() error {
	if c.List {
		return c.list(c.Profile)
	}
	return service.NewWritesArea(service.WritesDeps{
		Detect:      detectFacts,
		LoadProfile: profileLoad,
	}).Restore(service.RestoreOpts{
		ProfilePath: c.Profile,
		Targets:     c.Targets,
		Gen:         c.Gen,
		DryRun:      c.DryRun,
		Out:         c.writer(),
		Handover:    restoreHandover,
	})
}

func (c *RestoreCmd) writer() io.Writer {
	if c.Out != nil {
		return c.Out
	}
	return os.Stdout
}

// list prints the browse view: every module layer's generations with file
// counts, or — when targets are given — the generations holding each.
func (c *RestoreCmd) list(profilePath string) error {
	_, p, err := loadProfile(profilePath, nil)
	if err != nil {
		return err
	}
	out := c.writer()
	home, _ := os.UserHomeDir()
	hits := service.IndexBackups(service.ModuleLayers(p))

	if len(c.Targets) == 0 {
		counts := map[string]map[string]int{}
		for _, byModule := range hits {
			for dir, list := range byModule {
				if counts[dir] == nil {
					counts[dir] = map[string]int{}
				}
				for _, h := range list {
					counts[dir][h.Gen]++
				}
			}
		}
		var dirs []string
		for dir := range counts {
			dirs = append(dirs, dir)
		}
		sort.Strings(dirs)
		for _, dir := range dirs {
			fmt.Fprintf(out, "%s:\n", service.ModuleRel(p.Root, dir))
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
		target, err := service.NormalizeRestoreTarget(spec, home)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s:\n", spec)
		byModule := hits[target]
		var lines []string
		for dir, list := range byModule {
			for _, h := range list {
				lines = append(lines, fmt.Sprintf("  %s %s", service.ModuleRel(p.Root, dir), h.Gen))
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
