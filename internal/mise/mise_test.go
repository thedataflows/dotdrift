package mise_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
)

func TestEnsureMise_detectsInPath(t *testing.T) {
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			require.Equal(t, "mise", name)
			return "/usr/bin/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			require.Equal(t, "/usr/bin/mise", name)
			require.Equal(t, []string{"--version"}, args)
			return "2026.6.6 linux-x64", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindSystemWide },
	}

	path, err := m.Ensure()
	require.NoError(t, err)
	require.Equal(t, "/usr/bin/mise", path)
}

func TestEnsureMise_installsWhenMissing(t *testing.T) {
	calls := 0
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "", errors.New("not found")
		},
		Run: func(name string, args ...string) (string, error) {
			require.Equal(t, "/fake/mise", name)
			require.Equal(t, []string{"--version"}, args)
			return "2026.6.6", nil
		},
		Install: func() (string, error) {
			calls++
			return "/fake/mise", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}

	path, err := m.Ensure()
	require.NoError(t, err)
	require.Equal(t, "/fake/mise", path)
	require.Equal(t, 1, calls)
}

func TestEnsureMise_upgradesWhenTooOld_userManaged(t *testing.T) {
	calls := 0
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/home/user/.local/bin/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			if len(args) == 1 && args[0] == "--version" {
				if calls == 0 {
					return "2024.1.1", nil
				}
				return "2026.6.6", nil
			}
			require.Equal(t, []string{"self-update"}, args)
			calls++
			return "", nil
		},
		Install: func() (string, error) {
			return "", errors.New("should not install when self-update succeeds")
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}

	path, err := m.Ensure()
	require.NoError(t, err)
	require.Equal(t, "/home/user/.local/bin/mise", path)
	require.Equal(t, 1, calls)
}

func TestEnsureMise_upgradesWhenTooOld_userManagedSelfUpdateFails(t *testing.T) {
	calls := 0
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/home/user/.local/bin/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			if len(args) == 1 && args[0] == "--version" {
				if calls == 0 {
					return "2024.1.1", nil
				}
				return "2026.6.6", nil
			}
			calls++
			return "", errors.New("self-update failed")
		},
		Install: func() (string, error) {
			calls++
			return "/fake/mise", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}

	path, err := m.Ensure()
	require.NoError(t, err)
	require.Equal(t, "/fake/mise", path)
	require.Equal(t, 2, calls)
}

func TestEnsureMise_systemWideTooOld_noUpgrade(t *testing.T) {
	installCalled := false
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/usr/bin/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			require.Equal(t, "/usr/bin/mise", name)
			require.Equal(t, []string{"--version"}, args)
			return "2024.1.1", nil
		},
		Install: func() (string, error) {
			installCalled = true
			return "", errors.New("should not install")
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindSystemWide },
	}

	_, err := m.Ensure()
	require.Error(t, err)
	require.Contains(t, err.Error(), "system install")
	require.Contains(t, err.Error(), "package manager")
	require.False(t, installCalled)
}

func TestEnsureMise_unknownTooOld_noUpgrade(t *testing.T) {
	installCalled := false
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/tmp/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			require.Equal(t, "/tmp/mise", name)
			require.Equal(t, []string{"--version"}, args)
			return "2024.1.1", nil
		},
		Install: func() (string, error) {
			installCalled = true
			return "", errors.New("should not install")
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUnknown },
	}

	_, err := m.Ensure()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambiguous install")
	require.False(t, installCalled)
}

func TestEnsureMise_returnsErrorWhenInstallFails(t *testing.T) {
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "", errors.New("not found")
		},
		Install: func() (string, error) {
			return "", errors.New("network down")
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}

	_, err := m.Ensure()
	require.Error(t, err)
	require.Contains(t, err.Error(), "network down")
}

func TestVersionCompare_calendarVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2026.6.6", "2025.1.0", 1},
		{"2024.12.0", "2025.1.0", -1},
		{"2025.1.0", "2025.1.0", 0},
		{"2025.2.0", "2025.1.9", 1},
		{"2025.1", "2025.1.0", 0},
	}
	for _, tc := range cases {
		got, err := mise.CompareVersions(tc.a, tc.b)
		require.NoError(t, err)
		require.Equal(t, tc.want, got, "%s vs %s", tc.a, tc.b)
	}
}

func TestVersionCompare_prereleaseBelowRelease(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want int
	}{
		{"rc below release", "2025.1.0-rc1", "2025.1.0", -1},
		{"release above rc", "2025.1.0", "2025.1.0-rc1", 1},
		{"dev below release", "2025.1.0-dev.1", "2025.1.0", -1},
		{"nightly below release", "2025.1.0-nightly", "2025.1.0", -1},
		{"build metadata below release", "2025.1.0+build.5", "2025.1.0", -1},
		{"two prereleases equal", "2025.1.0-rc1", "2025.1.0-rc2", 0},
		{"newer rc above older release", "2025.2.0-rc1", "2025.1.0", 1},
		{"v-prefixed prerelease", "v2025.1.0-rc1", "2025.1.0", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mise.CompareVersions(tc.a, tc.b)
			require.NoError(t, err)
			require.Equal(t, tc.want, got, "%s vs %s", tc.a, tc.b)
		})
	}
}

func TestVersionCompare_garbageIsError(t *testing.T) {
	for _, garbage := range []string{"", "abc", "v", "2025..1", "2025.x.0", "linux-x64"} {
		_, err := mise.CompareVersions(garbage, "2025.1.0")
		require.Error(t, err, "input %q must not parse silently", garbage)
		_, err = mise.CompareVersions("2025.1.0", garbage)
		require.Error(t, err, "input %q must not parse silently", garbage)
	}
}

func TestEnsureMise_prereleaseMinVersionIsTooOld(t *testing.T) {
	// "2025.1.0-rc1" must NOT satisfy MinMiseVersion 2025.1.0: a pre-release
	// compares below the release it precedes.
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/usr/bin/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			return "2025.1.0-rc1 linux-x64", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindSystemWide },
	}

	_, err := m.Ensure()
	require.Error(t, err)
	require.Contains(t, err.Error(), "older than required")
}

func TestClassifyInstall(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	userMise := filepath.Join(home, ".local", "bin", "mise")
	require.NoError(t, os.MkdirAll(filepath.Dir(userMise), 0o755))
	require.NoError(t, os.WriteFile(userMise, []byte("fake"), 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(userMise) })

	cases := []struct {
		path     string
		env      string
		expected mise.InstallKind
	}{
		{"/usr/bin/mise", "", mise.InstallKindSystemWide},
		{"/usr/local/bin/mise", "", mise.InstallKindSystemWide},
		{"/bin/mise", "", mise.InstallKindSystemWide},
		{"/opt/mise/bin/mise", "", mise.InstallKindSystemWide},
		{userMise, "", mise.InstallKindUserManaged},
		{"/tmp/mise", "", mise.InstallKindUnknown},
		{"/home/other/.local/bin/mise", "", mise.InstallKindUnknown},
		{"/usr/bin/mise", "1", mise.InstallKindSystemWide},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("DOTDRIFT_MISE_SYSTEM", tc.env)
			} else {
				t.Setenv("DOTDRIFT_MISE_SYSTEM", "")
			}
			kind := mise.ClassifyInstall(tc.path)
			require.Equal(t, tc.expected, kind)
		})
	}
}

func TestEnsureMise_versionOutputWithLeadingToken(t *testing.T) {
	// Given `mise --version` output that starts with a non-version token,
	// Ensure must scan for the first token that looks like a version.
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/usr/bin/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			return "mise 2026.7.10 linux-x64 (2026-07-18)", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindSystemWide },
	}

	path, err := m.Ensure()
	require.NoError(t, err)
	require.Equal(t, "/usr/bin/mise", path)
}

func TestEnsureMise_versionOutputGarbageIsError(t *testing.T) {
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/usr/bin/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			return "mise: unknown command", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindSystemWide },
	}

	_, err := m.Ensure()
	require.Error(t, err)
	require.Contains(t, err.Error(), "version")
}

func TestEnsureMise_ensureRunsOnceAcrossCalls(t *testing.T) {
	var lookPathCalls, versionCalls int32
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			atomic.AddInt32(&lookPathCalls, 1)
			return "/fake/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			if len(args) == 1 && args[0] == "--version" {
				atomic.AddInt32(&versionCalls, 1)
				return "2026.7.10 linux-x64 (2026-07-18)", nil
			}
			return "", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}

	path, err := m.Ensure()
	require.NoError(t, err)
	require.Equal(t, "/fake/mise", path)

	execM := mise.NewExecMise(m)
	require.NoError(t, execM.EnsureAndInstall(context.Background(), "/fake/mise.toml"))
	require.NoError(t, execM.DotfilesApply(context.Background(), "/fake/mise.toml", true))

	require.Equal(t, int32(1), atomic.LoadInt32(&lookPathCalls), "Ensure must run at most once per process")
	require.Equal(t, int32(1), atomic.LoadInt32(&versionCalls), "version probe must run at most once per process")
}

func TestEnsureMise_ensureOnceConcurrent(t *testing.T) {
	var lookPathCalls int32
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			atomic.AddInt32(&lookPathCalls, 1)
			return "/fake/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			return "2026.7.10", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path, err := m.Ensure()
			require.NoError(t, err)
			require.Equal(t, "/fake/mise", path)
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), atomic.LoadInt32(&lookPathCalls))
}

func captureZerolog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := log.Logger
	log.Logger = old.Output(&buf)
	t.Cleanup(func() { log.Logger = old })
	return &buf
}

func TestEnsureMise_logsInstallMessage(t *testing.T) {
	buf := captureZerolog(t)
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "", errors.New("not found")
		},
		Run: func(name string, args ...string) (string, error) {
			return "2026.7.10", nil
		},
		Install: func() (string, error) {
			return "/fake/mise", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}

	_, err := m.Ensure()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "mise not found; installing via https://mise.run")
}

func TestEnsureMise_logsUserUpgradeMessage(t *testing.T) {
	buf := captureZerolog(t)
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/home/user/.local/bin/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			if len(args) == 1 && args[0] == "--version" {
				return "2024.1.1", nil
			}
			return "", errors.New("self-update failed")
		},
		Install: func() (string, error) {
			return "", errors.New("stop here")
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}

	_, err := m.Ensure()
	require.Error(t, err)
	require.Contains(t, buf.String(), "upgrading user install")
}

func TestEnsureMise_logsSystemTooOldMessage(t *testing.T) {
	buf := captureZerolog(t)
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/usr/bin/mise", nil
		},
		Run: func(name string, args ...string) (string, error) {
			return "2024.1.1", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindSystemWide },
	}

	_, err := m.Ensure()
	require.Error(t, err)
	require.Contains(t, buf.String(), "system install")
	require.Contains(t, buf.String(), "package manager")
}

func TestEnsureContext_cancelPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &mise.Mise{
		LookPath: func(name string) (string, error) {
			return "/fake/mise", nil
		},
		RunContext: func(ctx context.Context, name string, args ...string) (string, error) {
			return "", ctx.Err()
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}

	_, err := m.EnsureContext(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
