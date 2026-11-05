package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) parsingSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the policy file has (\d+) unclosed "([^"]*)" flow collections in the "([^"]*)"$`, func(count int, shape, position string) error {
		var data string
		switch shape {
		case "sequences":
			data = strings.Repeat("[", count)
		case "mappings":
			data = strings.Repeat("{a: ", count)
		case "tagged sequences":
			data = strings.Repeat("!x [", count)
		case "complex keys":
			data = "? " + strings.Repeat("[", count)
		default:
			return fmt.Errorf("unknown flow fixture shape %q", shape)
		}
		return w.writeParsingDocument(data+"0\n", position)
	})
	sc.Step(`^the policy file has (\d+) "([^"]*)" block levels with an unfinished value in the "([^"]*)"$`, func(count int, shape, position string) error {
		var entry string
		switch shape {
		case "mappings":
			entry = "a:\n"
		case "sequences":
			entry = "-\n"
		default:
			return fmt.Errorf("unknown block fixture shape %q", shape)
		}
		var data strings.Builder
		for i := range count {
			data.WriteString(strings.Repeat("  ", i) + entry)
		}
		data.WriteString(strings.Repeat("  ", count) + "[\n")
		return w.writeParsingDocument(data.String(), position)
	})
	sc.Step(`^the policy file has a complete document using (\d+) aggregate "([^"]*)"$`, func(count int, budget string) error {
		var data string
		switch budget {
		case "nodes":
			data = "version: 1\nmanifest: a.json\nextra: [" + strings.Repeat("0,", count-7) + "]\n"
		case "path bytes":
			data = "? '" + strings.Repeat("a", count/2-1) + "'\n: 0\n"
		default:
			return fmt.Errorf("unknown representation budget %q", budget)
		}
		return w.writeParsingDocument(data, "first document")
	})
	sc.Step(`^the following bytes are appended to the policy file:$`, func(doc *godog.DocString) error {
		file, err := os.OpenFile(filepath.Join(w.dir, "itos-container.yaml"), os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		_, err = file.WriteString(doc.Content + "\n")
		closeErr := file.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}

func (w *world) writeParsingDocument(data, position string) error {
	switch position {
	case "first document":
	case "second document":
		data = "version: 1\nmanifest: a.json\n---\n" + data
	default:
		return fmt.Errorf("unknown document position %q", position)
	}
	return os.WriteFile(filepath.Join(w.dir, "itos-container.yaml"), []byte(data), 0o600)
}
