package main

import (
	"strings"
	"testing"
	"testing/synctest"
)

func TestTerminalFaultResults(t *testing.T) {
	for _, tc := range []struct {
		fault, child, rule string
		code               int
	}{
		{"terminal observation fails", "unknown", "run.terminal-unavailable", 75},
		{"resized terminal has zero rows", "17", "run.terminal-unavailable", 17},
		{"running worker resize fails", "0", "run.terminal-resize", 75},
		{"running worker resize times out", "unknown", "run.call-timeout", 75},
		{"terminal output blocks during exit", "17", "run.stream-timeout", 17},
		{"terminal input read fails", "unknown", "run.execution-unavailable", 75},
		{"saved terminal restoration fails", "0", "run.terminal-restore", 75},
		{"saved terminal restoration fails", "17", "run.terminal-restore", 17},
		{"distinct stdin restoration fails", "0", "run.terminal-restore", 75},
		{"distinct stdin restoration fails", "17", "run.terminal-restore", 17},
		{"distinct stdout restoration fails", "0", "run.terminal-restore", 75},
		{"distinct stdout restoration fails", "17", "run.terminal-restore", 17},
	} {
		t.Run(tc.fault+tc.child, func(t *testing.T) {
			for _, backend := range []string{"ports", "protected records"} {
				t.Run(backend, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						r := terminalContractCase(t, backend, "fault", tc.fault, tc.child)
						if r.Code != tc.code || !r.Ready || !r.TTY || !r.ObserversEnded || r.Error != "" || !strings.Contains(r.Diagnostic, tc.rule) {
							t.Fatalf("terminal failure: code=%d ready=%v tty=%v ended=%v error=%s diagnostic=%s", r.Code, r.Ready, r.TTY, r.ObserversEnded, r.Error, r.Diagnostic)
						}
					})
				})
			}
		})
	}
}

func TestTerminalDisconnectResults(t *testing.T) {
	for _, tc := range []struct {
		condition, child string
		code             int
	}{
		{"cleanup succeeds without a wait", "unknown", 75},
		{"primary exits zero during stop", "0", 75},
		{"owned removal fails", "17", 17},
		{"engine becomes unreachable", "unknown", 75},
		{"inspected ownership changes", "unknown", 75},
	} {
		t.Run(tc.condition, func(t *testing.T) {
			for _, backend := range []string{"ports", "protected records"} {
				t.Run(backend, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						r := terminalContractCase(t, backend, "disconnect", tc.condition, tc.child)
						if r.Code != tc.code || !r.Ready || !r.Fired || !r.ObserversEnded || !strings.Contains(r.Diagnostic, "run.terminal-disconnected") || r.RecoveryRestarted {
							t.Fatalf("disconnect: code=%d ready=%v fired=%v ended=%v restarted=%v diagnostic=%s", r.Code, r.Ready, r.Fired, r.ObserversEnded, r.RecoveryRestarted, r.Diagnostic)
						}
						if tc.condition == "engine becomes unreachable" && r.Result.Cleanup == "cleanup-complete" {
							t.Fatal("unreachable engine was reported as confirmed absent")
						}
					})
				})
			}
		})
	}
}
