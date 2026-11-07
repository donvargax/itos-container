package features

func (w *world) buildOfflineProbe() error {
	return w.installNativeProbe("slice-10-probe", func(path string) error {
		return recoveryBuildHelper(w.root, path, offlineProbeSource)
	})
}

// Trusted source is compiled by the harness but executed only as explicit worker
// argv. It uses no image additions, host executable lookup, shell or credentials.
const offlineProbeSource = `package main

import (
    "bytes"
    "encoding/binary"
    "encoding/json"
    "io"
    "os"
    "strings"
)

func frame(values []string) {
    must(binary.Write(os.Stdout, binary.BigEndian, uint32(len(values))))
    for _, value := range values {
        must(binary.Write(os.Stdout, binary.BigEndian, uint32(len(value))))
        _, err := io.WriteString(os.Stdout, value)
        must(err)
    }
}

func must(err error) { if err != nil { os.Exit(98) } }
func write(file *os.File, value string) { _, err := io.WriteString(file, value); must(err) }

func main() {
    if len(os.Args) < 2 { os.Exit(99) }
    switch os.Args[1] {
    case "argv":
        write(os.Stdout, "slice-10:argv\x00")
        frame(os.Args[2:])
    case "streams":
        write(os.Stdout, "slice-10:stdout-ready\x00\xff\n")
        write(os.Stderr, "slice-10:stderr-ready\x00\xfe\n")
        input, err := io.ReadAll(os.Stdin)
        must(err)
        if !bytes.Equal(input, []byte("slice-10:input\x00\xfd\n")) { os.Exit(97) }
        write(os.Stdout, "slice-10:stdin-and-eof\x00")
        frame([]string{string(input)})
        write(os.Stdout, "slice-10:stdout-tail\x00\xfc\n")
        write(os.Stderr, "slice-10:stderr-tail\x00\xfb\n")
    case "environment":
        // Report only presence for synthetic host-only names. Never dump an
        // inherited environment, even if the implementation forwards it wrongly.
        present := map[string]bool{}
        for _, key := range []string{"HOST_ONLY", "GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "ITOS_ROOT_MARKER"} {
            _, present[key] = os.LookupEnv(key)
        }
        present["host-path"] = strings.Contains(os.Getenv("PATH"), "slice-10-host-bin")
        writable := map[string]bool{}
        for _, key := range []string{"HOME", "TMPDIR"} {
            f, err := os.CreateTemp(os.Getenv(key), "slice-10-worker-")
            writable[key] = err == nil
            if err == nil { name := f.Name(); must(f.Close()); must(os.Remove(name)) }
        }
        must(json.NewEncoder(os.Stdout).Encode(struct {
            Present map[string]bool
            Writable map[string]bool
        }{present, writable}))
    default:
        os.Exit(96)
    }
}
`
