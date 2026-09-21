package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// T-tui-workspace: the sectioned read surface over the 0065 config seam
// (strict decode + raw text), layer tabs synced with the nav, designed
// empty states, parse-error raw-text mode. Goldens pin composited frames;
// message-driven tests pin sync and cycling.

// wsShell builds a compositor over a temp profile whose files the test
// dictates (rel path → content), fully loaded: the nav read landed and
// the first selection's layer read landed. The layer reader is the real
// config area over the temp root.
func wsShell(t *testing.T, files map[string]string) (map[string]string, *Compositor) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}

	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Kernel: "6.12.1-arch1-1", Distro: "arch", Backend: "pacman"}
	area := service.NewReadsArea(service.ReadsDeps{
		Detect:      func() (*facts.Facts, error) { return f, nil },
		LoadProfile: profile.LoadTolerant,
	})
	c := NewCompositor(area, dir, func(*facts.Facts) LayerReader {
		return service.NewConfigArea(dir, service.ConfigDeps{Facts: f})
	})
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	c, cmd := cstep(c, mustMsg(c.Init()))
	require.False(t, c.nav.pending)
	c = wsSettle(t, c, cmd)
	return map[string]string{dir: "$PROFILE"}, c
}

// wsPress steps a key and settles any scheduled layer read.
func wsPress(t *testing.T, c *Compositor, s string) *Compositor {
	t.Helper()
	c, cmd := cstep(c, keyPress(s))
	return wsSettle(t, c, cmd)
}

// wsSettle applies pending layer loads until none remain.
func wsSettle(t *testing.T, c *Compositor, cmd tea.Cmd) *Compositor {
	t.Helper()
	for i := 0; i < 10 && cmd != nil; i++ {
		msg := mustMsg(cmd)
		if msg == nil {
			return c
		}
		c, cmd = cstep(c, msg)
	}
	require.False(t, c.ws.pending, "the layer read settles")
	return c
}

const wsAllSections = `id = "demo"
app = "demo-app"
description = "the demo module"
scope = "user"

[packages]
present = ["neovim", "ripgrep"]
absent = ["emacs"]

[tools]
node = "20"
python = "3.12"

[dotfiles]
"~/.bashrc" = { source = ".bashrc", mode = "symlink" }
"~/.config/demo.toml" = { line = "key = 1", comment = "#" }
"~/.profile" = { block = "export PATH=$PATH:~/bin" }

[when]
os = ["linux"]
hosts = ["myhost"]

[hooks]
pre = ["echo pre"]
post = ["echo post"]

[systemd.units."demo.service"]
Service = { ExecStart = "/usr/bin/demo" }

[secrets]
API_KEY = { env = "DEMO_API_KEY", description = "demo key" }
`

func TestWorkspace_allSections(t *testing.T) {
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	requireGolden(t, "workspace-all.golden", c.View().Content, subs)
}

func TestWorkspace_emptyStatesDesigned(t *testing.T) {
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\napp = \"demo\"\n"})
	requireGolden(t, "workspace-empty.golden", c.View().Content, subs)
}

func TestWorkspace_statusGlyphs(t *testing.T) {
	// A module with a superuser overlay carries the needs-root marker on
	// its meta section (fed from the profile's 0029 classification; the
	// plan-fed elevation set lands with T-tui-modals).
	_, c := wsShell(t, map[string]string{
		"modules/.keep":                        "",
		"users/root/modules/vault/module.toml": "id = \"vault\"\napp = \"vault\"\n",
	})
	require.True(t, c.ws.needsRoot, "a superuser overlay marks the workspace")
	require.Contains(t, c.View().Content, "needs root")
}

// T-0082-override: an overlay tab says what merging means — only the six
// families merge, meta/scope/when come from the base file (editing those
// in an overlay would otherwise be a silent no-op).
func TestWorkspace_overlayHintOnNonBaseTab(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":           "id = \"demo\"\napp = \"demo-app\"\n",
		"users/cri/modules/demo/module.toml": "id = \"demo\"\napp = \"demo-app\"\n",
	})

	frame := c.ws.view(100, 30, c.th)
	require.NotContains(t, frame, "merge", "the base tab carries no overlay hint")

	c = wsPress(t, c, "enter") // focus the workspace
	c = wsPress(t, c, "L")     // the user tab
	frame = c.ws.view(100, 30, c.th)
	require.Contains(t, frame, "packages, tools, dotfiles, hooks, mounts, smb")
	require.Contains(t, frame, "the rest comes from base")
}

func TestWorkspace_layerTabsWithAndWithoutOverlays(t *testing.T) {
	subs, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":              "id = \"demo\"\napp = \"demo\"\n",
		"users/cri/modules/demo/module.toml":    "id = \"demo\"\n",
		"hosts/myhost/modules/demo/module.toml": "id = \"demo\"\n",
		"modules/plain/module.toml":             "id = \"plain\"\napp = \"plain\"\n",
	})
	requireGolden(t, "workspace-tabs.golden", c.View().Content, subs)
	// The plain module has no overlays: one tab.
	c = wsPress(t, c, "j") // demo → plain
	require.Len(t, c.ws.tabs, 1)
	requireGolden(t, "workspace-tabs-single.golden", c.View().Content, subs)
}

func TestWorkspace_parseErrorShowsRawText(t *testing.T) {
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n[broken\n"})
	requireGolden(t, "workspace-parse-error.golden", c.View().Content, subs)
	frame := c.View().Content
	require.Contains(t, frame, "raw text mode", "a broken file opens in raw text mode")
	require.Contains(t, frame, "[broken", "the raw text stays visible")
}

func TestWorkspace_loadPlaceholder(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "modules", "demo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "modules", "demo", "module.toml"), []byte("id = \"demo\"\n"), 0o644))
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}
	area := service.NewReadsArea(service.ReadsDeps{
		Detect:      func() (*facts.Facts, error) { return f, nil },
		LoadProfile: profile.LoadTolerant,
	})
	c := NewCompositor(area, dir, func(*facts.Facts) LayerReader {
		return service.NewConfigArea(dir, service.ConfigDeps{Facts: f})
	})
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	c, _ = cstep(c, mustMsg(c.Init())) // nav landed; the layer read is still in flight
	requireGolden(t, "workspace-loading.golden", c.View().Content, map[string]string{dir: "$PROFILE"})
}

// wsThreeLayer is wsShell over a base+user+host demo profile.
func wsThreeLayer(t *testing.T) (map[string]string, *Compositor) {
	t.Helper()
	return wsShell(t, map[string]string{
		"modules/demo/module.toml":              "id = \"demo\"\ndescription = \"base layer\"\n",
		"users/cri/modules/demo/module.toml":    "id = \"demo\"\ndescription = \"user layer\"\n",
		"hosts/myhost/modules/demo/module.toml": "id = \"demo\"\ndescription = \"host layer\"\n",
	})
}

func TestWorkspace_layerTabCycles(t *testing.T) {
	_, c := wsThreeLayer(t)
	require.Equal(t, "base", c.ws.tabs[c.ws.active].layer)

	c = cpress(c, "tab") // focus the workspace
	c = wsPress(t, c, "L")
	require.Equal(t, "user", c.ws.tabs[c.ws.active].layer, "L cycles base → user")

	c = wsPress(t, c, "L")
	require.Equal(t, "host", c.ws.tabs[c.ws.active].layer)

	c = wsPress(t, c, "L")
	require.Equal(t, "base", c.ws.tabs[c.ws.active].layer, "L wraps to base")
}

func TestWorkspace_layerTabsSyncWithNav(t *testing.T) {
	_, c := wsThreeLayer(t)

	// Nav → workspace: select the user child, the tab follows.
	c = wsPress(t, c, "l") // expand demo
	c = wsPress(t, c, "j") // base child
	c = wsPress(t, c, "j") // user child
	require.Equal(t, "user", c.ws.tabs[c.ws.active].layer, "selecting a nav child moves the tab")
	require.Contains(t, c.ws.placeholderOrBody(), "user layer")

	// Workspace → nav: L cycles the tab, the nav cursor follows.
	c = cpress(c, "tab")
	c = wsPress(t, c, "L")
	require.Equal(t, "host", c.ws.tabs[c.ws.active].layer)
	sel := c.nav.selected()
	require.Equal(t, "host", sel.layer, "cycling the tab moves the nav cursor")
	require.Contains(t, c.ws.placeholderOrBody(), "host layer")
}

func TestWorkspace_selectionFollowsNav(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/alpha/module.toml": "id = \"alpha\"\ndescription = \"first\"\n",
		"modules/beta/module.toml":  "id = \"beta\"\ndescription = \"second\"\n",
	})
	require.Contains(t, c.ws.placeholderOrBody(), "first")
	c = wsPress(t, c, "j")
	require.Contains(t, c.ws.placeholderOrBody(), "second", "a module switch swaps the whole surface")
	require.NotContains(t, c.ws.placeholderOrBody(), "first")
}

func TestWorkspace_cursorWalksEntryRows(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	c = cpress(c, "tab")
	start := c.ws.cursor
	c = cpress(c, "j")
	require.Greater(t, c.ws.cursor, start, "j walks to the next entry row")
	require.False(t, c.ws.rows[c.ws.cursor].header, "the cursor never lands on a section header")
}

func TestWorkspace_disclosureRendersOnlySet(t *testing.T) {
	// 0075 T-tui-disclosure: a surface shows what IS — empty sections
	// are headers alone, meta keeps its identity rows, and every header
	// here is selectable because its section is empty.
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\napp = \"demo\"\n"})
	body := c.ws.placeholderOrBody()
	require.Contains(t, body, "meta")
	require.Contains(t, body, "description", "the identity rows stay")
	require.Contains(t, body, "scope", "the identity rows stay")
	require.Contains(t, body, "packages")
	require.Contains(t, body, "when")
	require.Contains(t, body, "smb")
	require.NotContains(t, body, "(none)", "empty sections render the header alone")
	for _, r := range c.ws.rows {
		if r.header && r.section != "meta" {
			require.True(t, r.selectable, "every empty section's header is the way in")
		}
	}

	// a on an empty section's header opens the section's add form.
	c = cpress(c, "tab") // focus the workspace
	wsToHeader(t, c, "packages")
	c = cpress(c, "a")
	addFormTop(t, c)
	cpress(c, "esc")
}

func TestWorkspace_pageKeysFollowTheCursor(t *testing.T) {
	// 0075 T-tui-page: end/home jump across a long surface, pgdown/pgup
	// step a page, and the visible window follows the cursor — before
	// this task the workspace offset never moved after a load, so the
	// tail rows were unreachable on screen.
	var present []string
	for i := 0; i < 40; i++ {
		present = append(present, fmt.Sprintf("\"p%02d\"", i))
	}
	files := map[string]string{"modules/demo/module.toml": "id = \"demo\"\n\n[packages]\npresent = [" +
		strings.Join(present, ", ") + "]\n"}
	_, c := wsShell(t, files)
	c = cpress(c, "tab")
	require.Equal(t, "id demo", c.ws.rows[c.ws.cursor].text, "the cursor starts on the first entry")

	page := c.ws.bodyH - 1 // the title line takes one body row
	require.Greater(t, page, 5, "the test window steps more than a screenful")

	c = wsPress(t, c, "end")
	require.Equal(t, len(c.ws.rows)-1, c.ws.cursor, "end lands on the last selectable row")
	require.Contains(t, c.View().Content, "smb", "the window follows the cursor to the tail")
	c = wsPress(t, c, "home")
	start := c.ws.cursor
	require.Equal(t, "id demo", c.ws.rows[c.ws.cursor].text, "home returns to the first row")
	c = wsPress(t, c, "pgdown")
	steps := 0
	for i := start; i < c.ws.cursor; i++ {
		if !c.ws.rows[i].header {
			steps++
		}
	}
	require.Equal(t, page, steps, "pgdown steps a page of entry rows")
	require.Contains(t, c.View().Content, c.ws.rows[c.ws.cursor].text, "the window keeps the paged row visible")
	c = wsPress(t, c, "pgup")
	require.Equal(t, start, c.ws.cursor, "pgup steps back")
	c = wsPress(t, c, "home")
	c = wsPress(t, c, "pgup")
	require.Equal(t, start, c.ws.cursor, "pgup clamps at the top")
}

func TestWorkspace_cursorRowBar(t *testing.T) {
	// 0075 T-tui-selection: the workspace's cursor row renders the bar;
	// the active field input carries it too.
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	c = cpress(c, "tab")
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "│ id demo", "the cursor row renders the bar")
	require.Contains(t, frame, "  description", "plain rows keep the two-space lead")

	c = cpress(c, "j") // the app row
	c = cpress(c, "enter")
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "│ ▸ demo-app", "the active input renders the bar")
}

func TestWorkspace_headerCursorBar(t *testing.T) {
	// 0076 T-tui-location: a selectable header under the cursor renders
	// the bar — an empty section's header is the way in, it must say so.
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\napp = \"demo\"\n"})
	c = cpress(c, "tab")
	for i := 0; !c.ws.atSection("packages") || !c.ws.rows[c.ws.cursor].header; i++ {
		require.Less(t, i, 32, "the packages header never took the cursor")
		c = cpress(c, "j")
	}
	require.True(t, c.ws.rows[c.ws.cursor].header, "the cursor rests on the header")
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "│ packages", "the header under the cursor renders the bar")
}

func TestWorkspace_titleShowsSection(t *testing.T) {
	// 0076 T-tui-location: the title line names the cursor's section, so
	// an empty stretch of surface still says where you are.
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\napp = \"demo\"\n"})
	c = cpress(c, "tab")
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "· meta", "the title names the cursor's section")

	for i := 0; !c.ws.atSection("writes"); i++ {
		require.Less(t, i, 32, "the writes header never took the cursor")
		c = cpress(c, "j")
	}
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "· writes", "the title follows the cursor")
}
