package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) policyLimitSteps(sc *godog.ScenarioContext) {
	write := func(data string) error {
		return os.WriteFile(filepath.Join(w.dir, "itos-container.yaml"), []byte(data), 0o600)
	}
	sc.Step(`^the policy file has representation depth (\d+) using "([^"]*)"$`, func(depth int, shape string) error {
		var data string
		switch shape {
		case "sequences":
			data = "extra: " + strings.Repeat("[", depth-2) + "0" + strings.Repeat("]", depth-2)
		case "mappings":
			data = "extra: " + strings.Repeat("{a: ", depth-2) + "0" + strings.Repeat("}", depth-2)
		case "complex keys":
			data = "? " + strings.Repeat("[", depth-2) + "0" + strings.Repeat("]", depth-2) + "\n: ignored"
		default:
			return fmt.Errorf("unknown depth fixture shape %q", shape)
		}
		return write(data + "\n")
	})
	sc.Step(`^the policy file has (\d+) representation nodes using "([^"]*)"$`, func(nodes int, shape string) error {
		var data string
		switch shape {
		case "sequences":
			data = "extra: [" + strings.Repeat("0,", nodes-3) + "]"
		case "mappings":
			data = "extra: {" + strings.Repeat("a: 0,", (nodes-3)/2) + "}"
		default:
			return fmt.Errorf("unknown node fixture shape %q", shape)
		}
		return write(data + "\n")
	})
	sc.Step(`^the policy file contains (\d+) nested tagged sequences below an unknown field$`, func(depth int) error {
		return write("extra: " + strings.Repeat("!x [", depth) + "0" + strings.Repeat("]", depth) + "\n")
	})
}
