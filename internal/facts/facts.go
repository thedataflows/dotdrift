// Package facts holds the system facts used for module selection.
package facts

// Facts holds the system facts used for module selection.
type Facts struct {
	Hostname string
	Username string
	OS       string
	Distro   string
	GPU      string
	Backend  string
}
