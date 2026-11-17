package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// White-box unit tests for the systemFilesStep internals, moved here from
// cmd/apply_scope_test.go by issue 0070 (the step types now live in this
// package — 0064-D6 absorbed the steps into the session, 0070 deleted the
// cmd copy these tests targeted).

// Whole-file system entries converge via [dotfiles] with the declared
// mode preserved (issue 0156): a non-writable target elevates the batch
// through the handover seam, exactly like system edits — dotdrift runs
// the sudo pass, mise does not self-elevate a [bootstrap.files] batch.
func TestSystemFilesStep_wholeFilesElevatedViaDotfilesApply(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("elevation test requires a non-root user")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")
	editsPath := filepath.Join(dir, "system-edits", "mise.toml")

	var names []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			names = append(names, name)
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	var handed []*exec.Cmd
	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: "/etc/test.conf", Source: "test.conf", Mode: "copy"},
		},
		homeDir:    "/home/test",
		configPath: configPath,
		editsPath:  editsPath,
		yes:        true,
		handover: func(cmd *exec.Cmd) error {
			handed = append(handed, cmd)
			return nil
		},
	}

	require.NoError(t, step.Run(context.Background()))
	require.Empty(t, names, "the parent (/etc) exists, so no directories pass runs on the piped runner")
	require.Len(t, handed, 1, "a non-writable whole-file target elevates in one sudo pass")
	require.True(t, strings.HasSuffix(handed[0].Path, "/sudo"))
	require.Contains(t, handed[0].Args, "dotfiles")
	require.Contains(t, handed[0].Args, "apply")

	cfg, err := os.ReadFile(editsPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[dotfiles]")
	require.Contains(t, string(cfg), `"/etc/test.conf" = { source = "test.conf", mode = "copy" }`)
}

// A dotfiles apply failure propagates verbatim — no dotdrift-side retry
// or elevation second-guessing.
func TestSystemFilesStep_dotfilesFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")

	var names []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			names = append(names, name)
			return "", fmt.Errorf("exit status 1\nmise ERROR config parse error")
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: filepath.Join(dir, "mine.conf"), Source: "mine.conf", Mode: "copy"},
		},
		homeDir:    "/home/test",
		configPath: configPath,
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	err := step.Run(context.Background())
	require.Error(t, err)
	require.NotContains(t, names, "sudo")
	require.Contains(t, err.Error(), "config parse error")
}

// System-scope EDIT entries keep the elevated [dotfiles] path: a
// non-writable edit target converges via one "sudo -E mise dotfiles
// apply" child handed to the consumer's terminal (contract #18 —
// bootstrap.files has no edit concept; 0071 handover seam).
func TestSystemFilesStep_editEntriesElevatedWhenNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("elevation test requires a non-root user")
	}
	dir := t.TempDir()

	var names []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			names = append(names, name)
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	editsPath := filepath.Join(dir, "system-edits", "mise.toml")
	var handed []*exec.Cmd
	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: "/etc/hosts/dev", Line: "127.0.0.1 dev.local"},
		},
		homeDir:    "/home/test",
		configPath: filepath.Join(dir, "system", "mise.toml"),
		editsPath:  editsPath,
		yes:        true,
		handover: func(cmd *exec.Cmd) error {
			handed = append(handed, cmd)
			return nil
		},
	}

	require.NoError(t, step.Run(context.Background()))
	require.Len(t, handed, 1, "a non-writable edit target converges elevated in one sudo pass")
	require.True(t, strings.HasSuffix(handed[0].Path, "/sudo"))
	require.Contains(t, handed[0].Args, "dotfiles")
	require.Contains(t, handed[0].Args, "apply")
	require.Contains(t, handed[0].Args, editsPath[0:len(editsPath)-len("/mise.toml")],
		"the elevated child applies the system-edits config dir")
	require.Empty(t, names, "the edit parent (/etc) already exists, so no directories bootstrap pass runs (issue 0154); the elevated apply does not use the piped runner")

	cfg, err := os.ReadFile(editsPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[dotfiles]")
	require.Contains(t, string(cfg), `line = "127.0.0.1 dev.local"`)

	// The edit file's parent (/etc) already exists, so it is not declared
	// as a [bootstrap.directories] entry and no system config is written
	// at all (issue 0154); a missing parent would ride a bootstrap pass
	// before the elevated edit apply (TestSystemFilesStep_createsParentDirectories).
	require.NoFileExists(t, filepath.Join(dir, "system", "mise.toml"))
}

// Every system dotfile operation creates its target's parent directory
// (issue 0151): the parent of every whole-file target (the target
// directory itself for symlink-each) and of every edit file is emitted
// as a [bootstrap.directories] entry in the same config, so mise creates
// it in the same bootstrap run, before the files and edits land.
func TestSystemFilesStep_createsParentDirectories(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "profile")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "files", "each"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "files", "each", "a.txt"), []byte("a"), 0o644))
	configPath := filepath.Join(dir, "system", "mise.toml")

	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + string(rune(10)), nil
			}
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: filepath.Join(dir, "sys", "deep", "copy.conf"), Source: "files/copy.conf", Mode: "copy"},
			{Target: filepath.Join(dir, "sys", "link.conf"), Source: "files/link.conf", Mode: "symlink"},
			{Target: filepath.Join(dir, "sys", "tmpl", "t.conf"), Source: "files/t.conf", Mode: "template"},
			{Target: filepath.Join(dir, "sys", "eachdir"), Source: "files/each", Mode: "symlink-each"},
			{Target: filepath.Join(dir, "sys", "edit", "e.conf", "myline"), Line: "x"},
		},
		homeDir:    dir,
		configPath: configPath,
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	cfg, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.directories]")
	for _, parent := range []string{
		filepath.Join(dir, "sys", "deep"),
		filepath.Join(dir, "sys"),
		filepath.Join(dir, "sys", "tmpl"),
		filepath.Join(dir, "sys", "eachdir"),
		filepath.Join(dir, "sys", "edit"),
	} {
		require.Contains(t, string(cfg), fmt.Sprintf(`%q = { state = "present" }`, parent),
			"parent %s must be a bootstrap directory", parent)
	}
}

// Mount destination directories are emitted as [bootstrap.directories] in the
// same system config and converge in the same bootstrap --only files call —
// no dotdrift-side mkdir/ensureDir (issue 0042).
// A dotfile parent that already exists is NOT declared as a
// [bootstrap.directories] entry (issue 0154, regression from 0151):
// declaring it makes mise plan the files under it as user-level changes,
// so a copy into /root runs as the current user and fails with Permission
// denied instead of escalating in mise's privileged batch. A missing
// parent is still declared, so mise creates it in the same run.
func TestSystemFilesStep_existingParentNotDeclared(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing")
	require.NoError(t, os.MkdirAll(existing, 0o755))
	missing := filepath.Join(dir, "missing", "deep")
	configPath := filepath.Join(dir, "system", "mise.toml")

	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + string(rune(10)), nil
			}
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: filepath.Join(existing, "a.conf"), Source: "a.conf", Mode: "copy"},
			{Target: filepath.Join(missing, "b.conf"), Source: "b.conf", Mode: "copy"},
		},
		homeDir:    dir,
		configPath: configPath,
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	cfg, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.directories]")
	require.NotContains(t, string(cfg), fmt.Sprintf("%q = { state = \"present\" }", existing),
		"an existing parent must not be declared: it suppresses mise's privilege escalation for the files under it")
	require.Contains(t, string(cfg), fmt.Sprintf("%q = { state = \"present\" }", missing),
		"a missing parent must still be declared so mise creates it")
}

// Chained system entries converge in one apply pass (issue 0158):
// an entry whose source is another entry's target declares depends on
// that target, and mise writes the target before it reads the source.
// Here .bashrc links to the .profile that the first entry creates; the
// symlink mode survives into the applied config.
func TestSystemFilesStep_chainedEntriesOnePassWithDepends(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")
	editsPath := filepath.Join(dir, "system-edits", "mise.toml")
	profileTarget := filepath.Join(dir, "root", ".profile")
	bashrcTarget := filepath.Join(dir, "root", ".bashrc")

	var configs []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + string(rune(10)), nil
			}
			if len(args) > 0 && args[0] == "dotfiles" {
				cfg, err := os.ReadFile(editsPath)
				if err != nil {
					return "", err
				}
				configs = append(configs, string(cfg))
			}
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: bashrcTarget, Source: profileTarget, Mode: "symlink"},
			{Target: profileTarget, Source: "home/.profile", Mode: "copy"},
		},
		homeDir:    dir,
		configPath: configPath,
		editsPath:  editsPath,
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	require.Len(t, configs, 1, "one dotfiles apply pass for the whole chain")
	require.Contains(t, configs[0], fmt.Sprintf("%q = { source", profileTarget))
	require.Contains(t, configs[0], fmt.Sprintf("%q = { source", bashrcTarget))
	require.Contains(t, configs[0], "mode = \"symlink\"", "the chained symlink keeps its mode")
	require.Contains(t, configs[0], "depends = [\""+profileTarget+"\"]", "the chained entry depends on its source target")
}

// A source-dependency cycle between system entries is an error naming
// the targets, not a mise failure.
func TestSystemFilesStep_chainedCycleErrors(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + string(rune(10)), nil
			}
			return "", nil
		},
	}
	step := &systemFilesStep{
		exec: mise.NewExecMise(m),
		entries: []resolve.DotfileEntry{
			{Target: a, Source: b, Mode: "copy"},
			{Target: b, Source: a, Mode: "copy"},
		},
		homeDir:    dir,
		configPath: filepath.Join(dir, "system", "mise.toml"),
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	err := step.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "cycle")
	require.Contains(t, err.Error(), a)
	require.Contains(t, err.Error(), b)
}

func TestSystemFilesStep_mountDirsViaBootstrapDirectories(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")

	var names []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			names = append(names, name)
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec:       em,
		homeDir:    "/home/test",
		dirs:       []string{"/mnt/data", "/mnt/backup"},
		configPath: configPath,
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, []string{"/fake/mise"}, names)

	cfg, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.directories]")
	require.Contains(t, string(cfg), "/mnt/data")
	require.Contains(t, string(cfg), "/mnt/backup")
}

// A system entry declared as symlink reaches the applied config as
// mode = "symlink" with its declared source (issue 0156): nothing
// translates it to a content copy.
func TestSystemFilesStep_symlinkModePreserved(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")
	editsPath := filepath.Join(dir, "system-edits", "mise.toml")

	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: filepath.Join(dir, "symlinked.conf"), Source: "/fake/profile/files/symlinked.conf", Mode: "symlink"},
		},
		homeDir:    "/home/test",
		configPath: configPath,
		editsPath:  editsPath,
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	cfg, err := os.ReadFile(editsPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[dotfiles]")
	require.Contains(t, string(cfg), "mode = \"symlink\"")
	require.Contains(t, string(cfg), "source = \"/fake/profile/files/symlinked.conf\"")
}

// Declared secrets are emitted as [bootstrap.secrets] into the system
// dotfiles config (issues 0044, 0156): current mise reads them in
// [dotfiles] templates too.
func TestSystemFilesStep_includesSecrets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")
	editsPath := filepath.Join(dir, "system-edits", "mise.toml")

	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: filepath.Join(dir, "svc.env"), Source: "svc.env", Mode: "template"},
		},
		homeDir:    "/home/test",
		configPath: configPath,
		editsPath:  editsPath,
		yes:        true,
		secrets: map[string]profile.Secret{
			"cache_token": {Env: "MISE_CACHE_TOKEN"},
		},
	}

	require.NoError(t, step.Run(context.Background()))
	cfg, err := os.ReadFile(editsPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.secrets]")
	require.Contains(t, string(cfg), `cache_token = "MISE_CACHE_TOKEN"`)
	require.Contains(t, string(cfg), "mode = \"template\"")
}

// No declared secrets → no [bootstrap.secrets] section.
func TestSystemFilesStep_noSecretsNoSection(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")

	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	editsPath := filepath.Join(dir, "system-edits", "mise.toml")
	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: filepath.Join(dir, "test.conf"), Source: "test.conf", Mode: "copy"},
		},
		homeDir:    "/home/test",
		configPath: configPath,
		editsPath:  editsPath,
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	cfg, err := os.ReadFile(editsPath)
	require.NoError(t, err)
	require.NotContains(t, string(cfg), "[bootstrap.secrets]")
}
