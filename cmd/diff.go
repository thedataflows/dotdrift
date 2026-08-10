package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// showDotfileDiffs prints unified diffs for copy-mode dotfiles whose content
// differs between the profile source and the live target. Called when --diff
// is set, after the report/plan and before the pipeline (apply) or after the
// report (status). tool is "internal" for the built-in diff or a command spec
// for external. External tools are invoked once per differing file with
// <target> <source> as arguments. Returns an error if an external tool fails.
func showDotfileDiffs(plan *resolve.Plan, profileRoot string, tool string, out io.Writer) error {
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
				if err := externalDiff(tool, f.Target, f.Source, out); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// externalDiff runs a diff tool as a subprocess with the target and source
// file paths as arguments. The tool spec may include arguments (e.g.
// "delta --no-gitconfig"); it is split on whitespace. "diff" gets -u prepended
// for unified format. Only diff's exit code 1 (files differ) is tolerated;
// any other failure — tool not found, crash, non-zero exit — is returned as an
// error including captured stderr. The full tool spec appears in single quotes.
func externalDiff(toolSpec, target, source string, out io.Writer) error {
	parts := strings.Fields(toolSpec)
	if len(parts) == 0 {
		return fmt.Errorf("diff tool: empty command")
	}
	tool := parts[0]
	userArgs := parts[1:]
	isDiff := tool == "diff"
	var args []string
	if isDiff {
		args = append(args, "-u")
	}
	args = append(args, userArgs...)
	args = append(args, target, source)
	cmd := exec.Command(tool, args...)
	cmd.Stdout = out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		// diff exits 1 when files differ — that's the normal case, not an error.
		if exitErr, ok := err.(*exec.ExitError); ok && isDiff && exitErr.ExitCode() == 1 {
			return nil
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("diff tool '%s': %w: %s", toolSpec, err, detail)
		}
		return fmt.Errorf("diff tool '%s': %w", toolSpec, err)
	}
	return nil
}
