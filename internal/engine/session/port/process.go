package port

const (
	DetectorVerified = "verified"
	DetectorExited   = "not observed: worker exited"
	DetectorRefused  = "refused"
)

func ValidDetector(value string) bool {
	return value == "" || value == DetectorVerified || value == DetectorExited || value == DetectorRefused
}

// Process holds a stable reference to the inspected worker init process.
// Exited observes that reference, never a newly resolved numeric PID.
type Process interface {
	Exited() (bool, Outcome)
	Close()
}

type ProcessWorkspace interface {
	HoldProcess(int) (Process, Outcome)
}
