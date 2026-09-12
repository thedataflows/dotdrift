package tui

// volumeAction decides the volume path after lsblk: use the detected
// list, fall back to manual source input, or propagate the failure.
// Front-end policy for the wizard's volume prompt; it dies with the huh
// front-end (issue 0066, task 3) and its successor lives with the
// editors (issue 0065).

type volumeAction int

const (
	volumeList volumeAction = iota
	volumeManual
	volumeFail
)

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
