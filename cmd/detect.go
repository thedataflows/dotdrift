package dotdrift

import (
	"fmt"
	"io"
	"os"

	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// DetectCmd prints system facts.
type DetectCmd struct {
	Out io.Writer `kong:"-"`
	// OSReader and GPUReader allow tests to inject fakes. When nil, real system
	// readers are used.
	OSReader  detect.OSReleaseReader `kong:"-"`
	GPUReader detect.GPUReader       `kong:"-"`
}

// Run gathers and prints system facts in a stable line-oriented format.
func (c *DetectCmd) Run() error {
	var f *facts.Facts
	var err error
	if c.OSReader != nil || c.GPUReader != nil {
		f, err = detect.DetectWith(c.OSReader, c.GPUReader)
	} else {
		f, err = detect.Detect()
	}
	if err != nil {
		return fmt.Errorf("detect: %w", err)
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
