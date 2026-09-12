package generate

// Flow decisions for the interactive producers, separated from any
// front-end so tests pin the policy (which kind to preselect, how flag
// prefill merges into volume defaults, when a shares loop ends).
// Exported: the TUI editors (issue 0065) reuse them across the
// tui -> generate seam.

// KindForDefaults preselects the mount kind from the flag prefill: an
// explicit type decides by registry kind, otherwise a network-shaped
// source implies network, else volume.
func KindForDefaults(reg *Registry, defaults MountChoice) string {
	if e, ok := reg.Entry(defaults.Type); ok {
		return e.Kind
	}
	if defaults.Source != "" && ValidateNetworkSource(defaults.Source) == nil {
		return KindNetwork
	}
	return KindVolume
}

// PrefillForVolume merges flag prefill into a picked volume's defaults:
// the first picked volume takes the flag values with empty fields filled
// from the volume; later volumes always use their own. Source always
// comes from the picked volume.
func PrefillForVolume(c VolumeChoice, defaults MountChoice, first bool) MountChoice {
	d := MountChoice{Name: c.Name, Destination: c.Destination, Type: c.Type}
	if first && defaults.Name != "" {
		d = defaults
		if d.Destination == "" {
			d.Destination = c.Destination
		}
		if d.Type == "" {
			d.Type = c.Type
		}
		if d.Name == "" {
			d.Name = c.Name
		}
	}
	d.Source = "UUID=" + c.Volume.UUID
	return d
}

// ShareLoopDone reports whether the share loop ends: declining to add
// another share ends it only when at least one share exists (CLI parity:
// a shareless smb module is not writable).
func ShareLoopDone(addAnother bool, shareCount int) bool {
	return !addAnother && shareCount > 0
}
