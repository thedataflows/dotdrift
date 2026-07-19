package mise_test

import (
	"context"
	"errors"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

var testFacts = &facts.Facts{
	Hostname: "myhost",
	Username: "cri",
	OS:       "linux",
	Backend:  "paru",
}

type hookTask struct {
	Run []string          `toml:"run"`
	Dir string            `toml:"dir"`
	Env map[string]string `toml:"env"`
}

func decodeHookTasks(t *testing.T, cfg string) map[string]hookTask {
	t.Helper()
	var decoded struct {
		Tasks map[string]hookTask `toml:"tasks"`
	}
	_, err := toml.Decode(cfg, &decoded)
	require.NoError(t, err, "generated TOML must be parseable: %q", cfg)
	return decoded.Tasks
}

// GenerateHookTasks emits one mise task per non-empty hook list, with the
// command list as `run`, dir set to the absolute profile root, and the
// DOTDRIFT_* facts environment.
func TestGenerateHookTasks_preAndPost(t *testing.T) {
	hooks := resolve.HooksStep{
		Pre:  []string{"echo base-pre", "echo host-pre"},
		Post: []string{"echo base-post"},
	}
	out := mise.GenerateHookTasks(hooks, "/profiles/main", testFacts)

	require.Contains(t, out, `[tasks."hooks:pre"]`)
	require.Contains(t, out, `[tasks."hooks:post"]`)

	tasks := decodeHookTasks(t, out)
	pre, ok := tasks["hooks:pre"]
	require.True(t, ok, "hooks:pre task must exist")
	require.Equal(t, []string{"echo base-pre", "echo host-pre"}, pre.Run)
	require.Equal(t, "/profiles/main", pre.Dir)
	require.Equal(t, map[string]string{
		"DOTDRIFT_PROFILE":  "/profiles/main",
		"DOTDRIFT_HOSTNAME": "myhost",
		"DOTDRIFT_USERNAME": "cri",
		"DOTDRIFT_OS":       "linux",
		"DOTDRIFT_BACKEND":  "paru",
	}, pre.Env)

	post, ok := tasks["hooks:post"]
	require.True(t, ok, "hooks:post task must exist")
	require.Equal(t, []string{"echo base-post"}, post.Run)
	require.Equal(t, "/profiles/main", post.Dir)
	require.Equal(t, pre.Env, post.Env)
}

// No hook commands → no [tasks] section at all.
func TestGenerateHookTasks_empty(t *testing.T) {
	require.Empty(t, mise.GenerateHookTasks(resolve.HooksStep{}, "/profiles/main", testFacts))
}

// Only the non-empty side is emitted.
func TestGenerateHookTasks_preOnly(t *testing.T) {
	out := mise.GenerateHookTasks(resolve.HooksStep{Pre: []string{"echo hi"}}, "/profiles/main", testFacts)
	tasks := decodeHookTasks(t, out)
	require.Contains(t, tasks, "hooks:pre")
	require.NotContains(t, tasks, "hooks:post")
}

// Shell metacharacters in commands must survive the TOML round-trip.
func TestGenerateHookTasks_escapesCommands(t *testing.T) {
	hooks := resolve.HooksStep{Pre: []string{`echo "a b" && sed -i 's\x\y\g' f`}}
	out := mise.GenerateHookTasks(hooks, "/profiles/main", testFacts)
	tasks := decodeHookTasks(t, out)
	require.Equal(t, hooks.Pre, tasks["hooks:pre"].Run)
}

// The apply-time config composes tools, dotfiles, and hook tasks.
func TestGenerateApplyConfig_includesToolsDotfilesAndTasks(t *testing.T) {
	plan := &resolve.Plan{
		Tools: resolve.ToolsStep{Versions: map[string]string{"node": "22"}},
		Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
			{Target: "~/.bashrc", Source: "/src/.bashrc", Mode: "link"},
		}},
		Hooks: resolve.HooksStep{Pre: []string{"echo pre"}, Post: []string{"echo post"}},
	}
	out := mise.GenerateApplyConfig(plan, "/profiles/main", testFacts)

	require.Contains(t, out, "[tools]")
	require.Contains(t, out, "[dotfiles]")
	require.Contains(t, out, `[tasks."hooks:pre"]`)
	require.Contains(t, out, `[tasks."hooks:post"]`)

	tasks := decodeHookTasks(t, out)
	require.Equal(t, []string{"echo pre"}, tasks["hooks:pre"].Run)
	require.Equal(t, []string{"echo post"}, tasks["hooks:post"].Run)
}

// A plan without hooks keeps the apply config task-free.
func TestGenerateApplyConfig_noHooks(t *testing.T) {
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "22"}}}
	out := mise.GenerateApplyConfig(plan, "/profiles/main", testFacts)
	require.Contains(t, out, "[tools]")
	require.NotContains(t, out, "[tasks]")
}

// recordingRunMise fakes a mise binary and records every runner invocation.
func recordingRunMise(calls *[][]string, runErr error) *mise.Mise {
	return &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, _ string, args ...string) (string, error) {
			*calls = append(*calls, append([]string{}, args...))
			for _, a := range args {
				if a == "--version" {
					return mise.MinMiseVersion + "\n", nil
				}
			}
			return "", runErr
		},
	}
}

// HooksStep runs its mise task against the generated apply config.
func TestHooksStep_runsTask(t *testing.T) {
	var calls [][]string
	exec := mise.NewExecMise(recordingRunMise(&calls, nil))
	step := &mise.HooksStep{
		Exec:       exec,
		Commands:   []string{"echo pre"},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks:pre",
		StepName:   "hooks-pre",
	}

	require.Equal(t, "hooks-pre", step.Name())
	require.NoError(t, step.Run(context.Background()))
	require.Contains(t, calls, []string{"run", "--cd", "/state/mise", "hooks:pre"})
}

// The pipeline ctx reaches the mise runner.
func TestHooksStep_ctxPropagates(t *testing.T) {
	type ctxKey struct{}
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(ctx context.Context, _ string, args ...string) (string, error) {
			if v := ctx.Value(ctxKey{}); v != "marker" {
				return "", errors.New("ctx value missing")
			}
			for _, a := range args {
				if a == "--version" {
					return mise.MinMiseVersion + "\n", nil
				}
			}
			return "", nil
		},
	}
	step := &mise.HooksStep{
		Exec:       mise.NewExecMise(m),
		Commands:   []string{"echo pre"},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks:pre",
		StepName:   "hooks-pre",
	}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	require.NoError(t, step.Run(ctx))
}

// An empty command list never touches the mise runner (construction in
// cmd/apply.go skips empty hooks; Run no-ops as a second line of defense).
func TestHooksStep_emptyCommandsSkipsRunner(t *testing.T) {
	var calls [][]string
	exec := mise.NewExecMise(recordingRunMise(&calls, nil))
	step := &mise.HooksStep{
		Exec:       exec,
		Commands:   nil,
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks:post",
		StepName:   "hooks-post",
	}
	require.NoError(t, step.Run(context.Background()))
	require.Empty(t, calls, "empty hooks must not invoke mise")
}

// A mise task failure fails the step so resume re-runs it.
func TestHooksStep_runnerErrorPropagates(t *testing.T) {
	boom := errors.New("task failed")
	var calls [][]string
	exec := mise.NewExecMise(recordingRunMise(&calls, boom))
	step := &mise.HooksStep{
		Exec:       exec,
		Commands:   []string{"echo pre"},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks:pre",
		StepName:   "hooks-pre",
	}
	err := step.Run(context.Background())
	require.ErrorIs(t, err, boom)
	require.Contains(t, err.Error(), "hooks:pre")
}

// A step with commands but no exec is a wiring error, not a silent skip.
func TestHooksStep_nilExecErrors(t *testing.T) {
	step := &mise.HooksStep{
		Commands:   []string{"echo pre"},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks:pre",
		StepName:   "hooks-pre",
	}
	require.Error(t, step.Run(context.Background()))
}
