package detect_test

import (
	"errors"
	"os"
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
