// Package mise bootstraps the mise binary and runs mise operations.
package mise

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/thedataflows/dotdrift/internal/resolve"
)

// MinMiseVersion is the hardcoded minimum mise version required by dotdrift.
const MinMiseVersion = "2025.1.0"

// InstallerURL is the official mise installer.
const InstallerURL = "https://mise.run"

// InstallKind classifies how a mise binary is managed.
type InstallKind int

const (
	InstallKindUnknown InstallKind = iota
	InstallKindSystemWide
	InstallKindUserManaged
)

func (k InstallKind) String() string {
	switch k {
	case InstallKindSystemWide:
		return "system-wide"
	case InstallKindUserManaged:
		return "user-managed"
	default:
		return "unknown"
	}
}

// Mise finds, installs, or upgrades a mise binary.
type Mise struct {
	LookPath func(string) (string, error)
	Run      func(string, ...string) (string, error)
	Install  func() (string, error)
	Classify func(string) InstallKind
}

// DefaultMise returns a Mise configured with real OS dependencies.
func DefaultMise() *Mise {
	return &Mise{
		LookPath: defaultLookPath,
		Run: func(name string, args ...string) (string, error) {
			cmd := exec.Command(name, args...)
			out, err := cmd.CombinedOutput()
			return string(out), err
		},
		Install:  defaultInstall,
		Classify: ClassifyInstall,
	}
}

func defaultLookPath(name string) (string, error) {
	if name != "mise" {
		return exec.LookPath(name)
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	wellKnown := []string{
		filepath.Join(home, ".local", "bin", "mise"),
		filepath.Join(home, ".local", "share", "mise", "bin", "mise"),
	}
	for _, path := range wellKnown {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("mise not found")
}

func defaultInstall() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	installPath := filepath.Join(home, ".local", "bin", "mise")
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		return "", err
	}
	script := fmt.Sprintf("curl -fsSL %s | MISE_INSTALL_PATH=%s sh", InstallerURL, installPath)
	cmd := exec.Command("sh", "-c", script)
	if _, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("install mise: %w", err)
	}
	return installPath, nil
}

// Ensure finds or installs a mise binary meeting the minimum version.
func (m *Mise) Ensure() (string, error) {
	lookPath := m.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath("mise")
	if err != nil || path == "" {
		if m.Install == nil {
			return "", fmt.Errorf("mise not found and no installer configured")
		}
		path, err = m.Install()
		if err != nil {
			return "", fmt.Errorf("install mise: %w", err)
		}
		if path == "" {
			return "", fmt.Errorf("mise not found after install")
		}
	}

	run := m.Run
	if run == nil {
		run = func(name string, args ...string) (string, error) {
			cmd := exec.Command(name, args...)
			out, err := cmd.CombinedOutput()
			return string(out), err
		}
	}

	version, err := m.version(run, path)
	if err != nil {
		return "", fmt.Errorf("check mise version: %w", err)
	}
	if CompareVersions(version, MinMiseVersion) >= 0 {
		return path, nil
	}

	classify := m.Classify
	if classify == nil {
		classify = ClassifyInstall
	}
	kind := classify(path)

	switch kind {
	case InstallKindSystemWide:
		return "", fmt.Errorf("mise %s at %s is older than required %s; system install — upgrade with your package manager", version, path, MinMiseVersion)
	case InstallKindUnknown:
		return "", fmt.Errorf("mise %s at %s is older than required %s; ambiguous install — upgrade with your package manager or reinstall via %s", version, path, MinMiseVersion, InstallerURL)
	}

	if m.Install == nil {
		return "", fmt.Errorf("mise %s at %s is older than required %s and no installer configured", version, path, MinMiseVersion)
	}

	// Prefer self-update for user-managed installs.
	if _, err := run(path, "self-update"); err == nil {
		newVersion, verr := m.version(run, path)
		if verr == nil && CompareVersions(newVersion, MinMiseVersion) >= 0 {
			return path, nil
		}
	}

	path, err = m.Install()
	if err != nil {
		return "", fmt.Errorf("upgrade mise: %w", err)
	}
	version, err = m.version(run, path)
	if err != nil {
		return "", fmt.Errorf("re-check mise version after upgrade: %w", err)
	}
	if CompareVersions(version, MinMiseVersion) < 0 {
		return "", fmt.Errorf("mise %s at %s is still older than required %s after upgrade", version, path, MinMiseVersion)
	}
	return path, nil
}

func (m *Mise) version(run func(string, ...string) (string, error), path string) (string, error) {
	out, err := run(path, "--version")
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(out)
	if fields := strings.Fields(version); len(fields) > 0 {
		version = fields[0]
	}
	version = strings.TrimPrefix(version, "v")
	return version, nil
}

// ClassifyInstall determines whether a mise binary is system-wide or user-managed.
//
// System-wide (no auto-upgrade):
//   - DOTDRIFT_MISE_SYSTEM=1
//   - path under /usr/bin, /usr/local/bin, /bin, /sbin, /usr/sbin, /opt
//   - path under $HOME but not writable by the current user
//
// User-managed (may auto-upgrade):
//   - path under $HOME and writable by the current user
func ClassifyInstall(path string) InstallKind {
	if os.Getenv("DOTDRIFT_MISE_SYSTEM") == "1" {
		return InstallKindSystemWide
	}
	if path == "" {
		return InstallKindUnknown
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)

	systemDirs := []string{"/usr/bin", "/usr/local/bin", "/bin", "/sbin", "/usr/sbin", "/opt"}
	for _, dir := range systemDirs {
		dir = filepath.Clean(dir)
		if abs == dir || strings.HasPrefix(abs, dir+string(filepath.Separator)) {
			return InstallKindSystemWide
		}
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		home = filepath.Clean(home)
		if abs == home || strings.HasPrefix(abs, home+string(filepath.Separator)) {
			if isWritable(abs) {
				return InstallKindUserManaged
			}
			return InstallKindSystemWide
		}
	}

	return InstallKindUnknown
}

func isWritable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// CompareVersions compares calendar-style versions like 2026.6.6.
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func CompareVersions(a, b string) int {
	pa := parseVersion(a)
	pb := parseVersion(b)
	maxLen := len(pa)
	if len(pb) > maxLen {
		maxLen = len(pb)
	}
	for i := range maxLen {
		va := 0
		if i < len(pa) {
			va = pa[i]
		}
		vb := 0
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

func parseVersion(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

// ExecMise wraps a Mise value so it can be used as a Runner by step code.
type ExecMise struct {
	mise *Mise
}

// NewExecMise creates a Runner backed by a Mise instance.
func NewExecMise(m *Mise) *ExecMise {
	return &ExecMise{mise: m}
}

func (e *ExecMise) EnsureAndInstall(configPath string) error {
	path, err := e.mise.Ensure()
	if err != nil {
		return err
	}
	_, err = e.mise.Run(path, "install", "--cd", filepath.Dir(configPath))
	return err
}

func (e *ExecMise) DotfilesApply(configPath string, yes bool) error {
	path, err := e.mise.Ensure()
	if err != nil {
		return err
	}
	args := []string{"dotfiles", "apply", "--cd", filepath.Dir(configPath)}
	if yes {
		args = append(args, "--yes")
	}
	_, err = e.mise.Run(path, args...)
	return err
}

// Runner abstracts mise operations used by apply steps.
type Runner interface {
	EnsureAndInstall(configPath string) error
	DotfilesApply(configPath string, yes bool) error
}

// FakeRunner records mise invocations for tests.
type FakeRunner struct {
	InstallCalled  bool
	DotfilesCalled bool
	Yes            bool
	Err            error
}

func (f *FakeRunner) EnsureAndInstall(configPath string) error {
	f.InstallCalled = true
	return f.Err
}

func (f *FakeRunner) DotfilesApply(configPath string, yes bool) error {
	f.DotfilesCalled = true
	f.Yes = yes
	return f.Err
}

// GenerateTools emits a mise.toml [tools] section from the resolved plan.
func GenerateTools(versions map[string]string) string {
	if len(versions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[tools]\n")
	keys := make([]string, 0, len(versions))
	for k := range versions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = \"%s\"\n", k, versions[k])
	}
	return b.String()
}

// GenerateDotfiles emits a mise.toml [dotfiles] section from the resolved plan.
func GenerateDotfiles(entries []resolve.DotfileEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[dotfiles]\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "\"%s\" = { source = \"%s\", mode = \"%s\" }\n", e.Target, e.Source, e.Mode)
	}
	return b.String()
}

// GenerateConfig emits a complete mise.toml with tools and dotfiles sections.
func GenerateConfig(plan *resolve.Plan) string {
	var out string
	if tools := GenerateTools(plan.Tools.Versions); tools != "" {
		out += tools + "\n"
	}
	if dotfiles := GenerateDotfiles(plan.Dotfiles.Entries); dotfiles != "" {
		out += dotfiles + "\n"
	}
	return out
}

// DotfileEntry is a single resolved dotfile target.
type DotfileEntry struct {
	Target string
	Source string
	Mode   string
	Module string
	Layer  string
}

// Plan mirrors the parts of resolve.Plan needed for mise config generation.
type Plan struct {
	Tools    map[string]string
	Dotfiles []DotfileEntry
}
