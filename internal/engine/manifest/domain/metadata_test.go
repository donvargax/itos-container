package domain_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/manifest/domain"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
)

func TestMetadataByteLimitBeforeParser(t *testing.T) {
	for _, size := range []int{65536, 65537} {
		base, _, _, _ := inspect(map[string]port.Value{"image": str("literal-image")})
		parser := &porttest.Parser{Value: arr()}
		got := domain.WithMetadata(base, port.Label{Present: true, Text: strings.Repeat(" ", size)}, parser)
		if size == 65536 {
			if len(parser.Calls) != 1 || len(got.Issues) != 0 || got.Effective == nil {
				t.Fatal("at-limit metadata did not reach parser")
			}
		} else if len(parser.Calls) != 0 || got.Effective != nil || !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.MetadataFailure}}) {
			t.Fatal("oversized label reached parser or exposed partial settings")
		}
	}
}

func TestMetadataNeverMergesAndDistinguishesAbsence(t *testing.T) {
	base, _, _, _ := inspect(map[string]port.Value{"image": str("literal-image")})
	parser := &porttest.Parser{}
	absent := domain.WithMetadata(base, port.Label{}, parser)
	if !absent.Metadata.Checked || absent.Metadata.Present == nil || *absent.Metadata.Present || len(parser.Calls) != 0 || !reflect.DeepEqual(base.Effective, absent.Effective) {
		t.Fatal(absent, parser)
	}
	parser.Value = arr(obj(map[string]port.Value{"name": str("unused name")}), obj(map[string]port.Value{"customizations": obj(map[string]port.Value{"vscode": obj(map[string]port.Value{"settings": obj(map[string]port.Value{"literal": str("${HOST}")})})})}))
	got := domain.WithMetadata(base, port.Label{Present: true, Text: "raw label"}, parser)
	if len(got.Issues) != 0 || !got.Metadata.Checked || got.Metadata.Present == nil || !*got.Metadata.Present || !reflect.DeepEqual(got.Metadata.Unused, []string{"/0/name", "/1/customizations/vscode"}) || !reflect.DeepEqual(got.Effective, base.Effective) {
		t.Fatalf("%#v", got)
	}
	if !reflect.DeepEqual(parser.Calls, []porttest.ParseCall{{Data: "raw label", Strict: true}}) {
		t.Fatal(parser.Calls)
	}
	if base.Metadata.Checked || base.Metadata.Present != nil || base.Effective.Name != nil {
		t.Fatal("caller result was changed")
	}
}

func TestInvalidMetadataPathsAndNoPartialSettings(t *testing.T) {
	for _, tc := range []struct {
		value  port.Value
		issues []port.Issue
		path   string
	}{
		{value: obj(nil), path: ""}, {value: arr(port.Value{}), path: "/0"},
		{issues: []port.Issue{{Kind: port.Syntax}}, path: ""}, {issues: []port.Issue{{Kind: port.Duplicate, Path: []string{"0", "a/b~c"}}}, path: "/0/a~1b~0c"},
		{value: arr(obj(map[string]port.Value{"name": str("")})), path: "/0/name"},
		{value: arr(obj(nil), obj(map[string]port.Value{"image": str("operational")})), path: "/1/image"},
		{value: arr(obj(map[string]port.Value{"customizations": obj(map[string]port.Value{"other": obj(nil)})})), path: "/0/customizations/other"},
	} {
		base, _, _, _ := inspect(map[string]port.Value{"image": str("literal-image")})
		parser := &porttest.Parser{Value: tc.value, Issues: tc.issues}
		got := domain.WithMetadata(base, port.Label{Present: true}, parser)
		if got.Effective != nil || got.Image != "literal-image" || len(got.Issues) != 1 || got.Issues[0].Kind != port.MetadataFailure || port.Pointer(got.Issues[0].Path) != tc.path {
			t.Fatalf("%#v, want metadata problem at %s", got, tc.path)
		}
	}
}

func TestInvalidFileSkipsMetadata(t *testing.T) {
	for _, base := range []port.Result{{Issues: []port.Issue{{Kind: port.Missing}}}, {Effective: &port.Settings{Image: "image"}, Issues: []port.Issue{{Kind: port.Syntax}}}} {
		parser := &porttest.Parser{}
		got := domain.WithMetadata(base, port.Label{Present: true}, parser)
		if !reflect.DeepEqual(got, base) || len(parser.Calls) != 0 {
			t.Fatal(got, parser)
		}
	}
}
