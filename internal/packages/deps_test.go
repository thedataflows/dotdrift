package packages_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/packages"
)

// Real `pacman -Si bash` shape: version specs (libreadline.so=8-64) and
// double-space separators must be stripped/split to bare names.
func TestParsePacmanSiDepends(t *testing.T) {
	out := `Name            : bash
Version         : 5.2.037-1
Description     : The GNU Bourne Again shell
Architecture    : x86_64
Depends On      : readline  libreadline.so=8-64  glibc  ncurses
Optional Deps   : bash-completion: for programmable completion
`
	// Parser is unexported; exercise it through Paru.DirectDeps with a
	// canned runner.
	r := &recordingRunner{out: out}
	paru := &packages.Paru{Runner: r}
	deps, err := paru.DirectDeps(context.Background(), "bash")
	require.NoError(t, err)
	require.Equal(t, []string{"readline", "libreadline.so", "glibc", "ncurses"}, deps)
}

func TestParsePacmanSiDepends_none(t *testing.T) {
	r := &recordingRunner{out: "Name            : foo\nDepends On      : None\n"}
	paru := &packages.Paru{Runner: r}
	deps, err := paru.DirectDeps(context.Background(), "foo")
	require.NoError(t, err)
	require.Empty(t, deps)
}

// Real `apt-cache depends` shape: PreDepends counts, |Depends alternatives
// and <virtual> targets are skipped, Recommends/Suggests/Conflicts ignored.
func TestParseAptDepends(t *testing.T) {
	out := `bash
  Depends: base-files
  Depends: bash-completion
 |Depends: debianutils
  Depends: libc6
  PreDepends: dpkg
  Depends: <debconf-2.0>
    debconf
  Recommends: bsdmainutils
  Suggests: bash-doc
  Conflicts: bash-completion
`
	r := &recordingRunner{out: out}
	apt := &packages.Apt{Runner: r}
	deps, err := apt.DirectDeps(context.Background(), "bash")
	require.NoError(t, err)
	require.Equal(t, []string{"base-files", "bash-completion", "libc6", "dpkg"}, deps)
}

// Unknown package: apt-cache exits 100 → error propagates (the tree layer
// turns it into an Unknown node).
func TestApt_DirectDeps_errorPropagates(t *testing.T) {
	boom := errors.New("E: No packages found")
	r := &recordingRunner{err: boom}
	apt := &packages.Apt{Runner: r}
	_, err := apt.DirectDeps(context.Background(), "nope")
	require.ErrorIs(t, err, boom)
}

// dnf repoquery lists provider names including the queried package itself;
// the self entry must be dropped.
func TestParseDnfRepoquery(t *testing.T) {
	r := &recordingRunner{out: "glibc\njq\noniguruma\n"}
	dnf := &packages.Dnf{Runner: r}
	deps, err := dnf.DirectDeps(context.Background(), "jq")
	require.NoError(t, err)
	require.Equal(t, []string{"glibc", "oniguruma"}, deps)
}

func TestParu_DirectDeps_argv(t *testing.T) {
	r := &recordingRunner{}
	paru := &packages.Paru{Runner: r}
	_, err := paru.DirectDeps(context.Background(), "jq")
	require.NoError(t, err)
	require.Len(t, r.calls, 1)
	require.Equal(t, "paru", r.calls[0].Name)
	require.Equal(t, []string{"-Si", "jq"}, r.calls[0].Args)
}

func TestApt_DirectDeps_argv(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	_, err := apt.DirectDeps(context.Background(), "jq")
	require.NoError(t, err)
	require.Len(t, r.calls, 1)
	require.Equal(t, "apt-cache", r.calls[0].Name)
	require.Equal(t, []string{"depends", "jq"}, r.calls[0].Args)
}

func TestDnf_DirectDeps_argv(t *testing.T) {
	r := &recordingRunner{}
	dnf := &packages.Dnf{Runner: r}
	_, err := dnf.DirectDeps(context.Background(), "jq")
	require.NoError(t, err)
	require.Len(t, r.calls, 1)
	require.Equal(t, "dnf", r.calls[0].Name)
	require.Equal(t, []string{"repoquery", "--requires", "--resolve", "--qf", "%{name}\n", "jq"}, r.calls[0].Args)
}

// treeBackend serves canned DirectDeps answers and counts queries per package.
type treeBackend struct {
	deps  map[string][]string
	err   map[string]error
	calls map[string]int
}

func newTreeBackend(deps map[string][]string) *treeBackend {
	return &treeBackend{deps: deps, calls: map[string]int{}}
}

func (b *treeBackend) Present(context.Context, []string) error      { return nil }
func (b *treeBackend) Absent(context.Context, []string) error       { return nil }
func (b *treeBackend) IsInstalled(context.Context, string) (bool, error) { return false, nil }

func (b *treeBackend) DirectDeps(_ context.Context, pkg string) ([]string, error) {
	b.calls[pkg]++
	if err := b.err[pkg]; err != nil {
		return nil, err
	}
	return b.deps[pkg], nil
}

func TestDepsTree_depthOne(t *testing.T) {
	b := newTreeBackend(map[string][]string{
		"a": {"b", "c"},
		"b": {"d"},
	})
	tree := packages.DepsTree(context.Background(), b, []string{"a"}, 1)
	require.Len(t, tree, 1)
	require.Equal(t, "a", tree[0].Name)
	require.Len(t, tree[0].Deps, 2)
	require.Equal(t, "b", tree[0].Deps[0].Name)
	require.Equal(t, "c", tree[0].Deps[1].Name)
	// Depth 1: children carry no deps of their own, and their deps are
	// never queried.
	require.Empty(t, tree[0].Deps[0].Deps)
	require.Zero(t, b.calls["b"])
	require.Zero(t, b.calls["d"])
}

func TestDepsTree_depthTwo(t *testing.T) {
	b := newTreeBackend(map[string][]string{
		"a": {"b"},
		"b": {"d"},
	})
	tree := packages.DepsTree(context.Background(), b, []string{"a"}, 2)
	require.Len(t, tree, 1)
	require.Len(t, tree[0].Deps, 1)
	require.Equal(t, "b", tree[0].Deps[0].Name)
	require.Equal(t, []packages.PackageDeps{{Name: "d"}}, tree[0].Deps[0].Deps)
}

// pacman has real dependency cycles (harfbuzz ↔ freetype2); the walk must
// terminate instead of recursing forever.
func TestDepsTree_cycleGuard(t *testing.T) {
	b := newTreeBackend(map[string][]string{
		"a": {"b"},
		"b": {"a"},
	})
	tree := packages.DepsTree(context.Background(), b, []string{"a"}, 5)
	require.Len(t, tree, 1)
	require.Len(t, tree[0].Deps, 1)
	// b's child is a again, but a is on the path: rendered as a bare leaf.
	require.Equal(t, []packages.PackageDeps{{Name: "a"}}, tree[0].Deps[0].Deps)
}

// Two parents sharing a dependency must query it once.
func TestDepsTree_memoized(t *testing.T) {
	b := newTreeBackend(map[string][]string{
		"a": {"c"},
		"b": {"c"},
		"c": {"d"},
	})
	packages.DepsTree(context.Background(), b, []string{"a", "b"}, 2)
	require.Equal(t, 1, b.calls["c"])
}

// A failed query marks the node Unknown and never aborts the walk: the
// other packages still resolve.
func TestDepsTree_queryErrorUnknown(t *testing.T) {
	b := newTreeBackend(map[string][]string{"b": {"c"}})
	b.err = map[string]error{"a": errors.New("unknown package")}
	tree := packages.DepsTree(context.Background(), b, []string{"a", "b"}, 1)
	require.Len(t, tree, 2)
	require.Equal(t, "a", tree[0].Name)
	require.True(t, tree[0].Unknown)
	require.Empty(t, tree[0].Deps)
	require.Equal(t, "b", tree[1].Name)
	require.False(t, tree[1].Unknown)
	require.Equal(t, []packages.PackageDeps{{Name: "c"}}, tree[1].Deps)
}
