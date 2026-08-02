package dotdrift

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/packages"
)

// depsBackend is a packages.Backend fake serving canned DirectDeps answers.
type depsBackend struct {
	deps map[string][]string
	err  map[string]error
}

var _ packages.Backend = (*depsBackend)(nil)

func (b *depsBackend) Present(context.Context, []string) error          { return nil }
func (b *depsBackend) Absent(context.Context, []string) error           { return nil }
func (b *depsBackend) IsInstalled(context.Context, string) (bool, error) { return false, nil }

func (b *depsBackend) DirectDeps(_ context.Context, pkg string) ([]string, error) {
	if err := b.err[pkg]; err != nil {
		return nil, err
	}
	return b.deps[pkg], nil
}

// stubPlanDeps swaps the packagesFor seam for a canned deps backend.
func stubPlanDeps(t *testing.T, b packages.Backend) {
	t.Helper()
	orig := packagesFor
	t.Cleanup(func() { packagesFor = orig })
	packagesFor = func(string) packages.Backend { return b }
}

func planDepsCmd(buf *bytes.Buffer, deps bool, depth int) *PlanCmd {
	return &PlanCmd{
		Profile:   filepath.Join("..", "testdata", "profiles", "resolve"),
		JSON:      false,
		Deps:      deps,
		DepsDepth: depth,
		Facts:     &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "test"},
		Out:       buf,
	}
}

func TestCLI_plan_depsTextRendering(t *testing.T) {
	stubPlanDeps(t, &depsBackend{deps: map[string][]string{
		"neovim": {"glibc", "libuv"},
	}})
	var buf bytes.Buffer
	c := planDepsCmd(&buf, true, 1)
	require.NoError(t, c.Run())

	out := buf.String()
	require.Contains(t, out, "  - neovim\n    deps:\n    - glibc\n    - libuv\n")

	// Without --deps the output is byte-identical to today: no deps lines.
	var plain bytes.Buffer
	c = planDepsCmd(&plain, false, 1)
	require.NoError(t, c.Run())
	require.Contains(t, plain.String(), "  - neovim\n")
	require.NotContains(t, plain.String(), "deps:")
}

func TestCLI_plan_depsDepthTwo(t *testing.T) {
	stubPlanDeps(t, &depsBackend{deps: map[string][]string{
		"neovim": {"glibc"},
		"glibc":  {"linux-api-headers"},
	}})
	var buf bytes.Buffer
	c := planDepsCmd(&buf, true, 2)
	require.NoError(t, c.Run())
	require.Contains(t, buf.String(), "    - glibc\n      deps:\n      - linux-api-headers\n")
}

func TestCLI_plan_depsDepthValidation(t *testing.T) {
	var buf bytes.Buffer
	c := planDepsCmd(&buf, true, 0)
	require.ErrorContains(t, c.Run(), "--deps-depth must be >= 1")

	buf.Reset()
	c = planDepsCmd(&buf, false, 2)
	require.ErrorContains(t, c.Run(), "--deps-depth requires --deps")
}

func TestCLI_plan_depsUnknownMarker(t *testing.T) {
	stubPlanDeps(t, &depsBackend{
		deps: map[string][]string{"fd": {"glibc"}},
		err:  map[string]error{"neovim": errors.New("unknown package")},
	})
	var buf bytes.Buffer
	c := planDepsCmd(&buf, true, 1)
	require.NoError(t, c.Run())
	require.Contains(t, buf.String(), "- neovim (deps unknown)")
	// Other packages still resolve.
	require.Contains(t, buf.String(), "  - fd\n    deps:\n    - glibc\n")
}

func TestCLI_plan_jsonDeps(t *testing.T) {
	stubPlanDeps(t, &depsBackend{deps: map[string][]string{
		"neovim": {"glibc"},
		"glibc":  {"linux-api-headers"},
	}})
	var buf bytes.Buffer
	c := planDepsCmd(&buf, true, 2)
	c.JSON = true
	require.NoError(t, c.Run())

	var doc map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	pkgs, ok := doc["packages"].(map[string]any)
	require.True(t, ok, "packages object")
	deps, ok := pkgs["deps"].([]any)
	require.True(t, ok, "packages.deps present under --deps")

	var neovim map[string]any
	for _, d := range deps {
		n := d.(map[string]any)
		if n["name"] == "neovim" {
			neovim = n
		}
	}
	require.NotNil(t, neovim, "neovim entry in deps")
	children, ok := neovim["deps"].([]any)
	require.True(t, ok, "neovim deps present at depth 2")
	require.Len(t, children, 1)
	glibc := children[0].(map[string]any)
	require.Equal(t, "glibc", glibc["name"])
	grandchildren, ok := glibc["deps"].([]any)
	require.True(t, ok, "glibc deps present at depth 2")
	require.Len(t, grandchildren, 1)
	require.Equal(t, "linux-api-headers", grandchildren[0].(map[string]any)["name"])

	// Without --deps the deps key is omitted entirely.
	buf.Reset()
	c = planDepsCmd(&buf, false, 1)
	c.JSON = true
	require.NoError(t, c.Run())
	var plain map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plain))
	_, present := plain["packages"].(map[string]any)["deps"]
	require.False(t, present, "no deps key without --deps")
}
