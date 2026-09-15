package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/state"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// ApplyOpts configures one apply session (0064-D8). The consumer seams —
// Handover and Output — pick the two consumption policies: Output attached
// means children stream to it fd-direct (the CLI's passthrough), Output
// absent means line-buffered StepOutput events (the TUI's event mode).
type ApplyOpts struct {
	ProfilePath string
	StatePath   string
	Modules     []string        // nil = all
	Sections    map[string]bool // nil/empty = all; the CLI adapter maps flags
	Yes         bool
	Force       bool
	Backup      bool
	// Handover runs one child command under the consumer's real terminal
	// (sudo prompts, interactive hooks). Required: the session wraps it
	// with the D9 contract (session-built cmd, Setpgid, stdio wired by the
	// consumer) before steps see it.
	Handover func(*exec.Cmd) error
	Output   io.Writer // nil = event mode; attached = fd passthrough (D2)
	// HandoverAvailable overrides StdinIsTerminal for the interactive-hook
	// opt-in at config-write (0064-D4's absorption row): nil = the deps'
	// stdin reality (today's behavior); a UI that can hand the terminal
	// over sets it true.
	HandoverAvailable *bool
}

// ApplyArea is the session area of the service layer: it starts and owns
// apply sessions.
type ApplyArea struct {
	deps ApplyDeps
}

// NewApplyArea builds the area on the given deps (zero-value dep fields
// fall back to the real implementations — WithDefaults).
func NewApplyArea(deps ApplyDeps) *ApplyArea {
	return &ApplyArea{deps: deps.WithDefaults()}
}

// SessionResult is the outcome of a finished session (0064-D7): returned
// by Wait for every completed decision. A failed step is a result, not a
// Go error; Wait errors only with a *SessionCancelledError when the
// session was cancelled.
type SessionResult struct {
	Outcome     SessionOutcome
	StepError   *StepError // Failed only
	FinalCursor string     // "" when Completed (state file removed)
	Backups     []string   // backup generation dirs, when Backup ran
}

// ApplySession is one running apply: a closed event stream, an up-front
// TTY classification, cancel, and a terminal result. Events MUST be
// drained — the run goroutine blocks on send once the buffer fills.
type ApplySession struct {
	events     <-chan Event
	previews   []StepPreview
	cancelOnce sync.Once
	cancel     context.CancelFunc
	done       chan struct{}
	waitOnce   sync.Once
	result     *SessionResult
	waitErr    error
}

// Events returns the session's event stream, closed after SessionEnded.
func (s *ApplySession) Events() <-chan Event { return s.events }

// Preview returns the per-step TTY classification frozen at Start
// (0064-D4): stable for the session's lifetime.
func (s *ApplySession) Preview() []StepPreview {
	out := make([]StepPreview, len(s.previews))
	copy(out, s.previews)
	return out
}

// Cancel aborts the session: the run context dies, children are killed by
// process group, and the pipeline returns without advancing the cursor
// past the interrupted step (contract 2). Idempotent; a no-op after the
// session already ended.
func (s *ApplySession) Cancel() { s.cancelOnce.Do(func() { s.cancel() }) }

// Wait blocks until the session ends and returns its result. The error is
// non-nil only when the outcome is Cancelled (a *SessionCancelledError);
// a failed step is carried by the result, not the error (0064-D7).
func (s *ApplySession) Wait() (*SessionResult, error) {
	s.waitOnce.Do(func() { <-s.done })
	return s.result, s.waitErr
}

// prepared carries the shared session prologue's results (reads, path
// layout, the interactive decision): everything Start and Preview need
// before steps exist.
type prepared struct {
	facts          *facts.Facts
	profile        *profile.Profile
	plan           *resolve.Plan
	sections       SectionSet
	statePath      string
	profileRoot    string
	misePluginsDir string
	interactive    bool
}

// prepare runs the shared prologue: validate, detect, load, filter,
// resolve, sections, and path layout — reads only, no lock, no writes.
// Shared by Start and Preview so the up-front classification cannot
// drift from what a real session builds.
func (a *ApplyArea) prepare(opts ApplyOpts) (*prepared, error) {
	if opts.ProfilePath == "" {
		return nil, fmt.Errorf("apply session: ProfilePath is required")
	}
	f, err := a.deps.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect: %w", err)
	}
	p, err := a.deps.LoadProfile(opts.ProfilePath, f)
	if err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	// Warn before LimitTo: a superuser-overlay module id passed as a filter
	// errors as unknown; that nudge stays CLI-side (renderer output).
	if err := p.LimitTo(profile.ParseModuleFilter(opts.Modules)); err != nil {
		return nil, err
	}
	plan, err := a.deps.Resolve(p, f)
	if err != nil {
		return nil, fmt.Errorf("resolve plan: %w", err)
	}

	sections, err := ResolveSections(opts.Sections)
	if err != nil {
		return nil, err
	}

	statePath := opts.StatePath
	if statePath == "" {
		statePath = state.ProfileStatePath(opts.ProfilePath)
	}

	profileRoot, err := filepath.Abs(p.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve profile root: %w", err)
	}

	var misePluginsDir string
	if f.Backend == "paru" {
		misePluginsDir = mise.PluginsDirFromEnv()
	}

	// D4: the interactive-hook opt-in keys on handover availability, not
	// raw stdin — CLI passes its own reality via deps, a UI that can hand
	// the terminal over sets HandoverAvailable. Decided once, here: it
	// drives both the config-write (runSpec) and the hook classification.
	interactive := a.deps.StdinIsTerminal()
	if opts.HandoverAvailable != nil {
		interactive = *opts.HandoverAvailable
	}

	return &prepared{
		facts:          f,
		profile:        p,
		plan:           plan,
		sections:       sections,
		statePath:      statePath,
		profileRoot:    profileRoot,
		misePluginsDir: misePluginsDir,
		interactive:    interactive,
	}, nil
}

// classifySteps freezes the per-step TTY classification (0064-D4): one
// StepPreview per step in pipeline order, NeedsTTY with the step's own
// reason. Pure predicate reads (euid, writability, declarations).
func classifySteps(steps []apply.Step) []StepPreview {
	previews := make([]StepPreview, len(steps))
	for i, st := range steps {
		reason := ""
		if hs, ok := st.(apply.HandoverStep); ok {
			reason = hs.RequiresTTY()
		}
		var overwrites []string
		if os, ok := st.(apply.OverwriteStep); ok {
			overwrites = os.OverwriteTargets()
		}
		previews[i] = StepPreview{Name: st.Name(), NeedsTTY: reason != "", Reason: reason, Overwrites: overwrites}
	}
	return previews
}

// Preview classifies a would-be session's steps without starting one —
// the plan gate's data (0064-D4, the TUI's apply gate). The same
// buildSteps/RequiresTTY path Start uses, so the classification cannot
// drift from behavior; no sidecar lock, no run goroutine, no writes.
// Reads are pure, so a later Start re-runs them for its own goroutine.
func (a *ApplyArea) Preview(opts ApplyOpts) ([]StepPreview, error) {
	prep, err := a.prepare(opts)
	if err != nil {
		return nil, err
	}
	steps := buildSteps(prep.sections, prep.plan, mise.NewExecMise(a.deps.NewMise()),
		prep.facts, prep.profileRoot, nil, prep.misePluginsDir,
		opts, a.deps, newConfigPaths(filepath.Dir(prep.statePath)), prep.interactive)
	return classifySteps(steps), nil
}

// Start resolves the plan, takes the sidecar lock, classifies the steps,
// and launches the run goroutine (0064-D6). It returns once the session
// is armed — events flow from the goroutine, so drain Events concurrently.
func (a *ApplyArea) Start(ctx context.Context, opts ApplyOpts) (*ApplySession, error) {
	if opts.Handover == nil {
		return nil, fmt.Errorf("apply session: Handover callback is required")
	}
	prep, err := a.prepare(opts)
	if err != nil {
		return nil, err
	}
	f, p, plan := prep.facts, prep.profile, prep.plan

	store := state.NewFileStore(prep.statePath)
	// Single-flight: TryLock never blocks — a second apply is
	// AlreadyRunningError, not a queue (contract 11, 0064-D6).
	ok, err := store.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock state: %w", err)
	}
	if !ok {
		return nil, &AlreadyRunningError{StatePath: prep.statePath}
	}
	s0, err := store.Load()
	if err != nil {
		_ = store.Unlock()
		return nil, fmt.Errorf("load state: %w", err)
	}

	// One mise instance drives both the pre-steps and every pipeline step.
	m := a.deps.NewMise()
	runner := mise.NewExecMise(m)

	ch := make(chan Event, 128)
	run := &sessionRunner{
		opts:     opts,
		deps:     a.deps,
		store:    store,
		ch:       ch,
		needsTTY: map[string]bool{},
	}
	collector := &lineCollector{run: run}
	run.collector = collector

	// The steps' own writer (smb listings): passthrough gets the consumer
	// writer (locked unless fd-direct), event mode gets the collector.
	out := opts.Output
	if out == nil {
		out = collector
	} else if _, isFile := out.(*os.File); !isFile {
		out = &executil.LockedWriter{W: out}
	}

	paths := newConfigPaths(filepath.Dir(prep.statePath))

	run.steps = buildSteps(prep.sections, plan, runner, f, prep.profileRoot, out,
		prep.misePluginsDir, opts, a.deps, paths, prep.interactive)

	run.index = make(map[string]int, len(run.steps))
	for i, st := range run.steps {
		run.index[st.Name()] = i
	}

	// TTY classification + handover injection (D4/D9): the session wraps
	// the consumer's callback with the process-group and stdio contract,
	// then hands the wrapper to every step that wants the terminal.
	for _, st := range run.steps {
		if hs, ok := st.(apply.HandoverStep); ok {
			hs.SetHandover(run.handover)
		}
	}
	previews := classifySteps(run.steps)
	for _, pv := range previews {
		run.needsTTY[pv.Name] = pv.NeedsTTY
	}

	runCtx, cancel := context.WithCancel(ctx)
	sess := &ApplySession{
		events:   ch,
		previews: previews,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	run.sess = sess

	go run.run(runCtx, &runSpec{
		m:           m,
		plan:        plan,
		profile:     p,
		facts:       f,
		sections:    prep.sections,
		paths:       paths,
		profileRoot: prep.profileRoot,
		cursor:      s0.LastCompleted,
		interactive: prep.interactive,
	})
	return sess, nil
}

// sessionRunner is the run goroutine's state: the event stream, the
// observer view of the pipeline, and the terminal result.
type sessionRunner struct {
	opts      ApplyOpts
	deps      ApplyDeps
	store     *state.FileStore
	ch        chan Event
	sess      *ApplySession
	steps     []apply.Step
	index     map[string]int
	needsTTY  map[string]bool
	collector *lineCollector
	runCtx    context.Context // set when the run goroutine starts

	mu      sync.Mutex // guards current + failure; events are send-only
	current string     // started-but-unfinished step (cancel reporting)
	failure *StepError // last StepFailed seen
}

func (r *sessionRunner) emit(ev Event) { r.ch <- ev }

// handover is the D9 contract enforcement point: from the step's command
// spec it derives a session-ctx twin — Setpgid on (D5's group kill reaches
// handover children), stdio nil (wiring stdio is the consumer's job; a
// pre-wired stream would strip the child's tty) — and hands the twin to
// the consumer, which runs it synchronously and returns the outcome (a
// refusal included); the step turns an error into a resumable failure.
// Cancellation rides exec's own ctx watcher — twin.Cancel group-SIGKILLs,
// synchronized with the consumer's Start/Wait, so cancel mid-handover
// (sudo included) kills without racing the process handle (D5).
func (r *sessionRunner) handover(cmd *exec.Cmd) error {
	twin := exec.CommandContext(r.runCtx, cmd.Path, cmd.Args[1:]...)
	twin.Args = cmd.Args // preserve the step's exact argv (argv[0] included)
	twin.Env = cmd.Env
	twin.Dir = cmd.Dir
	twin.Err = cmd.Err
	executil.SetGroupKill(twin)
	twin.WaitDelay = 5 * time.Second
	err := r.opts.Handover(twin)
	if err != nil && r.runCtx.Err() != nil {
		// The session died while the child ran: our own group kill produced
		// this failure ("signal: killed" wraps no context error), so the
		// step ends cancelled, not failed (contract 2, 0064-D5).
		return fmt.Errorf("%w: %v", context.Canceled, err)
	}
	return err
}

// runSpec carries everything the run goroutine needs, resolved at Start.
type runSpec struct {
	m           *mise.Mise
	plan        *resolve.Plan
	profile     *profile.Profile
	facts       *facts.Facts
	sections    SectionSet
	paths       configPaths
	profileRoot string
	cursor      string
	interactive bool
}

// run executes the absorbed orchestration (the cmd/apply.go Run body,
// minus the CLI-only --diff print path): PlanResolved → [backups] →
// mise ensure → D8a full-config snapshot → pipeline → SessionEnded.
func (r *sessionRunner) run(ctx context.Context, spec *runSpec) {
	r.runCtx = ctx
	res := &SessionResult{FinalCursor: spec.cursor}
	r.sess.result = res
	defer func() {
		_ = r.store.Unlock()
		close(r.ch)
		close(r.sess.done)
	}()

	r.emit(PlanResolved{
		Plan:            spec.plan,
		Profile:         spec.profile,
		Facts:           spec.facts,
		Cursor:          spec.cursor,
		CursorEffective: r.cursorEffective(spec.cursor),
	})

	// Back up copy-mode destinations before anything can overwrite them
	// (issue 0025); skipped when the dotfiles section is deselected (no
	// copy step runs, so there is nothing to safeguard). A safety flag
	// must not fail open: a backup failure aborts the run.
	if spec.sections.Has("dotfiles") && r.opts.Backup {
		taken, err := backupCopyTargets(spec.plan, spec.profileRoot, spec.facts)
		if err != nil {
			r.finishPreStepFailure(res, &StepError{Step: "backup", Err: fmt.Errorf("backup: %w", err)})
			return
		}
		for _, bt := range taken {
			res.Backups = append(res.Backups, bt.Dir)
			r.emit(bt)
		}
	}

	if _, err := spec.m.Ensure(); err != nil {
		r.finishPreStepFailure(res, &StepError{Step: "mise", Err: fmt.Errorf("ensure mise: %w", err)})
		return
	}

	// D8a (issue 0053): the FULL config lands before the pipeline starts —
	// the tools/dotfiles steps rewrite it per-section later, so a crash
	// leaves an on-disk config mirroring the whole resolved plan for crash
	// recovery and manual mise runs.
	if err := writeBootstrapConfig(spec.paths.shared, mise.GenerateApplyConfig(spec.plan, spec.profileRoot, spec.facts, spec.interactive)); err != nil {
		r.finishPreStepFailure(res, &StepError{Step: "config", Err: fmt.Errorf("write mise config: %w", err)})
		return
	}

	// Output policy (D2): attached writer = children stream to it exactly
	// as StreamLive wires them today (a terminal writer keeps child color);
	// absent = forced streaming into the line-buffered collector. The
	// passthrough stderr stays nil — mise's writers() then defaults it to
	// the process's stderr, byte-parity with the pre-session CLI (the old
	// Run left both writers nil); merging it into Output would move mise
	// warnings and the --verbose echo onto the stdout stream.
	if r.opts.Output != nil {
		spec.m.Out = r.opts.Output
		spec.m.Err = nil
	} else {
		spec.m.Out, spec.m.Err = r.collector, r.collector
		spec.m.ForceStream = true
	}

	pl := apply.NewPipeline(r.steps, r.store.Save)
	pl.SetState(&state.State{LastCompleted: spec.cursor})
	pl.SetObserver(r)
	if err := pl.Run(ctx); err != nil {
		r.finishPipelineOutcome(res, pl, err, ctx)
		return
	}
	if err := r.store.Remove(); err != nil {
		res.Outcome = OutcomeFailed
		res.StepError = &StepError{Step: "state", Err: fmt.Errorf("remove state file: %w", err)}
		r.emit(SessionEnded{Outcome: OutcomeFailed, Err: res.StepError, ResumeCursor: res.FinalCursor})
		return
	}
	res.Outcome = OutcomeCompleted
	res.FinalCursor = ""
	r.emit(SessionEnded{Outcome: OutcomeCompleted})
}

// cursorEffective mirrors the pipeline's rule: a cursor naming a step
// absent from this run is stale and ignored (contract 2).
func (r *sessionRunner) cursorEffective(cursor string) bool {
	if cursor == "" {
		return false
	}
	_, ok := r.index[cursor]
	return ok
}

// finishPreStepFailure ends the session Failed before any step ran; the
// cursor on disk is untouched, so FinalCursor stays the loaded cursor.
func (r *sessionRunner) finishPreStepFailure(res *SessionResult, se *StepError) {
	res.Outcome = OutcomeFailed
	res.StepError = se
	r.emit(SessionEnded{Outcome: OutcomeFailed, Err: se, ResumeCursor: res.FinalCursor})
}

// finishPipelineOutcome classifies a pipeline error. A genuine step
// failure is a Failed decision — even if the consumer's ctx died right
// after — because StepFailed already told the stream the truth; only a
// step killed by cancellation (its error wrapping ctx.Canceled), or a
// stop between steps, is Cancelled (0064-D3/D5, contract 2).
func (r *sessionRunner) finishPipelineOutcome(res *SessionResult, pl *apply.Pipeline, err error, ctx context.Context) {
	r.mu.Lock()
	failure := r.failure
	step := r.current
	if step == "" && failure != nil {
		step = failure.Step
	}
	r.mu.Unlock()

	cancelKilled := errors.Is(err, context.Canceled) ||
		(failure != nil && errors.Is(failure.Err, context.Canceled))
	if failure != nil && !cancelKilled {
		// A step failed on its own merits; a cancel racing in afterwards
		// must not rewrite the outcome the StepFailed event announced.
		res.Outcome = OutcomeFailed
		res.StepError = failure
		res.FinalCursor = pl.State().LastCompleted
		r.emit(SessionEnded{Outcome: OutcomeFailed, Err: failure, ResumeCursor: res.FinalCursor})
		return
	}

	if cancelKilled || ctx.Err() != nil {
		res.Outcome = OutcomeCancelled
		res.FinalCursor = pl.State().LastCompleted
		r.sess.waitErr = &SessionCancelledError{StepName: step}
		r.emit(SessionEnded{Outcome: OutcomeCancelled, ResumeCursor: res.FinalCursor})
		return
	}

	// No observer-recorded failure and no cancellation: belt for a pipeline
	// error that bypassed the observer.
	res.Outcome = OutcomeFailed
	res.StepError = &StepError{Step: "apply", Err: err}
	res.FinalCursor = pl.State().LastCompleted
	r.emit(SessionEnded{Outcome: OutcomeFailed, Err: res.StepError, ResumeCursor: res.FinalCursor})
}

// StepStarted implements apply.Observer: announce the step and arm the
// output collector's step name.
func (r *sessionRunner) StepStarted(name string) {
	r.mu.Lock()
	r.current = name
	r.mu.Unlock()
	r.emit(StepStarted{
		Name:     name,
		Index:    r.index[name],
		Total:    len(r.steps),
		NeedsTTY: r.needsTTY[name],
		Sub:      nil,
	})
}

// StepFinished implements apply.Observer: flush any partial output line
// and clear the current step.
func (r *sessionRunner) StepFinished(name string) {
	r.collector.flush(name)
	r.mu.Lock()
	if r.current == name {
		r.current = ""
	}
	r.mu.Unlock()
	r.emit(StepFinished{Name: name})
}

// StepFailed implements apply.Observer: record the typed failure.
func (r *sessionRunner) StepFailed(name string, err error) {
	r.mu.Lock()
	r.failure = &StepError{Step: name, Err: err}
	if r.current == name {
		r.current = ""
	}
	r.mu.Unlock()
	r.emit(StepFailed{Name: name, Err: r.failure})
}

// HookStarted implements apply.Observer: announce one hook command inside
// a hook step (0071) — a StepStarted carrying Sub, same step identity.
func (r *sessionRunner) HookStarted(step string, sub apply.SubStep) {
	s := sub
	r.emit(StepStarted{
		Name:     step,
		Index:    r.index[step],
		Total:    len(r.steps),
		NeedsTTY: r.needsTTY[step],
		Sub:      &s,
	})
}

// HookFailed implements apply.Observer: announce the failing hook command
// with a Sub-carrying StepFailed. The step-level failure (Sub nil) follows
// from StepFailed when the hook was required.
func (r *sessionRunner) HookFailed(step string, sub apply.SubStep, err error) {
	s := sub
	se := &StepError{Step: step, Err: fmt.Errorf("hook %q: %w", sub.Command, err)}
	r.emit(StepFailed{Name: step, Err: se, Sub: &s})
}

// lineCollector turns child output into line-buffered StepOutput events
// (D2's event mode). Bytes arrive through the mise runner's writers; a
// trailing partial line flushes when the step finishes.
type lineCollector struct {
	run *sessionRunner
	mu  sync.Mutex
	buf []byte
}

func (c *lineCollector) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	for {
		i := bytes.IndexByte(c.buf, '\n')
		if i < 0 {
			break
		}
		c.emitLine(c.buf[:i])
		c.buf = c.buf[i+1:]
	}
	return len(p), nil
}

// flush emits a trailing partial line when the named step ends.
func (c *lineCollector) flush(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.buf) > 0 {
		c.emitLine(c.buf)
		c.buf = nil
	}
}

func (c *lineCollector) emitLine(line []byte) {
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	chunk := make([]byte, len(line))
	copy(chunk, line)
	c.run.mu.Lock()
	name := c.run.current
	c.run.mu.Unlock()
	// Send from the collector: the pipeline is single-goroutine, so this
	// is the run goroutine; a blocking send matches emit's contract.
	c.run.ch <- StepOutput{Name: name, Chunk: chunk}
}
