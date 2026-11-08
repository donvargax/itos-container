// Command release-version is a compiling red-first stub for T-25.
// The implementation will be adapted from donvargax/itos-template at
// 79301eba51b9768a27906c720e39c49b08491e67, originally harvested from itos.
package main

import "io"

func main() {}

func run(_ []string, _, _ io.Writer) int { return 0 }

type bump struct {
	kind    string
	commits int
}

func bumpOf(_ []string) bump      { return bump{} }
func bumped(_, _ string) string   { return "" }
func mismatch(_, _ string) string { return "" }
func messages(_ string) []string  { return nil }
