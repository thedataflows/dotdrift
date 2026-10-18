package profile

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"

	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/packages"
)

// regexMetachars are the characters that make a when.packages/when.tools
// entry a regex rather than a plain name: the entry is matched as an
// anchored full-name regex (apollo.* matches apollo and apollo-cuda-git).
// Entries without any of these keep exact-match semantics — a name like
// "c++" is found by exact lookup first and is never re-read as a regex
// unless it also needs pattern matching.
const regexMetachars = `.+*?()|[]{}^$\`

// isRegexEntry reports whether a when.packages/when.tools entry carries
// regex metacharacters and must be evaluated (and validated) as a pattern.
func isRegexEntry(entry string) bool {
	return strings.ContainsAny(entry, regexMetachars)
}

// anchoredRegex compiles an entry as an anchored full-name pattern.
func anchoredRegex(entry string) (*regexp.Regexp, error) {
	return regexp.Compile("^(?:" + entry + ")$")
}

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

// probeInstalledPackageList returns the installed set of ALL package names
// via one list query (Backend.Installed) — the probe path for regex
// entries, which IsInstalled(name) cannot answer. nil means the list is
// unavailable (unsupported backend, query failure): regex entries fail
// open and callers fall back to exact probes for plain entries.
var probeInstalledPackageList = func(backend string) map[string]bool {
	list, err := packages.For(backend).Installed(context.Background())
	if err != nil {
		return nil
	}
	installed := make(map[string]bool, len(list))
	for _, name := range list {
		installed[name] = true
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

// probeInstalledToolList returns the installed set of ALL mise tool names
// via one `mise ls --json` call (its output is an object keyed by tool
// name) — the probe path for regex entries. nil means unavailable (no
// mise, query failure, unparsable output): regex entries fail open.
var probeInstalledToolList = func() map[string]bool {
	path, err := exec.LookPath("mise")
	if err != nil {
		return nil
	}
	out, err := exec.Command(path, "ls", "--json").Output()
	if err != nil {
		return nil
	}
	var tools map[string]any
	if json.Unmarshal(out, &tools) != nil {
		return nil
	}
	installed := make(map[string]bool, len(tools))
	for name := range tools {
		installed[name] = true
	}
	return installed
}

// enrichProbes returns the facts to select with, probed for the installed
// status of every package/tool any module's when.packages/when.tools
// references, anywhere in its expression tree. Explicitly provided facts
// win (tests inject them); a profile declaring neither probes nothing, so
// existing profiles cost zero extra subprocesses. Regex entries (regex
// metacharacters present) switch the probe to ONE installed-list query —
// a list answers plain entries too; when the list is unavailable, plain
// entries fall back to exact per-name probes and regex entries fail open.
// The returned facts never alias-and-mutate the caller's value.
func enrichProbes(f *facts.Facts, modules []Module) *facts.Facts {
	c := newWhenCollector()
	for _, m := range modules {
		collectWhenLeaves(&m.Config.When, &c)
	}
	if c.empty() {
		return f
	}
	cp := *f
	if len(c.pkgPlain) > 0 || len(c.pkgRegex) > 0 {
		if cp.InstalledPackages == nil {
			cp.InstalledPackages = c.probePackages(f.Backend)
		}
	}
	if len(c.toolPlain) > 0 || len(c.toolRegex) > 0 {
		if cp.InstalledTools == nil {
			cp.InstalledTools = c.probeTools()
		}
	}
	return &cp
}

// probePackages answers plain and regex package entries: one list query
// when any regex entry exists (a list covers plain names as well), with a
// per-name exact fallback for plain entries when the list is unavailable.
func (c *whenCollector) probePackages(backend string) map[string]bool {
	if len(c.pkgRegex) > 0 {
		if installed := probeInstalledPackageList(backend); installed != nil {
			return installed
		}
	}
	if len(c.pkgPlain) > 0 {
		return probeInstalledPackages(backend, c.pkgPlain)
	}
	return nil
}

// probeTools answers plain and regex tool entries, same strategy as
// probePackages over the mise tool set.
func (c *whenCollector) probeTools() map[string]bool {
	if len(c.toolRegex) > 0 {
		if installed := probeInstalledToolList(); installed != nil {
			return installed
		}
	}
	if len(c.toolPlain) > 0 {
		return probeInstalledTools(c.toolPlain)
	}
	return nil
}

// whenCollector accumulates distinct when.packages/when.tools leaf names,
// partitioned into plain (exact-match) and regex entries.
type whenCollector struct {
	pkgPlain, pkgRegex   []string
	toolPlain, toolRegex []string
	pkgSeen, toolSeen    map[string]struct{}
}

func newWhenCollector() whenCollector {
	return whenCollector{
		pkgSeen:  map[string]struct{}{},
		toolSeen: map[string]struct{}{},
	}
}

func (c *whenCollector) empty() bool {
	return len(c.pkgPlain) == 0 && len(c.pkgRegex) == 0 &&
		len(c.toolPlain) == 0 && len(c.toolRegex) == 0
}

func (c *whenCollector) add(names []string, plain, regex *[]string, seen map[string]struct{}) {
	for _, name := range names {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		if isRegexEntry(name) {
			*regex = append(*regex, name)
		} else {
			*plain = append(*plain, name)
		}
	}
}

// collectWhenLeaves walks a when expression tree, partitioning every
// distinct package/tool leaf name into plain and regex entries.
func collectWhenLeaves(w *When, c *whenCollector) {
	c.add(w.Packages, &c.pkgPlain, &c.pkgRegex, c.pkgSeen)
	c.add(w.Tools, &c.toolPlain, &c.toolRegex, c.toolSeen)
	for i := range w.And {
		collectWhenLeaves(&w.And[i], c)
	}
	for i := range w.Or {
		collectWhenLeaves(&w.Or[i], c)
	}
	if w.Not != nil {
		collectWhenLeaves(w.Not, c)
	}
}
