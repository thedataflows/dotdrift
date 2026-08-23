package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// restoreFixture builds a minimal profile (two modules) and an isolated
// HOME; tests seed backups by hand at the mirrored absolute paths.
func restoreFixture(t *testing.T) (root, home string) {
	t.Helper()
	root = t.TempDir()
	home = t.TempDir()
	t.Setenv("HOME", home)
	for _, m := range []string{"app", "other"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", m), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "modules", m, "module.toml"), []byte(""), 0o644))
	}
	orig := detectFacts
	detectFacts = func() (*facts.Facts, error) {
		return &facts.Facts{Hostname: "h", Username: "cri", OS: "linux", Backend: "paru"}, nil
	}
	t.Cleanup(func() { detectFacts = orig })
	return root, home
}

// seedBackup writes one backed-up file for target into moduleDir's
// generation, at Run's mirrored layout (full absolute target path).
func seedBackup(t *testing.T, moduleDir, gen, target, content string, mode os.FileMode) {
	t.Helper()
	bk := filepath.Join(moduleDir, "backups", gen, strings.TrimPrefix(target, "/"))
	require.NoError(t, os.MkdirAll(filepath.Dir(bk), 0o755))
	require.NoError(t, os.WriteFile(bk, []byte(content), mode))
	require.NoError(t, os.Chmod(bk, mode))
}

func runRestore(t *testing.T, root string, targets []string, gen string, dryRun bool) string {
	t.Helper()
	var out strings.Builder
	cmd := &RestoreCmd{Profile: root, Targets: targets, Gen: gen, DryRun: dryRun, Out: &out}
	require.NoError(t, cmd.Run())
	return out.String()
}

// The default generation for a target is the NEWEST one holding it; the
// restored content lands at the live target and the notice names target
// and backup location.
func TestRestore_restoresNewestBackupOfTarget(t *testing.T) {
	root, home := restoreFixture(t)
	target := filepath.Join(home, ".config", "app", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("overwritten"), 0o644))
	seedBackup(t, filepath.Join(root, "modules", "app"), "20260823-101500", target, "older backup", 0o644)
	seedBackup(t, filepath.Join(root, "modules", "app"), "20260824-153000", target, "newest backup", 0o600)

	out := runRestore(t, root, []string{"~/.config/app/config.toml"}, "", false)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "newest backup", string(got))
	info, err := os.Stat(target)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the backup's mode is restored")
	require.Contains(t, out, "restored: "+target+" (modules/app/backups/20260824-153000)")
	require.Contains(t, out, "apply overwrites them again", "a hint explains the drift consequence")
}

// --gen pins an explicit generation (as shown by --list).
func TestRestore_generationFlagPicksOlder(t *testing.T) {
	root, home := restoreFixture(t)
	target := filepath.Join(home, ".config", "app", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("overwritten"), 0o644))
	seedBackup(t, filepath.Join(root, "modules", "app"), "20260823-101500", target, "older backup", 0o644)
	seedBackup(t, filepath.Join(root, "modules", "app"), "20260824-153000", target, "newest backup", 0o644)

	runRestore(t, root, []string{target}, "20260823-101500", false)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "older backup", string(got))
}

// --dry-run prints the would-restore lines and touches nothing.
func TestRestore_dryRunTouchesNothing(t *testing.T) {
	root, home := restoreFixture(t)
	target := filepath.Join(home, ".bashrc")
	require.NoError(t, os.WriteFile(target, []byte("live"), 0o644))
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", target, "backup", 0o644)

	out := runRestore(t, root, []string{target}, "", true)

	require.Contains(t, out, "would restore: "+target)
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "live", string(got), "dry-run must not write")
}

// A target no backup holds is an error naming the target and pointing at
// --list; a --gen that exists but holds nothing names the generation (so
// a typo is visible).
func TestRestore_missingTargetErrors(t *testing.T) {
	root, home := restoreFixture(t)
	target := filepath.Join(home, ".bashrc")
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", target+".other", "x", 0o644)

	var out strings.Builder
	cmd := &RestoreCmd{Profile: root, Targets: []string{target}, Out: &out}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), target)
	require.Contains(t, err.Error(), "--list", "the error points at the browse flag")

	cmd = &RestoreCmd{Profile: root, Targets: []string{target + ".other"}, Gen: "no-such-gen", Out: &out}
	err = cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no-such-gen", "a --gen mismatch names the generation")
	require.Contains(t, err.Error(), "g1", "and the generation that does hold the target")
}

// The same target in two modules' backups is ambiguous: an error names
// both — a wrong-file silent restore is unacceptable.
func TestRestore_ambiguousTargetErrors(t *testing.T) {
	root, home := restoreFixture(t)
	target := filepath.Join(home, ".bashrc")
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", target, "from app", 0o644)
	seedBackup(t, filepath.Join(root, "modules", "other"), "g1", target, "from other", 0o644)

	cmd := &RestoreCmd{Profile: root, Targets: []string{target}, Out: &strings.Builder{}}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "modules/app")
	require.Contains(t, err.Error(), "modules/other")
}

// A symlink currently at the live target is replaced by the restored
// content file; the link's destination is untouched.
func TestRestore_replacesSymlinkedTarget(t *testing.T) {
	root, home := restoreFixture(t)
	target := filepath.Join(home, ".bashrc")
	profileSource := filepath.Join(root, "modules", "app", "home", ".bashrc")
	require.NoError(t, os.MkdirAll(filepath.Dir(profileSource), 0o755))
	require.NoError(t, os.WriteFile(profileSource, []byte("profile content"), 0o644))
	require.NoError(t, os.Symlink(profileSource, target))
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", target, "precious", 0o644)

	runRestore(t, root, []string{target}, "", false)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "precious", string(got))
	info, err := os.Lstat(target)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0), info.Mode()&os.ModeSymlink, "a real file replaces the link")
	kept, err := os.ReadFile(profileSource)
	require.NoError(t, err)
	require.Equal(t, "profile content", string(kept), "the profile source is untouched")
}

// A target the current user cannot write (system files) restores through
// the elevated seam with the backup's mode.
func TestRestore_elevatedForUnwritableTarget(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes everything; nothing is elevated")
	}
	root, home := restoreFixture(t)
	sysRoot := filepath.Join(home, "etc-root")
	target := filepath.Join(sysRoot, "app", "demo.conf")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	// Make every ancestor up to sysRoot read-only for the user.
	for dir := filepath.Dir(target); ; dir = filepath.Dir(dir) {
		require.NoError(t, os.Chmod(dir, 0o555))
		if dir == sysRoot {
			break
		}
	}
	t.Cleanup(func() {
		for dir := filepath.Dir(target); ; dir = filepath.Dir(dir) {
			_ = os.Chmod(dir, 0o755)
			if dir == sysRoot {
				break
			}
		}
	})
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", target, "system copy", 0o640)

	var calls []string
	orig := elevatedRestore
	elevatedRestore = func(src, dst string, mode os.FileMode) error {
		calls = append(calls, fmt.Sprintf("%s|%s|%04o", src, dst, mode.Perm()))
		return nil
	}
	t.Cleanup(func() { elevatedRestore = orig })

	out := runRestore(t, root, []string{target}, "", false)

	require.Len(t, calls, 1, "one elevated copy")
	require.Contains(t, calls[0], "|"+target+"|0640", "elevated copy gets the backup's mode")
	require.Contains(t, out, "restored: "+target)
}

// --list browses generations: per module with file counts, and filtered to
// the generations holding a target when one is given.
func TestRestore_list(t *testing.T) {
	root, home := restoreFixture(t)
	appDir := filepath.Join(root, "modules", "app")
	otherDir := filepath.Join(root, "modules", "other")
	target1 := filepath.Join(home, ".bashrc")
	target2 := filepath.Join(home, ".zshrc")
	seedBackup(t, appDir, "20260823-101500", target1, "old", 0o644)
	seedBackup(t, appDir, "20260823-101500", target2, "old", 0o644)
	seedBackup(t, appDir, "20260824-153000", target1, "new", 0o644)
	seedBackup(t, otherDir, "20260822-090000", target2, "elsewhere", 0o644)

	var b strings.Builder
	cmd := &RestoreCmd{Profile: root, List: true, Out: &b}
	require.NoError(t, cmd.Run())
	listing := b.String()
	require.Contains(t, listing, "modules/app:")
	require.Contains(t, listing, "20260824-153000  1 file(s)")
	require.Contains(t, listing, "20260823-101500  2 file(s)")
	require.Contains(t, listing, "modules/other:")
	require.Contains(t, listing, "20260822-090000  1 file(s)")

	b.Reset()
	cmd = &RestoreCmd{Profile: root, List: true, Targets: []string{"~/.zshrc"}, Out: &b}
	require.NoError(t, cmd.Run())
	filtered := b.String()
	require.Contains(t, filtered, "~/.zshrc")
	require.Contains(t, filtered, "  modules/app 20260823-101500")
	require.Contains(t, filtered, "  modules/other 20260822-090000")
	require.NotContains(t, filtered, root+string(filepath.Separator), "labels are profile-relative, not absolute")
	require.NotContains(t, filtered, "20260824-153000", "target-filtered listing shows only holding generations")
}

// Bare restore (no targets, no --list) is an error demanding a target.
func TestRestore_requiresTarget(t *testing.T) {
	root, _ := restoreFixture(t)
	cmd := &RestoreCmd{Profile: root, Out: &strings.Builder{}}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one target")
}

// Kong wiring: the command parses with its flags and captures targets.
func TestRestore_parse(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli, kong.Name(appName))
	require.NoError(t, err)
	_, err = parser.Parse([]string{"restore", "~/.bashrc", "/etc/x.conf", "--gen", "g1", "--dry-run"})
	require.NoError(t, err)
	require.Equal(t, []string{"~/.bashrc", "/etc/x.conf"}, cli.Restore.Targets)
	require.Equal(t, "g1", cli.Restore.Gen)
	require.True(t, cli.Restore.DryRun)
}
