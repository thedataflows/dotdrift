package tui

import (
	"github.com/thedataflows/dotdrift/internal/generate"
)

// kindForDefaults preselects the mount kind for the wizard from the flag
// prefill: an explicit type decides by registry kind, otherwise a
// network-shaped source implies network, else volume.
func kindForDefaults(reg *generate.Registry, defaults MountChoice) string {
	if e, ok := reg.Entry(defaults.Type); ok {
		return e.Kind
	}
	if defaults.Source != "" && ValidateNetworkSource(defaults.Source) == nil {
		return generate.KindNetwork
	}
	return generate.KindVolume
}

type volumeAction int

const (
	volumeList volumeAction = iota
	volumeManual
	volumeFail
)

// volumePathAction decides the volume path after lsblk: use the detected
// list, fall back to manual source input, or propagate the failure.
func volumePathAction(detectErr error, volCount int, fallbackConfirmed bool) volumeAction {
	if detectErr != nil {
		if fallbackConfirmed {
			return volumeManual
		}
		return volumeFail
	}
	if volCount == 0 {
		return volumeManual
	}
	return volumeList
}

// prefillForVolume merges flag prefill into a picked volume's defaults:
// the first picked volume takes the flag values with empty fields filled
// from the volume; later volumes always use their own. Source always
// comes from the picked volume.
func prefillForVolume(c VolumeChoice, defaults MountChoice, first bool) MountChoice {
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

// shareLoopDone reports whether the share loop ends: declining to add
// another share ends it only when at least one share exists (CLI parity:
// a shareless smb module is not writable).
func shareLoopDone(addAnother bool, shareCount int) bool {
	return !addAnother && shareCount > 0
}

// smbParamsFromMounts carries the target selection across a mounts → smb
// tab switch; the smb wizard's own default module id applies.
func smbParamsFromMounts(p MountsParams) SmbParams {
	return SmbParams{Profile: p.Profile, Layer: p.Layer, Hostname: p.Hostname, Username: p.Username}
}

// mountsParamsFromSmb carries the target selection across an smb →
// mounts tab switch; the mounts wizard's own default module id applies.
func mountsParamsFromSmb(p SmbParams) MountsParams {
	return MountsParams{Profile: p.Profile, Layer: p.Layer, Hostname: p.Hostname, Username: p.Username}
}
