package packages

import (
	"context"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

// PackageDeps is one node of a dependency tree. Unknown marks a package
// whose dependency query failed (e.g. unknown/AUR-less package).
type PackageDeps struct {
	Name    string
	Deps    []PackageDeps
	Unknown bool
}

// DepsTree builds the dependency tree for pkgs, recursing depth levels
// (depth 1 = direct dependencies only). Each unique package is queried at
// most once. Dependency cycles (pacman has real ones, e.g. harfbuzz ↔
// freetype2) terminate by rendering the repeated package as a bare leaf.
// A failed query never aborts the walk: the node is marked Unknown and a
// warning is logged.
func DepsTree(ctx context.Context, b Backend, pkgs []string, depth int) []PackageDeps {
	cache := make(map[string][]string)
	var build func(name string, d int, path map[string]struct{}) PackageDeps
	build = func(name string, d int, path map[string]struct{}) PackageDeps {
		node := PackageDeps{Name: name}
		if _, onPath := path[name]; onPath {
			return node
		}
		deps, ok := cache[name]
		if !ok {
			var err error
			deps, err = b.DirectDeps(ctx, name)
			if err != nil {
				log.Warn().Err(err).Msgf("deps query failed for %s; continuing", name)
				node.Unknown = true
				return node
			}
			cache[name] = deps
		}
		if d <= 1 {
			for _, dep := range deps {
				node.Deps = append(node.Deps, PackageDeps{Name: dep})
			}
			return node
		}
		path[name] = struct{}{}
		defer delete(path, name)
		for _, dep := range deps {
			node.Deps = append(node.Deps, build(dep, d-1, path))
		}
		return node
	}
	tree := make([]PackageDeps, 0, len(pkgs))
	for _, pkg := range pkgs {
		tree = append(tree, build(pkg, depth, map[string]struct{}{}))
	}
	return tree
}

// DirectDeps returns pkg's direct runtime dependencies via `paru -Si`
// (paru, not pacman: `paru -Si` also covers AUR packages, consistent with
// Present using paru).
func (p *Paru) DirectDeps(ctx context.Context, pkg string) ([]string, error) {
	out, err := p.Runner.Run(ctx, "paru", "-Si", pkg)
	if err != nil {
		return nil, err
	}
	return parsePacmanSiDepends(out), nil
}

// parsePacmanSiDepends reads the `Depends On` line of `pacman/paru -Si`
// output, stripping version specs (readline  libreadline.so=8-64 →
// readline, libreadline.so) and deduping first-occurrence.
func parsePacmanSiDepends(out string) []string {
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "Depends On") {
			continue
		}
		_, rest, ok := strings.Cut(line, ":")
		if !ok {
			return nil
		}
		var deps []string
		seen := map[string]struct{}{}
		for _, field := range strings.Fields(rest) {
			if field == "None" {
				continue
			}
			if i := strings.IndexAny(field, "=<>"); i >= 0 {
				field = field[:i]
			}
			if field == "" {
				continue
			}
			if _, dup := seen[field]; dup {
				continue
			}
			seen[field] = struct{}{}
			deps = append(deps, field)
		}
		return deps
	}
	return nil
}

// DirectDeps returns pkg's direct runtime dependencies via
// `apt-cache depends`. Unknown packages exit 100 and the error propagates.
func (a *Apt) DirectDeps(ctx context.Context, pkg string) ([]string, error) {
	out, err := a.Runner.Run(ctx, "apt-cache", "depends", pkg)
	if err != nil {
		return nil, err
	}
	return parseAptDepends(out), nil
}

var aptDependsRe = regexp.MustCompile(`^\s+(?:Pre)?Depends:\s+(\S+)`)

// parseAptDepends keeps direct Depends/PreDepends targets only: `|Depends:`
// alternative disjunctions, `<virtual>` targets, and
// Recommends/Suggests/Conflicts/Breaks/Replaces are skipped.
func parseAptDepends(out string) []string {
	var deps []string
	seen := map[string]struct{}{}
	for _, line := range strings.Split(out, "\n") {
		m := aptDependsRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		if strings.HasPrefix(name, "<") {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		deps = append(deps, name)
	}
	return deps
}

// DirectDeps returns pkg's direct runtime dependencies via
// `dnf repoquery --requires --resolve` (--resolve turns capabilities into
// provider package names; --qf prints bare names).
func (d *Dnf) DirectDeps(ctx context.Context, pkg string) ([]string, error) {
	out, err := d.Runner.Run(ctx, "dnf", "repoquery", "--requires", "--resolve", "--qf", "%{name}\n", pkg)
	if err != nil {
		return nil, err
	}
	return parseDnfRepoquery(out, pkg), nil
}

// parseDnfRepoquery reads one name per line, dropping blanks and the
// queried package itself.
func parseDnfRepoquery(out, pkg string) []string {
	var deps []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == pkg {
			continue
		}
		deps = append(deps, line)
	}
	return deps
}
