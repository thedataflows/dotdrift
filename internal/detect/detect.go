// Package detect gathers system facts.
package detect

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/thedataflows/dotdrift/internal/facts"
)

// OSReleaseReader reads /etc/os-release content.
type OSReleaseReader interface {
	Read() (string, error)
}

// GPUReader reads GPU information.
type GPUReader interface {
	Read() (string, error)
}

type defaultOSReader struct{}

func (defaultOSReader) Read() (string, error) {
	data, err := os.ReadFile("/etc/os-release")
	return string(data), err
}

type defaultGPUReader struct{}

func (defaultGPUReader) Read() (string, error) {
	cmd := exec.Command("lspci")
	out, err := cmd.Output()
	return string(out), err
}

// Detect returns system facts using the default readers.
func Detect() (*facts.Facts, error) {
	return DetectWith(defaultOSReader{}, defaultGPUReader{})
}

// DetectWith returns system facts using the provided readers.
func DetectWith(osReader OSReleaseReader, gpuReader GPUReader) (*facts.Facts, error) {
	f := &facts.Facts{}

	hostname, _ := os.Hostname()
	f.Hostname = hostname

	f.Username = os.Getenv("USER")
	if f.Username == "" {
		f.Username = os.Getenv("USERNAME")
	}

	f.OS = strings.ToLower(runtime.GOOS)

	if osReader != nil {
		content, err := osReader.Read()
		if err != nil {
			return nil, fmt.Errorf("read os-release: %w", err)
		}
		f.Distro = parseOSRelease(content)
		f.Backend = backendForDistro(f.Distro)
	}

	f.GPU = "unknown"
	if gpuReader != nil {
		content, err := gpuReader.Read()
		if err == nil {
			f.GPU = classifyGPU(content)
		}
	}

	return f, nil
}

func parseOSRelease(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "ID=") {
			continue
		}
		id := strings.TrimPrefix(line, "ID=")
		id = strings.Trim(id, `"`)
		return strings.ToLower(id)
	}
	return ""
}

func backendForDistro(distro string) string {
	switch distro {
	case "arch", "cachyos", "manjaro":
		return "paru"
	case "debian", "ubuntu":
		return "apt"
	case "fedora":
		return "dnf"
	default:
		return "unknown"
	}
}

func classifyGPU(content string) string {
	for _, line := range strings.Split(content, "\n") {
		upper := strings.ToUpper(line)
		if !strings.Contains(upper, "VGA") && !strings.Contains(upper, "3D") && !strings.Contains(upper, "DISPLAY") {
			continue
		}
		switch {
		case strings.Contains(upper, "NVIDIA"):
			return "nvidia"
		case strings.Contains(upper, "AMD") || strings.Contains(upper, "ADVANCED MICRO DEVICES"):
			return "amd"
		case strings.Contains(upper, "INTEL"):
			return "intel"
		}
	}
	return "unknown"
}

// DetectEnv wraps Detect and returns an error if the facts are incomplete.
func DetectEnv() (*facts.Facts, error) {
	f, err := Detect()
	if err != nil {
		return nil, fmt.Errorf("detect: %w", err)
	}
	return f, nil
}
