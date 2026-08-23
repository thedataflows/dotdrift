// Package facts holds the system facts used for module selection.
package facts

// Facts holds the system facts used for module selection.
type Facts struct {
	Hostname string
	Username string
	OS       string
	// Kernel is the running kernel release (`uname -r`, e.g.
	// "6.12.1-arch1-1"); empty when detection failed.
	Kernel string
	// InstalledPackages records which packages a when.packages filter
	// referenced as installed on the running system, probed lazily at
	// profile load via the detected package backend. nil (nothing probed
	// or nothing installed) means a non-empty when.packages constraint
	// never matches — the same contract as an empty Kernel fact.
	InstalledPackages map[string]bool
	// InstalledTools records which mise-managed tools a when.tools filter
	// referenced as installed on the running system, probed lazily at
	// profile load via `mise current` (presence only, version ignored).
	// nil (nothing probed, mise missing, or nothing installed) means a
	// non-empty when.tools constraint never matches.
	InstalledTools map[string]bool
	Distro         string
	GPU            string
	Backend        string
}
