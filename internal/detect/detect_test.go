package detect_test

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/detect"
)

type fakeReader struct {
	content string
	err     error
}

func (f *fakeReader) Read() (string, error) {
	return f.content, f.err
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "detect", name))
	require.NoError(t, err)
	return string(data)
}

func TestDetectOS_osReleaseFixture(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		distro  string
		backend string
	}{
		{"arch", "os-release-arch", "arch", "paru"},
		{"cachyos", "os-release-cachyos", "cachyos", "paru"},
		{"manjaro", "os-release-manjaro", "manjaro", "paru"},
		{"debian", "os-release-debian", "debian", "apt"},
		{"ubuntu", "os-release-ubuntu", "ubuntu", "apt"},
		{"fedora", "os-release-fedora", "fedora", "dnf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			osReader := &fakeReader{content: fixture(t, tt.fixture)}
			gpuReader := &fakeReader{content: ""}
			got, err := detect.DetectWith(osReader, gpuReader)
			require.NoError(t, err)
			require.Equal(t, "linux", got.OS)
			require.Equal(t, tt.distro, got.Distro)
			require.Equal(t, tt.backend, got.Backend)
		})
	}
}

func TestDetectGPU_nvidiaAmdIntelUnknown(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{"nvidia", "lspci-nvidia", "nvidia"},
		{"amd", "lspci-amd", "amd"},
		{"intel", "lspci-intel", "intel"},
		{"unknown", "lspci-unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			osReader := &fakeReader{content: ""}
			gpuReader := &fakeReader{content: fixture(t, tt.fixture)}
			got, err := detect.DetectWith(osReader, gpuReader)
			require.NoError(t, err)
			require.Equal(t, tt.want, got.GPU)
		})
	}
}

func TestDetectGPU_failOpen(t *testing.T) {
	osReader := &fakeReader{content: ""}
	gpuReader := &fakeReader{content: "", err: errors.New("lspci not found")}
	got, err := detect.DetectWith(osReader, gpuReader)
	require.NoError(t, err)
	require.Equal(t, "unknown", got.GPU)
}

func stubUserLookup(t *testing.T, username string, err error) {
	t.Helper()
	orig := detect.CurrentUser
	detect.CurrentUser = func() (*user.User, error) {
		if err != nil {
			return nil, err
		}
		return &user.User{Username: username}, nil
	}
	t.Cleanup(func() { detect.CurrentUser = orig })
}

func TestDetectUsername_OSAccountWinsOverEnv(t *testing.T) {
	// sudo preserves $USER; the OS account must win over the env var.
	stubUserLookup(t, "alice", nil)
	t.Setenv("USER", "root")
	t.Setenv("USERNAME", "root")
	t.Log("env USER=root, os/user reports alice")

	got, err := detect.DetectWith(&fakeReader{content: ""}, &fakeReader{content: ""})
	require.NoError(t, err)
	require.Equal(t, "alice", got.Username)
}

func TestDetectUsername_EnvFallback(t *testing.T) {
	stubUserLookup(t, "", errors.New("no os/user"))
	t.Setenv("USER", "bob")
	t.Log("os/user fails, env USER=bob")

	got, err := detect.DetectWith(&fakeReader{content: ""}, &fakeReader{content: ""})
	require.NoError(t, err)
	require.Equal(t, "bob", got.Username)
}

func TestDetectUsername_EnvFallbackPrefersUSERNAME(t *testing.T) {
	stubUserLookup(t, "", errors.New("no os/user"))
	t.Setenv("USER", "")
	t.Setenv("USERNAME", "carol")

	got, err := detect.DetectWith(&fakeReader{content: ""}, &fakeReader{content: ""})
	require.NoError(t, err)
	require.Equal(t, "carol", got.Username)
}

func TestDetectUsername_BothFailReturnsError(t *testing.T) {
	stubUserLookup(t, "", errors.New("no os/user"))
	t.Setenv("USER", "")
	t.Setenv("USERNAME", "")
	t.Log("os/user fails and env empty: expect explicit error")

	_, err := detect.DetectWith(&fakeReader{content: ""}, &fakeReader{content: ""})
	require.Error(t, err)
	require.Contains(t, err.Error(), "username")
}

func TestDetectUsername_OSEmptyFallsBackToEnv(t *testing.T) {
	// os/user succeeding but returning an empty name must not yield "".
	stubUserLookup(t, "", nil)
	t.Setenv("USER", "dave")

	got, err := detect.DetectWith(&fakeReader{content: ""}, &fakeReader{content: ""})
	require.NoError(t, err)
	require.Equal(t, "dave", got.Username)
}
