package profile

import (
	"context"
	"os/exec"
	"strings"

	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/packages"
)

// probeInstalledPackages resolves which of names are installed on the
// running system via the distro backend (facts.Backend). It is a
// package-level var so tests can stub the lookup (same pattern as
// facts.KernelRelease). A query that errors — unknown backend, package
// manager failure — counts as not installed: a when.packages constraint
// fails open, exactly like an empty kernel fact.
var probeInstalledPackages = func(backend string, names []string) map[string]bool {
	installed := make(map[string]bool, len(names))
	b := packages.For(backend)
	for _, name := range names {
		if ok, err := b.IsInstalled(context.Background(), name); err == nil && ok {
			installed[name] = true
		}
	}
	return installed
}

// probeInstalledTools resolves which mise-managed tool names are installed
// (presence only, version ignored) via `mise current <tool>`; a missing mise
// binary, a failing probe, or empty output counts as not installed — the
// same fail-open contract as probeInstalledPackages.
//
// ponytail: duplicates the ~10-line lookup inside mise.ExecMise.Current
// because internal/mise imports internal/profile (the reverse dependency
// would cycle). Ceiling: two copies of "LookPath + mise current" must stay
// in sync. Upgrade path: extract a shared probe helper into internal/facts
// if a third consumer appears.
var probeInstalledTools = func(names []string) map[string]bool {
	installed := make(map[string]bool, len(names))
	path, err := exec.LookPath("mise")
	if err != nil {
		return installed
	}
	for _, name := range names {
		out, err := exec.Command(path, "current", name).Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			installed[name] = true
		}
	}
	return installed
}

// enrichProbes returns the facts to select with, probed for the installed
// status of every package/tool any module's when.packages/when.tools
// references. Explicitly provided facts win (tests inject them); a profile
// declaring neither probes nothing, so existing profiles cost zero extra
// subprocesses. The returned facts never alias-and-mutate the caller's value.
func enrichProbes(f *facts.Facts, modules []Module) *facts.Facts {
	var pkgNames, toolNames []string
	pkgSeen, toolSeen := make(map[string]struct{}), make(map[string]struct{})
	for _, m := range modules {
		for _, name := range m.Config.When.Packages {
			if _, dup := pkgSeen[name]; dup {
				continue
			}
			pkgSeen[name] = struct{}{}
			pkgNames = append(pkgNames, name)
		}
		for _, name := range m.Config.When.Tools {
			if _, dup := toolSeen[name]; dup {
				continue
			}
			toolSeen[name] = struct{}{}
			toolNames = append(toolNames, name)
		}
	}
	if len(pkgNames) == 0 && len(toolNames) == 0 {
		return f
	}
	cp := *f
	if len(pkgNames) > 0 && cp.InstalledPackages == nil {
		cp.InstalledPackages = probeInstalledPackages(f.Backend, pkgNames)
	}
	if len(toolNames) > 0 && cp.InstalledTools == nil {
		cp.InstalledTools = probeInstalledTools(toolNames)
	}
	return &cp
}
