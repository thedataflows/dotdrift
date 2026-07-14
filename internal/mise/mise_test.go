package mise_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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
	require.Equal(t, 1, mise.CompareVersions("2026.6.6", "2025.1.0"))
	require.Equal(t, -1, mise.CompareVersions("2024.12.0", "2025.1.0"))
	require.Equal(t, 0, mise.CompareVersions("2025.1.0", "2025.1.0"))
	require.Equal(t, 1, mise.CompareVersions("2025.2.0", "2025.1.9"))
	require.Equal(t, 0, mise.CompareVersions("2025.1", "2025.1.0"))
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
