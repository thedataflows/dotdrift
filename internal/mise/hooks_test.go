package mise_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

var testFacts = &facts.Facts{
	Hostname: "myhost",
	Username: "cri",
	OS:       "linux",
	Backend:  "paru",
}

type hookTask struct {
	Run         []string          `toml:"run"`
	Dir         string            `toml:"dir"`
	Env         map[string]string `toml:"env"`
	Interactive bool              `toml:"interactive"`
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

// GenerateHookTasks emits one mise task per command so each can be run (and
// tolerated on failure) individually while preserving order. Tasks are named
// <phase>-<index>: hooks-pre-0, hooks-pre-1, hooks-post-0.
func TestGenerateHookTasks_perCommandTasks(t *testing.T) {
	hooks := resolve.HooksStep{
		Pre:  []profile.HookCommand{{Command: "echo base-pre"}, {Command: "echo host-pre"}},
		Post: []profile.HookCommand{{Command: "echo base-post"}},
	}
	out := mise.GenerateHookTasks(hooks, "/profiles/main", testFacts)

	require.Contains(t, out, `[tasks."hooks-pre-0"]`)
	require.Contains(t, out, `[tasks."hooks-pre-1"]`)
	require.Contains(t, out, `[tasks."hooks-post-0"]`)

	tasks := decodeHookTasks(t, out)
	require.Equal(t, []string{"echo base-pre"}, tasks["hooks-pre-0"].Run)
	require.Equal(t, []string{"echo host-pre"}, tasks["hooks-pre-1"].Run)
	require.Equal(t, []string{"echo base-post"}, tasks["hooks-post-0"].Run)
	require.Equal(t, "/profiles/main", tasks["hooks-pre-0"].Dir)
	require.Equal(t, map[string]string{
		"DOTDRIFT_PROFILE":  "/profiles/main",
		"DOTDRIFT_HOSTNAME": "myhost",
		"DOTDRIFT_USERNAME": "cri",
		"DOTDRIFT_OS":       "linux",
		"DOTDRIFT_BACKEND":  "paru",
	}, tasks["hooks-pre-0"].Env)
	require.Equal(t, tasks["hooks-pre-0"].Env, tasks["hooks-pre-1"].Env)
}

// No hook commands → no [tasks] section at all.
func TestGenerateHookTasks_empty(t *testing.T) {
	require.Empty(t, mise.GenerateHookTasks(resolve.HooksStep{}, "/profiles/main", testFacts))
}

// Only the non-empty side is emitted.
func TestGenerateHookTasks_preOnly(t *testing.T) {
	out := mise.GenerateHookTasks(resolve.HooksStep{Pre: []profile.HookCommand{{Command: "echo hi"}}}, "/profiles/main", testFacts)
	tasks := decodeHookTasks(t, out)
	require.Contains(t, tasks, "hooks-pre-0")
	require.NotContains(t, tasks, "hooks-post-0")
}

// Shell metacharacters in commands must survive the TOML round-trip.
func TestGenerateHookTasks_escapesCommands(t *testing.T) {
	raw := `echo "a b" && sed -i 's\x\y\g' f`
	hooks := resolve.HooksStep{Pre: []profile.HookCommand{{Command: raw}}}
	out := mise.GenerateHookTasks(hooks, "/profiles/main", testFacts)
	require.Equal(t, []string{raw}, decodeHookTasks(t, out)["hooks-pre-0"].Run)
}

// Every emitted hook task carries interactive=true so mise connects it to the
// terminal — a hook running sudo can then disable echo instead of echoing the
// password. (Always, not conditionally — see 0088 and the sibling test.)
func TestGenerateHookTasks_interactiveTrue(t *testing.T) {
	hooks := resolve.HooksStep{
		Pre:  []profile.HookCommand{{Command: "echo pre"}},
		Post: []profile.HookCommand{{Command: "echo post"}},
	}
	out := mise.GenerateHookTasks(hooks, "/profiles/main", testFacts)
	require.Contains(t, out, "interactive = true")
	tasks := decodeHookTasks(t, out)
	require.True(t, tasks["hooks-pre-0"].Interactive, "pre task must be interactive")
	require.True(t, tasks["hooks-post-0"].Interactive, "post task must be interactive")
}

// Hook tasks are always interactive (issue 0088) — even when the writing
// session has no terminal: interactivity is the hook task's intrinsic
// property, not a function of how the config-writing run was attached. When
// mise runs such a task without a TTY it behaves exactly like a plain task
// (verified against mise 2026.9.10), so piped/CI runs are unchanged.
func TestGenerateHookTasks_interactiveEvenWhenSessionIsNot(t *testing.T) {
	out := mise.GenerateHookTasks(resolve.HooksStep{Pre: []profile.HookCommand{{Command: "echo pre"}}}, "/profiles/main", testFacts)
	require.Contains(t, out, "interactive = true")
	require.True(t, decodeHookTasks(t, out)["hooks-pre-0"].Interactive)
}

// The always-interactive rule flows through the full apply config, not just
// the standalone task generator.
func TestGenerateApplyConfig_interactiveFlowsThrough(t *testing.T) {
	plan := &resolve.Plan{Hooks: resolve.HooksStep{Pre: []profile.HookCommand{{Command: "echo pre"}}}}
	out := mise.GenerateApplyConfig(plan, "/profiles/main", testFacts)
	require.Contains(t, out, "interactive = true")
}

// The apply-time config composes tools, dotfiles, and hook tasks.
func TestGenerateApplyConfig_includesToolsDotfilesAndTasks(t *testing.T) {
	plan := &resolve.Plan{
		Tools: resolve.ToolsStep{Versions: map[string]string{"node": "22"}},
		Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
			{Target: "~/.bashrc", Source: "/src/.bashrc", Mode: "symlink"},
		}},
		Hooks: resolve.HooksStep{
			Pre:  []profile.HookCommand{{Command: "echo pre"}},
			Post: []profile.HookCommand{{Command: "echo post"}},
		},
	}
	out := mise.GenerateApplyConfig(plan, "/profiles/main", testFacts)

	require.Contains(t, out, "[tools]")
	require.Contains(t, out, "[dotfiles]")
	require.Contains(t, out, `[tasks."hooks-pre-0"]`)
	require.Contains(t, out, `[tasks."hooks-post-0"]`)

	tasks := decodeHookTasks(t, out)
	require.Equal(t, []string{"echo pre"}, tasks["hooks-pre-0"].Run)
	require.Equal(t, []string{"echo post"}, tasks["hooks-post-0"].Run)
}

// A plan without hooks keeps the apply config task-free.
func TestGenerateApplyConfig_noHooks(t *testing.T) {
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "22"}}}
	out := mise.GenerateApplyConfig(plan, "/profiles/main", testFacts)
	require.Contains(t, out, "[tools]")
	require.NotContains(t, out, "[tasks]")
}

// recordingRunMise fakes a mise binary, records every runner invocation, and
// returns runErr for every non-version call.
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

// recordingRunMiseFn lets each call decide its own error (by argv), so a test
// can fail one command's task and succeed the rest.
func recordingRunMiseFn(calls *[][]string, fn func(args []string) error) *mise.Mise {
	return &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, _ string, args ...string) (string, error) {
			*calls = append(*calls, append([]string{}, args...))
			for _, a := range args {
				if a == "--version" {
					return mise.MinMiseVersion + "\n", nil
				}
			}
			return "", fn(args)
		},
	}
}

// Each command runs as its own mise task, named by index against the Task
// prefix, in order.
func TestHooksStep_runsEachCommandAsTask(t *testing.T) {
	var calls [][]string
	exec := mise.NewExecMise(recordingRunMise(&calls, nil))
	step := &mise.HooksStep{
		Exec:       exec,
		Commands:   []profile.HookCommand{{Command: "echo a"}, {Command: "echo b"}},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks-pre",
		StepName:   "hooks-pre",
	}

	require.Equal(t, "hooks-pre", step.Name())
	require.NoError(t, step.Run(context.Background()))
	require.Contains(t, calls, []string{"run", "--cd", "/state/mise", "hooks-pre-0"})
	require.Equal(t, []string{"hooks-pre-0", "hooks-pre-1"}, taskNames(calls))
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
		Commands:   []profile.HookCommand{{Command: "echo pre"}},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks-pre",
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
		Task:       "hooks-pre",
		StepName:   "hooks-pre",
	}
	require.NoError(t, step.Run(context.Background()))
	require.Empty(t, calls, "empty hooks must not invoke mise")
}

// A required (non-optional) hook failing fails the step so resume re-runs it.
func TestHooksStep_requiredFailureFailsStep(t *testing.T) {
	boom := errors.New("task failed")
	var calls [][]string
	exec := mise.NewExecMise(recordingRunMise(&calls, boom))
	step := &mise.HooksStep{
		Exec:       exec,
		Commands:   []profile.HookCommand{{Command: "echo pre"}},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks-pre",
		StepName:   "hooks-pre",
	}
	err := step.Run(context.Background())
	require.ErrorIs(t, err, boom)
	require.Contains(t, err.Error(), "echo pre", "error must name the failing command")
}

// An optional hook failing does NOT fail the step: it runs, the failure is
// logged (warn) naming the command, and apply continues.
func TestHooksStep_optionalFailureContinues(t *testing.T) {
	buf := captureZerolog(t)
	boom := errors.New("task failed")
	var calls [][]string
	exec := mise.NewExecMise(recordingRunMise(&calls, boom))
	step := &mise.HooksStep{
		Exec:       exec,
		Commands:   []profile.HookCommand{{Command: "echo flaky", Optional: true}},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks-pre",
		StepName:   "hooks-pre",
	}
	require.NoError(t, step.Run(context.Background()))
	require.Len(t, taskNames(calls), 1, "optional hook must still run")
	require.Contains(t, buf.String(), "echo flaky", "optional failure must be logged with the command")
}

// An optional hook failing mid-sequence still runs the remaining hooks in
// order — optionality never reorders or short-circuits the sequence.
func TestHooksStep_optionalFailureRunsRemaining(t *testing.T) {
	var calls [][]string
	exec := mise.NewExecMise(recordingRunMiseFn(&calls, func(args []string) error {
		if len(args) > 0 && args[len(args)-1] == "hooks-pre-1" {
			return errors.New("flaky failed")
		}
		return nil
	}))
	step := &mise.HooksStep{
		Exec: exec,
		Commands: []profile.HookCommand{
			{Command: "echo first"},
			{Command: "echo flaky", Optional: true},
			{Command: "echo last"},
		},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks-pre",
		StepName:   "hooks-pre",
	}
	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, []string{"hooks-pre-0", "hooks-pre-1", "hooks-pre-2"}, taskNames(calls),
		"all hooks must run in order despite the optional failure")
}

// taskNames pulls the trailing task name off each recorded `mise run` argv,
// ignoring the EnsureContext --version probe that precedes the first run.
func taskNames(calls [][]string) []string {
	var names []string
	for _, c := range calls {
		if len(c) > 0 && c[0] == "run" {
			names = append(names, c[len(c)-1])
		}
	}
	return names
}

// A step with commands but no exec is a wiring error, not a silent skip.
func TestHooksStep_nilExecErrors(t *testing.T) {
	step := &mise.HooksStep{
		Commands:   []profile.HookCommand{{Command: "echo pre"}},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks-pre",
		StepName:   "hooks-pre",
	}
	require.Error(t, step.Run(context.Background()))
}

// recordingHooksObserver captures the per-command boundaries the step
// announces through the pipeline observer (0071).
type recordingHooksObserver struct {
	events []string
}

func (o *recordingHooksObserver) StepStarted(string)       {}
func (o *recordingHooksObserver) StepFinished(string)      {}
func (o *recordingHooksObserver) StepFailed(string, error) {}
func (o *recordingHooksObserver) HookStarted(step string, sub apply.SubStep) {
	o.events = append(o.events, fmt.Sprintf("start:%s:%d/%d:%s", step, sub.Index, sub.Total, sub.Command))
}
func (o *recordingHooksObserver) HookFailed(step string, sub apply.SubStep, err error) {
	o.events = append(o.events, fmt.Sprintf("fail:%s:%d/%d:%s", step, sub.Index, sub.Total, sub.Command))
}

var _ apply.Observer = (*recordingHooksObserver)(nil)

// Every hook command announces its sub-step boundary; an optional failure
// announces the failure and the sequence continues.
func TestHooksStep_subStepEventsFirePerCommand(t *testing.T) {
	boom := errors.New("task failed")
	var calls [][]string
	exec := mise.NewExecMise(recordingRunMiseFn(&calls, func(args []string) error {
		if len(args) > 0 && args[len(args)-1] == "hooks-pre-1" {
			return boom
		}
		return nil
	}))
	obs := &recordingHooksObserver{}
	step := &mise.HooksStep{
		Exec: exec,
		Commands: []profile.HookCommand{
			{Command: "echo first"},
			{Command: "echo flaky", Optional: true},
			{Command: "echo last"},
		},
		ConfigPath: "/state/mise/mise.toml",
		Task:       "hooks-pre",
		StepName:   "hooks-pre",
	}
	step.SetObserver(obs)

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, []string{
		"start:hooks-pre:0/3:echo first",
		"start:hooks-pre:1/3:echo flaky",
		"fail:hooks-pre:1/3:echo flaky",
		"start:hooks-pre:2/3:echo last",
	}, obs.events)
}

// With the interactive opt-in and a handover callback, every task routes
// through the handover seam as a spec-built `mise run` child — the piped
// runner is never invoked — and the step classifies NeedsTTY with the
// interactive-hook reason (0071, 0064-D4).
func TestHooksStep_interactiveRoutesThroughHandover(t *testing.T) {
	var calls [][]string
	runner := mise.NewExecMise(recordingRunMise(&calls, nil))
	var handed [][]string
	step := &mise.HooksStep{
		Exec:        runner,
		Commands:    []profile.HookCommand{{Command: "sudo chown"}, {Command: "echo done"}},
		ConfigPath:  "/state/mise/mise.toml",
		Task:        "hooks-pre",
		StepName:    "hooks-pre",
		Interactive: true,
		Handover: func(cmd *exec.Cmd) error {
			handed = append(handed, cmd.Args)
			return nil
		},
	}

	require.Contains(t, step.RequiresTTY(), "interactive hook")
	require.NoError(t, step.Run(context.Background()))
	for _, c := range calls {
		require.Equal(t, []string{"--version"}, c,
			"only mise's version probe may use the piped runner; tasks go through handover")
	}
	require.Equal(t, [][]string{
		{"/fake/mise", "run", "--cd", "/state/mise", "hooks-pre-0"},
		{"/fake/mise", "run", "--cd", "/state/mise", "hooks-pre-1"},
	}, handed)

	// Without the opt-in there is no terminal need and no handover.
	step.Interactive = false
	require.Empty(t, step.RequiresTTY())
}

// Interactive set without a handover callback fails loud (contract 13):
// classifying NeedsTTY and then piping an interactive command would lie.
func TestHooksStep_interactiveWithoutHandoverFailsLoud(t *testing.T) {
	var calls [][]string
	runner := mise.NewExecMise(recordingRunMise(&calls, nil))
	step := &mise.HooksStep{
		Exec:        runner,
		Commands:    []profile.HookCommand{{Command: "sudo chown"}},
		ConfigPath:  "/state/mise/mise.toml",
		Task:        "hooks-pre",
		StepName:    "hooks-pre",
		Interactive: true,
	}

	err := step.Run(context.Background())
	require.ErrorContains(t, err, "handover", "interactive hooks must fail loud without a handover callback")
	require.Empty(t, calls, "no task may run when the handover contract cannot be honored")
}
