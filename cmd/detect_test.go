package dotdrift_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	cmd "github.com/thedataflows/dotdrift/cmd"
)

type fakeOSReader struct {
	content string
}

func (f *fakeOSReader) Read() (string, error) {
	return f.content, nil
}

type fakeGPUReader struct {
	content string
}

func (f *fakeGPUReader) Read() (string, error) {
	return f.content, nil
}

func TestCLI_detect_output(t *testing.T) {
	osContent, err := os.ReadFile(filepath.Join("..", "testdata", "detect", "os-release-cachyos"))
	require.NoError(t, err)
	gpuContent, err := os.ReadFile(filepath.Join("..", "testdata", "detect", "lspci-nvidia"))
	require.NoError(t, err)

	var buf bytes.Buffer
	c := &cmd.DetectCmd{
		Out:       &buf,
		OSReader:  &fakeOSReader{content: string(osContent)},
		GPUReader: &fakeGPUReader{content: string(gpuContent)},
	}
	err = c.Run()
	require.NoError(t, err)
	out := buf.String()
	require.Contains(t, out, "host:")
	require.Contains(t, out, "user:")
	require.Contains(t, out, "os: linux")
	require.Contains(t, out, "distro: cachyos")
	require.Contains(t, out, "gpu: nvidia")
	require.Contains(t, out, "backend: paru")
}
