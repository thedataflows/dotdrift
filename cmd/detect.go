package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/service"
)

// DetectCmd prints system facts.
type DetectCmd struct {
	Out io.Writer `kong:"-"`
	// OSReader and GPUReader allow tests to inject fakes. When nil, real system
	// readers are used.
	OSReader  detect.OSReleaseReader `kong:"-"`
	GPUReader detect.GPUReader       `kong:"-"`
}

// Run gathers and prints system facts in a stable line-oriented format
// through the service reads area (T-tui-reads).
func (c *DetectCmd) Run() error {
	deps := service.ReadsDeps{}
	if c.OSReader != nil || c.GPUReader != nil {
		osr, gpr := c.OSReader, c.GPUReader
		deps.Detect = func() (*facts.Facts, error) { return detect.DetectWith(osr, gpr) }
	}
	f, err := service.NewReadsArea(deps).Detect()
	if err != nil {
		return err
	}

	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	_, err = fmt.Fprintf(out, "host: %s\n", f.Hostname)
	if err == nil {
		_, err = fmt.Fprintf(out, "user: %s\n", f.Username)
	}
	if err == nil {
		_, err = fmt.Fprintf(out, "os: %s\n", f.OS)
	}
	if err == nil {
		_, err = fmt.Fprintf(out, "kernel: %s\n", f.Kernel)
	}
	if err == nil {
		_, err = fmt.Fprintf(out, "distro: %s\n", f.Distro)
	}
	if err == nil {
		_, err = fmt.Fprintf(out, "gpu: %s\n", f.GPU)
	}
	if err == nil {
		_, err = fmt.Fprintf(out, "backend: %s\n", f.Backend)
	}
	return err
}
