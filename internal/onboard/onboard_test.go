package onboard_test

import (
	"bytes"
	"context"
	"fmt"
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
	// System paths are absolute paths outside $HOME. Use a temp file the test
	// owns rather than a real system file: /etc/pacman.conf exists only on
	// Arch/CachyOS, so hardcoding it failed the test on Ubuntu CI.
	sys := filepath.Join(t.TempDir(), "pacman.conf")
	require.NoError(t, writeFile(sys, "pacman"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src, sys},
		App:         "bash",
		Mode:        "symlink",
		Home:        home,
	})
	require.NoError(t, err)

	modToml := filepath.Join(profile, "modules", "bash", "module.toml")
	require.FileExists(t, modToml)
	content, err := readFile(modToml)
	require.NoError(t, err)
	require.Contains(t, content, `"~/.bashrc"`)
	require.Contains(t, content, `"home/.bashrc"`)
	// System path: target is the absolute path verbatim; the source strips the
	// leading separator and is prefixed with system/.
	require.Contains(t, content, fmt.Sprintf("%q", sys))
	require.Contains(t, content, fmt.Sprintf(`"system/%s"`, strings.TrimPrefix(sys, "/")))
}

// A bare relative path (no ~/) is home-relative, matching the ~/.config/...
// convention hand-authored modules use: ".bashrc" means "~/.bashrc". The
// materialized module.toml must carry the "~/..." target and "home/..."
// source, never the absolute home path.
func TestOnboard_relativePathIsHomeRelative(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	require.NoError(t, writeFile(filepath.Join(home, ".bashrc"), "bashrc"))

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{".bashrc"},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	content, err := readFile(filepath.Join(profile, "modules", "bash", "module.toml"))
	require.NoError(t, err)
	require.Contains(t, content, `"~/.bashrc"`)
	require.Contains(t, content, `"home/.bashrc"`)
	require.NotContains(t, content, home, "module.toml must not embed the absolute home path")
}

// App is mandatory (issue 0055): no first-path inference — an empty App on
// a live-path run is a loud error, not a guess at the module name.
func TestOnboard_emptyAppErrors(t *testing.T) {
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
	require.ErrorContains(t, err, "--app")
	require.NoDirExists(t, filepath.Join(profile, "modules", "nvim"), "a rejected run must not create a module")
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
		Packages:    []onboard.PackageEntry{{Name: "bash"}},
		Tools:       []string{"node=20"},
		Home:        home,
	})
	require.NoError(t, err)

	modToml := filepath.Join(profile, "modules", "bash", "module.toml")
	content, err := readFile(modToml)
	require.NoError(t, err)
	// id/app are not written: the module directory name already carries the
	// identity (profile.Load falls back to it), so declaring them is
	// redundant — hand-authored modules omit them and work fine.
	require.NotContains(t, content, `id = "bash"`)
	require.NotContains(t, content, `app = "bash"`)
	// Dotfiles render as inline tables, matching hand-authored modules.
	require.Contains(t, content, `"~/.bashrc" = { source = "home/.bashrc", mode = "copy" }`)
	// Packages (array) and tools (map).
	require.Contains(t, content, `"bash",`)
	require.Contains(t, content, `node = "20"`)
}

// Package entries may carry an inline description (name="desc") that
// renders as a trailing "# desc" comment next to the package, matching
// hand-authored modules like:  "bat", # Cat clone with syntax highlighting
func TestOnboard_packageDescriptionComments(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "x"))

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
		Packages: []onboard.PackageEntry{
			{Name: "bash"},
			{Name: "ripgrep", Description: "Search tool"},
		},
	})
	require.NoError(t, err)

	content, err := readFile(filepath.Join(profile, "modules", "bash", "module.toml"))
	require.NoError(t, err)
	require.Contains(t, content, "  \"bash\",\n")
	require.Contains(t, content, "  \"ripgrep\", # Search tool\n")
}

func TestParsePackages(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   []onboard.PackageEntry
	}{
		// kong splits "a,b" into ["a","b"] before ParsePackages sees it, so
		// each value here is already a single entry.
		{"two names", []string{"ripgrep", "bat"}, []onboard.PackageEntry{{Name: "ripgrep"}, {Name: "bat"}}},
		{"with description", []string{`bat="Cat clone"`}, []onboard.PackageEntry{{Name: "bat", Description: "Cat clone"}}},
		{"mixed", []string{"bat", `fd="Find files"`}, []onboard.PackageEntry{{Name: "bat"}, {Name: "fd", Description: "Find files"}}},
		{"aur prefix", []string{"aur/lmstudio-bin"}, []onboard.PackageEntry{{Name: "aur/lmstudio-bin"}}},
		// A trailing comma in the flag surfaces as an empty entry; it is dropped.
		{"empty entries dropped", []string{"a", "", "b"}, []onboard.PackageEntry{{Name: "a"}, {Name: "b"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := onboard.ParsePackages(tc.values)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParsePackages_errorsOnEmptyName(t *testing.T) {
	_, err := onboard.ParsePackages([]string{"=bad"})
	require.Error(t, err)
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

func TestOnboard_defaultModeSymlink(t *testing.T) {
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
	require.Contains(t, content, `mode = "symlink"`)
}

// Onboard takes over the live file it just copied into the module, so its
// dotfiles apply must run forced — real mise refuses to overwrite existing
// files otherwise (docker-e2e onboard failure: "refusing to overwrite
// existing files (use --force)").
func TestOnboard_dotfilesApplyForced(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	}))
	require.True(t, fr.DotfilesCalled)
	require.True(t, fr.Force, "onboard's dotfiles apply must pass force (it owns the content it just copied)")
}

// Re-onboarding the same path refreshes the module's copy from the live
// file instead of erroring — onboard snapshots live state, so updating is
// the default (there is no --force / conflict anymore).
func TestOnboard_updatesExistingFile(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "v1"))

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{src}, App: "bash", Home: home,
	}))

	require.NoError(t, writeFile(src, "v2"))
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{src}, App: "bash", Home: home,
	}), "re-onboarding an existing path must update, not fail")

	got, err := readFile(filepath.Join(profile, "modules", "bash", "home", ".bashrc"))
	require.NoError(t, err)
	require.Equal(t, "v2", got)
}

// Re-onboarding a directory refreshes its tree wholesale: removed files
// disappear, modified files update, new files appear.
func TestOnboard_updatesExistingDir(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	dir := filepath.Join(home, ".config", "app")
	require.NoError(t, writeFile(filepath.Join(dir, "old.conf"), "old"))
	require.NoError(t, writeFile(filepath.Join(dir, "keep.conf"), "v1"))

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{dir}, App: "app", Home: home,
	}))

	require.NoError(t, os.Remove(filepath.Join(dir, "old.conf")))
	require.NoError(t, writeFile(filepath.Join(dir, "keep.conf"), "v2"))
	require.NoError(t, writeFile(filepath.Join(dir, "new.conf"), "new"))
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{dir}, App: "app", Home: home,
	}))

	base := filepath.Join(profile, "modules", "app", "home", ".config", "app")
	require.NoFileExists(t, filepath.Join(base, "old.conf"))
	kept, err := readFile(filepath.Join(base, "keep.conf"))
	require.NoError(t, err)
	require.Equal(t, "v2", kept)
	require.FileExists(t, filepath.Join(base, "new.conf"))
}

// Re-onboarding a NEW path into an existing module merges into module.toml
// instead of overwriting it: the first path's entry and its packages survive.
func TestOnboard_mergeKeepsExistingEntries(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	require.NoError(t, writeFile(filepath.Join(home, ".bashrc"), "a"))
	require.NoError(t, writeFile(filepath.Join(home, ".vimrc"), "b"))

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{"~/.bashrc"},
		Packages: []onboard.PackageEntry{{Name: "bash"}}, App: "mix", Home: home,
	}))
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{"~/.vimrc"}, App: "mix", Home: home,
	}))

	content, err := readFile(filepath.Join(profile, "modules", "mix", "module.toml"))
	require.NoError(t, err)
	require.Contains(t, content, `"~/.bashrc"`)
	require.Contains(t, content, `"~/.vimrc"`)
	require.Contains(t, content, `"bash"`)
}

// Re-onboarding preserves sections onboard does not manage (scope, hooks):
// they pass through verbatim while [dotfiles] is merged.
func TestOnboard_mergePreservesOtherSections(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)
	require.NoError(t, writeFile(filepath.Join(home, ".bashrc"), "a"))

	modDir := filepath.Join(profile, "modules", "mix")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, writeFile(filepath.Join(modDir, "module.toml"),
		`scope = "system"`+"\n\n[hooks]\npre = [\"echo hi\"]\n"))

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{"~/.bashrc"}, App: "mix", Home: home,
	}))

	content, err := readFile(filepath.Join(modDir, "module.toml"))
	require.NoError(t, err)
	require.Contains(t, content, `scope = "system"`)
	require.Contains(t, content, `pre = ["echo hi"]`)
	require.Contains(t, content, `"~/.bashrc"`)
}

// Re-onboarding a module whose module.toml carries hand-written edit entries
// (line/block/template) must round-trip them verbatim — onboard decodes the
// existing [dotfiles] and re-encodes only the set fields, so an edit entry is
// preserved instead of being silently rewritten as { source = "", mode = "" }.
func TestOnboard_reonboardPreservesEditEntries(t *testing.T) {
	home := t.TempDir()
	profileRoot := t.TempDir()
	isolateState(t)
	require.NoError(t, writeFile(filepath.Join(home, ".bashrc"), "a"))

	modDir := filepath.Join(profileRoot, "modules", "mix")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, writeFile(filepath.Join(modDir, "module.toml"), `
[dotfiles]
"~/.zshrc/dev" = { line = "127.0.0.1 dev.local" }
"~/.zshrc/aliases" = { block = "alias ll='ls -l'", comment = "#" }
"~/.gitconfig/id" = { source = "git.tmpl", template = "tera" }
`))

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profileRoot, Paths: []string{"~/.bashrc"}, App: "mix", Home: home,
	}))

	content, err := readFile(filepath.Join(modDir, "module.toml"))
	require.NoError(t, err)
	// The three edit entries survive verbatim (fields re-encoded in field order).
	require.Contains(t, content, `"~/.zshrc/dev" = { line = "127.0.0.1 dev.local" }`)
	require.Contains(t, content, `"~/.zshrc/aliases" = { block = "alias ll='ls -l'", comment = "#" }`)
	require.Contains(t, content, `"~/.gitconfig/id" = { source = "git.tmpl", template = "tera" }`)
	// The newly onboarded whole-file entry is present too.
	require.Contains(t, content, `"~/.bashrc" = { source = "home/.bashrc", mode = "symlink" }`)
	// No entry was collapsed to empty source/mode.
	require.NotContains(t, content, `{ source = "", mode = "" }`)
}

// Adding a path without re-declaring packages keeps existing package
// description comments: the [packages] section is left untouched.
func TestOnboard_mergePreservesPackageDescriptions(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)
	require.NoError(t, writeFile(filepath.Join(home, ".bashrc"), "a"))
	require.NoError(t, writeFile(filepath.Join(home, ".vimrc"), "b"))

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{"~/.bashrc"},
		Packages: []onboard.PackageEntry{{Name: "bat", Description: "Cat clone"}}, App: "mix", Home: home,
	}))
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{"~/.vimrc"}, App: "mix", Home: home,
	}))

	content, err := readFile(filepath.Join(profile, "modules", "mix", "module.toml"))
	require.NoError(t, err)
	require.Contains(t, content, `"bat", # Cat clone`)
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

func (r *recordingRunner) DotfilesApply(ctx context.Context, configPath string, yes, force bool) error {
	r.ctx = ctx
	r.calls = append(r.calls, "dotfiles")
	r.configPath = configPath
	r.yes = yes
	return nil
}

func (r *recordingRunner) Bootstrap(ctx context.Context, configPath string, yes bool, only ...string) error {
	r.calls = append(r.calls, "bootstrap")
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

func (v *validatingRunner) DotfilesApply(ctx context.Context, configPath string, yes, force bool) error {
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

func (v *validatingRunner) Bootstrap(ctx context.Context, configPath string, yes bool, only ...string) error {
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

// --- Orphan adoption (issue 0015) ---
//
// Orphaned files in the onboarded module dir (unreferenced by any
// [dotfiles] entry, module.toml excluded) are adopted: each invertible
// orphan (home/<rel> -> ~/<rel>, system/<rel> -> /<rel>) gains a
// [dotfiles] entry with the run's mode. A fully-orphaned directory
// collapses to ONE whole-dir entry. The live target, when present, is
// snapshotted over the module source first (onboard snapshots live
// state), keeping the forced takeover apply lossless.

// mkModule pre-creates a module directory (dir, absolute or
// profile-relative) with files and returns its path.
func mkModule(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return dir
}

func modDir(profile, app string) string {
	return filepath.Join(profile, "modules", app)
}

func TestOnboard_adoptsOrphans(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	// Live counterpart of one orphan: live content must win over the
	// stale module copy before apply.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config"), 0o755))
	require.NoError(t, writeFile(filepath.Join(home, ".config", "stray.toml"), "live fresh"))

	live := filepath.Join(home, ".config", "app", "keep.toml")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "app"), 0o755))
	require.NoError(t, writeFile(live, "keep"))
	mkModule(t, modDir(profile, "app"), map[string]string{
		// module.toml declares nothing yet (empty file).
		"module.toml": "",
		// Orphan file with a live counterpart (stale copy).
		"home/.config/stray.toml": "stale copy",
		// Fully-orphaned dir: collapses to ONE entry.
		"home/.config/legacy/a.conf":   "a",
		"home/.config/legacy/sub/b.sh": "b",
		// Module-root junk: no derivable target, never adopted.
		"notes.md": "root junk",
	})

	var out bytes.Buffer
	o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &out}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{live}, App: "app", Home: home,
	}))

	content, err := readFile(filepath.Join(profile, "modules", "app", "module.toml"))
	require.NoError(t, err)
	t.Log(content)
	require.Contains(t, content,
		`"~/.config/app/keep.toml" = { source = "home/.config/app/keep.toml", mode = "symlink" }`)
	require.Contains(t, content,
		`"~/.config/stray.toml" = { source = "home/.config/stray.toml", mode = "symlink" }`)
	require.Contains(t, content,
		`"~/.config/legacy" = { source = "home/.config/legacy", mode = "symlink" }`,
		"a fully-orphaned dir collapses to one whole-dir entry")
	require.NotContains(t, content, "legacy/a.conf", "no per-file entries for the collapsed dir")
	require.NotContains(t, content, "notes.md", "module-root junk has no derivable target")

	// Live snapshot: the module copy was refreshed from the live file.
	snapshotted, err := readFile(filepath.Join(profile, "modules", "app", "home", ".config", "stray.toml"))
	require.NoError(t, err)
	require.Equal(t, "live fresh", snapshotted, "live content wins over the stale orphan copy")

	require.Contains(t, out.String(), "adopted: ~/.config/stray.toml (home/.config/stray.toml) [base]")
	require.Contains(t, out.String(), "adopted: ~/.config/legacy (home/.config/legacy) [base]")
}

// Files under a directory source are already deployed by that entry
// (issue 0014): adoption must not explode them into file entries.
func TestOnboard_adoptionSkipsReferencedSubtrees(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	live := filepath.Join(home, ".config", "app", "new.toml")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "app"), 0o755))
	require.NoError(t, writeFile(live, "new"))
	mkModule(t, modDir(profile, "app"), map[string]string{
		"module.toml": `[dotfiles]
"~/.config/app" = { source = "home/.config/app", mode = "symlink" }
`,
		"home/.config/app/existing.toml": "deployed via the dir entry",
		"home/.config/app/nested/x.conf": "deployed too",
	})

	var out bytes.Buffer
	o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &out}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{live}, App: "app", Home: home,
	}))

	content, err := readFile(filepath.Join(profile, "modules", "app", "module.toml"))
	require.NoError(t, err)
	t.Log(content)
	require.Contains(t, content, `"~/.config/app" = { source = "home/.config/app", mode = "symlink" }`)
	require.NotContains(t, content, "existing.toml\" =", "dir-source subtree is not re-adopted per file")
	require.NotContains(t, content, "nested", "dir-source subtree is not re-adopted per file")
	require.NotContains(t, out.String(), "adopted:")
}

// A module-level backups/ tree (apply --backup runtime output, issue 0025)
// is not adoptable content: it does not live under home/ or system/, so no
// target inverts from it and nothing is adopted or snapshotted.
func TestOnboard_adoptionSkipsBackupsDir(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	live := filepath.Join(home, ".config", "app", "keep.toml")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "app"), 0o755))
	require.NoError(t, writeFile(live, "keep"))
	mkModule(t, modDir(profile, "app"), map[string]string{
		"module.toml": "",
		// dotdrift-written backup generation mirroring absolute paths.
		"backups/20260824-120000/home/cri/.config/app/keep.toml": "old live copy",
		"backups/20260824-120000/etc/app.conf":                   "old system copy",
	})

	var out bytes.Buffer
	o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &out}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{live}, App: "app", Home: home,
	}))

	content, err := readFile(filepath.Join(profile, "modules", "app", "module.toml"))
	require.NoError(t, err)
	t.Log(content)
	require.Contains(t, content, `"~/.config/app/keep.toml" = { source = "home/.config/app/keep.toml", mode = "symlink" }`)
	require.NotContains(t, content, "backups", "backup generations are never adopted")
	require.NotContains(t, out.String(), "backups", "no adoption notices name backup paths")
	// The backup files are untouched.
	kept, err := readFile(filepath.Join(profile, "modules", "app", "backups", "20260824-120000", "etc", "app.conf"))
	require.NoError(t, err)
	require.Equal(t, "old system copy", kept)
}

// Re-onboarding an entry whose source path changed must not strand the
// old source as a new orphan: it is adopted under its own target.
func TestOnboard_reOnboardChangedSourceAdoptsOld(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	liveDir := filepath.Join(home, ".config", "app")
	require.NoError(t, os.MkdirAll(liveDir, 0o755))
	require.NoError(t, writeFile(filepath.Join(liveDir, "config.toml"), "cfg"))
	mkModule(t, modDir(profile, "app"), map[string]string{
		"module.toml": `[dotfiles]
"~/.config/app" = { source = "home/custom", mode = "symlink" }
`,
		"home/custom": "old source",
	})

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &bytes.Buffer{}}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{liveDir}, App: "app", Home: home,
	}))

	content, err := readFile(filepath.Join(profile, "modules", "app", "module.toml"))
	require.NoError(t, err)
	t.Log(content)
	require.Contains(t, content, `"~/.config/app" = { source = "home/.config/app", mode = "symlink" }`)
	require.Contains(t, content, `"~/custom" = { source = "home/custom", mode = "symlink" }`,
		"the stranded old source is adopted under its own target")
}

// An orphan whose derived target is already claimed by an existing entry
// is skipped, never double-claimed (entries are keyed by target).
func TestOnboard_adoptionSkipsClaimedTargets(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	live := filepath.Join(home, ".config", "app", "new.toml")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "app"), 0o755))
	require.NoError(t, writeFile(live, "new"))
	mkModule(t, modDir(profile, "app"), map[string]string{
		"module.toml": `[dotfiles]
"~/.config/dup" = { source = "home/claimed", mode = "symlink" }
`,
		"home/claimed":     "the referenced source",
		"home/.config/dup": "orphan deriving an already-claimed target",
	})

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &bytes.Buffer{}}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{live}, App: "app", Home: home,
	}))

	content, err := readFile(filepath.Join(profile, "modules", "app", "module.toml"))
	require.NoError(t, err)
	t.Log(content)
	require.Contains(t, content, `"~/.config/dup" = { source = "home/claimed"`,
		"the existing entry for the claimed target is untouched")
}

// A live target that is a symlink INTO the module (previously deployed)
// must not trick the snapshot into copying the source onto itself.
func TestOnboard_adoptionSelfCopyGuard(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	mod := mkModule(t, modDir(profile, "app"), map[string]string{
		"module.toml":       "",
		"home/.config/orph": "precious",
	})
	// Live target symlinks at the module source.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(mod, "home", ".config", "orph"),
		filepath.Join(home, ".config", "orph")))

	live := filepath.Join(home, ".config", "app", "keep.toml")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "app"), 0o755))
	require.NoError(t, writeFile(live, "keep"))

	o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &bytes.Buffer{}}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{live}, App: "app", Home: home,
	}))

	content, err := readFile(filepath.Join(mod, "home", ".config", "orph"))
	require.NoError(t, err)
	require.Equal(t, "precious", content, "self-copy through the live symlink must not wipe the source")
}

// --dry-run lists the would-be adoptions and touches nothing.
func TestOnboard_dryRunListsAdoptions(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	live := filepath.Join(home, ".config", "app", "keep.toml")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "app"), 0o755))
	require.NoError(t, writeFile(live, "keep"))
	mkModule(t, modDir(profile, "app"), map[string]string{
		"module.toml":                   "",
		"home/.config/legacy/orph.conf": "orphan until adopted",
	})

	var out bytes.Buffer
	o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &out}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{live}, App: "app", Home: home, DryRun: true,
	}))

	require.Contains(t, out.String(), "would adopt: ~/.config/legacy (home/.config/legacy) [base]")
	require.NotContains(t, out.String(), "would adopt: ~ (home)",
		"the whole-home unit is narrowed: it would nest under this run's target")
	// Dry-run writes nothing: module.toml stays empty, the orphan stays.
	c, err := readFile(filepath.Join(profile, "modules", "app", "module.toml"))
	require.NoError(t, err)
	require.Equal(t, "", c, "dry-run writes no adoption entries")
	kept, err := readFile(filepath.Join(profile, "modules", "app", "home", ".config", "legacy", "orph.conf"))
	require.NoError(t, err)
	require.Equal(t, "orphan until adopted", kept)
}

// A path INSIDE a module layer directory of the profile is rejected, not
// adopted (issue 0056, amending 0017): onboard brings EXTERNAL live paths
// into the profile. A module file is already inside — registering it is a
// hand-edit of that module.toml (or it rides the orphan sweep of any
// live-path onboard). The error names the module and layer; nothing is
// written; the --app value is never silently overridden.
func TestOnboard_profileInternalPathRejected(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	base := mkModule(t, filepath.Join(profile, "modules", "easyeffects"), map[string]string{
		"module.toml": `[dotfiles]
"~/.config/easyeffects" = { source = "home/.config/easyeffects", mode = "symlink-each" }
`,
		"home/.config/easyeffects/db/deesserrc": "base preset",
	})
	hostMod := mkModule(t, filepath.Join(profile, "hosts", "cri-pc", "modules", "easyeffects"), map[string]string{
		"module.toml": "",
		"home/.config/easyeffects/db/easyeffectsrc": "orphan preset",
	})
	userMod := mkModule(t, filepath.Join(profile, "users", "cri", "modules", "easyeffects"), map[string]string{
		"module.toml":                        "",
		"home/.config/easyeffects/db/userrc": "user-layer orphan",
	})

	reject := func(path string, wantLayer string) {
		t.Helper()
		o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &bytes.Buffer{}}
		err := o.Run(onboard.Options{
			ProfileRoot: profile, Paths: []string{path},
			App: "ignored-value", Home: home, Hostname: "cri-pc", Username: "cri",
		})
		require.ErrorContains(t, err, `already inside module "easyeffects"`)
		require.ErrorContains(t, err, wantLayer)
		require.ErrorContains(t, err, "onboard adopts live paths only")
	}

	reject(filepath.Join(hostMod, "home", ".config", "easyeffects", "db", "easyeffectsrc"), "hosts/cri-pc")
	reject(filepath.Join(base, "home", ".config", "easyeffects", "db", "deesserrc"), "base")
	reject(filepath.Join(userMod, "home", ".config", "easyeffects", "db", "userrc"), "users/cri")
	// A nonexistent module file is rejected by shape, before any stat.
	reject(filepath.Join(hostMod, "home", ".config", "gone.conf"), "hosts/cri-pc")

	// Nothing was written: module.tomls and file contents untouched.
	c, err := readFile(filepath.Join(hostMod, "module.toml"))
	require.NoError(t, err)
	require.Equal(t, "", c)
	kept, err := readFile(filepath.Join(hostMod, "home", ".config", "easyeffects", "db", "easyeffectsrc"))
	require.NoError(t, err)
	require.Equal(t, "orphan preset", kept)
}

// The ancestor chain never claims a shared namespace root: with no
// declarations bounding it, a deep orphan adopts its own subdirectory,
// never ~/.config (the "would adopt: ~/.config" field report).
func TestOnboard_adoptionNeverClaimsSharedRoots(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	isolateState(t)

	sys := filepath.Join(t.TempDir(), "etc-thing") // system target: no nesting bound
	require.NoError(t, writeFile(sys, "sys"))
	mkModule(t, modDir(profile, "app"), map[string]string{
		"module.toml":               "",
		"home/.config/foo/bar.conf": "orphan",
	})

	var out bytes.Buffer
	o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &out}
	require.NoError(t, o.Run(onboard.Options{
		ProfileRoot: profile, Paths: []string{sys}, App: "app", Home: home,
	}))

	content, err := readFile(filepath.Join(modDir(profile, "app"), "module.toml"))
	require.NoError(t, err)
	t.Log(content)
	require.Contains(t, content, `"~/.config/foo" = { source = "home/.config/foo", mode = "symlink" }`,
		"the orphan's own subdirectory is the adoption unit")
	require.NotContains(t, content, `"~/.config" =`, "never the shared ~/.config root")
	require.NotContains(t, content, `"~" =`, "never the whole home")
	require.Contains(t, out.String(), "adopted: ~/.config/foo (home/.config/foo) [base]")
}

// --user targets the users/<username> overlay like --host targets
// hosts/<hostname>; --host --user together onboard into BOTH layers.
func TestOnboard_userAndHostOverlays(t *testing.T) {
	newRun := func() (string, string) {
		home := t.TempDir()
		profile := t.TempDir()
		isolateState(t)
		require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "app"), 0o755))
		require.NoError(t, writeFile(filepath.Join(home, ".config", "app", "c.toml"), "c"))
		return home, profile
	}
	entry := `"~/.config/app/c.toml" = { source = "home/.config/app/c.toml", mode = "symlink" }`

	t.Run("user only", func(t *testing.T) {
		home, profile := newRun()
		var out bytes.Buffer
		o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &out}
		require.NoError(t, o.Run(onboard.Options{
			ProfileRoot: profile, Paths: []string{filepath.Join(home, ".config", "app", "c.toml")},
			App: "app", Home: home, User: true, Username: "cri",
		}))
		modToml := filepath.Join(profile, "users", "cri", "modules", "app", "module.toml")
		c, err := readFile(modToml)
		require.NoError(t, err)
		require.Contains(t, c, entry)
		require.NoDirExists(t, filepath.Join(profile, "modules", "app"),
			"--user does not touch the base layer")
		require.NoDirExists(t, filepath.Join(profile, "hosts", "cri-pc", "modules", "app"),
			"--user does not touch host layers")
	})

	t.Run("host and user together", func(t *testing.T) {
		home, profile := newRun()
		var out bytes.Buffer
		o := &onboard.Onboard{Mise: &mise.FakeRunner{}, Out: &out}
		require.NoError(t, o.Run(onboard.Options{
			ProfileRoot: profile, Paths: []string{filepath.Join(home, ".config", "app", "c.toml")},
			App: "app", Home: home, Host: true, User: true, Hostname: "cri-pc", Username: "cri",
		}))
		for _, mod := range []string{
			filepath.Join(profile, "hosts", "cri-pc", "modules", "app"),
			filepath.Join(profile, "users", "cri", "modules", "app"),
		} {
			c, err := readFile(filepath.Join(mod, "module.toml"))
			require.NoError(t, err)
			require.Contains(t, c, entry, "both overlays declare the entry")
			require.FileExists(t, filepath.Join(mod, "home", ".config", "app", "c.toml"),
				"both overlays hold the copied content")
		}
		require.NoDirExists(t, filepath.Join(profile, "modules", "app"),
			"host+user does not touch the base layer")
	})

	t.Run("user without a username errors", func(t *testing.T) {
		home, profile := newRun()
		o := &onboard.Onboard{Mise: &mise.FakeRunner{}}
		err := o.Run(onboard.Options{
			ProfileRoot: profile, Paths: []string{filepath.Join(home, ".config", "app", "c.toml")},
			App: "app", Home: home, User: true,
		})
		require.ErrorContains(t, err, "username required")
	})
}
