# Single palette for the wizard chrome and forms

The `dotdrift generate` wizard rendered two unrelated color palettes on the
same screen: the tab bar (`internal/tui/chrome.go`) hardcoded ANSI-256
colors 15/62/245/240 with bold purple, while the huh form directly below
it rendered with huh's default `ThemeCharm` (indigo/fuchsia). The three
review screens (`confirmMount`, `confirmShare`, `confirmSmbSpec`) and the
six stderr notice sites compounded this with three different hand-tuned
column widths and no severity styling.

We decided to consolidate all wizard chrome onto a single palette shared
with `huh.ThemeCharm`, exposed through a package-level registry in
`internal/tui/theme.go`. The palette uses `lipgloss.AdaptiveColor`
throughout so a light-background terminal picks the readable variant. The
registry holds one style per visible concept (active/inactive/quit tab,
label/value pair, warn/error/info notice); wizard files pick from this
list rather than spelling their own `lipgloss.NewStyle()` chains. Two
helpers — `renderKV([]KV)` for aligned key:value blocks and
`warnf`/`errorf`/`infof` for severity-prefixed notices — replace the
hand-rolled equivalents.

Why: the wizard is one screen; two palettes on one screen reads as broken
even when each is fine in isolation. Reusing `huh.ThemeCharm` rather than
building a custom huh theme keeps the change small (no `WithTheme` call
sites) and keeps the chrome consistent with whatever theme huh ships in
future upgrades. Centralizing styles in `theme.go` prevents the original
cause (chrome spelled its own styles inline while forms used a library
theme) from recurring.

Consequence: adding a new visible element now means adding a style to the
registry rather than reaching for `lipgloss.NewStyle()` in a wizard file.
The CLI text commands (`cmd/plan.go`, `cmd/status.go`, `cmd/detect.go`,
`cmd/modules.go`) deliberately remain plain `fmt.Fprintf` — they are
machine-parseable structured output (`plan --json` exists, exact-byte
tests assert their format), and styling would break those consumers. The
TUI is the only surface this decision covers. `PrintSummary` is also left
plain because `internal/tui/shared_test.go` asserts its exact bytes and
`cmd/generate.go` shares it with stdout in strict CLI mode.

When this changes: a future product decision to style the CLI output
end-to-end (e.g. a `--no-color` global flag plus styled default) should
extract `theme.go` into `internal/style/` so both `cmd/` and `internal/tui/`
can import it without `cmd/` depending on `internal/tui/`. The `Out io.Writer`
seam in `cmd/` already exists; the missing piece is a shared style package.
