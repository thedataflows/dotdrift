package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/thedataflows/dotdrift/internal/generate"
)

// huh forms for the individual wizard steps, shared by the volume,
// network, and target paths.

// promptTarget runs step 1: layer select, module id input, and the
// hostname/username inputs shown only for the matching layer.
func promptTarget(flagLayer, flagModule, defaultModule, flagHostname, flagUsername string) (generate.Selection, error) {
	layer := flagLayer
	if layer == "" {
		layer = generate.LayerBase
	}
	module := flagModule
	if module == "" {
		module = defaultModule
	}
	hostname := flagHostname
	username := flagUsername
	if hostname == "" {
		if h, err := os.Hostname(); err == nil {
			hostname = h
		}
	}
	if username == "" {
		if _, _, u, err := InvokingUser(); err == nil {
			username = u
		}
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Target layer").
				Options(
					huh.NewOption("base — applies to every host and user", generate.LayerBase),
					huh.NewOption("host — one machine", generate.LayerHost),
					huh.NewOption("user — one account", generate.LayerUser),
				).
				Value(&layer),
			huh.NewInput().
				Title("Module id").
				Value(&module).
				Validate(nonEmpty("module id")),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Hostname").
				Description("hosts/<hostname>/modules/").
				Value(&hostname).
				Validate(nonEmpty("hostname")),
		).WithHideFunc(func() bool { return layer != generate.LayerHost }),
		huh.NewGroup(
			huh.NewInput().
				Title("Username").
				Description("users/<username>/modules/").
				Value(&username).
				Validate(nonEmpty("username")),
		).WithHideFunc(func() bool { return layer != generate.LayerUser }),
	)
	if err := form.Run(); err != nil {
		return generate.Selection{}, err
	}
	return generate.Selection{
		Layer: layer, ModuleID: module, Hostname: hostname, Username: username,
	}, nil
}

// promptTypeSelect runs the registry type select for one kind, with
// *typ preselected. recommended names the kernel-recommended entry
// (volume path only); its option is marked "(recommended)".
func promptTypeSelect(reg *generate.Registry, kind string, typ *string, recommended string) error {
	var options []huh.Option[string]
	for _, e := range reg.Entries() {
		if e.Kind != kind {
			continue
		}
		label := e.Type
		if e.Type == recommended && recommended != "" {
			label += " (recommended)"
		}
		options = append(options, huh.NewOption(label, e.Type))
	}
	if len(options) == 0 {
		return fmt.Errorf("no registry entries for kind %q", kind)
	}
	if *typ == "" {
		*typ = options[0].Value
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Filesystem type").
			Options(options...).
			Value(typ),
	)).Run()
}

// promptOptionsScheduleState runs steps 4–6's input side: the options
// MultiSelect (registry preset pre-checked) plus free-form additions,
// the optional schedule, and the state select.
func promptOptionsScheduleState(reg *generate.Registry, choice MountChoice) (MountChoice, error) {
	preset, ok := reg.Entry(choice.Type)
	if !ok {
		return MountChoice{}, fmt.Errorf("unknown filesystem type %q", choice.Type)
	}

	selected := append([]string(nil), preset.Options...)
	if choice.Options != nil {
		selected = append([]string(nil), choice.Options...)
	}
	var additions string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Mount options (registry preset pre-checked)").
			Options(huh.NewOptions(preset.Options...)...).
			Value(&selected),
		huh.NewInput().
			Title("Additional options (comma-separated, optional)").
			Value(&additions),
	)).Run(); err != nil {
		return MountChoice{}, err
	}
	final := selected
	if additions != "" {
		for _, a := range strings.Split(additions, ",") {
			if a = strings.TrimSpace(a); a != "" {
				final = append(final, a)
			}
		}
	}
	// Accepting the preset unchanged keeps Options nil, so the registry
	// preset applies — identical to the CLI with no --option.
	if !slices.Equal(final, preset.Options) {
		choice.Options = final
	} else {
		choice.Options = nil
	}

	schedule := choice.StartAt != ""
	startAt := choice.StartAt
	state := defaultState(choice.State)
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add a schedule? (systemd .timer/.service pair)").
				Value(&schedule),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("OnCalendar expression").
				Placeholder("*-*-* 18:05:00").
				Value(&startAt).
				Validate(nonEmpty("OnCalendar expression")),
		).WithHideFunc(func() bool { return !schedule }),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("State").
				Options(
					huh.NewOption("enabled", "enabled"),
					huh.NewOption("disabled", "disabled"),
				).
				Value(&state),
		),
	).Run(); err != nil {
		return MountChoice{}, err
	}
	if schedule {
		choice.StartAt = startAt
	} else {
		choice.StartAt = ""
	}
	choice.State = state
	return choice, nil
}

// confirmMount renders the review screen and asks whether to keep the
// mount.
func confirmMount(c MountChoice) (bool, error) {
	pairs := []KV{
		{Key: "name", Value: c.Name},
		{Key: "source", Value: c.Source},
		{Key: "destination", Value: c.Destination},
		{Key: "type", Value: c.Type},
	}
	if len(c.Options) > 0 {
		pairs = append(pairs, KV{Key: "options", Value: strings.Join(c.Options, ",")})
	} else {
		pairs = append(pairs, KV{Key: "options", Value: "(registry preset)", Muted: true})
	}
	if c.StartAt != "" {
		pairs = append(pairs, KV{Key: "schedule", Value: c.StartAt})
	}
	pairs = append(pairs, KV{Key: "state", Value: defaultState(c.State)})

	keep := true
	if err := huh.NewForm(huh.NewGroup(
		huh.NewNote().Title("Review mount").Description(renderKV(pairs)),
		huh.NewConfirm().Title("Keep this mount?").Value(&keep),
	)).Run(); err != nil {
		return false, err
	}
	return keep, nil
}

