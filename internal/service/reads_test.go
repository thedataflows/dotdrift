package service

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/state"
)

// Reads-area tests (T-tui-reads): each read returns the typed model the
// domain packages already produce, errors are translated at the boundary
// (SchemaError), reads take no state lock, and the canonical renderers
// reproduce the pre-move CLI bytes (goldens captured from cmd).

var updateGoldens = flag.Bool("update", false, "rewrite golden files with actual render output")

// requireGoldenSub compares got against the checked-in golden after
// substituting run-specific paths ($PROFILE, $HOME) back to their
// placeholders. Same convention as internal/generate's requireGolden,
// plus placeholder expansion for temp-dir fixtures.
func requireGoldenSub(t *testing.T, name, got string, subs map[string]string) {
	t.Helper()
	for from, to := range subs {
		got = strings.ReplaceAll(got, from, to)
	}
	path := filepath.Join("testdata", "golden", name)
	if *updateGoldens {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s", name)
	require.Equal(t, string(want), got, "golden %s", name)
}

// readProfileFixture is the checked-in disabled profile: module b selected,
// module a disabled.
func readProfileFixture(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "profiles", "disabled"))
	require.NoError(t, err)
	return dir
}

// warnRecorder records WarnLoad invocations with the observed selected count.
type warnRecorder struct {
	calls    int
	selected []int
}

func (w *warnRecorder) warn(p *profile.Profile) {
	w.calls++
	w.selected = append(w.selected, len(p.Selected))
}

func TestService_modules_list(t *testing.T) {
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	r, err := area.Modules(readProfileFixture(t), nil)
	require.NoError(t, err)
	require.NotNil(t, r.Facts)
	require.NotNil(t, r.Profile)

	ids := make([]string, 0, len(r.Profile.Selected))
	for _, m := range r.Profile.Selected {
		ids = append(ids, m.ID)
	}
	require.Equal(t, []string{"b"}, ids, "typed model is the domain profile")
	require.Len(t, r.Profile.Skipped, 1)
	require.Equal(t, "a", r.Profile.Skipped[0].Module.ID)
	require.Equal(t, "disabled", r.Profile.Skipped[0].Reason)
}

func TestService_modules_filter(t *testing.T) {
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	r, err := area.Modules(readProfileFixture(t), []string{"b"})
	require.NoError(t, err)
	require.NotNil(t, r.Profile)
	require.Len(t, r.Profile.Selected, 1)
	require.Equal(t, "b", r.Profile.Selected[0].ID)
}

func TestService_modules_unknownFilterErrors(t *testing.T) {
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	_, err := area.Modules(readProfileFixture(t), []string{"zzz"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "zzz")
}

func TestService_modules_doesNotWarn(t *testing.T) {
	// `dotdrift modules` surfaces skips in its own listing; the load-time
	// zerolog nudges are the plan/status preamble's only (parity with the
	// old cmd loadProfile).
	var w warnRecorder
	area := NewReadsArea(ReadsDeps{WarnLoad: w.warn}.WithDefaults())
	_, err := area.Modules(readProfileFixture(t), nil)
	require.NoError(t, err)
	require.Zero(t, w.calls, "the modules read never warns")
}

func TestService_plan_read(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}
	profileDir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "profiles", "resolve"))
	require.NoError(t, err)
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	r, err := area.Plan(profileDir, nil, f)
	require.NoError(t, err)

	require.NotNil(t, r.Profile)
	require.NotNil(t, r.Plan)
	require.Same(t, f, r.Facts, "injected facts pass through")
	require.NotNil(t, r.Profile)
	require.NotNil(t, r.Plan)
	require.Contains(t, r.Plan.Packages.Install, "neovim")
	require.Equal(t, resolve.Fingerprint(r.Profile, r.Facts),
		resolve.Fingerprint(r.Profile, f), "plan is the resolved domain plan")
}

func TestService_plan_detectErrorWrapped(t *testing.T) {
	area := NewReadsArea(ReadsDeps{
		Detect: func() (*facts.Facts, error) { return nil, errors.New("boom") },
	}.WithDefaults())
	_, err := area.Plan(t.TempDir(), nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "detect: boom")
}

func TestService_plan_warnsBeforeFilter(t *testing.T) {
	// The nudge must observe the pre-filter selection (parity with the old
	// loadAndResolvePlan: warn runs between load and LimitTo).
	var w warnRecorder
	area := NewReadsArea(ReadsDeps{WarnLoad: w.warn}.WithDefaults())
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}
	r, err := area.Plan(readProfileFixture(t), []string{"b"}, f)
	require.NoError(t, err)
	require.NotNil(t, r.Profile)
	require.Equal(t, 1, w.calls, "the plan read warns once")
	require.Len(t, w.selected, 1)
	require.Len(t, r.Profile.Selected, 1, "filter applied after the warn")
}

func TestService_status_read(t *testing.T) {
	dir := statusFixture(t)
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	statePath := filepath.Join(dir, "state.json")
	require.NoError(t, state.NewFileStore(statePath).Save(&state.State{LastCompleted: "packages"}))

	probesFor := func(*facts.Facts) drift.Probes {
		pr := drift.DefaultProbes()
		pr.IsInstalled = func(context.Context, string) (bool, error) { return false, nil }
		pr.ToolCurrent = func(context.Context, string) (string, error) { return "", errors.New("no mise") }
		return pr
	}

	var otherCalls int
	area := NewReadsArea(ReadsDeps{
		OtherAccounts: func(string, *facts.Facts) ([]profile.Account, error) {
			otherCalls++
			return []profile.Account{{Name: "root", Uid: "0", Home: "/root"}}, nil
		},
	}.WithDefaults())
	r, err := area.Status(t.Context(), StatusOpts{
		ProfilePath: dir,
		StatePath:   statePath,
		ProbesFor:   probesFor,
	})
	require.NoError(t, err)
	require.NotNil(t, r.State, "the status read returns the loaded state")

	require.Equal(t, statePath, r.StatePath)
	require.Equal(t, "packages", r.State.LastCompleted)
	require.Equal(t, f.Username, r.Facts.Username)
	require.Equal(t, dir, r.ProfileRoot, "profile root is absolute")
	require.Len(t, r.Findings, 1, "the absent package is one finding")
	require.Equal(t, "packages", r.Findings[0].Section)
	require.Equal(t, drift.Drift, r.Findings[0].Status)
	require.Equal(t, "missing", r.Findings[0].Detail)
	require.Len(t, r.Others, 1)
	require.Equal(t, 1, otherCalls, "the accounts listing rides the read")
}

func TestService_status_defaultStatePath(t *testing.T) {
	dir := statusFixture(t)
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	r, err := area.Status(t.Context(), StatusOpts{
		ProfilePath: dir,
		ProbesFor: func(*facts.Facts) drift.Probes {
			pr := drift.DefaultProbes()
			pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }
			return pr
		},
	})
	require.NoError(t, err)
	require.Equal(t, state.ProfileStatePath(dir), r.StatePath)
}

func TestService_status_probeErrorsStayFindings(t *testing.T) {
	// A failing tool probe is Unknown drift, never a read error.
	dir := statusFixture(t)
	probesFor := func(*facts.Facts) drift.Probes {
		pr := drift.DefaultProbes()
		pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }
		pr.ToolCurrent = func(context.Context, string) (string, error) { return "", errors.New("mise missing") }
		return pr
	}
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	r, err := area.Status(t.Context(), StatusOpts{ProfilePath: dir, ProbesFor: probesFor})
	require.NoError(t, err)
	require.NotEmpty(t, r.Findings)
}

func TestService_diff_read(t *testing.T) {
	dir, plan := diffFixture(t)
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	entries, err := area.Diff(plan, dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	e := entries[0]
	require.Equal(t, "demo", e.Module)
	require.True(t, filepath.IsAbs(e.Target))
	require.Equal(t, "theme = \"light\"\n", e.TargetContent)
	require.Equal(t, "theme = \"dark\"\n", e.SourceContent)
	require.True(t, strings.HasSuffix(e.Source, "files/config"))
}

func TestService_diff_identicalAndMissingTargets(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "same")
	require.NoError(t, os.WriteFile(target, []byte("same\n"), 0o644))
	srcDir := filepath.Join(dir, "modules", "demo", "files")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a"), []byte("same\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "b"), []byte("other\n"), 0o644))
	plan := &resolve.Plan{Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
		{Target: target, Source: "files/a", Mode: "copy", Module: "demo"},                     // identical
		{Target: filepath.Join(dir, "gone"), Source: "files/b", Mode: "copy", Module: "demo"}, // target missing
		{Target: target, Source: "files/b", Mode: "link", Module: "demo"},                     // not copy mode
	}}}
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	entries, err := area.Diff(plan, dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestSchemaError_fromStrictLoad(t *testing.T) {
	// A module.toml typo is a SchemaError naming file and line.
	dir := t.TempDir()
	modDir := filepath.Join(dir, "modules", "demo")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte("id = \"demo\"\nbogus = 1\n"), 0o644))

	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	_, err := area.Modules(dir, nil)
	require.Error(t, err)
	var schemaErr *SchemaError
	require.ErrorAs(t, err, &schemaErr, "wrapped-string load errors translate at the boundary")
	require.True(t, strings.HasSuffix(schemaErr.Path, "modules/demo/module.toml"), "got %q", schemaErr.Path)
	require.Equal(t, 2, schemaErr.Line)

	// A broken dotdrift.toml translates too.
	dir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "dotdrift.toml"), []byte("[modules\n"), 0o644))
	_, err = area.Modules(dir2, nil)
	require.Error(t, err)
	schemaErr = nil
	require.ErrorAs(t, err, &schemaErr)
	require.True(t, strings.HasSuffix(schemaErr.Path, "dotdrift.toml"), "got %q", schemaErr.Path)
	require.Equal(t, 2, schemaErr.Line, "the parser's position, extracted verbatim")

	// Non-schema failures stay plain errors.
	_, err = area.Modules(readProfileFixture(t), []string{"zzz"})
	require.Error(t, err)
	schemaErr = nil
	require.False(t, errors.As(err, &schemaErr), "filter errors are not schema errors")
}

func TestReads_lockFree(t *testing.T) {
	// Contract 11: reads never take the state lock. Holding the sidecar
	// lock must not disturb any read.
	dir := statusFixture(t)
	statePath := filepath.Join(dir, "state.json")
	require.NoError(t, state.NewFileStore(statePath).Save(&state.State{LastCompleted: "packages"}))

	store := state.NewFileStore(statePath)
	ok, err := store.TryLock()
	require.NoError(t, err)
	require.True(t, ok, "test precondition: lock acquired")
	t.Cleanup(func() { _ = store.Unlock() })

	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	probesFor := func(*facts.Facts) drift.Probes {
		pr := drift.DefaultProbes()
		pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }
		return pr
	}

	_, err = area.Modules(dir, nil)
	require.NoError(t, err)
	_, err = area.Plan(dir, nil, &facts.Facts{Hostname: "h", Username: "u", OS: "linux"})
	require.NoError(t, err)
	_, err = area.Status(t.Context(), StatusOpts{ProfilePath: dir, StatePath: statePath, ProbesFor: probesFor})
	require.NoError(t, err, "status reads work while another apply holds the lock")
}

// statusFixture writes the minimal one-package profile the status tests use.
func statusFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	modDir := filepath.Join(dir, "modules", "demo")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte("id = \"demo\"\n\n[packages]\npresent = [\"demo-pkg\"]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))
	return dir
}

// TestReads_moduleConfigAt covers the raw-declaration read the TUI's raw
// view builds on: one layer's module.toml through the doorway, with the
// schema boundary translating broken files (T-tui-shell).
func TestReads_moduleConfigAt(t *testing.T) {
	dir := statusFixture(t)
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	modDir := filepath.Join(dir, "modules", "demo")

	cfg, err := area.ModuleConfigAt(modDir)
	require.NoError(t, err)
	require.NotNil(t, cfg, "an existing module.toml loads")
	require.Equal(t, "demo", cfg.ID)

	// A directory without module.toml is "no declaration here" — the
	// profile package's load contract, not an error.
	empty, err := area.ModuleConfigAt(filepath.Join(dir, "modules", "hollow"))
	require.NoError(t, err)
	require.Nil(t, empty)

	// A broken module.toml surfaces as *SchemaError so front ends read
	// Path/Line instead of parsing text.
	broken := filepath.Join(dir, "modules", "broken")
	require.NoError(t, os.MkdirAll(broken, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(broken, "module.toml"), []byte("id = ["), 0o644))
	_, err = area.ModuleConfigAt(broken)
	require.Error(t, err)
	var schemaErr *SchemaError
	require.ErrorAs(t, err, &schemaErr, "broken declarations translate at the boundary")
	require.Equal(t, filepath.Join(broken, "module.toml"), schemaErr.Path)
}

// diffFixture writes a profile whose one copy dotfile differs from its live
// target, and returns the root plus the resolved plan.
func diffFixture(t *testing.T) (string, *resolve.Plan) {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "live-target")
	require.NoError(t, os.WriteFile(target, []byte("theme = \"light\"\n"), 0o644))
	modDir := filepath.Join(dir, "modules", "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(modDir, "files"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "files", "config"), []byte("theme = \"dark\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte("id = \"demo\"\n\n[dotfiles]\n\""+target+"\" = { source = \"files/config\", mode = \"copy\" }\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))

	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}
	area := NewReadsArea(ReadsDeps{}.WithDefaults())
	r, err := area.Plan(dir, nil, f)
	require.NoError(t, err)
	return dir, r.Plan
}
