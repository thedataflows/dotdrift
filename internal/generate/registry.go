package generate

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed registry.toml
var embeddedRegistry []byte

// OverrideEnvVar overrides the user registry path; used by tests.
const OverrideEnvVar = "DOTDRIFT_GENERATE_REGISTRY"

// ErrNoRecommendation is returned by Recommend when no entry matches the
// requested family.
var ErrNoRecommendation = errors.New("no recommended filesystem entry")

// Entry is one filesystem preset from the registry.
type Entry struct {
	// Type is the filesystem type key (e.g. "nfs", "ntfs3").
	Type string `toml:"-"`
	// Kind is KindVolume or KindNetwork.
	Kind string `toml:"kind"`
	// Family groups alternative drivers (e.g. "ntfs" for ntfs3/ntfs-3g);
	// empty means the type is its own family.
	Family string `toml:"family"`
	// Options is the mount-option preset. The bare tokens "uid" and "gid"
	// are placeholders substituted at render time.
	Options []string `toml:"options"`
	// After lists systemd unit dependencies for the generated mount unit.
	After []string `toml:"after"`
	// RecommendedIf gates the entry behind a kernel comparison
	// ("kernel <op> <version>"); empty means always eligible.
	RecommendedIf string `toml:"recommended_if"`
	// Packages are the distro packages the filesystem type needs.
	Packages []string `toml:"packages"`
}

// familyName is the entry's family, defaulting to its own type.
func (e Entry) familyName() string {
	if e.Family != "" {
		return e.Family
	}
	return e.Type
}

// Registry is the merged filesystem preset registry: the embedded presets
// plus any user override entries.
type Registry struct {
	entries map[string]Entry
}

// Entry returns the preset for a filesystem type.
func (r *Registry) Entry(typ string) (Entry, bool) {
	e, ok := r.entries[typ]
	return e, ok
}

// Entries returns all presets, sorted by type for deterministic output.
func (r *Registry) Entries() []Entry {
	out := make([]Entry, 0, len(r.entries))
	for _, typ := range slices.Sorted(maps.Keys(r.entries)) {
		out = append(out, r.entries[typ])
	}
	return out
}

// Recommend returns the recommended preset for a filesystem family,
// evaluating each candidate's recommended_if against KernelRelease.
// Candidates are considered in type-name order; an entry with an empty
// recommended_if is always eligible.
func (r *Registry) Recommend(family string) (Entry, error) {
	release, err := KernelRelease()
	if err != nil {
		return Entry{}, err
	}
	for _, typ := range slices.Sorted(maps.Keys(r.entries)) {
		e := r.entries[typ]
		if e.familyName() != family {
			continue
		}
		if e.RecommendedIf == "" {
			return e, nil
		}
		ok, err := evalRecommended(e.RecommendedIf, release)
		if err != nil {
			return Entry{}, fmt.Errorf("filesystem %q: %w", typ, err)
		}
		if ok {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("%w for family %q", ErrNoRecommendation, family)
}

// LoadDefault loads the registry with the default override resolution.
func LoadDefault() (*Registry, error) {
	return Load()
}

// Load loads the embedded registry and merges the user override file
// whole-entry by filesystem type. pathOverride, when given, wins over the
// DOTDRIFT_GENERATE_REGISTRY env var, which wins over the default
// ~/.config/dotdrift/generate.toml. A missing override file is not an
// error; a malformed one is, and the error names its path.
func Load(pathOverride ...string) (*Registry, error) {
	entries, err := parseRegistry(embeddedRegistry, "embedded registry.toml")
	if err != nil {
		return nil, err
	}
	path := overridePath(pathOverride)
	if path == "" {
		return &Registry{entries: entries}, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Registry{entries: entries}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry override %s: %w", path, err)
	}
	over, err := parseRegistry(data, path)
	if err != nil {
		return nil, err
	}
	maps.Copy(entries, over)
	return &Registry{entries: entries}, nil
}

// overridePath resolves the user override file path: explicit argument,
// then the env var, then the XDG config default.
func overridePath(pathOverride []string) string {
	if len(pathOverride) > 0 && pathOverride[0] != "" {
		return pathOverride[0]
	}
	if p := os.Getenv(OverrideEnvVar); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "dotdrift", "generate.toml")
}

type registryFile struct {
	Filesystems map[string]Entry `toml:"filesystems"`
}

// parseRegistry decodes one registry TOML document; source identifies the
// file (or the embedded blob) in error messages.
func parseRegistry(data []byte, source string) (map[string]Entry, error) {
	var f registryFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse filesystem registry %s: %w", source, err)
	}
	entries := make(map[string]Entry, len(f.Filesystems))
	for typ, e := range f.Filesystems {
		if e.Kind != KindVolume && e.Kind != KindNetwork {
			return nil, fmt.Errorf("parse filesystem registry %s: filesystem %q: kind must be %q or %q, got %q",
				source, typ, KindVolume, KindNetwork, e.Kind)
		}
		e.Type = typ
		entries[typ] = e
	}
	return entries, nil
}

var versionPattern = regexp.MustCompile(`^\d+(\.\d+)*`)

// evalRecommended evaluates one recommended_if expression. Only
// "kernel <op> <version>" is supported, with <op> one of < <= > >= == !=.
// Version comparison is numeric per dotted segment (7.10 > 7.1), with
// missing segments treated as zero.
func evalRecommended(expr, release string) (bool, error) {
	fields := strings.Fields(expr)
	if len(fields) != 3 || fields[0] != "kernel" {
		return false, fmt.Errorf("unsupported recommended_if %q: only \"kernel <op> <version>\" is supported", expr)
	}
	want, err := parseVersion(fields[2])
	if err != nil {
		return false, fmt.Errorf("invalid recommended_if %q: %w", expr, err)
	}
	got, err := parseRelease(release)
	if err != nil {
		return false, err
	}
	cmp := compareVersions(got, want)
	switch fields[1] {
	case "<":
		return cmp < 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case ">=":
		return cmp >= 0, nil
	case "==":
		return cmp == 0, nil
	case "!=":
		return cmp != 0, nil
	default:
		return false, fmt.Errorf("unsupported operator in recommended_if %q: %q", expr, fields[1])
	}
}

// parseVersion parses a strict dotted-numeric version ("7.1", "6.12.3").
func parseVersion(s string) ([]int, error) {
	if !versionPattern.MatchString(s) || versionPattern.FindString(s) != s {
		return nil, fmt.Errorf("invalid version %q: want dotted numerics", s)
	}
	return splitSegments(s), nil
}

// parseRelease extracts the leading dotted-numeric prefix of a kernel
// release ("6.12.1-arch1-1" -> 6.12.1).
func parseRelease(release string) ([]int, error) {
	m := versionPattern.FindString(strings.TrimSpace(release))
	if m == "" {
		return nil, fmt.Errorf("unparseable kernel release %q", release)
	}
	return splitSegments(m), nil
}

func splitSegments(s string) []int {
	parts := strings.Split(s, ".")
	segs := make([]int, len(parts))
	for i, p := range parts {
		segs[i], _ = strconv.Atoi(p) // pattern guarantees digits
	}
	return segs
}

// compareVersions compares segment-wise, padding the shorter side with
// zeros; it returns -1, 0, or +1.
func compareVersions(a, b []int) int {
	for i := range max(len(a), len(b)) {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
