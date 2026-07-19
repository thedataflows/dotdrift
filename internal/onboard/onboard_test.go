package onboard_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/onboard"
)

// isolateState points the XDG state dir at a temp dir so onboard's generated
// mise config never lands in the real user state directory during tests.
func isolateState(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	return root
}

func TestPathMap_homeAndSystem(t *testing.T) {
	// Exposed through Run behavior; mapPath is internal, so we test via the
	// resulting module.toml source entries after a successful onboard.
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))
	sys := filepath.Join("/etc", "pacman.conf")

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src, sys},
		App:         "bash",
		Mode:        "link",
		Home:        home,
	})
	require.NoError(t, err)

	modToml := filepath.Join(profile, "modules", "bash", "module.toml")
	require.FileExists(t, modToml)
	content, err := readFile(modToml)
	require.NoError(t, err)
	require.Contains(t, content, `"~/.bashrc"`)
	require.Contains(t, content, `"home/.bashrc"`)
	require.Contains(t, content, `"/etc/pacman.conf"`)
	require.Contains(t, content, `"system/etc/pacman.conf"`)
}

func TestInferApp_configDir(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".config", "nvim", "init.lua")
	require.NoError(t, writeFile(src, "init"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		Home:        home,
	})
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(profile, "modules", "nvim"))
}

func TestOnboard_copiesTree(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".config", "app", "config.toml")
	require.NoError(t, writeFile(src, "config"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "app",
		Home:        home,
	})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(profile, "modules", "app", "home", ".config", "app", "config.toml"))
}

func TestOnboard_writesToml(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Mode:        "copy",
		Packages:    []string{"bash"},
		Tools:       []string{"node=20"},
		Home:        home,
	})
	require.NoError(t, err)

	modToml := filepath.Join(profile, "modules", "bash", "module.toml")
	content, err := readFile(modToml)
	require.NoError(t, err)
	require.Contains(t, content, `id = "bash"`)
	require.Contains(t, content, `app = "bash"`)
	require.Contains(t, content, `mode = "copy"`)
	require.Contains(t, content, `"bash"`)
	require.Contains(t, content, `node = "20"`)
}

func TestOnboard_noEnableFlagNeeded(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(profile, "modules", "bash"))
	require.FileExists(t, filepath.Join(profile, "modules", "bash", "module.toml"))
	// No --enable flag was used; presence selects the module.
}

func TestOnboard_orderCopyThenEnsureThenMiseApply(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	rr := &recordingRunner{}
	o := &onboard.Onboard{Mise: rr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	// Copy happened before any mise call: the materialized file exists.
	require.FileExists(t, filepath.Join(profile, "modules", "bash", "home", ".bashrc"))
	// And mise was invoked strictly as: install, then dotfiles apply.
	require.Equal(t, []string{"ensure", "dotfiles"}, rr.calls)
}

func TestOnboard_defaultModeLink(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	modToml := filepath.Join(profile, "modules", "bash", "module.toml")
	content, err := readFile(modToml)
	require.NoError(t, err)
	require.Contains(t, content, `mode = "link"`)
}

func TestOnboard_conflictKeepsModule(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	// Onboard the same path again should fail with conflict, but leave module.
	err = o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "conflict")
	require.Contains(t, err.Error(), "already exists in module (remove it or onboard into a different module id)")
	require.FileExists(t, filepath.Join(profile, "modules", "bash", "module.toml"))
}

func TestOnboard_forceReplacesExistingFile(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "v1"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	opts := onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	}
	require.NoError(t, o.Run(opts))

	// Live file changed since the first onboard.
	require.NoError(t, writeFile(src, "v2"))

	// Without force the second onboard still conflicts.
	err := o.Run(opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "conflict")

	// With force the module copy is refreshed from the live file.
	forced := opts
	forced.Force = true
	require.NoError(t, o.Run(forced))

	copied := filepath.Join(profile, "modules", "bash", "home", ".bashrc")
	content, err := readFile(copied)
	require.NoError(t, err)
	require.Equal(t, "v2", content)
}

func TestOnboard_forceReplacesExistingDir(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	dir := filepath.Join(home, ".config", "app")
	require.NoError(t, writeFile(filepath.Join(dir, "old.conf"), "old"))
	require.NoError(t, writeFile(filepath.Join(dir, "keep.conf"), "v1"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	opts := onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{dir},
		App:         "app",
		Home:        home,
	}
	require.NoError(t, o.Run(opts))

	// Live dir changed: one file removed, one modified, one added.
	require.NoError(t, os.Remove(filepath.Join(dir, "old.conf")))
	require.NoError(t, writeFile(filepath.Join(dir, "keep.conf"), "v2"))
	require.NoError(t, writeFile(filepath.Join(dir, "new.conf"), "new"))

	forced := opts
	forced.Force = true
	require.NoError(t, o.Run(forced))

	base := filepath.Join(profile, "modules", "app", "home", ".config", "app")
	require.NoFileExists(t, filepath.Join(base, "old.conf"))
	kept, err := readFile(filepath.Join(base, "keep.conf"))
	require.NoError(t, err)
	require.Equal(t, "v2", kept)
	require.FileExists(t, filepath.Join(base, "new.conf"))
}

func TestOnboard_dryRun_noSideEffects(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
		DryRun:      true,
	})
	require.NoError(t, err)

	require.NoDirExists(t, filepath.Join(profile, "modules", "bash"))
	require.False(t, fr.InstallCalled)
	require.False(t, fr.DotfilesCalled)
}

// recordingRunner captures the mise call sequence and, for the dotfiles step,
// the config path and yes flag, so tests can assert order and flag plumbing.
type recordingRunner struct {
	calls      []string
	configPath string
	yes        bool
	ctx        context.Context
}

func (r *recordingRunner) EnsureAndInstall(ctx context.Context, configPath string) error {
	r.ctx = ctx
	r.calls = append(r.calls, "ensure")
	return nil
}

func (r *recordingRunner) DotfilesApply(ctx context.Context, configPath string, yes bool) error {
	r.ctx = ctx
	r.calls = append(r.calls, "dotfiles")
	r.configPath = configPath
	r.yes = yes
	return nil
}

var sourceRe = regexp.MustCompile(`source = "([^"]+)"`)

// validatingRunner mimics the real ExecMise invocation (mise runs with
// --cd <dir of configPath>) and asserts the path semantics real mise needs:
// every dotfile source must be an absolute path that exists on disk.
type validatingRunner struct {
	t           *testing.T
	profileRoot string
	stateRoot   string
	configPath  string
}

func (v *validatingRunner) EnsureAndInstall(ctx context.Context, configPath string) error {
	return nil
}

func (v *validatingRunner) DotfilesApply(ctx context.Context, configPath string, yes bool) error {
	v.configPath = configPath
	cwd := filepath.Dir(configPath)
	require.True(v.t, strings.HasPrefix(cwd, v.stateRoot+string(os.PathSeparator)),
		"mise cwd must live under the XDG state dir, got %s", cwd)
	require.False(v.t, strings.HasPrefix(cwd, v.profileRoot+string(os.PathSeparator)),
		"mise cwd must not live inside the profile, got %s", cwd)

	data, err := os.ReadFile(configPath)
	require.NoError(v.t, err)
	sources := sourceRe.FindAllStringSubmatch(string(data), -1)
	require.NotEmpty(v.t, sources, "generated config must reference at least one dotfile source")
	for _, m := range sources {
		src := m[1]
		require.True(v.t, filepath.IsAbs(src), "source %q must be absolute (mise resolves it against %s)", src, cwd)
		require.FileExists(v.t, src)
	}
	return nil
}

func TestOnboard_miseConfigInStateDir_absoluteSources(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	stateRoot := isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	vr := &validatingRunner{t: t, profileRoot: profile, stateRoot: stateRoot}
	o := &onboard.Onboard{Mise: vr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	moduleDir := filepath.Join(profile, "modules", "bash")
	require.NoDirExists(t, filepath.Join(moduleDir, ".mise"),
		"onboard must not write runtime mise config inside the profile")

	// The module dir must contain only module content, no runtime files.
	var files []string
	require.NoError(t, filepath.Walk(moduleDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, err := filepath.Rel(moduleDir, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	}))
	require.ElementsMatch(t, []string{"module.toml", "home/.bashrc"}, files)
}

func TestOnboard_preservesDirTreeModes(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	dir := filepath.Join(home, ".config", "app")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret"), []byte("s"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "normal"), []byte("n"), 0o644))
	require.NoError(t, os.Chmod(filepath.Join(dir, "secret"), 0o600))
	require.NoError(t, os.Chmod(filepath.Join(dir, "sub"), 0o700))
	require.NoError(t, os.Chmod(filepath.Join(dir, "sub", "normal"), 0o640))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{dir},
		App:         "app",
		Home:        home,
	})
	require.NoError(t, err)

	base := filepath.Join(profile, "modules", "app", "home", ".config", "app")
	for path, want := range map[string]os.FileMode{
		"secret":     0o600,
		"sub":        0o700,
		"sub/normal": 0o640,
	} {
		info, err := os.Stat(filepath.Join(base, filepath.FromSlash(path)))
		require.NoError(t, err)
		require.Equal(t, want, info.Mode().Perm(), "mode of %s", path)
	}
}

func TestOnboard_yesPropagatesToDotfilesApply(t *testing.T) {
	for _, tc := range []struct {
		name string
		yes  bool
	}{
		{"yes flag set", true},
		{"yes flag unset", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			profile := t.TempDir()
			isolateState(t)

			src := filepath.Join(home, ".bashrc")
			require.NoError(t, writeFile(src, "bashrc"))

			rr := &recordingRunner{}
			o := &onboard.Onboard{Mise: rr}
			err := o.Run(onboard.Options{
				ProfileRoot: profile,
				Paths:       []string{src},
				App:         "bash",
				Home:        home,
				Yes:         tc.yes,
			})
			require.NoError(t, err)
			require.Equal(t, tc.yes, rr.yes)
		})
	}
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type ctxTestKey struct{}

func TestOnboard_ctxPropagatesToMiseRunner(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	rr := &recordingRunner{}
	o := &onboard.Onboard{Mise: rr}
	ctx := context.WithValue(context.Background(), ctxTestKey{}, "marker")
	err := o.Run(onboard.Options{
		Ctx:         ctx,
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)
	require.NotNil(t, rr.ctx)
	require.Equal(t, "marker", rr.ctx.Value(ctxTestKey{}))
}
