package porttest

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
)

// Reporter is the shared testing boundary, without production I/O.
type Reporter interface {
	Helper()
	Errorf(string, ...any)
}

// KernelAnswer names a controlled kernel reply in terms every implementation
// must report the same way.
type KernelAnswer uint8

const (
	// ReplyLevel is success with the ABI in KernelReply.Level.
	ReplyLevel KernelAnswer = iota
	// ReplyNotImplemented is ENOSYS: the kernel lacks Landlock.
	ReplyNotImplemented
	// ReplyDisabled is EOPNOTSUPP: the kernel has Landlock turned off.
	ReplyDisabled
	// ReplyDenied is EPERM, an error that is not an absence.
	ReplyDenied
	// ReplyNonPositive is success with a Level that is zero or negative.
	ReplyNonPositive
)

// KernelReply is one controlled answer to the next observation.
type KernelReply struct {
	Answer KernelAnswer
	Level  int
}

// KernelSubject is an implementation under test with the controls the contract
// needs. Reads counts the observations really made (queries the kernel saw).
type KernelSubject struct {
	Kernel port.Kernel
	Reply  func(KernelReply)
	Reads  func() int
}

// KernelContract holds the observable requirements for the memory fake and the
// native adapter alike. The factory returns an independent subject.
func KernelContract(t Reporter, create func() KernelSubject) {
	t.Helper()
	ctx := context.Background()
	for _, c := range []struct {
		name    string
		reply   KernelReply
		want    port.ABI
		failure port.Failure
	}{
		{"level 1", KernelReply{ReplyLevel, 1}, port.ABI{Level: 1}, port.None},
		{"level 8", KernelReply{ReplyLevel, 8}, port.ABI{Level: 8}, port.None},
		{"level 9", KernelReply{ReplyLevel, 9}, port.ABI{Level: 9}, port.None},
		{"level 12", KernelReply{ReplyLevel, 12}, port.ABI{Level: 12}, port.None},
		{"not implemented", KernelReply{Answer: ReplyNotImplemented}, port.ABI{Absent: true}, port.None},
		{"disabled", KernelReply{Answer: ReplyDisabled}, port.ABI{Absent: true}, port.None},
		{"denied", KernelReply{Answer: ReplyDenied}, port.ABI{}, port.ABIQuery},
		{"zero level", KernelReply{ReplyNonPositive, 0}, port.ABI{}, port.ABIInvalid},
		{"negative level", KernelReply{ReplyNonPositive, -1}, port.ABI{}, port.ABIInvalid},
	} {
		s := create()
		s.Reply(c.reply)
		abi, failure := s.Kernel.LandlockABI(ctx)
		if abi != c.want || failure != c.failure {
			t.Errorf("%s: observed %+v, %v; want %+v, %v", c.name, abi, failure, c.want, c.failure)
		}
		if s.Reads() != 1 {
			t.Errorf("%s: %d observations, want 1", c.name, s.Reads())
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		before := s.Reads()
		abi, failure = s.Kernel.LandlockABI(cancelled)
		if abi != (port.ABI{}) || failure != port.Cancelled || s.Reads() != before {
			t.Errorf("%s: cancelled call observed %+v, %v with %d new observations", c.name, abi, failure, s.Reads()-before)
		}
	}
	s := create()
	for i, step := range []struct {
		reply   KernelReply
		want    port.ABI
		failure port.Failure
	}{
		{KernelReply{ReplyLevel, 9}, port.ABI{Level: 9}, port.None},
		{KernelReply{Answer: ReplyDisabled}, port.ABI{Absent: true}, port.None},
		{KernelReply{ReplyLevel, 8}, port.ABI{Level: 8}, port.None},
		{KernelReply{Answer: ReplyDenied}, port.ABI{}, port.ABIQuery},
		{KernelReply{ReplyLevel, 10}, port.ABI{Level: 10}, port.None},
	} {
		s.Reply(step.reply)
		abi, failure := s.Kernel.LandlockABI(ctx)
		if abi != step.want || failure != step.failure || s.Reads() != i+1 {
			t.Errorf("observation %d reused an earlier answer: %+v, %v after %d queries", i, abi, failure, s.Reads())
		}
	}
}

// RuntimesReply is a controlled daemon state for the runtime named runsc.
type RuntimesReply uint8

const (
	// ReplyRegistered is a path-based registration that leaves host sockets unreachable.
	ReplyRegistered RuntimesReply = iota
	// ReplyAbsent is a reported inventory without runsc.
	ReplyAbsent
	// ReplyUnrecognized is a runsc entry whose identity or arguments are not established.
	ReplyUnrecognized
	// ReplyUnreachable is a daemon that fails the lookup.
	ReplyUnreachable
)

// RuntimesSubject is an implementation under test with the controls the
// contract needs. Reads counts the daemon requests really made.
type RuntimesSubject struct {
	Runtimes port.Runtimes
	Reply    func(RuntimesReply)
	Reads    func() int
}

// RuntimesContract holds the observable requirements for the memory fake and the
// daemon adapter alike. The factory returns an independent subject.
func RuntimesContract(t Reporter, create func() RuntimesSubject) {
	t.Helper()
	ctx := context.Background()
	for _, c := range []struct {
		name    string
		reply   RuntimesReply
		want    port.Registration
		failure port.Failure
	}{
		{"registered", ReplyRegistered, port.RunscRegistered, port.None},
		{"absent", ReplyAbsent, port.RunscAbsent, port.None},
		{"unrecognized", ReplyUnrecognized, port.RegistrationUnknown, port.RuntimeRegistration},
		{"unreachable", ReplyUnreachable, port.RegistrationUnknown, port.RuntimeInspect},
	} {
		s := create()
		s.Reply(c.reply)
		got, failure := s.Runtimes.Runsc(ctx)
		if got != c.want || failure != c.failure {
			t.Errorf("%s: observed %v, %v; want %v, %v", c.name, got, failure, c.want, c.failure)
		}
		if s.Reads() != 1 {
			t.Errorf("%s: %d requests, want 1", c.name, s.Reads())
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		before := s.Reads()
		got, failure = s.Runtimes.Runsc(cancelled)
		if got != port.RegistrationUnknown || failure != port.Cancelled || s.Reads() != before {
			t.Errorf("%s: cancelled call observed %v, %v with %d new requests", c.name, got, failure, s.Reads()-before)
		}
	}
	s := create()
	for i, step := range []struct {
		reply   RuntimesReply
		want    port.Registration
		failure port.Failure
	}{
		{ReplyRegistered, port.RunscRegistered, port.None},
		{ReplyAbsent, port.RunscAbsent, port.None},
		{ReplyUnreachable, port.RegistrationUnknown, port.RuntimeInspect},
		{ReplyRegistered, port.RunscRegistered, port.None},
	} {
		s.Reply(step.reply)
		got, failure := s.Runtimes.Runsc(ctx)
		if got != step.want || failure != step.failure || s.Reads() != i+1 {
			t.Errorf("observation %d reused an earlier answer: %v, %v after %d requests", i, got, failure, s.Reads())
		}
	}
}
