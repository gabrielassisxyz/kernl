package prompt

type ExecutionBoundary struct {
	AllowedExitStates []string
	BeadID            string
	StepName          string
}

func FormatBoundary(b ExecutionBoundary) string {
	boundary := "KERNL EXECUTION BOUNDARY\n"
	boundary += "Bead: " + b.BeadID + "\n"
	boundary += "Step: " + b.StepName + "\n"
	boundary += "Allowed exit states: "
	for i, s := range b.AllowedExitStates {
		if i > 0 {
			boundary += ", "
		}
		boundary += s
	}
	boundary += "\n"
	boundary += "Complete exactly one workflow action, then stop.\n"
	return boundary
}
