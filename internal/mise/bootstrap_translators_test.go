package mise_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// --- ResolveBootstrapFiles ---

func TestResolveBootstrapFiles_copyEntry(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "foo.conf"), []byte("x"), 0o644))
	entries := []resolve.DotfileEntry{{Target: "/etc/foo.conf", Source: "foo.conf", Mode: "copy"}}
	files, err := mise.ResolveBootstrapFiles(entries, root, "/home/user")
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "/etc/foo.conf", files[0].Target)
	require.Equal(t, filepath.Join(root, "foo.conf"), files[0].Source)
	require.False(t, files[0].Template)
}

func TestResolveBootstrapFiles_templateEntry(t *testing.T) {
	root := t.TempDir()
	entries := []resolve.DotfileEntry{{Target: "/etc/cfg.tmpl", Source: "cfg.tmpl", Mode: "template"}}
	files, err := mise.ResolveBootstrapFiles(entries, root, "/home/u")
	require.NoError(t, err)
	require.True(t, files[0].Template)
}

func TestResolveBootstrapFiles_homeExpansion(t *testing.T) {
	root := t.TempDir()
	entries := []resolve.DotfileEntry{{Target: "~/ secret", Source: "s", Mode: "copy"}}
	files, err := mise.ResolveBootstrapFiles(entries, root, "/home/cri")
	require.NoError(t, err)
	require.Equal(t, "/home/cri/ secret", files[0].Target)
}

func TestResolveBootstrapFiles_symlinkEachExpandsDir(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "units")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.mount"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "b.timer"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0o755)) // skipped

	entries := []resolve.DotfileEntry{{Target: "/etc/systemd/system", Source: "units", Mode: "symlink-each"}}
	files, err := mise.ResolveBootstrapFiles(entries, root, "/h")
	require.NoError(t, err)
	require.Len(t, files, 2, "symlink-each expands to files only, skipping subdirs")
	targets := []string{files[0].Target, files[1].Target}
	require.Contains(t, targets, "/etc/systemd/system/a.mount")
	require.Contains(t, targets, "/etc/systemd/system/b.timer")
}

func TestResolveBootstrapFiles_symlinkEachMissingDir(t *testing.T) {
	entries := []resolve.DotfileEntry{{Target: "/x", Source: "noexist", Mode: "symlink-each"}}
	_, err := mise.ResolveBootstrapFiles(entries, t.TempDir(), "/h")
	require.Error(t, err)
}

// --- GenerateBootstrapFiles ---

func TestGenerateBootstrapFiles_emitsSection(t *testing.T) {
	files := []mise.BootstrapFile{
		{Target: "/etc/foo.conf", Source: "/src/foo.conf"},
		{Target: "/etc/bar.conf", Source: "/src/bar.conf", Template: true},
	}
	got := mise.GenerateBootstrapFiles(files)
	require.Contains(t, got, "[bootstrap.files]")
	require.Contains(t, got, `"/etc/bar.conf" = { source = "/src/bar.conf", template = true }`)
	require.Contains(t, got, `"/etc/foo.conf" = { source = "/src/foo.conf" }`)
}

func TestGenerateBootstrapFiles_empty(t *testing.T) {
	require.Empty(t, mise.GenerateBootstrapFiles(nil))
}

func TestGenerateBootstrapFiles_sorted(t *testing.T) {
	files := []mise.BootstrapFile{
		{Target: "/z", Source: "/s"},
		{Target: "/a", Source: "/s"},
	}
	got := mise.GenerateBootstrapFiles(files)
	require.Contains(t, got, "/a")
	require.Less(t, indexOf(got, "/a"), indexOf(got, "/z"), "a should sort before z")
}

// --- GenerateBootstrapDirectories ---

func TestGenerateBootstrapDirectories_basic(t *testing.T) {
	got := mise.GenerateBootstrapDirectories([]string{"/mnt/data", "/mnt/media"})
	require.Contains(t, got, "[bootstrap.directories]")
	require.Contains(t, got, `"/mnt/data" = { state = "present" }`)
	require.Contains(t, got, `"/mnt/media" = { state = "present" }`)
}

func TestGenerateBootstrapDirectories_dedup(t *testing.T) {
	got := mise.GenerateBootstrapDirectories([]string{"/mnt/x", "/mnt/x"})
	lines := 0
	for _, c := range got {
		if c == '\n' {
			lines++
		}
	}
	require.Equal(t, 2, lines, "header + one deduped entry = 2 lines")
}

func TestGenerateBootstrapDirectories_empty(t *testing.T) {
	require.Empty(t, mise.GenerateBootstrapDirectories(nil))
}

// --- GenerateBootstrapServices ---

func TestGenerateBootstrapServices_basic(t *testing.T) {
	svcs := []mise.BootstrapService{
		{Name: "mnt-data.mount", Enabled: true, Running: true},
		{Name: "smb", Enabled: true, Running: false},
	}
	got := mise.GenerateBootstrapServices(svcs)
	require.Contains(t, got, "[bootstrap.services]")
	require.Contains(t, got, `"mnt-data.mount" = { state = "running", enabled = "enabled" }`)
	require.Contains(t, got, `"smb" = { state = "stopped", enabled = "enabled" }`)
}

func TestGenerateBootstrapServices_empty(t *testing.T) {
	require.Empty(t, mise.GenerateBootstrapServices(nil))
}

// --- GenerateBootstrapAccounts ---

func TestGenerateBootstrapAccounts_groupAndUsers(t *testing.T) {
	got := mise.GenerateBootstrapAccounts("smb", []string{"cri", "media"})
	require.Contains(t, got, "[bootstrap.groups]")
	require.Contains(t, got, `"smb" = { state = "present" }`)
	require.Contains(t, got, "[bootstrap.users]")
	require.Contains(t, got, `"cri" = { groups = ["smb"], state = "present" }`)
	require.Contains(t, got, `"media" = { groups = ["smb"], state = "present" }`)
}

func TestGenerateBootstrapAccounts_emptyGroup(t *testing.T) {
	require.Empty(t, mise.GenerateBootstrapAccounts("", []string{"x"}))
}

func TestGenerateBootstrapAccounts_sortedUsers(t *testing.T) {
	got := mise.GenerateBootstrapAccounts("g", []string{"zoe", "amy"})
	amyIdx := indexOf(got, `"amy"`)
	zoeIdx := indexOf(got, `"zoe"`)
	require.Greater(t, zoeIdx, amyIdx, "amy should appear before zoe")
}

// helpers

func countLines(s string) int {
	n := 0
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n + 1
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
