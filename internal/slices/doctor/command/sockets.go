package command

import (
	"context"
	"time"

	sockets "github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// socketsBound is the engine inspection bound, which detection runs inside.
const socketsBound = 5 * time.Second

// socketsReport is the confirmed backend, exactly one member.
type socketsReport struct {
	Backend string `json:"backend"`
}

// socketsOutcome is one detection's result: a confirmed backend with its
// plain line, or the one problem a typed failure produces. Never both.
type socketsOutcome struct {
	report  *socketsReport
	line    string
	problem *readinessProblem
}

// socketsProblems fixes each typed detection failure's rule, exit and subject.
// Messages carry no errno text, daemon text or runtime arguments.
var socketsProblems = map[sockets.Failure]readinessProblem{
	sockets.ABIQuery: {Rule: "doctor.sockets.kernel", Exit: 75, Field: "landlock_abi",
		Message: "The kernel's Landlock ABI could not be read",
		Fix:     "have the host operator check the kernel's Landlock support before inspecting again"},
	sockets.RuntimeInspect: {Rule: "doctor.sockets.runtimes", Exit: 75, Field: "runtimes",
		Message: "The engine's runtime inventory could not be read",
		Fix:     "have the host operator check engine health and its runtime list before inspecting again"},
	sockets.RuntimeRegistration: {Rule: "doctor.sockets.runsc", Exit: 1, Field: "runtimes.runsc",
		Message: "The engine's runsc runtime is not identifiable gVisor that keeps host sockets unreachable",
		Fix:     "register runsc by path with its default host socket policy, or remove it from the engine"},
}

// detectSockets names the checkout socket backend once the checkout and engine
// inspections have succeeded, whatever the image outcome. A refusal earlier
// leaves no kernel query and no outcome. Naming a backend is not enforcement.
func (c CLI) detectSockets(result port.Result) *socketsOutcome {
	if c.Sockets == nil || !detectable(result) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), socketsBound)
	defer cancel()
	selection := c.Sockets.Detect(ctx, result.Checkout.Root)
	if selection.Confirmed() {
		return confirmed(selection.Backend)
	}
	if selection.Failure == sockets.Cancelled {
		p := readinessDiagnostic(port.Issue{Kind: port.EngineTimeout, Subject: "DOCKER_HOST"})
		return &socketsOutcome{problem: &p}
	}
	// Unsupported, NoProbe, ABIInvalid and a zero selection cannot reach a
	// correctly assembled doctor; each is an unreadable kernel answer.
	p, known := socketsProblems[selection.Failure]
	if !known {
		p = socketsProblems[sockets.ABIQuery]
	}
	return &socketsOutcome{problem: &p}
}

func confirmed(backend sockets.Backend) *socketsOutcome {
	switch backend {
	case sockets.Landlock:
		return &socketsOutcome{report: &socketsReport{"landlock"}, line: "checkout socket backend: landlock"}
	case sockets.GVisor:
		return &socketsOutcome{report: &socketsReport{"gvisor"}, line: "checkout socket backend: gvisor"}
	}
	return &socketsOutcome{report: &socketsReport{"neither"}, line: "checkout socket backend: neither; late host sockets reachable"}
}

// detectable requires a validated checkout and engine and no later checkout,
// host or engine refusal: only image outcomes may remain.
func detectable(result port.Result) bool {
	if result.Checkout == nil || result.Engine == nil {
		return false
	}
	for _, issue := range result.Issues {
		switch issue.Kind {
		case port.ImageMissing, port.ImageInspect, port.ImageDigest, port.ImagePlatform, port.ImageUser, port.EgressImage, port.EgressImageMissing:
		default:
			return false
		}
	}
	return true
}
