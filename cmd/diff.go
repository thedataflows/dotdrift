package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// showDotfileDiffs prints unified diffs for copy-mode dotfiles whose content
// differs between the profile source and the live target. Called when apply
// --diff is set, after the plan is printed and before the pipeline runs.
// tool is "internal" for the built-in diff or a command name for external.
func showDotfileDiffs(plan *resolve.Plan, profileRoot string, tool string, out io.Writer) {
	home, _ := os.UserHomeDir()
	color := executil.ColorEnabled(out)
	for _, e := range plan.Dotfiles.Entries {
		if e.Mode != "copy" {
			continue
		}
		files, err := mise.ResolveBootstrapFiles([]resolve.DotfileEntry{e}, profileRoot, home)
		if err != nil {
			continue
		}
		for _, f := range files {
			src, err := os.ReadFile(f.Source)
			if err != nil {
				continue
			}
			tgt, err := os.ReadFile(f.Target)
			if err != nil {
				continue // missing target — nothing to diff
			}
			if string(src) == string(tgt) {
				continue
			}
			fmt.Fprintf(out, "\n[%s] %s\n", e.Module, f.Target)
			if tool == "internal" {
				diff := drift.UnifiedDiff(f.Target, string(tgt), f.Source, string(src))
				fmt.Fprint(out, drift.ColorDiff(diff, color))
			} else {
				externalDiff(tool, f.Target, f.Source, out)
			}
		}
	}
}

// externalDiff runs a diff tool as a subprocess, streaming output to out.
// The tool receives the target and source paths as arguments; "diff" gets
// -u prepended for unified format.
func externalDiff(tool, target, source string, out io.Writer) {
	args := []string{}
	if tool == "diff" {
		args = append(args, "-u")
	}
	args = append(args, target, source)
	cmd := exec.Command(tool, args...)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // diff exits 1 on differences; not an error
}
