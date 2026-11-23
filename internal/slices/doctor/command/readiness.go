package command

import (
	"fmt"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"slices"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

type checkoutReport struct {
	Root      string `json:"root"`
	Device    string `json:"device"`
	Inode     string `json:"inode"`
	GitDevice string `json:"git_device"`
	GitInode  string `json:"git_inode"`
}
type engineReport struct {
	Endpoint     string `json:"endpoint"`
	ID           string `json:"id"`
	APIVersion   string `json:"api_version"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}
type imageReport struct {
	Reference    string `json:"reference"`
	ID           string `json:"id"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}
type inspectionReport struct {
	Ready    bool            `json:"ready"`
	Checkout *checkoutReport `json:"checkout,omitempty"`
	Engine   *engineReport   `json:"engine,omitempty"`
	Image    *imageReport    `json:"image,omitempty"`
	// CheckoutSockets is the confirmed backend; a failed detection omits it.
	CheckoutSockets *socketsReport `json:"checkout_sockets,omitempty"`
}
type readinessProblem = port.Problem

func presentReadiness(u *cli.UI, result port.Result, withJSON bool, sockets *socketsOutcome) int {
	return 0
}

func presentApprovalReadiness(u *cli.UI, view approval.View, presentation approval.Presentation, withJSON bool, sockets *socketsOutcome) int {
	return 0
}

func present(u *cli.UI, result port.Result, withJSON bool, approvalReport *approval.Report, approvalProblems []approval.Problem, sockets *socketsOutcome) int {
	return 0
}

func readinessDiagnostic(issue port.Issue) readinessProblem { return port.Diagnostic(issue) }

// Blank references keep the declarations used while the bodies are stubs.
var (
	_ = fmt.Sprint
	_ = slices.Equal[[]string, string]
	_ = strconv.Quote
	_ = strings.Compare
	_ = presentReadiness
	_ = presentApprovalReadiness
)
