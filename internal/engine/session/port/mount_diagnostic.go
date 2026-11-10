package port

// MountSystemError contains only a syscall errno and its standard description,
// never the original error, pathname, response body or workload payload.
type MountSystemError struct {
	Errno   int
	Message string
}

// MountErrorSource reads only the most recent observation's in-memory diagnostic,
// without I/O or waiting. Callers serialize the read with the observation; it
// grants no resource authority.
type MountErrorSource interface{ MountError() MountSystemError }

type MountDiagnostic struct {
	Check, Reason                 string
	ExpectedDevice, ExpectedInode string
	ObservedDevice, ObservedInode string
	PID, ObservedPID              int
	System                        MountSystemError
}
