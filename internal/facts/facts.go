// Package facts holds the system facts used for module selection.
package facts

// Facts holds the system facts used for module selection.
type Facts struct {
	Hostname string
	Username string
	OS       string
	// Kernel is the running kernel release (`uname -r`, e.g.
	// "6.12.1-arch1-1"); empty when detection failed.
	Kernel  string
	Distro  string
	GPU     string
	Backend string
}
