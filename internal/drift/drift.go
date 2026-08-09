// Package drift compares a resolved plan against the live system.
// All checks are read-only probes; nothing is installed, written, or ensured.
package drift

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Status is the outcome of one drift check.
type Status int

const (
	OK Status = iota
	Drift
	// Unknown means a probe failed or the result was not decidable (e.g. mise
	// missing, permission denied).
	Unknown
)

func (s Status) String() string {
	switch s {
	case OK:
		return "ok"
	case Drift:
		return "drift"
	case Unknown:
		return "unknown"
	}
	return "unknown"
}

// Finding is one checked item. Detail is empty for OK.
type Finding struct {
	Section string // "packages" | "tools" | "dotfiles" | "mounts" | "smb"
	Item    string // package name, tool name, target path, unit name, share name
	Status  Status
	Detail  string
}

// Probes are the side-effect seams. DefaultProbes fills OS-backed defaults;
// tests override individual fields. IsInstalled and ToolCurrent have no
// generic OS default (they need a package backend / mise) and must be set by
// the caller before Check.
type Probes struct {
	IsInstalled func(ctx context.Context, pkg string) (bool, error)
	ToolCurrent func(ctx context.Context, tool string) (string, error) // err or "" → Unknown
	Run         func(ctx context.Context, name string, args ...string) (string, error)
	HomeDir     string
	Readlink    func(path string) (string, error)
	ReadFile    func(path string) ([]byte, error)
	StatDir     func(path string) (bool, error) // exists and is a directory
}

// DefaultProbes returns OS-backed probes for Run, Readlink, ReadFile, StatDir,
// and HomeDir. IsInstalled and ToolCurrent are left nil — wire them to a
// package backend and mise before calling Check.
func DefaultProbes() Probes {
	home, _ := os.UserHomeDir()
	return Probes{
		Run:      defaultRun,
		HomeDir:  home,
		Readlink: os.Readlink,
		ReadFile: os.ReadFile,
		StatDir:  defaultStatDir,
	}
}

func defaultRun(ctx context.Context, name string, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}

func defaultStatDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// sectionOrder is the fixed display order for report sections.
var sectionOrder = []string{"packages", "tools", "dotfiles", "mounts", "smb"}

// Check runs every read-only probe for the plan and returns findings in fixed
// section order (packages → tools → dotfiles → mounts → smb). Items within a
// section follow plan order (tools are sorted by name for determinism). A
// section whose plan has zero items produces zero findings and is omitted from
// Render.
func Check(ctx context.Context, plan *resolve.Plan, profileRoot string, pr Probes) []Finding {
	if plan == nil {
		return nil
	}
	var findings []Finding
	findings = append(findings, checkPackages(ctx, plan, pr)...)
	findings = append(findings, checkTools(ctx, plan, pr)...)
	findings = append(findings, checkDotfiles(ctx, plan, profileRoot, pr)...)
	findings = append(findings, checkMounts(ctx, plan, pr)...)
	findings = append(findings, checkSmb(ctx, plan, pr)...)
	return findings
}

func checkPackages(ctx context.Context, plan *resolve.Plan, pr Probes) []Finding {
	var out []Finding
	for _, pkg := range plan.Packages.Install {
		installed, err := pr.IsInstalled(ctx, pkg)
		switch {
		case err != nil:
			out = append(out, Finding{"packages", pkg, Unknown, err.Error()})
		case installed:
			out = append(out, Finding{"packages", pkg, OK, ""})
		default:
			out = append(out, Finding{"packages", pkg, Drift, "missing"})
		}
	}
	for _, pkg := range plan.Packages.Remove {
		installed, err := pr.IsInstalled(ctx, pkg)
		switch {
		case err != nil:
			out = append(out, Finding{"packages", pkg, Unknown, err.Error()})
		case installed:
			out = append(out, Finding{"packages", pkg, Drift, "still installed"})
		default:
			out = append(out, Finding{"packages", pkg, OK, ""})
		}
	}
	return out
}

func checkTools(ctx context.Context, plan *resolve.Plan, pr Probes) []Finding {
	var out []Finding
	for _, name := range sortedKeys(plan.Tools.Versions) {
		want := plan.Tools.Versions[name]
		got, err := pr.ToolCurrent(ctx, name)
		if err != nil || got == "" {
			out = append(out, Finding{"tools", name, Unknown, "mise not available or tool not installed"})
			continue
		}
		if got == want || strings.HasPrefix(got, want+".") {
			out = append(out, Finding{"tools", name, OK, ""})
		} else {
			out = append(out, Finding{"tools", name, Drift, fmt.Sprintf("installed %s, want %s", got, want)})
		}
	}
	return out
}

func checkDotfiles(ctx context.Context, plan *resolve.Plan, profileRoot string, pr Probes) []Finding {
	var out []Finding
	for _, e := range plan.Dotfiles.Entries {
		files, err := mise.ResolveBootstrapFiles([]resolve.DotfileEntry{e}, profileRoot, pr.HomeDir)
		if err != nil {
			out = append(out, Finding{"dotfiles", e.Target, Unknown, err.Error()})
			continue
		}
		for _, f := range files {
			out = append(out, checkDotfileFile(f, pr))
		}
	}
	return out
}

func checkDotfileFile(f mise.BootstrapFile, pr Probes) Finding {
	switch f.Mode {
	case "symlink", "symlink-each":
		target, err := pr.Readlink(f.Target)
		if err != nil {
			return Finding{"dotfiles", f.Target, Drift, "missing"}
		}
		if target == f.Source {
			return Finding{"dotfiles", f.Target, OK, ""}
		}
		return Finding{"dotfiles", f.Target, Drift, fmt.Sprintf("points to %s", target)}
	case "copy":
		src, err := pr.ReadFile(f.Source)
		if err != nil {
			return Finding{"dotfiles", f.Target, Unknown, err.Error()}
		}
		tgt, err := pr.ReadFile(f.Target)
		if err != nil {
			if os.IsNotExist(err) {
				return Finding{"dotfiles", f.Target, Drift, "missing"}
			}
			return Finding{"dotfiles", f.Target, Unknown, err.Error()}
		}
		if bytes.Equal(src, tgt) {
			return Finding{"dotfiles", f.Target, OK, ""}
		}
		return Finding{"dotfiles", f.Target, Drift, "content differs"}
	case "template":
		if _, err := pr.ReadFile(f.Target); err != nil {
			if os.IsNotExist(err) {
				return Finding{"dotfiles", f.Target, Drift, "missing"}
			}
			return Finding{"dotfiles", f.Target, Unknown, err.Error()}
		}
		return Finding{"dotfiles", f.Target, OK, ""}
	}
	return Finding{"dotfiles", f.Target, Unknown, fmt.Sprintf("unrecognized mode %q", f.Mode)}
}

func checkMounts(ctx context.Context, plan *resolve.Plan, pr Probes) []Finding {
	var out []Finding
	for _, e := range plan.Mounts.Entries {
		out = append(out, checkMount(ctx, e, pr))
	}
	return out
}

func checkMount(ctx context.Context, e resolve.MountEntry, pr Probes) Finding {
	const section = "mounts"
	item := e.Spec.Destination
	unit := generate.EscapePath(e.Spec.Destination) + ".mount"

	if ok, err := pr.StatDir(e.Spec.Destination); err != nil {
		return Finding{section, item, Unknown, err.Error()}
	} else if !ok {
		return Finding{section, item, Drift, "destination missing"}
	}

	enabled, st, detail := enabledState(ctx, pr, unit)
	if st == Unknown {
		return Finding{section, item, Unknown, detail}
	}
	if st == Drift {
		return Finding{section, item, Drift, detail}
	}
	if e.Spec.State == "disabled" {
		if enabled {
			return Finding{section, item, Drift, "enabled but declared disabled"}
		}
		return Finding{section, item, OK, ""}
	}
	if !enabled {
		return Finding{section, item, Drift, "not enabled"}
	}
	activeOut, err := pr.Run(ctx, "systemctl", "is-active", unit)
	if err != nil || strings.TrimSpace(activeOut) != "active" {
		return Finding{section, item, Drift, "not active"}
	}
	return Finding{section, item, OK, ""}
}

// enabledState runs `systemctl is-enabled <unit>` and classifies the output.
// Returns the enabled flag plus a Status for the probe itself: OK when the
// output was a decidable enabled/disabled/static/indirect value; Drift with
// detail "unit not found" when the command errored; Unknown with an
// "unexpected is-enabled output ..." detail otherwise.
func enabledState(ctx context.Context, pr Probes, unit string) (enabled bool, status Status, detail string) {
	out, err := pr.Run(ctx, "systemctl", "is-enabled", unit)
	if err != nil {
		return false, Drift, "unit not found"
	}
	trimmed := strings.TrimSpace(out)
	switch trimmed {
	case "enabled":
		return true, OK, ""
	case "disabled", "static", "indirect":
		return false, OK, ""
	}
	return false, Unknown, fmt.Sprintf("unexpected is-enabled output %s", trimmed)
}

func checkSmb(ctx context.Context, plan *resolve.Plan, pr Probes) []Finding {
	var out []Finding
	for _, mod := range plan.Smb.Modules {
		out = append(out, checkSmbModule(ctx, mod, pr)...)
	}
	return out
}

func checkSmbModule(ctx context.Context, mod resolve.SmbModuleSpec, pr Probes) []Finding {
	const section = "smb"
	var out []Finding

	group := mod.Spec.Group
	if group == "" {
		group = "smb"
	}
	if _, err := pr.Run(ctx, "getent", "group", group); err != nil {
		out = append(out, Finding{section, group, Drift, fmt.Sprintf("group %s missing", group)})
	} else {
		out = append(out, Finding{section, group, OK, ""})
	}

	for _, u := range mod.Spec.Users {
		got, err := pr.Run(ctx, "id", "-Gn", u)
		if err != nil {
			out = append(out, Finding{section, u, Drift, fmt.Sprintf("user %s missing", u)})
			continue
		}
		if !containsField(got, group) {
			out = append(out, Finding{section, u, Drift, fmt.Sprintf("user %s not in group %s", u, group)})
			continue
		}
		out = append(out, Finding{section, u, OK, ""})
	}

	out = append(out, checkService(ctx, section, "smb", pr))
	if mod.Spec.Avahi == nil || *mod.Spec.Avahi {
		out = append(out, checkService(ctx, section, "avahi-daemon", pr))
	}

	if len(mod.Spec.Shares) > 0 {
		tp, err := pr.Run(ctx, "testparm", "-s")
		names := make([]string, 0, len(mod.Spec.Shares))
		for name := range mod.Spec.Shares {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			switch {
			case err != nil:
				out = append(out, Finding{section, name, Unknown, "testparm failed"})
			case strings.Contains(tp, "["+name+"]"):
				out = append(out, Finding{section, name, OK, ""})
			default:
				out = append(out, Finding{section, name, Drift, fmt.Sprintf("share %s not in smb config", name)})
			}
		}
	}
	return out
}

// checkService verifies a systemd service is enabled and active (the enabled
// expectation). One finding per service.
func checkService(ctx context.Context, section, name string, pr Probes) Finding {
	enabled, st, detail := enabledState(ctx, pr, name)
	if st == Unknown {
		return Finding{section, name, Unknown, detail}
	}
	if st == Drift {
		return Finding{section, name, Drift, detail}
	}
	if !enabled {
		return Finding{section, name, Drift, "not enabled"}
	}
	activeOut, err := pr.Run(ctx, "systemctl", "is-active", name)
	if err != nil || strings.TrimSpace(activeOut) != "active" {
		return Finding{section, name, Drift, "not active"}
	}
	return Finding{section, name, OK, ""}
}

// Render writes the drift report. Sections appear in fixed order and only when
// they have ≥1 finding; non-OK findings print as `  <status>: <item> — <detail>`;
// a section whose findings are all OK prints `  ok: all N checks passed`. The
// final line is `no drift` when nothing drifted, otherwise `drift: N item(s)`
// (singular `item` at 1) with `, K unknown` appended when there are unknowns.
func Render(w io.Writer, findings []Finding) {
	driftCount, unknownCount := 0, 0
	for _, f := range findings {
		switch f.Status {
		case Drift:
			driftCount++
		case Unknown:
			unknownCount++
		}
	}

	wroteSection := false
	for _, sec := range sectionOrder {
		secFindings := findingsOfSection(findings, sec)
		if len(secFindings) == 0 {
			continue
		}
		wroteSection = true
		fmt.Fprintf(w, "%s:\n", sec)
		allOK := true
		for _, f := range secFindings {
			if f.Status == OK {
				continue
			}
			allOK = false
			fmt.Fprintf(w, "  %s: %s — %s\n", f.Status, f.Item, f.Detail)
		}
		if allOK {
			fmt.Fprintf(w, "  ok: all %d checks passed\n", len(secFindings))
		}
	}

	if wroteSection {
		fmt.Fprintln(w)
	}
	if driftCount == 0 && unknownCount == 0 {
		fmt.Fprintln(w, "no drift")
		return
	}
	itemWord := "items"
	if driftCount == 1 {
		itemWord = "item"
	}
	summary := fmt.Sprintf("drift: %d %s", driftCount, itemWord)
	if unknownCount > 0 {
		summary += fmt.Sprintf(", %d unknown", unknownCount)
	}
	fmt.Fprintln(w, summary)
}

func findingsOfSection(findings []Finding, section string) []Finding {
	var out []Finding
	for _, f := range findings {
		if f.Section == section {
			out = append(out, f)
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// containsField reports whether any space-separated field of s equals target.
func containsField(s, target string) bool {
	for _, f := range strings.Fields(s) {
		if f == target {
			return true
		}
	}
	return false
}
