// Package mise bootstraps the mise binary and runs mise operations.
package mise

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/thedataflows/dotdrift/internal/facts"
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
//
// RunContext is preferred over the legacy Run; both are kept so existing
// fakes that only set Run keep working. The result of the first Ensure call
// is memoized for the lifetime of the struct.
type Mise struct {
	LookPath   func(string) (string, error)
	Run        func(string, ...string) (string, error)
	RunContext func(context.Context, string, ...string) (string, error)
	Install    func() (string, error)
	Classify   func(string) InstallKind

	// Env is extra environment ("KEY=value") appended to the subprocess
	// environment on the real exec path; fakes (Run/RunContext) bypass it.
	Env []string

	ensureOnce sync.Once
	ensurePath string
	ensureErr  error
}

// DefaultMise returns a Mise configured with real OS dependencies. Run and
// RunContext stay nil so runner() uses the env-aware real exec path.
func DefaultMise() *Mise {
	return &Mise{
		LookPath: defaultLookPath,
		Install:  defaultInstall,
		Classify: ClassifyInstall,
	}
}

// defaultRunContext executes a command, cancelling it with ctx. On failure the
// trimmed combined output is appended so callers surface mise's own message.
func defaultRunContext(ctx context.Context, name string, args ...string) (string, error) {
	return runContextEnv(ctx, nil, name, args...)
}

// runContextEnv is defaultRunContext with extra environment entries appended
// to the inherited environment; a later duplicate key wins over an inherited
// one, so callers can override (merged) variables.
func runContextEnv(ctx context.Context, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			return string(out), fmt.Errorf("%w\n%s", err, trimmed)
		}
		return string(out), err
	}
	return string(out), nil
}

// runner resolves the ctx-aware runner: RunContext wins, then legacy Run,
// then the real exec implementation (which honors Env).
func (m *Mise) runner() func(context.Context, string, ...string) (string, error) {
	if m.RunContext != nil {
		return m.RunContext
	}
	if m.Run != nil {
		run := m.Run
		return func(_ context.Context, name string, args ...string) (string, error) {
			return run(name, args...)
		}
	}
	if len(m.Env) > 0 {
		env := m.Env
		return func(ctx context.Context, name string, args ...string) (string, error) {
			return runContextEnv(ctx, env, name, args...)
		}
	}
	return defaultRunContext
}

// runWithEnv executes one command with per-call extra environment merged
// after Env. Shared struct state is never mutated, so concurrent callers are
// race-free. Fakes (Run/RunContext) cannot receive env and are called as-is.
func (m *Mise) runWithEnv(ctx context.Context, extraEnv []string, name string, args ...string) (string, error) {
	if m.RunContext == nil && m.Run == nil && (len(m.Env) > 0 || len(extraEnv) > 0) {
		env := make([]string, 0, len(m.Env)+len(extraEnv))
		env = append(env, m.Env...)
		env = append(env, extraEnv...)
		return runContextEnv(ctx, env, name, args...)
	}
	return m.runner()(ctx, name, args...)
}

// trustEnv returns a MISE_TRUSTED_CONFIG_PATHS entry covering the directory
// of configPath so mise accepts dotdrift-generated configs that live outside
// its default trusted paths. Any pre-existing user value is preserved and the
// generated dir is appended (colon-separated), never clobbered.
func trustEnv(configPath string) []string {
	dir := filepath.Dir(configPath)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	value := dir
	if existing := os.Getenv("MISE_TRUSTED_CONFIG_PATHS"); existing != "" {
		value = existing + string(os.PathListSeparator) + dir
	}
	return []string{"MISE_TRUSTED_CONFIG_PATHS=" + value}
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
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", installFailure(installPath, err, out)
	}
	return installPath, nil
}

func installFailure(installPath string, err error, out []byte) error {
	msg := fmt.Sprintf("install mise via %s: %v", InstallerURL, err)
	if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
		msg += "\n" + trimmed
	}
	return fmt.Errorf("%s\nhint: check network connectivity or pre-seed %s", msg, installPath)
}

// Ensure finds or installs a mise binary meeting the minimum version.
// The result (success or failure) is computed once and memoized.
func (m *Mise) Ensure() (string, error) {
	return m.EnsureContext(context.Background())
}

// EnsureContext is Ensure with ctx propagated to every subprocess.
func (m *Mise) EnsureContext(ctx context.Context) (string, error) {
	m.ensureOnce.Do(func() {
		m.ensurePath, m.ensureErr = m.ensure(ctx)
	})
	return m.ensurePath, m.ensureErr
}

func (m *Mise) ensure(ctx context.Context) (string, error) {
	lookPath := m.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	run := m.runner()

	path, err := lookPath("mise")
	if err != nil || path == "" {
		if m.Install == nil {
			return "", fmt.Errorf("mise not found and no installer configured")
		}
		log.Info().Msgf("mise not found; installing via %s …", InstallerURL)
		path, err = m.Install()
		if err != nil {
			return "", fmt.Errorf("install mise: %w", err)
		}
		if path == "" {
			return "", fmt.Errorf("mise not found after install")
		}
	}

	version, err := m.version(ctx, run, path)
	if err != nil {
		return "", fmt.Errorf("check mise version: %w", err)
	}
	cmp, err := CompareVersions(version, MinMiseVersion)
	if err != nil {
		return "", fmt.Errorf("check mise version: %w", err)
	}
	if cmp >= 0 {
		return path, nil
	}

	classify := m.Classify
	if classify == nil {
		classify = ClassifyInstall
	}
	kind := classify(path)

	switch kind {
	case InstallKindSystemWide:
		log.Warn().Msgf("mise %s < required %s; system install at %s — upgrade with your package manager", version, MinMiseVersion, path)
		return "", fmt.Errorf("mise %s at %s is older than required %s; system install — upgrade with your package manager", version, path, MinMiseVersion)
	case InstallKindUnknown:
		return "", fmt.Errorf("mise %s at %s is older than required %s; ambiguous install — upgrade with your package manager or reinstall via %s", version, path, MinMiseVersion, InstallerURL)
	}

	if m.Install == nil {
		return "", fmt.Errorf("mise %s at %s is older than required %s and no installer configured", version, path, MinMiseVersion)
	}

	// Prefer self-update for user-managed installs.
	log.Info().Msgf("mise %s < required %s; upgrading user install…", version, MinMiseVersion)
	if _, err := run(ctx, path, "self-update"); err == nil {
		newVersion, verr := m.version(ctx, run, path)
		if verr == nil {
			if cmp, cerr := CompareVersions(newVersion, MinMiseVersion); cerr == nil && cmp >= 0 {
				return path, nil
			}
		}
	}

	path, err = m.Install()
	if err != nil {
		return "", fmt.Errorf("upgrade mise: %w", err)
	}
	version, err = m.version(ctx, run, path)
	if err != nil {
		return "", fmt.Errorf("re-check mise version after upgrade: %w", err)
	}
	cmp, err = CompareVersions(version, MinMiseVersion)
	if err != nil {
		return "", fmt.Errorf("re-check mise version after upgrade: %w", err)
	}
	if cmp < 0 {
		return "", fmt.Errorf("mise %s at %s is still older than required %s after upgrade", version, path, MinMiseVersion)
	}
	return path, nil
}

// version runs `mise --version` and scans the output for the first token
// that looks like a version (leading digit), so a leading program name or
// other token does not break parsing.
func (m *Mise) version(ctx context.Context, run func(context.Context, string, ...string) (string, error), path string) (string, error) {
	out, err := run(ctx, path, "--version")
	if err != nil {
		return "", err
	}
	for _, f := range strings.Fields(strings.TrimSpace(out)) {
		f = strings.TrimPrefix(f, "v")
		if f == "" {
			continue
		}
		if c := f[0]; c >= '0' && c <= '9' {
			return f, nil
		}
	}
	return "", fmt.Errorf("no version token in mise --version output %q", strings.TrimSpace(out))
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
// Returns -1 if a < b, 0 if equal, 1 if a > b. A version carrying a
// pre-release/build suffix (e.g. "2025.1.0-rc1", "2025.1.0+build.5")
// compares below the plain release. Unparseable input is an error.
func CompareVersions(a, b string) (int, error) {
	pa, preA, err := parseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("parse version %q: %w", a, err)
	}
	pb, preB, err := parseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("parse version %q: %w", b, err)
	}
	for i := range max(len(pa), len(pb)) {
		var va, vb int
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1, nil
		}
		if va > vb {
			return 1, nil
		}
	}
	switch {
	case preA == preB:
		return 0, nil
	case preA:
		return -1, nil
	default:
		return 1, nil
	}
}

// parseVersion splits v into numeric segments. Any non-numeric suffix on a
// segment (e.g. "-rc1", "-dev.1", "+build.5") marks the version as a
// pre-release and ends parsing; segments must start with a digit.
func parseVersion(v string) (segs []int, prerelease bool, err error) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return nil, false, fmt.Errorf("empty version string")
	}
	for _, p := range strings.Split(v, ".") {
		i := 0
		for i < len(p) && p[i] >= '0' && p[i] <= '9' {
			i++
		}
		if i == 0 {
			return nil, false, fmt.Errorf("invalid version segment %q", p)
		}
		n, cerr := strconv.Atoi(p[:i])
		if cerr != nil {
			return nil, false, fmt.Errorf("invalid version segment %q: %v", p, cerr)
		}
		segs = append(segs, n)
		if i < len(p) {
			prerelease = true
			break
		}
	}
	return segs, prerelease, nil
}

// ExecMise wraps a Mise value so it can be used as a Runner by step code.
type ExecMise struct {
	mise *Mise
}

// NewExecMise creates a Runner backed by a Mise instance.
func NewExecMise(m *Mise) *ExecMise {
	return &ExecMise{mise: m}
}

func (e *ExecMise) EnsureAndInstall(ctx context.Context, configPath string) error {
	path, err := e.mise.EnsureContext(ctx)
	if err != nil {
		return err
	}
	_, err = e.mise.runWithEnv(ctx, trustEnv(configPath), path, "install", "--cd", filepath.Dir(configPath))
	return err
}

func (e *ExecMise) DotfilesApply(ctx context.Context, configPath string, yes bool) error {
	path, err := e.mise.EnsureContext(ctx)
	if err != nil {
		return err
	}
	args := []string{"dotfiles", "apply", "--cd", filepath.Dir(configPath)}
	if yes {
		args = append(args, "--yes")
	}
	_, err = e.mise.runWithEnv(ctx, trustEnv(configPath), path, args...)
	return err
}

// geteuid is a test seam for the already-root check in dotfilesApplyArgv.
var geteuid = os.Geteuid

// dotfilesApplyArgv builds the argv for applying system-scope dotfiles:
// directly as <mise> dotfiles apply when already root (EUID 0, e.g.
// containers), otherwise elevated as sudo -E <mise> dotfiles apply. The -E
// preserves the MISE_TRUSTED_CONFIG_PATHS entry the trust plumbing sets on
// the sudo child environment, so the trust handling keeps working through
// the elevation.
func dotfilesApplyArgv(euid int, misePath, configPath string, yes bool) []string {
	args := []string{"dotfiles", "apply", "--cd", filepath.Dir(configPath)}
	if yes {
		args = append(args, "--yes")
	}
	if euid == 0 {
		return append([]string{misePath}, args...)
	}
	return append([]string{"sudo", "-E", misePath}, args...)
}

// DotfilesApplySudo applies system-scope dotfiles with root privileges. When
// the process is not already root it invokes sudo (failing loudly if sudo is
// missing or authentication fails); as root it applies directly.
func (e *ExecMise) DotfilesApplySudo(ctx context.Context, configPath string, yes bool) error {
	path, err := e.mise.EnsureContext(ctx)
	if err != nil {
		return err
	}
	argv := dotfilesApplyArgv(geteuid(), path, configPath, yes)
	_, err = e.mise.runWithEnv(ctx, trustEnv(configPath), argv[0], argv[1:]...)
	return err
}

// RunTask runs a named task (e.g. "hooks:pre") from the generated config.
func (e *ExecMise) RunTask(ctx context.Context, configPath, taskName string) error {
	path, err := e.mise.EnsureContext(ctx)
	if err != nil {
		return err
	}
	_, err = e.mise.runWithEnv(ctx, trustEnv(configPath), path, "run", "--cd", filepath.Dir(configPath), taskName)
	return err
}

// Runner abstracts mise operations used by apply steps.
type Runner interface {
	EnsureAndInstall(ctx context.Context, configPath string) error
	DotfilesApply(ctx context.Context, configPath string, yes bool) error
}

// FakeRunner records mise invocations for tests.
type FakeRunner struct {
	InstallCalled  bool
	DotfilesCalled bool
	Yes            bool
	Err            error
}

func (f *FakeRunner) EnsureAndInstall(ctx context.Context, configPath string) error {
	f.InstallCalled = true
	return f.Err
}

func (f *FakeRunner) DotfilesApply(ctx context.Context, configPath string, yes bool) error {
	f.DotfilesCalled = true
	f.Yes = yes
	return f.Err
}

func tomlEscape(s string) string {
	return tomlEscaper.Replace(s)
}

var tomlEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

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
		fmt.Fprintf(&b, "%s = \"%s\"\n", k, tomlEscape(versions[k]))
	}
	return b.String()
}

// MiseDotfileMode translates dotdrift's dotfile mode vocabulary into mise's
// at the config-generation boundary. dotdrift keeps `link` as its documented
// default mode, but real mise only accepts `symlink` — verified against mise
// 2026.7.10, where `mode = "link"` is ignored ("unknown mode 'link',
// ignoring entry") with exit code 0 and no file created. All other dotdrift
// modes (copy, template, symlink-each) are already valid mise vocabulary.
func MiseDotfileMode(mode string) string {
	if mode == "link" {
		return "symlink"
	}
	return mode
}

// GenerateDotfiles emits a mise.toml [dotfiles] section from the resolved plan.
func GenerateDotfiles(entries []resolve.DotfileEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[dotfiles]\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "\"%s\" = { source = \"%s\", mode = \"%s\" }\n",
			tomlEscape(e.Target), tomlEscape(e.Source), tomlEscape(MiseDotfileMode(e.Mode)))
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

// GenerateHookTasks emits a [tasks] section with one mise task per non-empty
// hook list ("hooks:pre" and/or "hooks:post"). Each task runs its commands in
// order from dir (the absolute profile root) with the DOTDRIFT_* facts
// environment. Returns "" when both lists are empty.
func GenerateHookTasks(hooks resolve.HooksStep, profileRoot string, f *facts.Facts) string {
	var b strings.Builder
	writeHookTask(&b, "hooks:pre", hooks.Pre, profileRoot, f)
	writeHookTask(&b, "hooks:post", hooks.Post, profileRoot, f)
	return b.String()
}

func writeHookTask(b *strings.Builder, name string, commands []string, profileRoot string, f *facts.Facts) {
	if len(commands) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "[tasks.%q]\n", name)
	b.WriteString("run = [\n")
	for _, c := range commands {
		fmt.Fprintf(b, "  \"%s\",\n", tomlEscape(c))
	}
	b.WriteString("]\n")
	fmt.Fprintf(b, "dir = \"%s\"\n", tomlEscape(profileRoot))
	fmt.Fprintf(b, "env = { DOTDRIFT_PROFILE = \"%s\", DOTDRIFT_HOSTNAME = \"%s\", DOTDRIFT_USERNAME = \"%s\", DOTDRIFT_OS = \"%s\", DOTDRIFT_BACKEND = \"%s\" }\n",
		tomlEscape(profileRoot), tomlEscape(f.Hostname), tomlEscape(f.Username), tomlEscape(f.OS), tomlEscape(f.Backend))
}

// GenerateApplyConfig emits the full apply-time mise.toml: tools, dotfiles,
// and hook tasks. The hook tasks need facts and the absolute profile root, so
// only apply uses this; onboard keeps using GenerateConfig.
func GenerateApplyConfig(plan *resolve.Plan, profileRoot string, f *facts.Facts) string {
	out := GenerateConfig(plan)
	if tasks := GenerateHookTasks(plan.Hooks, profileRoot, f); tasks != "" {
		out += tasks + "\n"
	}
	return out
}
