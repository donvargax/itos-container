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
	sc.Step(`^the policy file contains (\d+) nested sequences below an unknown field$`, func(depth int) error {
		return write("version: 1\nmanifest: a.json\nextra: " + strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth) + "\n")
	})
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
		return write("version: 1\nmanifest: a.json\n" + data + "\n")
	})
	sc.Step(`^the policy file has (\d+) representation nodes using "([^"]*)"$`, func(nodes int, shape string) error {
		var data string
		switch shape {
		case "sequences":
			// Root, three keys and their values contribute seven nodes.
			data = "version: 1\nmanifest: a.json\nextra: [" + strings.Repeat("0,", nodes-7) + "]"
		case "mappings":
			// The sequence and mapping contribute two nodes; each unique
			// key/value pair adds two. An odd budget adds one final element.
			var fields []string
			for i := 0; i < (nodes-2)/2; i++ {
				fields = append(fields, fmt.Sprintf("key%d: 0", i))
			}
			data = "[{" + strings.Join(fields, ",") + "}"
			if nodes%2 != 0 {
				data += ",0"
			}
			data += "]"
		default:
			return fmt.Errorf("unknown node fixture shape %q", shape)
		}
		return write(data + "\n")
	})
	sc.Step(`^the policy file contains (\d+) nested tagged sequences below an unknown field$`, func(depth int) error {
		return write("extra: " + strings.Repeat("!x [", depth) + "0" + strings.Repeat("]", depth) + "\n")
	})
	sc.Step(`^the policy file reuses a (\d+) byte tag prefix on (\d+) scalars in "([^"]*)"$`, func(size, count int, position string) error {
		data := "%TAG !e! " + strings.Repeat("x", size) + "\n---\n[" + strings.Repeat("!e!s 0,", count) + "]\n"
		switch position {
		case "first document":
		case "after initial BOM":
			data = "\ufeff" + data
		case "second document":
			data = "0\n...\n" + data
		default:
			return fmt.Errorf("unknown directive fixture position %q", position)
		}
		if len(data) > 65536 {
			return fmt.Errorf("directive fixture exceeds policy input limit: %d bytes", len(data))
		}
		return write(data)
	})
}
