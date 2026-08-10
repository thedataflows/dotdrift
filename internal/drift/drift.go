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
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/thedataflows/dotdrift/internal/executil"
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
	Module  string // owning module ID (empty if unknown)
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

// CheckOptions controls how Check runs its probes.
type CheckOptions struct {
	// Jobs bounds concurrent probe workers; <= 0 uses runtime.NumCPU().
	Jobs int
	// Verbose, when non-nil, receives one "checking <section>: <item>" line
	// per probe as it starts. Writes are serialized internally.
	Verbose io.Writer
}

// probeTask is one unit of concurrent work: a probe producing a single finding.
// section/module/item identify the probe for verbose progress and report
// attribution; run does the probe.
type probeTask struct {
	section string
	module  string
	item    string
	run     func(ctx context.Context) Finding
}

// Check runs every read-only probe for the plan concurrently (bounded by
// opts.Jobs; <= 0 → runtime.NumCPU) and returns findings in fixed section order
// (packages → tools → dotfiles → mounts → smb). Each worker writes only its own
// indexed result slot, so output order is deterministic regardless of
// completion order. opts.Verbose, when non-nil, receives one
// "checking <section>: <item>" line per probe as it starts. A nil plan returns nil.
func Check(ctx context.Context, plan *resolve.Plan, profileRoot string, pr Probes, opts CheckOptions) []Finding {
	if plan == nil {
		return nil
	}
	tasks := checkPackages(plan, pr)
	tasks = append(tasks, checkTools(plan, pr)...)
	tasks = append(tasks, checkDotfiles(plan, profileRoot, pr)...)
	tasks = append(tasks, checkMounts(plan, pr)...)
	tasks = append(tasks, checkSmb(plan, pr)...)

	jobs := opts.Jobs
	if jobs <= 0 {
		jobs = runtime.NumCPU()
	}

	var vw *executil.LockedWriter
	if opts.Verbose != nil {
		vw = &executil.LockedWriter{W: opts.Verbose}
	}

	results := make([]Finding, len(tasks))
	work := make(chan int)
	var wg sync.WaitGroup
	for range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				t := tasks[i]
				if vw != nil {
					fmt.Fprintf(vw, "checking %s: %s\n", t.section, t.item)
				}
				f := t.run(ctx)
				f.Module = t.module
				results[i] = f
			}
		}()
	}
	for i := range tasks {
		work <- i
	}
	close(work)
	wg.Wait()
	return results
}

func checkPackages(plan *resolve.Plan, pr Probes) []probeTask {
	var tasks []probeTask
	for _, pkg := range plan.Packages.Install {
		tasks = append(tasks, probeTask{
			section: "packages",
			module:  strings.Join(plan.Packages.PresentModules[pkg], ","),
			item:    pkg,
			run: func(ctx context.Context) Finding {
				installed, err := pr.IsInstalled(ctx, pkg)
				switch {
				case err != nil:
					return Finding{"packages", pkg, Unknown, err.Error(), ""}
				case installed:
					return Finding{"packages", pkg, OK, "", ""}
				default:
					return Finding{"packages", pkg, Drift, "missing", ""}
				}
			},
		})
	}
	for _, pkg := range plan.Packages.Remove {
		tasks = append(tasks, probeTask{
			section: "packages",
			module:  strings.Join(plan.Packages.AbsentModules[pkg], ","),
			item:    pkg,
			run: func(ctx context.Context) Finding {
				installed, err := pr.IsInstalled(ctx, pkg)
				switch {
				case err != nil:
					return Finding{"packages", pkg, Unknown, err.Error(), ""}
				case installed:
					return Finding{"packages", pkg, Drift, "still installed", ""}
				default:
					return Finding{"packages", pkg, OK, "", ""}
				}
			},
		})
	}
	return tasks
}

func checkTools(plan *resolve.Plan, pr Probes) []probeTask {
	var tasks []probeTask
	for _, name := range sortedKeys(plan.Tools.Versions) {
		want := plan.Tools.Versions[name]
		tasks = append(tasks, probeTask{
			section: "tools",
			module:  strings.Join(plan.Tools.ToolModules[name], ","),
			item:    name,
			run: func(ctx context.Context) Finding {
				got, err := pr.ToolCurrent(ctx, name)
				if err != nil || got == "" {
					return Finding{"tools", name, Unknown, "mise not available or tool not installed", ""}
				}
				if got == want || strings.HasPrefix(got, want+".") {
					return Finding{"tools", name, OK, "", ""}
				}
				return Finding{"tools", name, Drift, fmt.Sprintf("installed %s, want %s", got, want), ""}
			},
		})
	}
	return tasks
}

func checkDotfiles(plan *resolve.Plan, profileRoot string, pr Probes) []probeTask {
	var tasks []probeTask
	for _, e := range plan.Dotfiles.Entries {
		files, err := mise.ResolveBootstrapFiles([]resolve.DotfileEntry{e}, profileRoot, pr.HomeDir)
		if err != nil {
			target := e.Target
			errMsg := err.Error()
			tasks = append(tasks, probeTask{
				section: "dotfiles",
				module:  e.Module,
				item:    target,
				run: func(ctx context.Context) Finding {
					return Finding{"dotfiles", target, Unknown, errMsg, ""}
				},
			})
			continue
		}
		for _, f := range files {
			tasks = append(tasks, probeTask{
				section: "dotfiles",
				module:  e.Module,
				item:    f.Target,
				run: func(ctx context.Context) Finding {
					return checkDotfileFile(f, pr)
				},
			})
		}
	}
	return tasks
}

func checkDotfileFile(f mise.BootstrapFile, pr Probes) Finding {
	switch f.Mode {
	case "symlink", "symlink-each":
		target, err := pr.Readlink(f.Target)
		if err != nil {
			if os.IsNotExist(err) {
				return Finding{"dotfiles", f.Target, Drift, "missing", ""}
			}
			return Finding{"dotfiles", f.Target, Drift, "not a symlink", ""}
		}
		if target == f.Source {
			return Finding{"dotfiles", f.Target, OK, "", ""}
		}
		return Finding{"dotfiles", f.Target, Drift, fmt.Sprintf("points to %s", target), ""}
	case "copy":
		src, err := pr.ReadFile(f.Source)
		if err != nil {
			return Finding{"dotfiles", f.Target, Unknown, err.Error(), ""}
		}
		tgt, err := pr.ReadFile(f.Target)
		if err != nil {
			if os.IsNotExist(err) {
				return Finding{"dotfiles", f.Target, Drift, "missing", ""}
			}
			return Finding{"dotfiles", f.Target, Unknown, err.Error(), ""}
		}
		if bytes.Equal(src, tgt) {
			return Finding{"dotfiles", f.Target, OK, "", ""}
		}
		return Finding{"dotfiles", f.Target, Drift, "content differs", ""}
	case "template":
		if _, err := pr.ReadFile(f.Target); err != nil {
			if os.IsNotExist(err) {
				return Finding{"dotfiles", f.Target, Drift, "missing", ""}
			}
			return Finding{"dotfiles", f.Target, Unknown, err.Error(), ""}
		}
		return Finding{"dotfiles", f.Target, OK, "", ""}
	}
	return Finding{"dotfiles", f.Target, Unknown, fmt.Sprintf("unrecognized mode %q", f.Mode), ""}
}

func checkMounts(plan *resolve.Plan, pr Probes) []probeTask {
	var tasks []probeTask
	for _, e := range plan.Mounts.Entries {
		tasks = append(tasks, probeTask{
			section: "mounts",
			module:  e.Module,
			item:    e.Spec.Destination,
			run: func(ctx context.Context) Finding {
				return checkMount(ctx, e, pr)
			},
		})
	}
	return tasks
}

func checkMount(ctx context.Context, e resolve.MountEntry, pr Probes) Finding {
	const section = "mounts"
	item := e.Spec.Destination
	unit := generate.EscapePath(e.Spec.Destination) + ".mount"

	if ok, err := pr.StatDir(e.Spec.Destination); err != nil {
		return Finding{section, item, Unknown, err.Error(), ""}
	} else if !ok {
		return Finding{section, item, Drift, "destination missing", ""}
	}

	enabled, st, detail := enabledState(ctx, pr, unit)
	if st == Unknown {
		return Finding{section, item, Unknown, detail, ""}
	}
	if st == Drift {
		return Finding{section, item, Drift, detail, ""}
	}
	if e.Spec.State == "disabled" {
		if enabled {
			return Finding{section, item, Drift, "enabled but declared disabled", ""}
		}
		return Finding{section, item, OK, "", ""}
	}
	if !enabled {
		return Finding{section, item, Drift, "not enabled", ""}
	}
	activeOut, err := pr.Run(ctx, "systemctl", "is-active", unit)
	if err != nil || strings.TrimSpace(activeOut) != "active" {
		return Finding{section, item, Drift, "not active", ""}
	}
	return Finding{section, item, OK, "", ""}
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

func checkSmb(plan *resolve.Plan, pr Probes) []probeTask {
	var tasks []probeTask
	for _, mod := range plan.Smb.Modules {
		tasks = append(tasks, checkSmbModule(mod, pr)...)
	}
	return tasks
}

func checkSmbModule(mod resolve.SmbModuleSpec, pr Probes) []probeTask {
	const section = "smb"
	var tasks []probeTask

	group := mod.Spec.Group
	if group == "" {
		group = "smb"
	}

	tasks = append(tasks, probeTask{
		section: section,
		module:  mod.Module,
		item:    group,
		run: func(ctx context.Context) Finding {
			if _, err := pr.Run(ctx, "getent", "group", group); err != nil {
				return Finding{section, group, Drift, fmt.Sprintf("group %s missing", group), ""}
			}
			return Finding{section, group, OK, "", ""}
		},
	})

	for _, u := range mod.Spec.Users {
		tasks = append(tasks, probeTask{
			section: section,
			module:  mod.Module,
			item:    u,
			run: func(ctx context.Context) Finding {
				got, err := pr.Run(ctx, "id", "-Gn", u)
				if err != nil {
					return Finding{section, u, Drift, fmt.Sprintf("user %s missing", u), ""}
				}
				if !containsField(got, group) {
					return Finding{section, u, Drift, fmt.Sprintf("user %s not in group %s", u, group), ""}
				}
				return Finding{section, u, OK, "", ""}
			},
		})
	}

	tasks = append(tasks, probeTask{
		section: section,
		module:  mod.Module,
		item:    "smb",
		run: func(ctx context.Context) Finding {
			return checkService(ctx, section, "smb", pr)
		},
	})
	if mod.Spec.Avahi == nil || *mod.Spec.Avahi {
		tasks = append(tasks, probeTask{
			section: section,
			module:  mod.Module,
			item:    "avahi-daemon",
			run: func(ctx context.Context) Finding {
				return checkService(ctx, section, "avahi-daemon", pr)
			},
		})
	}

	// ponytail: each share calls testparm -s itself (was one call per module);
	// O(shares × testparm) — testparm is a fast local read, so the duplication
	// buys uniform one-finding-per-task granularity. If a profile with many
	// shares shows probe cost, memoize the output per module.
	if len(mod.Spec.Shares) > 0 {
		names := make([]string, 0, len(mod.Spec.Shares))
		for name := range mod.Spec.Shares {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			tasks = append(tasks, probeTask{
				section: section,
				module:  mod.Module,
				item:    name,
				run: func(ctx context.Context) Finding {
					tp, err := pr.Run(ctx, "testparm", "-s")
					switch {
					case err != nil:
						return Finding{section, name, Unknown, "testparm failed", ""}
					case strings.Contains(tp, "["+name+"]"):
						return Finding{section, name, OK, "", ""}
					default:
						return Finding{section, name, Drift, fmt.Sprintf("share %s not in smb config", name), ""}
					}
				},
			})
		}
	}
	return tasks
}

// checkService verifies a systemd service is enabled and active (the enabled
// expectation). One finding per service.
func checkService(ctx context.Context, section, name string, pr Probes) Finding {
	enabled, st, detail := enabledState(ctx, pr, name)
	if st == Unknown {
		return Finding{section, name, Unknown, detail, ""}
	}
	if st == Drift {
		return Finding{section, name, Drift, detail, ""}
	}
	if !enabled {
		return Finding{section, name, Drift, "not enabled", ""}
	}
	activeOut, err := pr.Run(ctx, "systemctl", "is-active", name)
	if err != nil || strings.TrimSpace(activeOut) != "active" {
		return Finding{section, name, Drift, "not active", ""}
	}
	return Finding{section, name, OK, "", ""}
}

// Render writes the drift report. Sections appear in fixed order and only when
// they have ≥1 finding. Non-OK findings print as `  <module>: <item> — <detail>`;
// the owning module replaces the status prefix (drift is implied for listed
// items; unknown items get a (?) suffix). A section whose findings are all OK
// prints `  ok: all N checks passed`. On a TTY, finding lines are colored by
// issue type: orange (missing/version drift), red (not-a-symlink/unknown),
// yellow (content differs). The final line is `no drift` when nothing drifted,
// otherwise `drift: N item(s)` with `, K unknown` appended when there are
// unknowns.
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

	color := executil.IsTerminal(w)

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
			renderFinding(w, f, color)
		}
		if allOK {
			line := fmt.Sprintf("  ok: all %d checks passed", len(secFindings))
			if color {
				line = ansiGreen + line + ansiReset
			}
			fmt.Fprintln(w, line)
		}
	}

	if wroteSection {
		fmt.Fprintln(w)
	}
	if driftCount == 0 && unknownCount == 0 {
		line := "no drift"
		if color {
			line = ansiGreen + line + ansiReset
		}
		fmt.Fprintln(w, line)
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

// ANSI color codes for TTY output; applied only when color is true.
const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiOrange = "\033[38;5;208m"
	ansiGreen  = "\033[32m"
)

// renderFinding writes one drift/unknown finding line. The owning module
// replaces the redundant status prefix; unknown items get a (?) suffix.
func renderFinding(w io.Writer, f Finding, color bool) {
	mod := f.Module
	if mod == "" {
		mod = "?"
	}
	suffix := ""
	if f.Status == Unknown {
		suffix = " (?)"
	}
	line := fmt.Sprintf("  %s: %s — %s%s", mod, f.Item, f.Detail, suffix)
	if color {
		line = findingColor(f) + line + ansiReset
	}
	fmt.Fprintln(w, line)
}

// findingColor returns the ANSI color for a finding: red for not-a-symlink and
// unknown (permissions/structural), yellow for content-differs and version
// mismatch (exists but doesn't match desired state), orange for everything
// else (missing, not enabled, still installed, etc.).
func findingColor(f Finding) string {
	if f.Status == Unknown || strings.Contains(f.Detail, "not a symlink") {
		return ansiRed
	}
	if f.Detail == "content differs" || strings.HasPrefix(f.Detail, "installed ") {
		return ansiYellow
	}
	return ansiOrange
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
