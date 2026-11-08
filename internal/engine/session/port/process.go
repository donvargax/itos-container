package port

// ProcessAbsent means the inspected PID no longer names a process. It is a
// reason to reconcile with the engine, never proof of the owned worker's exit.
const ProcessAbsent Outcome = "process-absent"

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
	// HoldProcess returns ProcessAbsent only for observed process absence;
	// all other acquisition errors remain refusals.
	HoldProcess(int) (Process, Outcome)
}
