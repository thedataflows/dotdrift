package drift_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// --- helpers ---

// fakeProbes returns DefaultProbes with no-op IsInstalled/ToolCurrent so Check
// never dereferences a nil; tests override the field under test.
func fakeProbes() drift.Probes {
	pr := drift.DefaultProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return false, errors.New("not wired") }
	pr.ToolCurrent = func(context.Context, string) (string, error) { return "", errors.New("not wired") }
	return pr
}

func section(findings []drift.Finding, sec string) []drift.Finding {
	var out []drift.Finding
	for _, f := range findings {
		if f.Section == sec {
			out = append(out, f)
		}
	}
	return out
}

func findByItem(t *testing.T, findings []drift.Finding, item string) drift.Finding {
	t.Helper()
	for _, f := range findings {
		if f.Item == item {
			return f
		}
	}
	t.Fatalf("no finding with item %q in %v", item, findings)
	return drift.Finding{}
}

// runFake builds a Run probe keyed by "name args...".
func runFake(responses map[string]string, errs map[string]error) func(context.Context, string, ...string) (string, error) {
	return func(_ context.Context, name string, args ...string) (string, error) {
		key := strings.Join(append([]string{name}, args...), " ")
		if err, ok := errs[key]; ok {
			return "", err
		}
		return responses[key], nil
	}
}

func check(ctx context.Context, plan *resolve.Plan, pr drift.Probes) []drift.Finding {
	return drift.Check(ctx, plan, "", pr, drift.CheckOptions{})
}

// --- packages ---

func TestCheck_packagesInstallMissing(t *testing.T) {
	pr := fakeProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return false, nil }
	plan := &resolve.Plan{Packages: resolve.PackagesStep{Install: []string{"ripgrep"}}}
	f := findByItem(t, check(context.Background(), plan, pr), "ripgrep")
	require.Equal(t, drift.Drift, f.Status)
	require.Equal(t, "missing", f.Detail)
}

func TestCheck_packagesInstallPresent(t *testing.T) {
	pr := fakeProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }
	plan := &resolve.Plan{Packages: resolve.PackagesStep{Install: []string{"ripgrep"}}}
	f := findByItem(t, check(context.Background(), plan, pr), "ripgrep")
	require.Equal(t, drift.OK, f.Status)
}

func TestCheck_packagesRemoveStillInstalled(t *testing.T) {
	pr := fakeProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }
	plan := &resolve.Plan{Packages: resolve.PackagesStep{Remove: []string{"nano"}}}
	f := findByItem(t, check(context.Background(), plan, pr), "nano")
	require.Equal(t, drift.Drift, f.Status)
	require.Equal(t, "still installed", f.Detail)
}

func TestCheck_packagesRemoveAbsent(t *testing.T) {
	pr := fakeProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return false, nil }
	plan := &resolve.Plan{Packages: resolve.PackagesStep{Remove: []string{"nano"}}}
	f := findByItem(t, check(context.Background(), plan, pr), "nano")
	require.Equal(t, drift.OK, f.Status)
}

func TestCheck_packagesProbeError(t *testing.T) {
	pr := fakeProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return false, errors.New("dpkg query failed") }
	plan := &resolve.Plan{Packages: resolve.PackagesStep{Install: []string{"jq"}}}
	f := findByItem(t, check(context.Background(), plan, pr), "jq")
	require.Equal(t, drift.Unknown, f.Status)
	require.Contains(t, f.Detail, "dpkg query failed")
}

// Packages findings are sorted by owning module first, then by package name,
// so a module's packages stay grouped in the report. Install and remove
// entries interleave under the same ordering.
func TestCheck_packagesSortedByModuleThenName(t *testing.T) {
	pr := fakeProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return false, nil }
	plan := &resolve.Plan{Packages: resolve.PackagesStep{
		Install: []string{"zzz", "aaa", "mmm"},
		Remove:  []string{"gone"},
		PresentModules: map[string][]string{
			"zzz": {"beta"}, "aaa": {"beta"}, "mmm": {"alpha"},
		},
		AbsentModules: map[string][]string{"gone": {"alpha"}},
	}}
	var got []string
	for _, f := range section(check(context.Background(), plan, pr), "packages") {
		got = append(got, f.Item)
	}
	require.Equal(t, []string{"gone", "mmm", "aaa", "zzz"}, got)
}

// --- tools ---

func TestCheck_toolsMatch(t *testing.T) {
	pr := fakeProbes()
	pr.ToolCurrent = func(_ context.Context, tool string) (string, error) { return "22", nil }
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "22"}}}
	f := findByItem(t, check(context.Background(), plan, pr), "node")
	require.Equal(t, drift.OK, f.Status)
}

func TestCheck_toolsPrefixMatch(t *testing.T) {
	pr := fakeProbes()
	pr.ToolCurrent = func(_ context.Context, tool string) (string, error) { return "22.1.0", nil }
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "22"}}}
	f := findByItem(t, check(context.Background(), plan, pr), "node")
	require.Equal(t, drift.OK, f.Status, "22.1.0 satisfies want 22 via prefix match")
}

func TestCheck_toolsMismatch(t *testing.T) {
	pr := fakeProbes()
	pr.ToolCurrent = func(_ context.Context, tool string) (string, error) { return "20.0.0", nil }
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "22"}}}
	f := findByItem(t, check(context.Background(), plan, pr), "node")
	require.Equal(t, drift.Drift, f.Status)
	require.Equal(t, "installed 20.0.0, want 22", f.Detail)
}

func TestCheck_toolsMiseMissing(t *testing.T) {
	pr := fakeProbes()
	pr.ToolCurrent = func(_ context.Context, tool string) (string, error) { return "", errors.New("mise: not found") }
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "22"}}}
	f := findByItem(t, check(context.Background(), plan, pr), "node")
	require.Equal(t, drift.Unknown, f.Status)
	require.Contains(t, f.Detail, "mise not available")
}

func TestCheck_toolsSortedByName(t *testing.T) {
	pr := fakeProbes()
	pr.ToolCurrent = func(_ context.Context, tool string) (string, error) { return "1", nil }
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"zls": "1", " node": "1", "aardvark": "1"}}}
	// drop the stray-space entry to keep it simple
	plan.Tools.Versions = map[string]string{"zls": "1", "node": "1", "aardvark": "1"}
	fs := section(check(context.Background(), plan, pr), "tools")
	var names []string
	for _, f := range fs {
		names = append(names, f.Item)
	}
	require.Equal(t, []string{"aardvark", "node", "zls"}, names, "tools must be sorted by name")
}

// --- dotfiles (real files on disk + DefaultProbes) ---

func dotfilePlan(entries ...resolve.DotfileEntry) *resolve.Plan {
	return &resolve.Plan{Dotfiles: resolve.DotfilesStep{Entries: entries}}
}

func TestCheck_dotfilesSymlinkOK(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "bashrc")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	tgt := filepath.Join(t.TempDir(), ".bashrc")
	require.NoError(t, os.Symlink(src, tgt))

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "symlink"}), root, pr, drift.CheckOptions{})
	require.Len(t, fs, 1)
	require.Equal(t, drift.OK, fs[0].Status)
}

func TestCheck_dotfilesSymlinkWrongTarget(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "bashrc")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	other := filepath.Join(root, "other")
	require.NoError(t, os.WriteFile(other, []byte("y"), 0o644))
	tgt := filepath.Join(t.TempDir(), ".bashrc")
	require.NoError(t, os.Symlink(other, tgt))

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "symlink"}), root, pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Contains(t, fs[0].Detail, "points to")
}

func TestCheck_dotfilesSymlinkMissing(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "bashrc")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	tgt := filepath.Join(t.TempDir(), ".bashrc") // never created

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "symlink"}), root, pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "missing", fs[0].Detail)
}

func TestCheck_dotfilesSymlinkRegularFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "gtkrc")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	tgt := filepath.Join(t.TempDir(), ".gtkrc-2.0")
	require.NoError(t, os.WriteFile(tgt, []byte("managed by hand"), 0o644)) // regular file, not a symlink

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "symlink"}), root, pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "not a symlink", fs[0].Detail,
		"a regular file at the target is drift but not 'missing'")
}

func TestCheck_dotfilesCopyEqual(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "foo")
	require.NoError(t, os.WriteFile(src, []byte("same"), 0o644))
	tgt := filepath.Join(t.TempDir(), "foo")
	require.NoError(t, os.WriteFile(tgt, []byte("same"), 0o644))

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "copy"}), root, pr, drift.CheckOptions{})
	require.Equal(t, drift.OK, fs[0].Status)
}

func TestCheck_dotfilesCopyDiffers(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "foo")
	require.NoError(t, os.WriteFile(src, []byte("new"), 0o644))
	tgt := filepath.Join(t.TempDir(), "foo")
	require.NoError(t, os.WriteFile(tgt, []byte("old"), 0o644))

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "copy"}), root, pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "content differs", fs[0].Detail)
}

func TestCheck_dotfilesCopyMissingTarget(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "foo")
	require.NoError(t, os.WriteFile(src, []byte("new"), 0o644))
	tgt := filepath.Join(t.TempDir(), "foo") // absent

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "copy"}), root, pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "missing", fs[0].Detail)
}

func TestCheck_dotfilesTemplateExists(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "cfg")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	tgt := filepath.Join(t.TempDir(), "cfg")
	require.NoError(t, os.WriteFile(tgt, []byte("rendered"), 0o644))

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "template"}), root, pr, drift.CheckOptions{})
	require.Equal(t, drift.OK, fs[0].Status, "template is existence-only; rendered content is not compared")
}

func TestCheck_dotfilesTemplateMissing(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "cfg")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	tgt := filepath.Join(t.TempDir(), "cfg") // absent

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "template"}), root, pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "missing", fs[0].Detail)
}

func TestCheck_dotfilesSymlinkEachPerChild(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "units")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.mount"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "b.timer"), []byte("x"), 0o644))
	tgtDir := filepath.Join(t.TempDir(), "system")
	require.NoError(t, os.MkdirAll(tgtDir, 0o755))
	// Create real symlinks for both children.
	require.NoError(t, os.Symlink(filepath.Join(srcDir, "a.mount"), filepath.Join(tgtDir, "a.mount")))
	require.NoError(t, os.Symlink(filepath.Join(srcDir, "b.timer"), filepath.Join(tgtDir, "b.timer")))

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgtDir, Source: "units", Mode: "symlink-each"}), root, pr, drift.CheckOptions{})
	require.Len(t, fs, 2, "symlink-each expands to one finding per child")
	for _, f := range fs {
		require.Equal(t, drift.OK, f.Status)
	}
}

func TestCheck_dotfilesExpansionError(t *testing.T) {
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	// symlink-each against a missing source dir → ResolveBootstrapFiles errors.
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: "/etc/x", Source: "nope", Mode: "symlink-each"}), t.TempDir(), pr, drift.CheckOptions{})
	require.Len(t, fs, 1)
	require.Equal(t, drift.Unknown, fs[0].Status)
}

// --- edit entries (line/block/template drift probes) ---

func TestCheck_dotfileEditLinePresent(t *testing.T) {
	tgt := filepath.Join(t.TempDir(), "hosts")
	require.NoError(t, os.WriteFile(tgt, []byte("127.0.0.1 localhost\n127.0.0.1 dev.local\n"), 0o644))
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt + "/dev", Line: "127.0.0.1 dev.local", Module: "m"}),
		t.TempDir(), pr, drift.CheckOptions{})
	require.Len(t, fs, 1)
	require.Equal(t, drift.OK, fs[0].Status)
}

func TestCheck_dotfileEditLineMissing(t *testing.T) {
	tgt := filepath.Join(t.TempDir(), "hosts")
	require.NoError(t, os.WriteFile(tgt, []byte("127.0.0.1 localhost\n"), 0o644))
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt + "/dev", Line: "127.0.0.1 dev.local", Module: "m"}),
		t.TempDir(), pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "missing", fs[0].Detail)
}

func TestCheck_dotfileEditBlockMatch(t *testing.T) {
	tgt := filepath.Join(t.TempDir(), "zshrc")
	require.NoError(t, os.WriteFile(tgt, []byte(
		"# header\n# >>> mise:activate >>> managed by mise\nalias ll='ls -l'\nalias la='ls -la'\n# <<< mise:activate <<<\n"), 0o644))
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt + "/activate", Block: "alias ll='ls -l'\nalias la='ls -la'", Module: "m"}),
		t.TempDir(), pr, drift.CheckOptions{})
	require.Equal(t, drift.OK, fs[0].Status)
}

func TestCheck_dotfileEditBlockDiffers(t *testing.T) {
	tgt := filepath.Join(t.TempDir(), "zshrc")
	require.NoError(t, os.WriteFile(tgt, []byte(
		"# >>> mise:activate >>>\nstale\n# <<< mise:activate <<<\n"), 0o644))
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt + "/activate", Block: "alias ll='ls -l'", Module: "m"}),
		t.TempDir(), pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "content differs", fs[0].Detail)
}

func TestCheck_dotfileEditBlockMissing(t *testing.T) {
	tgt := filepath.Join(t.TempDir(), "zshrc")
	require.NoError(t, os.WriteFile(tgt, []byte("# nothing here\n"), 0o644))
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt + "/activate", Block: "alias ll='ls -l'", Module: "m"}),
		t.TempDir(), pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "missing", fs[0].Detail)
}

func TestCheck_dotfileEditBlockCorrupted(t *testing.T) {
	tgt := filepath.Join(t.TempDir(), "zshrc")
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"open-only", "# >>> mise:activate >>>\nalias ll='ls -l'\n"},
		{"close-before-open", "# <<< mise:activate <<<\n# >>> mise:activate >>>\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(tgt, []byte(tc.content), 0o644))
			fs := drift.Check(context.Background(),
				dotfilePlan(resolve.DotfileEntry{Target: tgt + "/activate", Block: "alias ll='ls -l'", Module: "m"}),
				t.TempDir(), pr, drift.CheckOptions{})
			require.Equal(t, drift.Drift, fs[0].Status)
			require.Equal(t, "corrupted markers", fs[0].Detail)
		})
	}
}

func TestCheck_dotfileEditSymlinkTarget(t *testing.T) {
	src := filepath.Join(t.TempDir(), "real")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	tgt := filepath.Join(t.TempDir(), "zshrc")
	require.NoError(t, os.Symlink(src, tgt))
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt + "/activate", Block: "alias ll='ls -l'", Module: "m"}),
		t.TempDir(), pr, drift.CheckOptions{})
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "target is a symlink", fs[0].Detail)
}

func TestCheck_dotfileEditTemplateMarkerPresenceOnly(t *testing.T) {
	tgt := filepath.Join(t.TempDir(), "gitconfig")
	// Markers present but content differs from the source template — still OK,
	// because rendering needs mise's tera engine (existence-only, like whole-file template).
	require.NoError(t, os.WriteFile(tgt, []byte("# >>> mise:id >>>\nrendered-but-different\n# <<< mise:id <<<\n"), 0o644))
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt + "/id", Source: "git.tmpl", Template: "tera", Module: "m"}),
		t.TempDir(), pr, drift.CheckOptions{})
	require.Equal(t, drift.OK, fs[0].Status)
}

func TestCheck_dotfileEditUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-denial test is meaningless as root")
	}
	tgt := filepath.Join(t.TempDir(), "zshrc")
	require.NoError(t, os.WriteFile(tgt, []byte("x"), 0o644))
	require.NoError(t, os.Chmod(tgt, 0o000))
	t.Cleanup(func() { _ = os.Chmod(tgt, 0o644) })
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	pr.ReadFile = os.ReadFile // real ReadFile surfaces EACCES
	fs := drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt + "/activate", Block: "alias ll='ls -l'", Module: "m"}),
		t.TempDir(), pr, drift.CheckOptions{})
	require.Equal(t, drift.Unknown, fs[0].Status, "EACCES → Unknown")
}

func mountPlan(spec profile.MountSpec) *resolve.Plan {
	return &resolve.Plan{Mounts: resolve.MountsStep{Entries: []resolve.MountEntry{
		{Name: "m", Spec: spec, Module: "mod"},
	}}}
}

func mountProbes(t *testing.T, isEnabledOut, isActiveOut string, runErr error) drift.Probes {
	t.Helper()
	pr := fakeProbes()
	pr.StatDir = func(string) (bool, error) { return true, nil }
	unit := "mnt-data" // generate.EscapePath("/mnt/data")
	pr.Run = func(_ context.Context, name string, args ...string) (string, error) {
		key := strings.Join(append([]string{name}, args...), " ")
		switch key {
		case "systemctl is-enabled " + unit + ".mount":
			if runErr != nil {
				return "", runErr
			}
			return isEnabledOut, nil
		case "systemctl is-active " + unit + ".mount":
			return isActiveOut, nil
		}
		return "", errors.New("unexpected run: " + key)
	}
	return pr
}

func TestCheck_mountEnabledOK(t *testing.T) {
	pr := mountProbes(t, "enabled", "active", nil)
	fs := check(context.Background(), mountPlan(profile.MountSpec{Destination: "/mnt/data", State: "enabled"}), pr)
	require.Len(t, fs, 1)
	require.Equal(t, drift.OK, fs[0].Status)
}

func TestCheck_mountNotEnabled(t *testing.T) {
	pr := mountProbes(t, "disabled", "", nil)
	fs := check(context.Background(), mountPlan(profile.MountSpec{Destination: "/mnt/data", State: "enabled"}), pr)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "not enabled", fs[0].Detail)
}

func TestCheck_mountNotActive(t *testing.T) {
	pr := mountProbes(t, "enabled", "inactive", nil)
	fs := check(context.Background(), mountPlan(profile.MountSpec{Destination: "/mnt/data", State: "enabled"}), pr)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "not active", fs[0].Detail)
}

func TestCheck_mountDisabledOK(t *testing.T) {
	pr := mountProbes(t, "disabled", "", nil)
	fs := check(context.Background(), mountPlan(profile.MountSpec{Destination: "/mnt/data", State: "disabled"}), pr)
	require.Equal(t, drift.OK, fs[0].Status)
}

func TestCheck_mountDisabledButEnabled(t *testing.T) {
	pr := mountProbes(t, "enabled", "active", nil)
	fs := check(context.Background(), mountPlan(profile.MountSpec{Destination: "/mnt/data", State: "disabled"}), pr)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Contains(t, fs[0].Detail, "enabled but declared disabled")
}

func TestCheck_mountUnitNotFound(t *testing.T) {
	pr := mountProbes(t, "", "", errors.New("exit 1"))
	fs := check(context.Background(), mountPlan(profile.MountSpec{Destination: "/mnt/data", State: "enabled"}), pr)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "unit not found", fs[0].Detail)
}

func TestCheck_mountDestinationMissing(t *testing.T) {
	pr := fakeProbes()
	pr.StatDir = func(string) (bool, error) { return false, nil }
	fs := check(context.Background(), mountPlan(profile.MountSpec{Destination: "/mnt/data", State: "enabled"}), pr)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "destination missing", fs[0].Detail)
}

func TestCheck_mountUnexpectedIsEnabled(t *testing.T) {
	pr := mountProbes(t, "masked", "", nil)
	fs := check(context.Background(), mountPlan(profile.MountSpec{Destination: "/mnt/data", State: "enabled"}), pr)
	require.Equal(t, drift.Unknown, fs[0].Status)
	require.Contains(t, fs[0].Detail, "unexpected is-enabled output")
}

// --- smb ---

func smbPlan(mod resolve.SmbModuleSpec) *resolve.Plan {
	return &resolve.Plan{Smb: resolve.SmbStep{Modules: []resolve.SmbModuleSpec{mod}}}
}

func TestCheck_smbGroupMissing(t *testing.T) {
	pr := fakeProbes()
	pr.Run = runFake(nil, map[string]error{"getent group smb": errors.New("no")})
	fs := section(check(context.Background(), smbPlan(resolve.SmbModuleSpec{Spec: profile.SmbSpec{}}), pr), "smb")
	// group missing + smb service (unit not found) + avahi service (unit not found)
	grp := findByItem(t, fs, "smb")
	// "smb" appears as both group finding and service finding; the group one is Drift.
	foundDrift := false
	for _, f := range fs {
		if f.Item == "smb" && f.Status == drift.Drift && strings.Contains(f.Detail, "group smb missing") {
			foundDrift = true
		}
	}
	require.True(t, foundDrift, "group smb missing drift finding must be present in %v", fs)
	_ = grp
}

func TestCheck_smbUserMissing(t *testing.T) {
	pr := fakeProbes()
	pr.Run = runFake(
		map[string]string{"getent group smb": "smb"},
		map[string]error{"id -Gn alice": errors.New("no such user")},
	)
	mod := resolve.SmbModuleSpec{Spec: profile.SmbSpec{Users: []string{"alice"}}}
	f := findByItem(t, check(context.Background(), smbPlan(mod), pr), "alice")
	require.Equal(t, drift.Drift, f.Status)
	require.Contains(t, f.Detail, "user alice missing")
}

func TestCheck_smbUserNotInGroup(t *testing.T) {
	pr := fakeProbes()
	pr.Run = runFake(map[string]string{
		"getent group smb": "smb",
		"id -Gn alice":     "users wheel",
	}, nil)
	mod := resolve.SmbModuleSpec{Spec: profile.SmbSpec{Users: []string{"alice"}}}
	f := findByItem(t, check(context.Background(), smbPlan(mod), pr), "alice")
	require.Equal(t, drift.Drift, f.Status)
	require.Contains(t, f.Detail, "not in group")
}

func TestCheck_smbUserInGroupOK(t *testing.T) {
	pr := fakeProbes()
	pr.Run = runFake(map[string]string{
		"getent group smb": "smb",
		"id -Gn alice":     "smb wheel",
	}, nil)
	mod := resolve.SmbModuleSpec{Spec: profile.SmbSpec{Users: []string{"alice"}}}
	f := findByItem(t, check(context.Background(), smbPlan(mod), pr), "alice")
	require.Equal(t, drift.OK, f.Status)
}

func TestCheck_smbServiceNotActive(t *testing.T) {
	pr := fakeProbes()
	pr.Run = runFake(map[string]string{
		"systemctl is-enabled smb":          "enabled",
		"systemctl is-active smb":           "inactive",
		"systemctl is-enabled avahi-daemon": "enabled",
		"systemctl is-active avahi-daemon":  "active",
	}, nil)
	mod := resolve.SmbModuleSpec{Spec: profile.SmbSpec{}}
	var svc drift.Finding
	for _, f := range check(context.Background(), smbPlan(mod), pr) {
		if f.Item == "smb" && f.Status == drift.Drift {
			svc = f
		}
	}
	require.Equal(t, drift.Drift, svc.Status)
	require.Equal(t, "not active", svc.Detail)
}

func TestCheck_smbShareAbsent(t *testing.T) {
	pr := fakeProbes()
	pr.Run = runFake(map[string]string{
		"getent group smb":                  "smb",
		"systemctl is-enabled smb":          "enabled",
		"systemctl is-active smb":           "active",
		"systemctl is-enabled avahi-daemon": "enabled",
		"systemctl is-active avahi-daemon":  "active",
		"testparm -s":                       "[homes]\n",
	}, nil)
	mod := resolve.SmbModuleSpec{Spec: profile.SmbSpec{Shares: map[string]profile.ShareSpec{"media": {Path: "/srv/media"}}}}
	f := findByItem(t, check(context.Background(), smbPlan(mod), pr), "media")
	require.Equal(t, drift.Drift, f.Status)
	require.Contains(t, f.Detail, "share media not in smb config")
}

func TestCheck_smbSharePresent(t *testing.T) {
	pr := fakeProbes()
	pr.Run = runFake(map[string]string{
		"getent group smb":                  "smb",
		"systemctl is-enabled smb":          "enabled",
		"systemctl is-active smb":           "active",
		"systemctl is-enabled avahi-daemon": "enabled",
		"systemctl is-active avahi-daemon":  "active",
		"testparm -s":                       "[global]\n[media]\npath=/srv/media\n",
	}, nil)
	mod := resolve.SmbModuleSpec{Spec: profile.SmbSpec{Shares: map[string]profile.ShareSpec{"media": {Path: "/srv/media"}}}}
	f := findByItem(t, check(context.Background(), smbPlan(mod), pr), "media")
	require.Equal(t, drift.OK, f.Status)
}

func TestCheck_smbTestparmFailed(t *testing.T) {
	pr := fakeProbes()
	pr.Run = runFake(
		map[string]string{
			"getent group smb":                  "smb",
			"systemctl is-enabled smb":          "enabled",
			"systemctl is-active smb":           "active",
			"systemctl is-enabled avahi-daemon": "enabled",
			"systemctl is-active avahi-daemon":  "active",
		},
		map[string]error{"testparm -s": errors.New("boom")},
	)
	mod := resolve.SmbModuleSpec{Spec: profile.SmbSpec{Shares: map[string]profile.ShareSpec{"media": {Path: "/srv/media"}}}}
	f := findByItem(t, check(context.Background(), smbPlan(mod), pr), "media")
	require.Equal(t, drift.Unknown, f.Status)
	require.Equal(t, "testparm failed", f.Detail)
}

func TestCheck_smbAvahiOnlyWhenEnabled(t *testing.T) {
	// avahi unset (nil) → avahi checked; a finding for avahi-daemon appears.
	pr := fakeProbes()
	pr.Run = runFake(map[string]string{
		"getent group smb":                  "smb",
		"systemctl is-enabled smb":          "enabled",
		"systemctl is-active smb":           "active",
		"systemctl is-enabled avahi-daemon": "enabled",
		"systemctl is-active avahi-daemon":  "active",
	}, nil)
	mod := resolve.SmbModuleSpec{Spec: profile.SmbSpec{}} // Avahi nil
	fs := check(context.Background(), smbPlan(mod), pr)
	require.NotEmpty(t, findByItem(t, fs, "avahi-daemon").Item)

	// avahi explicitly false → avahi NOT checked.
	off := false
	mod2 := resolve.SmbModuleSpec{Spec: profile.SmbSpec{Avahi: &off}}
	pr2 := fakeProbes()
	pr2.Run = pr.Run
	fs2 := check(context.Background(), smbPlan(mod2), pr2)
	for _, f := range fs2 {
		require.NotEqual(t, "avahi-daemon", f.Item, "avahi-daemon must not be checked when avahi is false")
	}
}

// --- ordering & omission ---

func TestCheck_sectionOrder(t *testing.T) {
	pr := fakeProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }
	pr.ToolCurrent = func(_ context.Context, _ string) (string, error) { return "1", nil }
	plan := &resolve.Plan{
		Packages: resolve.PackagesStep{Install: []string{"a"}},
		Tools:    resolve.ToolsStep{Versions: map[string]string{"t": "1"}},
	}
	fs := check(context.Background(), plan, pr)
	var secs []string
	for _, f := range fs {
		if len(secs) == 0 || secs[len(secs)-1] != f.Section {
			secs = append(secs, f.Section)
		}
	}
	require.Equal(t, []string{"packages", "tools"}, secs)
}

func TestCheck_zeroItemSectionsOmitted(t *testing.T) {
	pr := fakeProbes()
	plan := &resolve.Plan{Packages: resolve.PackagesStep{Install: []string{"a"}}}
	pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }
	fs := check(context.Background(), plan, pr)
	for _, f := range fs {
		require.Equal(t, "packages", f.Section, "only packages section should produce findings")
	}
}

func TestCheck_nilPlan(t *testing.T) {
	require.Nil(t, drift.Check(context.Background(), nil, "", fakeProbes(), drift.CheckOptions{}))
}

// --- concurrency ---

func TestCheck_concurrentOrderStable(t *testing.T) {
	pr := fakeProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }
	pr.ToolCurrent = func(context.Context, string) (string, error) { return "1.0.0", nil }
	pr.StatDir = func(string) (bool, error) { return true, nil }
	pr.Run = runFake(map[string]string{
		"systemctl is-enabled mnt-data.mount": "enabled",
		"systemctl is-active mnt-data.mount":  "active",
		"getent group smb":                    "smb",
		"id -Gn alice":                        "smb wheel",
		"systemctl is-enabled smb":            "enabled",
		"systemctl is-active smb":             "active",
		"systemctl is-enabled avahi-daemon":   "enabled",
		"systemctl is-active avahi-daemon":    "active",
		"testparm -s":                         "[global]\n[media]\npath=/srv/media\n",
	}, nil)

	plan := &resolve.Plan{
		Packages: resolve.PackagesStep{Install: []string{"pkg-a"}, Remove: []string{"pkg-b"}},
		Tools:    resolve.ToolsStep{Versions: map[string]string{"go": "1.0.0"}},
		Mounts: resolve.MountsStep{Entries: []resolve.MountEntry{
			{Name: "m", Spec: profile.MountSpec{Destination: "/mnt/data", State: "enabled"}, Module: "mod"},
		}},
		Smb: resolve.SmbStep{Modules: []resolve.SmbModuleSpec{
			{Spec: profile.SmbSpec{Users: []string{"alice"}, Shares: map[string]profile.ShareSpec{"media": {Path: "/srv/media"}}}},
		}},
	}

	seq := drift.Check(context.Background(), plan, "", pr, drift.CheckOptions{Jobs: 1})
	par := drift.Check(context.Background(), plan, "", pr, drift.CheckOptions{Jobs: 8})
	require.Equal(t, seq, par)
}

func TestCheck_runsConcurrently(t *testing.T) {
	pr := fakeProbes()
	arrived := make(chan string, 4)
	release := make(chan struct{})
	pr.IsInstalled = func(_ context.Context, pkg string) (bool, error) {
		arrived <- pkg
		<-release
		return true, nil
	}

	plan := &resolve.Plan{
		Packages: resolve.PackagesStep{Install: []string{"p1", "p2", "p3", "p4"}},
	}

	done := make(chan []drift.Finding, 1)
	go func() {
		done <- drift.Check(context.Background(), plan, "", pr, drift.CheckOptions{Jobs: 4})
	}()

	// All 4 probes must arrive before any finishes (each blocks on release).
	for range 4 {
		select {
		case <-arrived:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for concurrent probe starts — probes did not run concurrently")
		}
	}
	close(release)

	select {
	case fs := <-done:
		require.Len(t, fs, 4)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Check to finish")
	}
}

func TestCheck_verboseProgress(t *testing.T) {
	pr := fakeProbes()
	pr.IsInstalled = func(context.Context, string) (bool, error) { return true, nil }

	plan := &resolve.Plan{
		Packages: resolve.PackagesStep{Install: []string{"alpha", "beta"}, Remove: []string{"gamma"}},
	}

	var buf bytes.Buffer
	fs := drift.Check(context.Background(), plan, "", pr, drift.CheckOptions{Verbose: &buf})

	require.Equal(t, len(fs), strings.Count(buf.String(), "\n"), "one verbose line per finding")
	for _, f := range fs {
		require.Contains(t, buf.String(), "checking "+f.Section+": "+f.Item)
	}
}

// --- Render ---

func TestRender_mixed(t *testing.T) {
	findings := []drift.Finding{
		{Section: "packages", Item: "ripgrep", Status: drift.Drift, Detail: "missing"},
		{Section: "packages", Item: "jq", Status: drift.Unknown, Detail: "dpkg query failed"},
		{Section: "tools", Item: "node", Status: drift.OK},
		{Section: "tools", Item: "go", Status: drift.OK},
	}
	var buf bytes.Buffer
	drift.Render(&buf, findings)
	want := "packages:\n" +
		"  ?: ripgrep — missing\n" +
		"  ?: jq — dpkg query failed (?)\n" +
		"tools:\n" +
		"  ok: all 2 checks passed\n" +
		"\n" +
		"drift: 1 item, 1 unknown\n"
	require.Equal(t, want, buf.String())
}

func TestRender_modulePrefix(t *testing.T) {
	var buf bytes.Buffer
	drift.Render(&buf, []drift.Finding{
		{Section: "packages", Item: "neovim", Status: drift.Drift, Detail: "missing", Module: "editor"},
		{Section: "packages", Item: "vim", Status: drift.Drift, Detail: "missing", Module: ""},
	})
	out := buf.String()
	require.Contains(t, out, "editor: neovim — missing")
	require.Contains(t, out, "?: vim — missing")
}

func TestRender_noColorDisablesANSI(t *testing.T) {
	origTerminal, origNoColor := executil.IsTerminal, executil.NoColor
	t.Cleanup(func() { executil.IsTerminal, executil.NoColor = origTerminal, origNoColor })

	executil.IsTerminal = func(io.Writer) bool { return true }
	executil.NoColor = true

	var buf bytes.Buffer
	drift.Render(&buf, []drift.Finding{
		{Section: "packages", Item: "neovim", Status: drift.Drift, Detail: "missing", Module: "editor"},
	})
	require.NotContains(t, buf.String(), "\033[", "no ANSI codes when NoColor is set, even on a terminal")
}

func TestRender_allOK(t *testing.T) {
	var buf bytes.Buffer
	drift.Render(&buf, []drift.Finding{
		{Section: "packages", Item: "ripgrep", Status: drift.OK},
	})
	require.Equal(t, "packages:\n  ok: all 1 checks passed\n\nno drift\n", buf.String())
}

func TestRender_noFindings(t *testing.T) {
	var buf bytes.Buffer
	drift.Render(&buf, nil)
	require.Equal(t, "no drift\n", buf.String())
}

func TestRender_pluralSummary(t *testing.T) {
	var buf bytes.Buffer
	drift.Render(&buf, []drift.Finding{
		{Section: "packages", Item: "a", Status: drift.Drift, Detail: "missing"},
		{Section: "packages", Item: "b", Status: drift.Drift, Detail: "missing"},
	})
	require.Contains(t, buf.String(), "drift: 2 items\n")
}

// With color on, finding lines render the module and the item in different
// shades of the same (issue-type) hue — faint for the module, bold for the
// item — in every section.
func TestRender_shadesModuleVsItem(t *testing.T) {
	origTerminal, origNoColor := executil.IsTerminal, executil.NoColor
	t.Cleanup(func() { executil.IsTerminal, executil.NoColor = origTerminal, origNoColor })
	executil.IsTerminal = func(io.Writer) bool { return true }
	executil.NoColor = false

	var buf bytes.Buffer
	drift.Render(&buf, []drift.Finding{
		{Section: "packages", Item: "bbb", Status: drift.Drift, Detail: "missing", Module: "alpha"},
		{Section: "dotfiles", Item: "~/.zshrc", Status: drift.Drift, Detail: "content differs", Module: "zsh"},
	})
	out := buf.String()
	missing := "\033[38;5;208m" // missing → orange
	require.Contains(t, out, "\033[2m"+missing+"alpha\033[0m", "packages: module in faint shade of the finding hue")
	require.Contains(t, out, "\033[1m"+missing+"bbb\033[0m", "packages: item in bold shade of the finding hue")
	require.Contains(t, out, missing+" — missing\033[0m", "detail in the plain finding hue")
	differs := "\033[33m" // content differs → yellow
	require.Contains(t, out, "\033[2m"+differs+"zsh\033[0m", "dotfiles: module in faint shade of the finding hue")
	require.Contains(t, out, "\033[1m"+differs+"~/.zshrc\033[0m", "dotfiles: item in bold shade of the finding hue")
}

// The hue follows the issue type in every section: an unknown finding shades
// red, a version/content mismatch yellow, anything else orange.
func TestRender_shadesFollowIssueType(t *testing.T) {
	origTerminal, origNoColor := executil.IsTerminal, executil.NoColor
	t.Cleanup(func() { executil.IsTerminal, executil.NoColor = origTerminal, origNoColor })
	executil.IsTerminal = func(io.Writer) bool { return true }
	executil.NoColor = false

	var buf bytes.Buffer
	drift.Render(&buf, []drift.Finding{
		{Section: "packages", Item: "jq", Status: drift.Unknown, Detail: "dpkg query failed", Module: "alpha"},
		{Section: "tools", Item: "node", Status: drift.Drift, Detail: "installed 1, want 2", Module: "alpha"},
		{Section: "mounts", Item: "/mnt/data", Status: drift.Drift, Detail: "not enabled", Module: "nas"},
	})
	out := buf.String()
	require.Contains(t, out, "\033[2m\033[31malpha\033[0m", "unknown → faint red module")
	require.Contains(t, out, "\033[1m\033[31mjq\033[0m", "unknown → bold red item")
	require.Contains(t, out, "\033[2m\033[33malpha\033[0m", "version drift → faint yellow module")
	require.Contains(t, out, "\033[1m\033[33mnode\033[0m", "version drift → bold yellow item")
	require.Contains(t, out, "\033[2m\033[38;5;208mnas\033[0m", "other drift → faint orange module")
	require.Contains(t, out, "\033[1m\033[38;5;208m/mnt/data\033[0m", "other drift → bold orange item")
}
