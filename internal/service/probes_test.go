package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Probe-assembly tests (issue 0108, slice 2b): the service owns the sudo
// elevation helpers that cmd/status.go used to carry. The moved
// TestElevateProbes_* tests pin the wrapper behavior against the sudoRead
// seam; the TestProbes_* tests pin the elevated/unelevated switch of
// defaultProbes.

// stubSudo swaps the sudoRead seam for the test's fake and restores it.
func stubSudo(t *testing.T, fn func(string, ...string) ([]byte, error)) {
	t.Helper()
	orig := sudoRead
	t.Cleanup(func() { sudoRead = orig })
	sudoRead = fn
}

// probeDeps returns the defaultProbes seams for tests: a backend that
// reports nothing installed and a mise that never touches the OS.
func probeDeps(events *[]string) (func(string) packages.Backend, func() *mise.Mise) {
	return func(string) packages.Backend { return &recordingBackend{events: events} },
		func() *mise.Mise { return fakeMise(events) }
}

func TestProbes_elevatedRetriesOnPermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission bits — no denial to retry")
	}
	secret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secret, []byte("secret\n"), 0o000))

	stubSudo(t, func(name string, args ...string) ([]byte, error) {
		require.Equal(t, "cat", name)
		require.Equal(t, []string{secret}, args)
		return []byte("secret\n"), nil
	})
	packagesFor, newMise := probeDeps(&[]string{})
	pr := defaultProbes(&facts.Facts{Backend: "paru"}, true, packagesFor, newMise)
	got, err := pr.ReadFile(secret)
	require.NoError(t, err, "elevated probes retry permission-denied reads through sudo")
	require.Equal(t, "secret\n", string(got))
}

func TestProbes_unelevatedDoesNotSudo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission bits — no denial to observe")
	}
	secret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secret, []byte("secret\n"), 0o000))

	called := false
	stubSudo(t, func(string, ...string) ([]byte, error) {
		called = true
		return nil, errors.New("sudo must not run unelevated")
	})
	packagesFor, newMise := probeDeps(&[]string{})
	pr := defaultProbes(&facts.Facts{Backend: "paru"}, false, packagesFor, newMise)
	_, err := pr.ReadFile(secret)
	require.Error(t, err)
	require.True(t, os.IsPermission(err), "the raw permission error is the Unauthorized carrier (0108)")
	require.False(t, called, "unelevated probes never run sudo")
}

func TestService_status_usesDefaultProbes(t *testing.T) {
	// No ProbesFor in the opts: the read builds probes from the deps.
	dir := statusFixture(t)
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events := []string{}
	area := NewReadsArea(ReadsDeps{
		Detect:      func() (*facts.Facts, error) { return f, nil },
		PackagesFor: func(string) packages.Backend { return &recordingBackend{events: &events} },
		NewMise:     func() *mise.Mise { return fakeMise(&events) },
	}.WithDefaults())
	r, err := area.Status(t.Context(), StatusOpts{ProfilePath: dir, Elevated: false})
	require.NoError(t, err)
	require.Len(t, r.Findings, 1, "the default probes still probe the plan's package")
	require.Equal(t, "packages", r.Findings[0].Section)
	require.Equal(t, drift.Drift, r.Findings[0].Status)
	require.Equal(t, "missing", r.Findings[0].Detail)
}

func TestStatus_latestToolUsesLocalAlias(t *testing.T) {
	const tool = "github:storytold/pdfcraft"
	const version = "0.21.1"
	for _, tc := range []struct {
		name, target string
		want         drift.Status
	}{
		{"active version is latest", "./0.21.1", drift.OK},
		{"another version is latest", "./0.22.0", drift.Drift},
		{"missing latest link", "", drift.Unknown},
		{"dangling latest link", "./0.23.0", drift.Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			active := filepath.Join(dir, version)
			require.NoError(t, os.Mkdir(active, 0o755))
			require.NoError(t, os.Mkdir(filepath.Join(dir, "0.22.0"), 0o755))
			if tc.target != "" {
				require.NoError(t, os.Symlink(tc.target, filepath.Join(dir, "latest")))
			}
			packagesFor, _ := probeDeps(&[]string{})
			pr := defaultProbes(&facts.Facts{Backend: "paru"}, false, packagesFor, func() *mise.Mise {
				return &mise.Mise{
					LookPath: func(string) (string, error) { return "/usr/bin/mise", nil },
					Run: func(_ string, args ...string) (string, error) {
						switch {
						case len(args) == 2 && args[0] == "current" && args[1] == tool:
							return version + "\n", nil
						case len(args) == 2 && args[0] == "where" && args[1] == tool+"@"+version:
							return active + "\n", nil
						default:
							return "", errors.New("unexpected mise command")
						}
					},
				}
			})
			plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{tool: "latest"}}}
			findings := drift.Check(t.Context(), plan, "", pr, drift.CheckOptions{})
			require.Len(t, findings, 1)
			require.Equal(t, tc.want, findings[0].Status, findings[0].Detail)
			if tc.want == drift.Drift {
				require.Equal(t, "installed 0.21.1, want latest", findings[0].Detail)
			}
		})
	}
}

// elevateProbes wraps Readlink/ReadFile/StatDir/ListDir to retry elevated on
// permission denied. These tests verify the wrapping behavior using fake
// inner functions and a fake sudoRead seam (moved from cmd/status_test.go).
func TestElevateProbes_readlinkRetriesElevated(t *testing.T) {
	stubSudo(t, func(name string, args ...string) ([]byte, error) {
		require.Equal(t, "readlink", name)
		require.Equal(t, []string{"/etc/secret"}, args)
		return []byte("/target/path\n"), nil
	})
	inner := func(path string) (string, error) {
		return "", os.ErrPermission
	}
	wrapped := elevateReadlink(inner)
	got, err := wrapped("/etc/secret")
	require.NoError(t, err)
	require.Equal(t, "/target/path", got)
}

func TestElevateProbes_readlinkNoRetryOnSuccess(t *testing.T) {
	called := false
	stubSudo(t, func(string, ...string) ([]byte, error) {
		called = true
		return nil, errors.New("should not be called")
	})
	inner := func(path string) (string, error) { return "/existing/target", nil }
	wrapped := elevateReadlink(inner)
	got, err := wrapped("/etc/foo")
	require.NoError(t, err)
	require.Equal(t, "/existing/target", got)
	require.False(t, called, "sudo must not be called when inner succeeds")
}

func TestElevateProbes_readlinkNoRetryOnNonPermission(t *testing.T) {
	called := false
	stubSudo(t, func(string, ...string) ([]byte, error) {
		called = true
		return nil, errors.New("should not be called")
	})
	innerErr := errors.New("not a symlink")
	inner := func(path string) (string, error) { return "", innerErr }
	wrapped := elevateReadlink(inner)
	_, err := wrapped("/etc/foo")
	require.ErrorIs(t, err, innerErr)
	require.False(t, called, "sudo must not be called for non-permission errors")
}

func TestElevateProbes_readFileRetriesElevated(t *testing.T) {
	stubSudo(t, func(name string, args ...string) ([]byte, error) {
		require.Equal(t, "cat", name)
		return []byte("file content"), nil
	})
	inner := func(path string) ([]byte, error) {
		return nil, os.ErrPermission
	}
	wrapped := elevateReadFile(inner)
	got, err := wrapped("/etc/secret")
	require.NoError(t, err)
	require.Equal(t, "file content", string(got))
}

func TestElevateProbes_statDirRetriesElevated(t *testing.T) {
	stubSudo(t, func(name string, args ...string) ([]byte, error) {
		require.Equal(t, "stat", name)
		return []byte("directory\n"), nil
	})
	inner := func(path string) (bool, error) {
		return false, os.ErrPermission
	}
	wrapped := elevateStatDir(inner)
	got, err := wrapped("/mnt/secret")
	require.NoError(t, err)
	require.True(t, got)
}

func TestElevateProbes_statDirNotDirectoryElevated(t *testing.T) {
	stubSudo(t, func(string, ...string) ([]byte, error) {
		return []byte("regular file\n"), nil
	})
	inner := func(path string) (bool, error) {
		return false, os.ErrPermission
	}
	wrapped := elevateStatDir(inner)
	got, err := wrapped("/etc/secret")
	require.NoError(t, err)
	require.False(t, got)
}

// When sudo itself fails, the wrapper returns the original error.
func TestElevateProbes_sudoFailureReturnsOriginalError(t *testing.T) {
	stubSudo(t, func(string, ...string) ([]byte, error) {
		return nil, errors.New("sudo: command not found")
	})
	inner := func(path string) ([]byte, error) {
		return nil, os.ErrPermission
	}
	wrapped := elevateReadFile(inner)
	_, err := wrapped("/etc/secret")
	require.ErrorIs(t, err, os.ErrPermission, "should return original permission error when sudo fails")
}
