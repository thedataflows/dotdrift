// Package facts holds the system facts used for module selection.
package facts

import (
	"os"
	"runtime"
)

// Facts holds the system facts used for module selection.
type Facts struct {
	Hostname string
	Username string
	OS       string
	Distro   string
	GPU      string
	Backend  string
}

// Detect gathers system facts from the runtime.
func Detect() *Facts {
	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}
	return &Facts{
		Hostname: hostname,
		Username: username,
		OS:       runtime.GOOS,
	}
}
