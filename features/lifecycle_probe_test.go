package features

func (w *world) buildLifecycleProbe() error {
	return w.installNativeProbe("slice-12-probe", func(path string) error {
		return recoveryBuildHelper(w.root, path, lifecycleProbeSource)
	})
}

// Compiled on the host, executed ONLY as the explicitly supplied native worker
// argv in guarded CI. Receipt and progress files are not product recovery data.
// The signal handler never waits for the deliberately blocked output writer.
const lifecycleProbeSource = `package main
import (
    "bytes"
    "fmt"
    "os"
    "os/signal"
    "strconv"
    "syscall"
    "time"
)
func put(name, value string) {
    path := "/workspace/slice-12-"+name
    if os.WriteFile(path+".next", []byte(value), 0600) != nil { os.Exit(96) }
    if os.Rename(path+".next", path) != nil { os.Exit(96) }
}
func main() {
    if len(os.Args) != 4 { os.Exit(96) }
    mode, condition := os.Args[1], os.Args[3]
    code, err := strconv.Atoi(os.Args[2]); if err != nil { os.Exit(96) }
    signals := make(chan os.Signal, 16)
    signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
    put("worker-edit", "slice-12 retained edit")
    f, err := os.CreateTemp(os.TempDir(), "slice-12-private-")
    if err != nil { os.Exit(96) }; f.Close()
    put("temporary", f.Name())
    fmt.Fprintln(os.Stdout, "slice-12:ready")
    put("ready", "ready")
    if mode == "exit" { fmt.Fprintln(os.Stdout, "slice-12:final"); os.Exit(code) }
    if condition == "unread workload stdout" || condition == "unread workload stderr" {
        go func() {
            for { if _, err := os.Stat("/workspace/slice-12-release-output"); err == nil { break }; time.Sleep(10*time.Millisecond) }
            put("fill-started", "started")
            sink := os.Stdout; if condition == "unread workload stderr" { sink = os.Stderr }
            block := bytes.Repeat([]byte("SLICE_12_SYNTHETIC_WORKLOAD_OUTPUT\n"), 32768)
            for n := 0; n < 128; n++ { if _, err := sink.Write(block); err != nil { return } }
            put("fill-finished", "finished")
        }()
    }
    ticker := time.NewTicker(50*time.Millisecond)
    n := 0
    receipt, err := os.OpenFile("/workspace/slice-12-receipts", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
    if err != nil { os.Exit(96) }
    for {
        select {
        case s := <-signals:
            name := "SIGINT"; if s == syscall.SIGTERM { name = "SIGTERM" }
            if _, err := fmt.Fprintln(receipt, name); err != nil { os.Exit(96) }
            if receipt.Sync() != nil { os.Exit(96) }
            if mode != "ignore" {
                // The named output may be full. Never make receiving the signal
                // or exiting depend on a second write to that same blocked fd.
                if condition != "unread workload stdout" { fmt.Fprintln(os.Stdout, "slice-12:final") }
                os.Exit(code)
            }
        case <-ticker.C:
            n++; put("heartbeat", strconv.Itoa(n))
        }
    }
}
`
