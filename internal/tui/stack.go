package tui

// The view stack (0062-D4): selection opens a view, esc pops, exactly one
// view is active. Views are values keyed by viewID; the stack owns the
// navigation history and the dirty ledger — a popped view carrying
// unsaved editor state stays alive for the session and is restored when
// its id reopens (the placeholder semantics T-tui-editors builds on;
// clean views are discarded and rebuild fresh).

// viewID identifies a view across the stack and ledger.
type viewID struct {
	kind viewKind
	key  string // module id, account name, … ("" for singletons)
}

// view is one stacked view: identity plus render state. Content and
// loading are the main pane's; raw marks a module view showing raw layer
// declarations; dirty means unsaved editor state (always false until the
// editor suite lands). item is the tree node the view opened from — the
// esc-pop path walks the cursor back to it.
type view struct {
	id      viewID
	title   string
	item    treeItem
	raw     bool
	dirty   bool
	loading bool
	content string
}

type viewStack struct {
	stack  []view
	ledger map[viewID]view
}

func (s *viewStack) len() int { return len(s.stack) }

func (s *viewStack) top() view {
	if len(s.stack) == 0 {
		return view{}
	}
	return s.stack[len(s.stack)-1]
}

// setTop replaces the top view (the raw-toggle / dirty-mark path).
func (s *viewStack) setTop(v view) {
	if len(s.stack) > 0 {
		s.stack[len(s.stack)-1] = v
	}
}

// open brings id's view to the top: restoring a parked dirty view, or
// truncating back to an earlier copy of the same view, or pushing fresh.
func (s *viewStack) open(v view) {
	if s.ledger == nil {
		s.ledger = map[viewID]view{}
	}
	if parked, ok := s.ledger[v.id]; ok {
		delete(s.ledger, v.id)
		v = parked
	}
	for i := len(s.stack) - 1; i >= 0; i-- {
		if s.stack[i].id == v.id {
			s.stack = s.stack[:i]
			break
		}
	}
	if len(s.stack) > 0 && s.stack[len(s.stack)-1].id == v.id {
		s.stack[len(s.stack)-1] = v
		return
	}
	s.stack = append(s.stack, v)
}

// pop removes and returns the top view (parking it when dirty); popping
// the last remaining view is a no-op that returns it.
func (s *viewStack) pop() view {
	if len(s.stack) <= 1 {
		return s.top()
	}
	v := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	if v.dirty {
		if s.ledger == nil {
			s.ledger = map[viewID]view{}
		}
		s.ledger[v.id] = v
	}
	return v
}

// dirtyAnywhere reports whether any live or parked view carries unsaved
// state — the quit-confirm and header-dirty inputs.
func (s *viewStack) dirtyAnywhere() bool {
	for _, v := range s.stack {
		if v.dirty {
			return true
		}
	}
	return len(s.ledger) > 0
}

// hasDirtyParked reports whether the ledger currently holds anything.
func (s *viewStack) hasDirtyParked() bool { return len(s.ledger) > 0 }
