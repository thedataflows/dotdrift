package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/smb"
	"github.com/thedataflows/dotdrift/internal/state"
)

// recordingBackend is a packages.Backend fake that records calls and can be
// made to fail the install.
type recordingBackend struct {
	events     *[]string
	presentErr error
}

var _ packages.Backend = (*recordingBackend)(nil)

func (b *recordingBackend) Present(_ context.Context, pkgs []string) error {
	*b.events = append(*b.events, "packages:present "+strings.Join(pkgs, ","))
	return b.presentErr
}

func (b *recordingBackend) Absent(_ context.Context, pkgs []string) error {
	*b.events = append(*b.events, "packages:absent "+strings.Join(pkgs, ","))
	return nil
}

func (b *recordingBackend) IsInstalled(context.Context, string) (bool, error) { return false, nil }

func (b *recordingBackend) DirectDeps(context.Context, string) ([]string, error) { return nil, nil }

// fakeMise returns a mise bootstrapper that never touches the OS: the binary
// is always found and reports the minimum required version.
func fakeMise(events *[]string) *mise.Mise {
	return &mise.Mise{
		LookPath: func(string) (string, error) {
			*events = append(*events, "mise:ensure")
			return "/fake/mise", nil
		},
		Run: func(_ string, args ...string) (string, error) {
			*events = append(*events, "mise:run "+strings.Join(args, " "))
			for _, a := range args {
				if a == "--version" {
					return mise.MinMiseVersion + "\n", nil
				}
			}
			return "", nil
		},
		Install:  func() (string, error) { return "", errors.New("test: unexpected mise install") },
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}
}

// recordingSmbRunner is a smb.Runner fake that records every command and
// reports success with empty output (so pdbedit -L lists no accounts).
type recordingSmbRunner struct {
	events *[]string
}

func (r *recordingSmbRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	*r.events = append(*r.events, "smb:run "+name+" "+strings.Join(args, " "))
	return "", nil
}

func (r *recordingSmbRunner) RunInteractive(_ context.Context, name string, args ...string) error {
	*r.events = append(*r.events, "smb:run-interactive "+name+" "+strings.Join(args, " "))
	return nil
}

// stubApplyDeps swaps the package-level seams in apply.go for fakes and
// returns the shared event log plus the recording packages backend.
// profile.Load and resolve.Resolve run for real (wrapped to record order) so
// the wiring is exercised end-to-end against a fixture profile.
func stubApplyDeps(t *testing.T, f *facts.Facts) (*[]string, *recordingBackend) {
	t.Helper()
	events := &[]string{}
	backend := &recordingBackend{events: events}

	origDetect, origLoad, origResolve, origMise, origFor := detectFacts, profileLoad, resolvePlan, defaultMise, packagesFor
	origSmbRunner := newSmbRunner
	t.Cleanup(func() {
		detectFacts, profileLoad, resolvePlan, defaultMise, packagesFor = origDetect, origLoad, origResolve, origMise, origFor
		newSmbRunner = origSmbRunner
	})

	detectFacts = func() (*facts.Facts, error) { return f, nil }
	profileLoad = func(root string, ff *facts.Facts) (*profile.Profile, error) {
		*events = append(*events, "load")
		return origLoad(root, ff)
	}
	resolvePlan = func(p *profile.Profile, ff *facts.Facts) (*resolve.Plan, error) {
		*events = append(*events, "resolve")
		return origResolve(p, ff)
	}
	defaultMise = func() *mise.Mise { return fakeMise(events) }
	packagesFor = func(string) packages.Backend { return backend }
	newSmbRunner = func() smb.Runner { return &recordingSmbRunner{events: events} }
	return events, backend
}

// requireOrder asserts that the given event prefixes appear in order.
func requireOrder(t *testing.T, events []string, want ...string) {
	t.Helper()
	at := 0
	for _, w := range want {
		found := -1
		for i := at; i < len(events); i++ {
			if strings.HasPrefix(events[i], w) {
				found = i
				break
			}
		}
		require.GreaterOrEqual(t, found, 0, "event %q missing after position %d in %v", w, at, events)
		at = found + 1
	}
}

func loadStateFile(t *testing.T, path string) *state.State {
	t.Helper()
	s, err := state.NewFileStore(path).Load()
	require.NoError(t, err)
	return s
}

func fingerprintFor(t *testing.T, profileDir string, f *facts.Facts) string {
	t.Helper()
	p, err := profile.Load(profileDir, f)
	require.NoError(t, err)
	return resolve.Fingerprint(p, f)
}

func planHashFor(t *testing.T, profileDir string, f *facts.Facts) string {
	t.Helper()
	p, err := profile.Load(profileDir, f)
	require.NoError(t, err)
	plan, err := resolve.Resolve(p, f)
	require.NoError(t, err)
	return resolve.PlanHash(plan)
}

func resolveFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "testdata", "profiles", "resolve")
}

// Happy path: detect → load → resolve → ensure → hooks-pre →
// packages/tools/dotfiles → hooks-post in order, ending in a complete state
// with the current selection fingerprint.
func TestApply_happyPath(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	requireOrder(t, *events,
		"load",
		"resolve",
		"mise:ensure",
		"mise:run run --cd "+filepath.Join(dir, "mise")+" hooks-pre-0",
		"packages:absent",
		"mise:run bootstrap",
		"mise:run install",
		"mise:run dotfiles apply",
		"mise:run run --cd "+filepath.Join(dir, "mise")+" hooks-post-0",
	)
	requireOrder(t, *events, "mise:run bootstrap")
	require.Contains(t, *events, "packages:absent emacs,nano")
	require.Contains(t, *events, "mise:run dotfiles apply --cd "+filepath.Join(dir, "mise", "dotfiles")+" --yes")

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.Empty(t, s.Current)
	require.Empty(t, s.Error)
	require.Equal(t, fingerprintFor(t, resolveFixture(t), f), s.Selection)
	for _, step := range []string{"hooks-pre", "packages", "tools", "dotfiles", "hooks-post"} {
		require.True(t, s.IsCompleted(step), "step %s not completed", step)
	}
}

// When dotdrift's stdin is a terminal, the generated hook tasks are marked
// interactive so a hook running an interactive command (e.g. sudo) reaches a
// controlling terminal and can disable echo; when stdin is not a terminal the
// key is omitted so mise runs normally.
func TestApply_hookTaskInteractiveReflectsStdinTTY(t *testing.T) {
	orig := stdinIsTerminal
	t.Cleanup(func() { stdinIsTerminal = orig })

	run := func(t *testing.T, tty bool) string {
		t.Helper()
		dir := t.TempDir()
		statePath := filepath.Join(dir, "state.json")
		f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
		stubApplyDeps(t, f)
		stdinIsTerminal = func() bool { return tty }
		require.NoError(t, (&ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true}).Run())
		cfg, err := os.ReadFile(filepath.Join(dir, "mise", "mise.toml"))
		require.NoError(t, err)
		return string(cfg)
	}

	t.Run("tty marks tasks interactive", func(t *testing.T) {
		require.Contains(t, run(t, true), "interactive = true")
	})
	t.Run("no tty omits the key", func(t *testing.T) {
		require.NotContains(t, run(t, false), "interactive")
	})
}

// The tools/dotfiles steps each get their own config (in their own
// directory) so rewriting them cannot clobber the shared full config: the
// [tasks] hook definitions must survive the whole pipeline, or hooks-post
// would run `mise run` against a config with no tasks.
func TestApply_stepsDoNotClobberSharedMiseConfig(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	shared, err := os.ReadFile(filepath.Join(dir, "mise", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(shared), `[tasks."hooks-post-0"]`, "shared config must keep hook tasks after the pipeline")
	require.Contains(t, string(shared), "[dotfiles]")

	dotfiles, err := os.ReadFile(filepath.Join(dir, "mise", "dotfiles", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(dotfiles), "[dotfiles]")
}

// When the persisted selection fingerprint differs from the current one, apply
// warns and resets the resume state, so previously completed steps re-run.
func TestApply_selectionChangeResetsStateAndWarns(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	stale := state.New()
	stale.Selection = "stale-fingerprint"
	stale.Completed["packages"] = true
	stale.Current = "packages"
	stale.Status = state.StatusFailed
	stale.Error = "boom"
	require.NoError(t, state.NewFileStore(statePath).Save(stale))

	var logBuf bytes.Buffer
	origLogger := log.Logger
	log.Logger = log.Output(&logBuf)
	t.Cleanup(func() { log.Logger = origLogger })

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath}
	require.NoError(t, cmd.Run())

	require.Contains(t, logBuf.String(), "changed")
	require.Contains(t, logBuf.String(), "resume state was reset")
	requireOrder(t, *events, "mise:run bootstrap") // packages re-ran after reset

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.Equal(t, fingerprintFor(t, resolveFixture(t), f), s.Selection)
	for _, step := range []string{"hooks-pre", "packages", "tools", "dotfiles", "hooks-post"} {
		require.True(t, s.IsCompleted(step), "step %s not completed", step)
	}
}

// When the selection is unchanged but the resolved plan content differs
// (e.g. a module's dotfiles were edited since the last failed apply), apply
// resets resume state so previously-completed steps re-run instead of being
// skipped — the bug that left edited dotfiles unapplied.
func TestApply_planContentChangeResetsState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	// Same selection as the current profile, but a stale plan hash and a
	// completed packages step that must NOT survive the content change.
	stale := state.New()
	stale.Selection = fingerprintFor(t, resolveFixture(t), f)
	stale.PlanHash = "stale-plan-hash"
	stale.Completed["packages"] = true
	stale.Current = "packages"
	stale.Status = state.StatusFailed
	stale.Error = "boom"
	require.NoError(t, state.NewFileStore(statePath).Save(stale))

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath}
	require.NoError(t, cmd.Run())

	requireOrder(t, *events, "mise:run bootstrap") // steps re-ran after plan-content reset

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.Equal(t, planHashFor(t, resolveFixture(t), f), s.PlanHash)
	require.Equal(t, fingerprintFor(t, resolveFixture(t), f), s.Selection)
}

// Apply prints the effective resolved plan (same rendering as `dotdrift plan`)
// to its Out writer before the pipeline runs.
func TestApply_printsPlan(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubApplyDeps(t, f)

	var buf bytes.Buffer
	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true, Out: &buf}
	require.NoError(t, cmd.Run())

	out := buf.String()
	t.Log(out)
	require.Contains(t, out, "fingerprint:")
	require.Contains(t, out, "packages:")
	require.Contains(t, out, "  - ripgrep")
	require.Contains(t, out, "remove:")
	require.Contains(t, out, "  - emacs")
	require.Contains(t, out, "tools:")
	require.Contains(t, out, "  node: 22")
	require.Contains(t, out, "dotfiles:")
	require.Contains(t, out, "~/.bashrc")

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
}

// Decision D8a (keep + test): apply writes the FULL mise config (tools +
// dotfiles) before the pipeline starts. When apply fails at the first
// pipeline step, the crash snapshot on disk still contains that full config
// and the state file records the failed step.
func TestApply_crashSnapshotKeepsFullMiseConfig(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)
	// Inject a bootstrap failure (packages install now goes through mise
	// bootstrap, not the backend's Present).
	defaultMise = func() *mise.Mise {
		m := fakeMise(events)
		m.Run = func(_ string, args ...string) (string, error) {
			for _, a := range args {
				if a == "bootstrap" {
					return "", errors.New("bootstrap boom")
				}
				if a == "--version" {
					return mise.MinMiseVersion + "\n", nil
				}
			}
			return "", nil
		}
		return m
	}

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "packages")

	s := loadStateFile(t, statePath)
	require.Equal(t, "packages", s.Current)
	require.Equal(t, state.StatusFailed, s.Status)
	require.Contains(t, s.Error, "boom")
	require.False(t, s.IsCompleted("packages"))
	require.True(t, s.IsCompleted("hooks-pre"), "hooks-pre runs before packages and must be complete")
	require.Equal(t, fingerprintFor(t, resolveFixture(t), f), s.Selection)

	configPath := filepath.Join(dir, "mise", "mise.toml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err, "pre-pipeline full mise config must exist after a crash")
	cfg := string(data)
	require.Contains(t, cfg, "[tools]")
	require.Contains(t, cfg, `node = "22"`)
	require.Contains(t, cfg, "[dotfiles]")
	require.Contains(t, cfg, "~/.bashrc")
	require.Contains(t, cfg, `[tasks."hooks-pre-0"]`)
	require.Contains(t, cfg, `[tasks."hooks-post-0"]`)

	for _, e := range *events {
		require.NotContains(t, e, "mise:run install", "tools step must not run after the packages failure")
	}
}

// --no-hooks suppresses both hooks steps: no mise task runs and the state
// records only packages/tools/dotfiles.
func TestApply_noHooksFlag(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true, NoHooks: true}
	require.NoError(t, cmd.Run())

	for _, e := range *events {
		require.NotContains(t, e, "hooks:", "--no-hooks must not run any hook task")
	}

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.False(t, s.IsCompleted("hooks-pre"))
	require.False(t, s.IsCompleted("hooks-post"))
	for _, step := range []string{"packages", "tools", "dotfiles"} {
		require.True(t, s.IsCompleted(step), "step %s not completed", step)
	}
}

// DOTDRIFT_NO_HOOKS=1 suppresses hooks exactly like --no-hooks.
func TestApply_noHooksEnv(t *testing.T) {
	t.Setenv("DOTDRIFT_NO_HOOKS", "1")
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	for _, e := range *events {
		require.NotContains(t, e, "hooks:", "DOTDRIFT_NO_HOOKS=1 must not run any hook task")
	}

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.False(t, s.IsCompleted("hooks-pre"))
	require.False(t, s.IsCompleted("hooks-post"))
}

func mountsFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "testdata", "profiles", "mounts")
}

// eventIdx returns the index of the first event containing sub, or -1.
func eventIdx(events []string, sub string) int {
	for i, e := range events {
		if strings.Contains(e, sub) {
			return i
		}
	}
	return -1
}

// A plan with mount entries gains a mounts step constructed after
// dotfiles-system and before smb/hooks-post; it runs mkdir for every
// destination, one daemon-reload, then enable/disable per entry state.
func TestApply_mountsStepConditional(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: mountsFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	requireOrder(t, *events,
		"mise:run run",       // hooks:pre
		"mise:run bootstrap", // packages
		"mise:run bootstrap", // system files
		"mise:run bootstrap", // mounts services
		"mise:run bootstrap", // smb accounts+services
		"smb:run",            // PostBootstrap testparm
		"mise:run run",       // hooks:post
	)

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	for _, step := range []string{"dotfiles-system", "mounts", "smb", "hooks-post"} {
		require.True(t, s.IsCompleted(step), "step %s not completed", step)
	}
}

// A plan with smb modules gains an smb step that activates group/users,
// validates config, and ensures the service — after mounts, before
// hooks-post. The missing-password notice goes to the command's Out writer.
func TestApply_smbStepConditional(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	var buf bytes.Buffer
	cmd := &ApplyCmd{Profile: mountsFixture(t), State: statePath, Yes: true, Out: &buf}
	require.NoError(t, cmd.Run())

	// Bootstrap handles group/user/service convergence; PostBootstrap still
	// uses the smbRunner for testparm + smbpasswd.
	requireOrder(t, *events,
		"mise:run bootstrap", // mounts services
		"mise:run bootstrap", // smb accounts+services
		"smb:run",            // PostBootstrap testparm
		"mise:run run",       // hooks:post
	)

	joined := strings.Join(*events, "\n")
	require.Contains(t, joined, "testparm -s")
	require.Contains(t, joined, "pdbedit -L")

	if eventIdx(*events, "smb:run-interactive") >= 0 {
		require.Contains(t, joined, "smbpasswd -a cri")
	} else {
		require.Contains(t, buf.String(), "samba password missing for cri; run: sudo smbpasswd -a cri")
	}

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.True(t, s.IsCompleted("smb"))
}

// Without mounts/smb aggregates no mounts/smb step is constructed: the
// recorded steps are exactly today's set (S3 byte-equal guard).
func TestApply_noMountsNoSmb_stepsAbsent(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	for _, e := range *events {
		require.NotContains(t, e, "mounts:run", "no mounts step must run for a plan without mounts")
		require.NotContains(t, e, "smb:run", "no smb step must run for a plan without smb")
	}

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.False(t, s.IsCompleted("mounts"))
	require.False(t, s.IsCompleted("smb"))
	completed := make([]string, 0, len(s.Completed))
	for step := range s.Completed {
		completed = append(completed, step)
	}
	sort.Strings(completed)
	require.Equal(t, []string{"dotfiles", "hooks-post", "hooks-pre", "packages", "tools"}, completed,
		"completed steps must be exactly today's unconditional+hooks set")
}

// A positional module filter limits the apply: resolvePlan observes a profile
// whose Selected holds only the filtered ids, with excluded modules skipped
// as "module filter" (observed via the resolvePlan seam wrapper). The scope
// fixture selects demo (system) and shell (user); filtering to shell keeps
// demo out of the plan entirely.
func TestApply_moduleFilterLimitsSelection(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	var selectedIDs []string
	skippedReasons := map[string]string{}
	innerResolve := resolvePlan
	resolvePlan = func(p *profile.Profile, ff *facts.Facts) (*resolve.Plan, error) {
		for _, m := range p.Selected {
			selectedIDs = append(selectedIDs, m.ID)
		}
		for _, s := range p.Skipped {
			skippedReasons[s.Module.ID] = s.Reason
		}
		return innerResolve(p, ff)
	}

	cmd := &ApplyCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "scope"),
		State:   statePath,
		Yes:     true,
		Modules: []string{"shell"},
	}
	require.NoError(t, cmd.Run())

	requireOrder(t, *events, "load", "resolve")
	require.Equal(t, []string{"shell"}, selectedIDs)
	require.Equal(t, "module filter", skippedReasons["demo"])

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
}

// An unknown module id fails after profile load and before plan resolution:
// Run returns the error and resolvePlan is never called.
func TestApply_moduleFilterUnknownErrors(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Modules: []string{"nope"}}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "nope")
	require.Contains(t, err.Error(), "shell", "error must list the valid module ids")
	require.NotContains(t, *events, "resolve", "resolvePlan must not run when the filter is invalid")
}

// verboseRecordingBackend records SetVerbose calls while satisfying
// packages.Backend through the embedded recording fake.
type verboseRecordingBackend struct {
	*recordingBackend
	verbose bool
}

func (b *verboseRecordingBackend) SetVerbose(v bool) { b.verbose = v }

// verboseSmbRunner records SetVerbose calls while satisfying smb.Runner
// through the embedded recording fake.
type verboseSmbRunner struct {
	*recordingSmbRunner
	verbose bool
}

func (r *verboseSmbRunner) SetVerbose(v bool) { r.verbose = v }

// stubVerboseDeps swaps the runner-construction seams for verbose-recording
// fakes and returns them plus the captured mise, so tests can assert where
// --verbose landed.
func stubVerboseDeps(t *testing.T, f *facts.Facts) (miseCapture **mise.Mise, backend *verboseRecordingBackend, sr *verboseSmbRunner) {
	t.Helper()
	events := &[]string{}
	backend = &verboseRecordingBackend{recordingBackend: &recordingBackend{events: events}}
	sr = &verboseSmbRunner{recordingSmbRunner: &recordingSmbRunner{events: events}}

	var captured *mise.Mise
	origDetect, origMise, origFor := detectFacts, defaultMise, packagesFor
	origSmbRunner := newSmbRunner
	t.Cleanup(func() {
		detectFacts, defaultMise, packagesFor = origDetect, origMise, origFor
		newSmbRunner = origSmbRunner
	})

	detectFacts = func() (*facts.Facts, error) { return f, nil }
	defaultMise = func() *mise.Mise { captured = fakeMise(events); return captured }
	packagesFor = func(string) packages.Backend { return backend }
	newSmbRunner = func() smb.Runner { return sr }
	return &captured, backend, sr
}

// --verbose threads through the existing construction seams: the mise
// bootstrapper's Verbose field is set and every interface runner receives
// SetVerbose(true).
func TestApply_verbosePropagatesToRunners(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	capturedMise, backend, sr := stubVerboseDeps(t, f)

	cmd := &ApplyCmd{Profile: mountsFixture(t), State: statePath, Yes: true, Verbose: true}
	require.NoError(t, cmd.Run())

	require.NotNil(t, *capturedMise)
	require.True(t, (*capturedMise).Verbose, "mise must stream in verbose mode")
	require.True(t, backend.verbose, "packages backend must receive --verbose")
	require.True(t, sr.verbose, "smb runner must receive --verbose")
}

// Without --verbose every runner stays non-verbose (today's behavior).
func TestApply_nonVerboseLeavesRunnersQuiet(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	capturedMise, backend, sr := stubVerboseDeps(t, f)

	cmd := &ApplyCmd{Profile: mountsFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	require.NotNil(t, *capturedMise)
	require.False(t, (*capturedMise).Verbose, "mise must stay quiet without --verbose")
	require.False(t, backend.verbose, "packages backend must stay quiet without --verbose")
	require.False(t, sr.verbose, "smb runner must stay quiet without --verbose")
}
