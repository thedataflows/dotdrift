package tui

import (
	"context"
	"errors"
	"os"
	"slices"

	"github.com/charmbracelet/huh"
	"github.com/thedataflows/dotdrift/internal/generate"
)

// RunMountsWizard runs the interactive mounts wizard: tab chrome, the
// target steps, the per-mount loop, and a single WriteModule at the
// end. Aborting (ctrl+c/esc) exits cleanly with nothing written.
func RunMountsWizard(ctx context.Context, p MountsParams) error {
	tab, err := switchTab("mounts")
	if err != nil || tab == "quit" {
		return err
	}
	if tab == "smb" {
		return RunSmbWizard(ctx, smbParamsFromMounts(p))
	}
	return runMountsFlow(ctx, p)
}

// switchTab renders the tab bar and runs the tab switcher; a user
// abort maps to a clean quit (nil, nil is returned as "quit").
func switchTab(active string) (string, error) {
	renderTabBar(os.Stderr, active)
	tab, err := selectTab(active)
	if errors.Is(err, huh.ErrUserAborted) {
		return "quit", nil
	}
	if err != nil {
		return "", err
	}
	return tab, nil
}

// runMountsFlow is the mounts wizard body once the mounts tab is chosen.
func runMountsFlow(ctx context.Context, p MountsParams) error {
	reg, err := generate.Load()
	if err != nil {
		return err
	}

	sel, err := promptTarget(p.Layer, p.ModuleID, "mounts", p.Hostname, p.Username)
	if err != nil {
		return friendlyAbort(err)
	}

	wizard := NewMountsWizard(sel, reg)
	defaults := p.Prefill()
	for {
		choices, err := promptMount(ctx, reg, p.Profile, sel, defaults)
		if err != nil {
			return friendlyAbort(err)
		}
		for _, choice := range choices {
			save, err := confirmMount(choice)
			if err != nil {
				return friendlyAbort(err)
			}
			if save {
				if err := wizard.AddMount(choice); err != nil {
					return err
				}
			}
		}
		another, err := confirm("Add another mount?", false)
		if err != nil {
			return friendlyAbort(err)
		}
		if !another {
			break
		}
		defaults = MountChoice{}
	}

	if len(wizard.Mounts()) == 0 {
		warnf(os.Stderr, "no mounts configured; nothing written")
		return nil
	}
	uid, gid, _, err := InvokingUser()
	if err != nil {
		return err
	}
	if err := wizard.Write(p.Profile, uid, gid); err != nil {
		return err
	}
	return PrintSummary(os.Stderr, p.Profile, sel)
}

// promptMount runs one mounts-loop iteration (steps 2–5) and returns
// the assembled choices — one per picked volume, or a single one for
// the network/manual paths.
func promptMount(ctx context.Context, reg *generate.Registry, root string, sel generate.Selection, defaults MountChoice) ([]MountChoice, error) {
	kind := kindForDefaults(reg, defaults)
	kindForm := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Mount kind").
			Options(
				huh.NewOption("volume — local block device", generate.KindVolume),
				huh.NewOption("network — remote export (nfs, cifs)", generate.KindNetwork),
			).
			Value(&kind),
	))
	if err := kindForm.Run(); err != nil {
		return nil, err
	}
	if kind == generate.KindNetwork {
		return promptNetworkMount(reg, defaults)
	}
	return promptVolumeMounts(ctx, reg, root, sel, defaults)
}

// promptVolumeMounts runs the volume path: lsblk detection (with a
// manual-source fallback), the volume picker, and the per-volume forms.
func promptVolumeMounts(ctx context.Context, reg *generate.Registry, root string, sel generate.Selection, defaults MountChoice) ([]MountChoice, error) {
	sources, err := ExistingMountSources(root, sel)
	if err != nil {
		return nil, err
	}
	vols, err := generate.Volumes(ctx, reg, sources)
	if err != nil {
		errorf(os.Stderr, "detecting volumes failed: %v", err)
		fallback, cerr := confirm("Fall back to a manual source input?", true)
		if cerr != nil {
			return nil, cerr
		}
		if volumePathAction(err, 0, fallback) == volumeFail {
			return nil, err
		}
		return promptManualVolume(reg, defaults)
	}
	if volumePathAction(nil, len(vols), false) == volumeManual {
		warnf(os.Stderr, "no volumes detected; falling back to a manual source input")
		return promptManualVolume(reg, defaults)
	}

	choices := VolumeChoices(reg, vols)
	options := make([]huh.Option[string], len(choices))
	for i, c := range choices {
		options[i] = huh.NewOption(c.Label(), c.Volume.UUID)
	}
	var picked []string
	pickForm := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Volumes to mount (space to toggle)").
			Options(options...).
			Value(&picked),
	))
	if err := pickForm.Run(); err != nil {
		return nil, err
	}
	if len(picked) == 0 {
		return nil, errors.New("no volumes selected")
	}

	out := make([]MountChoice, 0, len(picked))
	for i, uuid := range picked {
		c := choices[slices.IndexFunc(choices, func(c VolumeChoice) bool { return c.Volume.UUID == uuid })]
		choice, err := promptMountDetails(reg, c, prefillForVolume(c, defaults, i == 0))
		if err != nil {
			return nil, err
		}
		out = append(out, choice)
	}
	return out, nil
}

// promptManualVolume is the volume fallback: free-form source input and
// a type select over the registry's volume entries.
func promptManualVolume(reg *generate.Registry, defaults MountChoice) ([]MountChoice, error) {
	choice := defaults
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Mount source (e.g. UUID=<uuid> or /dev/sdXN)").
			Value(&choice.Source).
			Validate(nonEmpty("source")),
	)).Run(); err != nil {
		return nil, err
	}
	details, err := promptMountDetails(reg, VolumeChoice{}, choice)
	if err != nil {
		return nil, err
	}
	return []MountChoice{details}, nil
}

// promptNetworkMount runs the network path (step 3b).
func promptNetworkMount(reg *generate.Registry, defaults MountChoice) ([]MountChoice, error) {
	choice := defaults
	name := choice.Name
	source := choice.Source
	destination := choice.Destination
	typ := choice.Type
	if typ == "" {
		typ = "nfs"
	}
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Mount name").Value(&name).Validate(nonEmpty("name")),
			huh.NewInput().
				Title("Network source").
				Placeholder("server:/export or //server/share").
				Value(&source).
				Validate(ValidateNetworkSource),
			huh.NewInput().
				Title("Destination").
				Placeholder("/mnt/<name>").
				Value(&destination).
				Validate(nonEmpty("destination")),
		),
	).Run(); err != nil {
		return nil, err
	}
	choice.Name, choice.Source, choice.Destination = name, source, destination
	if err := promptTypeSelect(reg, generate.KindNetwork, &typ, ""); err != nil {
		return nil, err
	}
	choice.Type = typ
	final, err := promptOptionsScheduleState(reg, choice)
	if err != nil {
		return nil, err
	}
	return []MountChoice{final}, nil
}

// promptMountDetails runs the per-volume form: name, type (recommended
// preselected), destination, then options/schedule/state.
func promptMountDetails(reg *generate.Registry, c VolumeChoice, choice MountChoice) (MountChoice, error) {
	name := choice.Name
	destination := choice.Destination
	typ := choice.Type
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Mount name").Value(&name).Validate(nonEmpty("name")),
		huh.NewInput().Title("Destination").Value(&destination).Validate(nonEmpty("destination")),
	)).Run(); err != nil {
		return MountChoice{}, err
	}
	if err := promptTypeSelect(reg, generate.KindVolume, &typ, c.Type); err != nil {
		return MountChoice{}, err
	}
	choice.Name, choice.Destination, choice.Type = name, destination, typ
	return promptOptionsScheduleState(reg, choice)
}

