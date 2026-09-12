package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The view stack (0062-D4): selection opens a view, esc pops. The bottom
// view never pops — the shell always shows something. Dirty editor state
// survives navigation for the session: dirty views popped from above park
// in a ledger and are restored on reopen (the draft placeholder
// T-tui-editors builds on); clean views are discarded and rebuild fresh.

func modView(id string) view {
	return view{id: viewID{kind: viewModule, key: id}, title: "MODULE " + id}
}

func TestViewStack_openPop(t *testing.T) {
	var s viewStack
	require.Zero(t, s.len(), "fresh stack is empty")

	shell := modView("shell")
	status := view{id: viewID{kind: viewStatus}, title: "STATUS"}
	plan := view{id: viewID{kind: viewPlan}, title: "PLAN"}

	s.open(shell)
	require.Equal(t, 1, s.len())
	s.open(status)
	require.Equal(t, 2, s.len())
	s.open(plan)
	require.Equal(t, 3, s.len())
	require.Equal(t, plan.id, s.top().id, "top = most recent open")

	popped := s.pop()
	require.Equal(t, plan.id, popped.id, "pop returns the top view")
	require.Equal(t, status.id, s.top().id)
	popped = s.pop()
	require.Equal(t, status.id, popped.id)
	require.Equal(t, shell.id, s.top().id)
	// The bottom view never pops — the shell always shows something.
	require.Equal(t, shell.id, s.pop().id, "popping the last view is a no-op")
	require.Equal(t, 1, s.len())
	require.Equal(t, shell.id, s.top().id)
}

func TestViewStack_reopenTruncates(t *testing.T) {
	var s viewStack
	shell := modView("shell")
	status := view{id: viewID{kind: viewStatus}, title: "STATUS"}

	s.open(shell)
	s.open(status)
	s.open(shell) // navigating back to an earlier view truncates the stack
	require.Equal(t, 1, s.len(), "no duplicate growth")
	require.Equal(t, shell.id, s.top().id)
}

func TestViewStack_dirtyEditorSurvivesNavigation(t *testing.T) {
	var s viewStack
	shell := modView("shell")
	origin := view{id: viewID{kind: viewOrigin, key: "shell/base"}, title: "RAW base"}
	status := view{id: viewID{kind: viewStatus}, title: "STATUS"}

	// The dirty module view keeps its state while other views stack on top
	// and while those are popped away — navigation never touches it.
	s.open(shell)
	top := s.top()
	top.raw = true // view state a later editor draft will piggyback on
	top.dirty = true
	s.setTop(top)
	s.open(status)
	s.open(plan2())
	require.Equal(t, 3, s.len())
	s.pop()
	s.pop()
	require.Equal(t, shell.id, s.top().id)
	require.True(t, s.top().dirty, "the dirty view survived navigation")
	require.True(t, s.top().raw, "its state survived too")

	// A dirty view popped from above parks in the ledger and restores on
	// reopen — the draft survives being navigated away from.
	s.open(origin)
	top = s.top()
	top.dirty = true
	s.setTop(top)
	s.open(status)
	s.pop() // discards clean status
	s.pop() // parks the dirty origin view
	require.True(t, s.hasDirtyParked(), "dirty popped views park in the ledger")
	s.open(origin)
	require.True(t, s.top().dirty, "reopening restores the live draft")
	require.False(t, s.hasDirtyParked(), "restored views leave the ledger")
}

func plan2() view { return view{id: viewID{kind: viewPlan}, title: "PLAN"} }

func TestViewStack_cleanViewsAreDiscarded(t *testing.T) {
	var s viewStack
	status := view{id: viewID{kind: viewStatus}, title: "STATUS"}

	s.open(modView("shell"))
	s.open(status)
	top := s.top()
	top.loading = true // a rendered async view in some state
	s.setTop(top)
	s.pop()
	require.False(t, s.hasDirtyParked(), "clean views are not parked")
	s.open(status)
	require.False(t, s.top().loading, "clean views rebuild fresh on reopen")
}

func TestViewStack_dirtyAnywhere(t *testing.T) {
	var s viewStack
	require.False(t, s.dirtyAnywhere(), "empty stack is clean")

	s.open(modView("shell"))
	require.False(t, s.dirtyAnywhere())
	top := s.top()
	top.dirty = true
	s.setTop(top)
	require.True(t, s.dirtyAnywhere(), "a dirty top view trips the guard")

	s.open(view{id: viewID{kind: viewStatus}})
	require.True(t, s.dirtyAnywhere(), "a parked dirty view trips the guard")
}
