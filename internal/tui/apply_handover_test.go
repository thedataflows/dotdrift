package tui

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The handover bridge runs real child processes: Update answers the
// handover message with tea.ExecProcess (the program's own stdio is the
// child's), the callback releases the blocked session step, and the
// program keeps running after the child exits. A real pipe-backed
// tea.Program drives these tests — the pure state-machine tests cannot
// exercise bubbletea's exec machinery.

// applyHost hosts an applyModel as a program: keys go to handleKey,
// every other message to update — exactly as the shell routes them.
type applyHost struct{ m *applyModel }

func (h applyHost) Init() tea.Cmd  { return nil }
func (h applyHost) View() tea.View { return tea.NewView("host") }

func (h applyHost) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		return h, h.m.handleKey(k.String())
	}
	return h, h.m.update(msg)
}

// safeBuf is a concurrency-safe output sink: the renderer and the
// handover child both write to the program's output.
type safeBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func startApplyProgram(t *testing.T, m *applyModel) (func(tea.Msg), *safeBuf, func()) {
	t.Helper()
	out := &safeBuf{}
	p := tea.NewProgram(applyHost{m: m},
		tea.WithInput(strings.NewReader("")),
		tea.WithOutput(out),
	)
	runErr := make(chan error, 1)
	go func() {
		_, err := p.Run()
		runErr <- err
	}()
	quit := func() {
		p.Quit()
		select {
		case err := <-runErr:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("program did not quit")
		}
	}
	return p.Send, out, quit
}

// A handover child runs on the program's stdio through tea.ExecProcess,
// its outcome reaches the blocked session step through done, and the
// program keeps processing afterwards (alt-screen restored).
func TestHandover_ExecProcess(t *testing.T) {
	m := newApplyModel(&fakeLauncher{previews: []service.StepPreview{{Name: "packages"}}},
		"/profile", "/state", func(tea.Msg) {}, newTheme(true))
	send, out, quit := startApplyProgram(t, m)
	defer quit()

	done := make(chan error, 1)
	send(applyHandoverMsg{cmd: exec.Command("sh", "-c", "echo child-ran"), done: done})

	select {
	case err := <-done:
		require.NoError(t, err)
		require.Contains(t, out.String(), "child-ran",
			"the child ran with the program's output wired")
	case <-time.After(10 * time.Second):
		t.Fatal("handover done never released — Update did not answer with tea.ExecProcess")
	}

	// A second handover proves the program survived the first exec.
	done2 := make(chan error, 1)
	send(applyHandoverMsg{cmd: exec.Command("true"), done: done2})
	select {
	case err := <-done2:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("program stopped processing messages after the first handover")
	}
}
