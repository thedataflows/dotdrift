package service

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"slices"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/palette"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/smb"
)

// Canonical text renderers (0061-D5): the service layer owns the text
// rendering of the surfaces both front ends show fact-identically — the
// plan report, the dotfile diff, and the status summary. --json stays a
// dumb marshal in cmd over the typed plan; TUI-styled rendering never
// touches these. Every renderer here reproduces the pre-migration CLI
// bytes (goldens under testdata/golden).

// RenderPlanReport renders the canonical text plan report; depTree is the
// optional dependency tree rendered in place of the flat package list.
func RenderPlanReport(w io.Writer, r *PlanRead, depTree []packages.PackageDeps) error {
	plan, p, f := r.Plan, r.Profile, r.Facts
	if len(p.Selected) == 0 {
		fmt.Fprintln(w, "warning: no modules selected")
	}
	fmt.Fprintf(w, "fingerprint:\n%s", resolve.Fingerprint(p, f))
	fmt.Fprintln(w, "packages:")
	if depTree != nil {
		for _, node := range depTree {
			printDepTree(w, node, "  ")
		}
	} else {
		for _, pkg := range plan.Packages.Install {
			fmt.Fprintf(w, "  - %s\n", pkg)
		}
	}
	fmt.Fprintln(w, "remove:")
	for _, pkg := range plan.Packages.Remove {
		fmt.Fprintf(w, "  - %s\n", pkg)
	}
	fmt.Fprintln(w, "tools:")
	for _, k := range sortedKeys(plan.Tools.Versions) {
		fmt.Fprintf(w, "  %s: %s\n", k, plan.Tools.Versions[k])
	}
	fmt.Fprintln(w, "dotfiles:")
	for _, e := range plan.Dotfiles.Entries {
		fmt.Fprintf(w, "  %s:\n", e.Target)
		switch {
		case e.Line != "":
			fmt.Fprintf(w, "    line: %q\n", e.Line)
		case e.Block != "":
			fmt.Fprintf(w, "    block: %q\n", e.Block)
			if e.Comment != "" {
				fmt.Fprintf(w, "    comment: %q\n", e.Comment)
			}
		case e.Template != "":
			fmt.Fprintf(w, "    source: %s\n", e.Source)
			fmt.Fprintf(w, "    template: %s\n", e.Template)
		default:
			fmt.Fprintf(w, "    source: %s\n", e.Source)
			fmt.Fprintf(w, "    mode: %s\n", e.Mode)
		}
		// System-scope entries are marked on the module line; user scope is
		// the default and stays unmarked. (Edit entries are always user-scope.)
		marker := ""
		if e.Scope == profile.ScopeSystem {
			marker = " [system]"
		}
		fmt.Fprintf(w, "    module: %s%s\n", e.Module, marker)
		fmt.Fprintf(w, "    layer: %s\n", e.Layer)
	}
	fmt.Fprintln(w, "hooks:")
	fmt.Fprintln(w, "  pre:")
	for _, c := range plan.Hooks.Pre {
		fmt.Fprintf(w, "    - %s%s\n", c.Command, optionalMarker(c.Optional))
	}
	fmt.Fprintln(w, "  post:")
	for _, c := range plan.Hooks.Post {
		fmt.Fprintf(w, "    - %s%s\n", c.Command, optionalMarker(c.Optional))
	}
	// systemd/mounts/smb sections render last and are omitted entirely when
	// the profile declares none, keeping output for other profiles stable.
	if len(plan.Systemd.Units) > 0 {
		fmt.Fprintln(w, "systemd:")
		for _, u := range plan.Systemd.Units {
			fmt.Fprintf(w, "  %s: %s (%s) [%s]\n", u.Module, u.Name, u.Kind, u.Layer)
		}
	}
	if len(plan.Mounts.Entries) > 0 {
		fmt.Fprintln(w, "mounts:")
		for _, e := range plan.Mounts.Entries {
			fmt.Fprintf(w, "  %s: %s %s %s -> %s [%s][%s]",
				e.Module, e.Name, e.Spec.Type, e.Spec.Source, e.Spec.Destination, e.Layer, e.Scope)
			if e.Spec.StartAt != "" {
				fmt.Fprintf(w, " startat=%s", e.Spec.StartAt)
			}
			if e.Spec.State != "" {
				fmt.Fprintf(w, " state=%s", e.Spec.State)
			}
			fmt.Fprintln(w)
		}
	}
	if len(plan.Smb.Modules) > 0 {
		fmt.Fprintln(w, "smb:")
		for _, m := range plan.Smb.Modules {
			fmt.Fprintf(w, "  %s:\n", m.Module)
			// Group and avahi render the effective activation values, not the
			// raw spec: an unset group activates as smb.DefaultGroup and an
			// unset avahi defaults to enabled.
			group := m.Spec.Group
			if group == "" {
				group = smb.DefaultGroup
			}
			fmt.Fprintf(w, "    group: %s\n", group)
			fmt.Fprintf(w, "    users: %s\n", strings.Join(m.Spec.Users, ", "))
			fmt.Fprintf(w, "    avahi: %t\n", m.Spec.Avahi == nil || *m.Spec.Avahi)
			if len(m.Spec.Shares) > 0 {
				fmt.Fprintln(w, "    shares:")
				for _, name := range slices.Sorted(maps.Keys(m.Spec.Shares)) {
					fmt.Fprintf(w, "      %s -> %s\n", name, m.Spec.Shares[name].Path)
				}
			}
		}
	}
	return nil
}

// RenderDiff writes the dotfile diff section: one header per differing
// file, then the internal unified diff or the named external tool.
func RenderDiff(w io.Writer, entries []DiffEntry, tool string, color bool) error {
	for _, e := range entries {
		fmt.Fprintf(w, "\n[%s] %s\n", e.Module, e.Target)
		if tool == "internal" {
			diff := drift.UnifiedDiff(e.Target, e.TargetContent, e.Source, e.SourceContent)
			fmt.Fprint(w, drift.ColorDiff(diff, color))
		} else {
			if err := externalDiff(tool, e.Target, e.Source, w); err != nil {
				return err
			}
		}
	}
	return nil
}

// RenderStatusHeader writes the profile/state/resume header and the drift
// report. Color follows the writer (executil.ColorEnabled) and the
// profile's palette.
func RenderStatusHeader(w io.Writer, r *StatusRead) error {
	pal, err := palette.FromConfig(r.Profile.Config.Colors)
	if err != nil {
		return err // already validated at load; unreachable double-check
	}
	fmt.Fprintf(w, "profile: %s\n", r.ProfilePath)
	fmt.Fprintf(w, "state: %s\n", r.StatePath)
	// The resume line's MESSAGE carries a role hue on a TTY (the literal
	// `resume: ` prefix stays plain): ok when clean, warn when a cursor
	// is pending (an interrupted apply awaits resuming).
	resumeMsg := "clean - next apply starts from the beginning"
	resumeRole := palette.OK
	if r.State.LastCompleted != "" {
		resumeMsg = fmt.Sprintf("last completed %q - next apply resumes after it", r.State.LastCompleted)
		resumeRole = palette.Warn
	}
	if executil.ColorEnabled(w) {
		resumeMsg = pal.Wrap(resumeRole, resumeMsg)
	}
	fmt.Fprintf(w, "resume: %s\n", resumeMsg)
	drift.Render(w, r.Findings, drift.WithPalette(pal))
	return nil
}

// RenderStatusNote writes the configuration notice for other accounts
// (issue 0038, ADR-0006): no per-account probing — per-account drift
// sections proved too noisy (sudo-dependent, cwd-sensitive). Just names
// each existing account that has configuration on this machine and the
// apply command for it. Empty when there are none.
func RenderStatusNote(w io.Writer, r *StatusRead) {
	if len(r.Others) == 0 {
		return
	}
	fmt.Fprintln(w, "note: configuration exists for other accounts on this machine:")
	for _, acct := range r.Others {
		fmt.Fprintf(w, "  users/%s — apply with: %s\n", acct.Name, applyCommandFor(acct))
	}
	fmt.Fprintln(w, "  (each account needs dotdrift on its PATH — install it system-wide, e.g. via mise, system scope)")
}

// applyCommandFor is the per-account apply instruction in the status notice:
// the uid-0 account converges with plain sudo; any other account needs a
// login shell so its own PATH applies (issue 0038).
func applyCommandFor(acct profile.Account) string {
	if acct.Uid == "0" {
		return "sudo dotdrift apply"
	}
	return "sudo -iu " + acct.Name + " dotdrift apply"
}

// externalDiff runs a diff tool as a subprocess with the target and source
// file paths as arguments. The tool spec may include arguments (e.g.
// "delta --no-gitconfig"); it is split on whitespace. "diff" gets -u prepended
// for unified format. Exit code 1 (differences found) is tolerated for all
// tools — it is the standard exit for diff, git diff, delta, difftastic.
// Any other failure — tool not found, crash, exit >1 — is returned as an
// error including captured stderr. The full tool spec appears in single quotes.
func externalDiff(toolSpec, target, source string, out io.Writer) error {
	parts := strings.Fields(toolSpec)
	if len(parts) == 0 {
		return fmt.Errorf("diff tool: empty command")
	}
	tool := parts[0]
	userArgs := parts[1:]
	var args []string
	if tool == "diff" {
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
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil // exit 1 = differences found (standard across diff tools)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("diff tool '%s': %w: %s", toolSpec, err, detail)
		}
		return fmt.Errorf("diff tool '%s': %w", toolSpec, err)
	}
	return nil
}

// printDepTree renders one dependency node at indent: `- name` (marked
// `(deps unknown)` when the query failed), then a `deps:` block one level
// deeper when children exist.
func printDepTree(out io.Writer, node packages.PackageDeps, indent string) {
	if node.Unknown {
		fmt.Fprintf(out, "%s- %s (deps unknown)\n", indent, node.Name)
	} else {
		fmt.Fprintf(out, "%s- %s\n", indent, node.Name)
	}
	if len(node.Deps) > 0 {
		child := indent + "  "
		fmt.Fprintf(out, "%sdeps:\n", child)
		for _, d := range node.Deps {
			printDepTree(out, d, child)
		}
	}
}

// optionalMarker renders the text-plan suffix for a hook: " (optional)" when
// the hook is best-effort, "" when required.
func optionalMarker(optional bool) string {
	if optional {
		return " (optional)"
	}
	return ""
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
