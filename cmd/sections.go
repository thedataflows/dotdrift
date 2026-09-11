package cmd

import (
	"os"
	"slices"

	"github.com/alecthomas/kong"
	"github.com/thedataflows/dotdrift/internal/service"
)

// AfterApply captures the parse context so Run can read which section
// flags were explicitly set (positive or negated) — a plain bool field
// cannot distinguish `--hooks` from an untouched default.
func (c *ApplyCmd) AfterApply(kctx *kong.Context) error {
	c.kctx = kctx
	return nil
}

// resolveSections resolves the run's executed sections: the programmatic
// onlySections override when set (tests, library callers), otherwise the
// explicitly parsed section flags. DOTDRIFT_NO_HOOKS=1 subtracts hooks on
// top, exactly like --no-hooks (a kill-switch that never resurrects).
// Resolution and validation live in the service layer (single source,
// issue 0070); unknown names and an empty selection error before any
// output is produced.
func (c *ApplyCmd) resolveSections() (service.SectionSet, error) {
	explicit := map[string]bool{}
	if c.onlySections != nil {
		for _, n := range c.onlySections {
			explicit[n] = true
		}
	} else {
		if c.kctx != nil {
			for _, fl := range c.kctx.Flags() {
				if fl.Flag == nil || !fl.Set {
					continue
				}
				on, ok := fl.Target.Interface().(bool)
				if !ok || !slices.Contains(service.SectionNames, fl.Name) {
					continue
				}
				explicit[fl.Name] = on
			}
		}
		if os.Getenv("DOTDRIFT_NO_HOOKS") == "1" {
			explicit["hooks"] = false
		}
	}
	return service.ResolveSections(explicit)
}
