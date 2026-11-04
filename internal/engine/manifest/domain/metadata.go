package domain

import (
	"slices"
	"strconv"

	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

// WithMetadata validates supplied inspection data without merging effective
// settings. A failed file inspection never starts label validation.
func WithMetadata(result port.Result, label port.Label, parser port.Parser) port.Result {
	if result.Effective == nil || len(result.Issues) != 0 {
		return result
	}
	result.Metadata = port.Metadata{Checked: true, Present: &label.Present}
	if !label.Present {
		return result
	}
	// Reject before copying caller-supplied text into parser-owned bytes.
	if len(label.Text) > port.MaxBytes {
		result.Issues = []port.Issue{{Kind: port.MetadataFailure}}
		result.Effective = nil
		return result
	}
	value, issues := parser.Parse([]byte(label.Text), true)
	if len(issues) == 0 {
		if value.Kind != port.Array {
			issues = []port.Issue{{Kind: port.MetadataFailure}}
		} else {
			for i, entry := range value.Elements {
				unused, problems := metadataEntry(entry, []string{strconv.Itoa(i)})
				result.Metadata.Unused = append(result.Metadata.Unused, unused...)
				issues = append(issues, problems...)
			}
		}
	}
	for i := range issues {
		issues[i].Kind = port.MetadataFailure
	}
	sortIssues(issues)
	slices.Sort(result.Metadata.Unused)
	result.Issues = issues
	if len(issues) != 0 {
		result.Effective = nil
	}
	return result
}

func metadataEntry(value port.Value, path []string) ([]string, []port.Issue) {
	if value.Kind != port.Object {
		return nil, []port.Issue{{Kind: port.MetadataFailure, Path: path}}
	}
	var unused []string
	var issues []port.Issue
	for key, value := range value.Fields {
		at := child(path, key)
		switch key {
		case "name":
			if !validName(value) {
				issues = append(issues, port.Issue{Kind: port.MetadataFailure, Path: at})
			} else {
				unused = append(unused, port.Pointer(at))
			}
		case "customizations":
			pointers, problems := customizations(value, at)
			unused = append(unused, pointers...)
			issues = append(issues, problems...)
		default:
			issues = append(issues, port.Issue{Kind: port.MetadataFailure, Path: at})
		}
	}
	return unused, issues
}
