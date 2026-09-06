package cmd

import (
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// warnSuperuserOverlays emits one warning when the profile holds modules under
// superuser-owned user layers that this run cannot select (issue 0029). The
// canonical surfacing is the modules command's skipped list; plan, status, and
// apply only get the nudge. Restore reads backup indexes, not the selection,
// and stays quiet.
func warnSuperuserOverlays(p *profile.Profile) {
	n := 0
	for _, s := range p.Skipped {
		if s.Reason == profile.ReasonSuperuserOverlay {
			n++
		}
	}
	if n == 0 {
		return
	}
	log.Warn().Int("modules", n).Msg("superuser-owned user overlays skipped; re-run with sudo to select them")
}

// warnMisplacedModules emits one warning when the profile holds module dirs
// missing the modules/ level (issue 0033): they are invisible to discovery
// and selection, so the skip list entry alone may never be seen. Same
// placement as warnSuperuserOverlays.
func warnMisplacedModules(p *profile.Profile) {
	n := 0
	for _, s := range p.Skipped {
		if strings.HasPrefix(s.Reason, profile.ReasonMisplacedModule+":") {
			n++
		}
	}
	if n == 0 {
		return
	}
	log.Warn().Int("modules", n).Msg("misplaced module dirs skipped; module.toml must live under <layer>/modules/ — see dotdrift modules")
}
