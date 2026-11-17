package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/profile"
)

const servicesFixture = "id = \"demo\"\n\n[services.my-agent]\ncommand = \"~/.local/bin/my-agent --serve\"\nstate = \"running\"\nenabled = true\nrestart = \"on-failure\"\nenvironment = { FOO = \"bar\", BAZ = \"qux\" }\nworking_directory = \"/tmp\"\n\n[services.syncthing]\nstate = \"stopped\"\nenabled = false\n"

func servicesShell(t *testing.T) (map[string]string, *Compositor) {
	t.Helper()
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": servicesFixture})
	return subs, cpress(c, "right")
}

func TestServices_rowsRender(t *testing.T) {
	_, c := servicesShell(t)
	var containers, fields int
	var envValue string
	for _, r := range c.ws.rows {
		if r.section != "services" {
			continue
		}
		if r.container {
			containers++
			require.Equal(t, profile.FamilyServices, r.family)
			continue
		}
		if r.family == profile.FamilyServices {
			fields++
			if r.key == pathKey("my-agent", "environment") {
				envValue = r.value
			}
		}
	}
	require.Equal(t, 2, containers, "one container row per service")
	require.Equal(t, 8, fields, "only the set fields render")
	require.Equal(t, "BAZ=qux, FOO=bar", envValue, "environment renders sorted as K=V pairs")
	body := c.ws.placeholderOrBody()
	require.Contains(t, body, "services")
	require.Contains(t, body, "my-agent")
	require.Contains(t, body, "command ~/.local/bin/my-agent --serve")
	require.Contains(t, body, "environment BAZ=qux, FOO=bar")
	require.Contains(t, body, "working_directory /tmp")
}

func TestServices_fieldEdits(t *testing.T) {
	_, c := servicesShell(t)
	wsToKey(t, c, pathKey("my-agent", "command"), "services")
	c = cpress(c, "enter")
	require.Equal(t, "~/.local/bin/my-agent --serve", c.ws.editing.inputString())
	c.ws.editing.set("~/.local/bin/my-agent --serve --verbose")
	c = wsPress(t, c, "enter")
	require.Equal(t, "~/.local/bin/my-agent --serve --verbose", c.ws.draft.cfg.Services["my-agent"].Command)
	require.Contains(t, c.ws.draft.raw, "command = \"~/.local/bin/my-agent --serve --verbose\"", "the edit lands in the draft raw TOML")

	wsToKey(t, c, pathKey("my-agent", "state"), "services")
	c = cpress(c, "enter")
	ch := c.modals[0].(*choiceModel)
	require.Equal(t, []string{"running", "stopped", "absent"}, ch.choices)
	c = cpress(c, "down")
	c = cpress(c, "enter")
	require.Equal(t, "stopped", c.ws.draft.cfg.Services["my-agent"].State)

	wsToKey(t, c, pathKey("my-agent", "restart"), "services")
	c = cpress(c, "enter")
	ch = c.modals[0].(*choiceModel)
	require.Equal(t, []string{"on-failure", "always", "never"}, ch.choices)
	c = cpress(c, "down")
	c = cpress(c, "enter")
	require.Equal(t, "always", c.ws.draft.cfg.Services["my-agent"].Restart)

	wsToKey(t, c, pathKey("syncthing", "enabled"), "services")
	c = cpress(c, "enter")
	ch = c.modals[0].(*choiceModel)
	require.Equal(t, []string{"false", "true"}, ch.choices)
	c = cpress(c, "down")
	c = cpress(c, "enter")
	require.NotNil(t, c.ws.draft.cfg.Services["syncthing"].Enabled)
	require.True(t, *c.ws.draft.cfg.Services["syncthing"].Enabled)

	wsToKey(t, c, pathKey("my-agent", "environment"), "services")
	c = cpress(c, "enter")
	require.Equal(t, "BAZ=qux, FOO=bar", c.ws.editing.inputString())
	c.ws.editing.set("FOO=baz, NEW=1")
	c = wsPress(t, c, "enter")
	require.Equal(t, map[string]string{"FOO": "baz", "NEW": "1"}, c.ws.draft.cfg.Services["my-agent"].Environment)
	require.Contains(t, c.ws.draft.raw, "environment")
}

func TestServices_addAndRemove(t *testing.T) {
	_, c := servicesShell(t)
	wsToHeader(t, c, "services")
	c = cpress(c, "a")
	f := addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "add service · demo")
	c = typeText(c, "newsvc")
	c = wsPress(t, c, "enter")
	require.Contains(t, c.ws.draft.cfg.Services, "newsvc", "the bare service lands")
	require.Contains(t, c.ws.placeholderOrBody(), "newsvc")

	wsToKey(t, c, "newsvc", "services")
	c = cpress(c, "a")
	f = addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "add field · newsvc")
	c = cpress(c, "down")
	c = typeText(c, "/usr/bin/newsvc")
	c = wsPress(t, c, "enter")
	require.Equal(t, "/usr/bin/newsvc", c.ws.draft.cfg.Services["newsvc"].Command, "the field lands in the service")
	require.Contains(t, c.ws.draft.raw, "[services.newsvc]")

	wsToKey(t, c, "newsvc", "services")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1)
	c = cpress(c, "y")
	require.NotContains(t, c.ws.draft.cfg.Services, "newsvc", "y removes the service")
	require.NotContains(t, c.ws.placeholderOrBody(), "newsvc")
}
