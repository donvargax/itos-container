package domain

import (
	"context"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port/porttest"
)

var linuxAmd64 = port.Platform{OS: "linux", Architecture: "amd64"}

func TestBackendSelection(t *testing.T) {
	for _, c := range []struct {
		name         string
		platform     *port.Platform
		cancelled    bool
		abi          port.ABI
		abiFailure   port.Failure
		runtime      port.Registration
		runtimeFail  port.Failure
		want         port.Selection
		kernelCalls  int
		runtimeCalls int
	}{
		{name: "ABI 8 with gVisor", abi: port.ABI{Level: 8}, runtime: port.RunscRegistered, want: port.Selection{Backend: port.GVisor}, kernelCalls: 1, runtimeCalls: 1},
		{name: "ABI 8 alone", abi: port.ABI{Level: 8}, runtime: port.RunscAbsent, want: port.Selection{Backend: port.Neither}, kernelCalls: 1, runtimeCalls: 1},
		{name: "ABI 9 with gVisor", abi: port.ABI{Level: 9}, runtime: port.RunscRegistered, want: port.Selection{Backend: port.Landlock}, kernelCalls: 1},
		{name: "ABI 9 alone", abi: port.ABI{Level: 9}, runtime: port.RunscAbsent, want: port.Selection{Backend: port.Landlock}, kernelCalls: 1},
		{name: "newer ABI with gVisor", abi: port.ABI{Level: 12}, runtime: port.RunscRegistered, want: port.Selection{Backend: port.Landlock}, kernelCalls: 1},
		{name: "newer ABI ignores a failing lookup", abi: port.ABI{Level: 10}, runtimeFail: port.RuntimeInspect, want: port.Selection{Backend: port.Landlock}, kernelCalls: 1},
		{name: "ABI 1 falls back to gVisor", abi: port.ABI{Level: 1}, runtime: port.RunscRegistered, want: port.Selection{Backend: port.GVisor}, kernelCalls: 1, runtimeCalls: 1},
		{name: "absent Landlock with gVisor", abi: port.ABI{Absent: true}, runtime: port.RunscRegistered, want: port.Selection{Backend: port.GVisor}, kernelCalls: 1, runtimeCalls: 1},
		{name: "neither", abi: port.ABI{Absent: true}, runtime: port.RunscAbsent, want: port.Selection{Backend: port.Neither}, kernelCalls: 1, runtimeCalls: 1},
		{name: "ABI failure stops with gVisor registered", abiFailure: port.ABIQuery, runtime: port.RunscRegistered, want: port.Selection{Failure: port.ABIQuery}, kernelCalls: 1},
		{name: "invalid ABI failure", abiFailure: port.ABIInvalid, runtime: port.RunscRegistered, want: port.Selection{Failure: port.ABIInvalid}, kernelCalls: 1},
		{name: "zero ABI is not an answer", runtime: port.RunscRegistered, want: port.Selection{Failure: port.ABIInvalid}, kernelCalls: 1},
		{name: "negative ABI", abi: port.ABI{Level: -1}, runtime: port.RunscRegistered, want: port.Selection{Failure: port.ABIInvalid}, kernelCalls: 1},
		{name: "absence with a level contradicts", abi: port.ABI{Level: 3, Absent: true}, runtime: port.RunscRegistered, want: port.Selection{Failure: port.ABIInvalid}, kernelCalls: 1},
		{name: "failed lookup after ABI 8", abi: port.ABI{Level: 8}, runtimeFail: port.RuntimeInspect, want: port.Selection{Failure: port.RuntimeInspect}, kernelCalls: 1, runtimeCalls: 1},
		{name: "unrecognized registration", abi: port.ABI{Absent: true}, runtimeFail: port.RuntimeRegistration, want: port.Selection{Failure: port.RuntimeRegistration}, kernelCalls: 1, runtimeCalls: 1},
		{name: "failed lookup with a registration", abi: port.ABI{Level: 8}, runtime: port.RunscRegistered, runtimeFail: port.RuntimeInspect, want: port.Selection{Failure: port.RuntimeInspect}, kernelCalls: 1, runtimeCalls: 1},
		{name: "unknown registration cannot certify absence", abi: port.ABI{Level: 8}, want: port.Selection{Failure: port.RuntimeInspect}, kernelCalls: 1, runtimeCalls: 1},
		{name: "unsupported architecture", platform: &port.Platform{OS: "linux", Architecture: "arm64"}, abi: port.ABI{Level: 9}, runtime: port.RunscRegistered, want: port.Selection{Failure: port.Unsupported}},
		{name: "unsupported system", platform: &port.Platform{OS: "darwin", Architecture: "amd64"}, abi: port.ABI{Level: 9}, runtime: port.RunscRegistered, want: port.Selection{Failure: port.Unsupported}},
		{name: "unset platform", platform: &port.Platform{}, abi: port.ABI{Level: 9}, want: port.Selection{Failure: port.Unsupported}},
		{name: "cancelled before either probe", cancelled: true, abi: port.ABI{Level: 9}, runtime: port.RunscRegistered, want: port.Selection{Failure: port.Cancelled}},
	} {
		t.Run(c.name, func(t *testing.T) {
			kernel := &porttest.Kernel{ABI: c.abi, Failure: c.abiFailure}
			runtimes := &porttest.Runtimes{Registration: c.runtime, Failure: c.runtimeFail}
			platform := linuxAmd64
			if c.platform != nil {
				platform = *c.platform
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if c.cancelled {
				cancel()
			}
			got := Detector{Kernel: kernel, Runtimes: runtimes}.Detect(ctx, platform)
			if got != c.want {
				t.Errorf("selection = %+v, want %+v", got, c.want)
			}
			if kernel.Calls != c.kernelCalls || runtimes.Calls != c.runtimeCalls {
				t.Errorf("probes ran kernel %d, runtimes %d; want %d, %d", kernel.Calls, runtimes.Calls, c.kernelCalls, c.runtimeCalls)
			}
		})
	}
}

func TestDetectionObservesAfresh(t *testing.T) {
	kernel, runtimes := &porttest.Kernel{}, &porttest.Runtimes{}
	d := Detector{Kernel: kernel, Runtimes: runtimes}
	for i, step := range []struct {
		abi         port.ABI
		abiFailure  port.Failure
		runtime     port.Registration
		runtimeFail port.Failure
		want        port.Selection
	}{
		{abi: port.ABI{Level: 9}, runtime: port.RunscAbsent, want: port.Selection{Backend: port.Landlock}},
		{abi: port.ABI{Level: 8}, runtime: port.RunscRegistered, want: port.Selection{Backend: port.GVisor}},
		{abi: port.ABI{Absent: true}, runtime: port.RunscAbsent, want: port.Selection{Backend: port.Neither}},
		{abi: port.ABI{Level: 8}, runtimeFail: port.RuntimeInspect, want: port.Selection{Failure: port.RuntimeInspect}},
		{abiFailure: port.ABIQuery, runtime: port.RunscRegistered, want: port.Selection{Failure: port.ABIQuery}},
		{abi: port.ABI{Level: 11}, want: port.Selection{Backend: port.Landlock}},
		{abi: port.ABI{Level: 2}, runtime: port.RunscRegistered, want: port.Selection{Backend: port.GVisor}},
	} {
		kernel.ABI, kernel.Failure = step.abi, step.abiFailure
		runtimes.Registration, runtimes.Failure = step.runtime, step.runtimeFail
		if got := d.Detect(context.Background(), linuxAmd64); got != step.want {
			t.Errorf("detection %d = %+v, want %+v", i, got, step.want)
		}
	}
}

func TestDetectorWithoutObservationsCertifiesNothing(t *testing.T) {
	kernel, runtimes := &porttest.Kernel{ABI: port.ABI{Absent: true}}, &porttest.Runtimes{Registration: port.RunscAbsent}
	for _, c := range []struct {
		name string
		d    Detector
	}{
		{"zero value", Detector{}},
		{"no kernel", Detector{Runtimes: runtimes}},
		{"no runtimes", Detector{Kernel: kernel}},
	} {
		if got := c.d.Detect(context.Background(), linuxAmd64); got != (port.Selection{Failure: port.NoProbe}) {
			t.Errorf("%s: selection = %+v", c.name, got)
		}
	}
	if kernel.Calls != 0 || runtimes.Calls != 0 {
		t.Errorf("a half-built detector probed: kernel %d, runtimes %d", kernel.Calls, runtimes.Calls)
	}
}
