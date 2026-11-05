package jsonc_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
)

func TestParserContract(t *testing.T) {
	porttest.ParserContract(t, func(porttest.ParserFixture) port.Parser { return jsonc.Parser{} })
}

func TestInspectionBudgetBoundaries(t *testing.T) {
	members := func(count int) string {
		var fields []string
		for i := range count {
			fields = append(fields, fmt.Sprintf(`"k%d":0`, i))
		}
		// One extra element makes 1023 members exactly 2048 total nodes.
		fields[0] = `"k0":[0]`
		return "{" + strings.Join(fields, ",") + "}"
	}
	for _, tc := range []struct {
		name, data string
		failure    port.Failure
	}{
		{"bytes at", "0" + strings.Repeat(" ", 65535), port.None},
		{"bytes over", "0" + strings.Repeat(" ", 65536), port.Complexity},
		{"array depth at", strings.Repeat("[", 31) + "0" + strings.Repeat("]", 31), port.None},
		{"array depth over", strings.Repeat("[", 32) + "0" + strings.Repeat("]", 32), port.Complexity},
		{"empty container depth at", strings.Repeat("[", 32) + strings.Repeat("]", 32), port.None},
		{"unfinished depth over", strings.Repeat("[", 33), port.Complexity},
		{"unfinished depth at", strings.Repeat("[", 32), port.Syntax},
		{"object depth at", strings.Repeat(`{"k":`, 31) + "0" + strings.Repeat("}", 31), port.None},
		{"object depth over", strings.Repeat(`{"k":`, 32) + "0" + strings.Repeat("}", 32), port.Complexity},
		{"array nodes at", "[" + strings.Repeat("0,", 2046) + "0]", port.None},
		{"array nodes over", "[" + strings.Repeat("0,", 2047) + "0]", port.Complexity},
		{"depth over after empty comment", "/**/" + strings.Repeat("[", 33) + strings.Repeat("]", 33), port.Complexity},
		{"unfinished depth over after empty comment", "/**/" + strings.Repeat("[", 33), port.Complexity},
		{"nodes over after empty comment", "/**/[" + strings.Repeat("0,", 2047) + "0]", port.Complexity},
		{"object nodes at", members(1023), port.None},
		{"object nodes over", members(1024), port.Complexity},
		{"paths at", `{"` + strings.Repeat("a", 32767) + `":0}`, port.None},
		{"paths over", `{"` + strings.Repeat("a", 32768) + `":0}`, port.Complexity},
		{"escaped paths at", `{"` + strings.Repeat("~/", 8191) + `abc":0}`, port.None},
		{"escaped paths over", `{"` + strings.Repeat("~/", 8192) + `":0}`, port.Complexity},
		{"array paths at", `{"` + strings.Repeat("a", 16382) + `":[0,0]}`, port.None},
		{"array paths over", `{"` + strings.Repeat("a", 16383) + `":[0,0]}`, port.Complexity},
		{"unmatched closer", "]", port.Syntax},
		{"excess after unmatched closer", "]" + strings.Repeat("[", 33), port.Complexity},
		{"bare slash", "/", port.Syntax},
		{"invalid slash token", "/foo", port.Syntax},
		{"unterminated escape", `"\`, port.Syntax},
		{"unterminated string", `"` + strings.Repeat("[", 100), port.Syntax},
	} {
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/strict=%t", tc.name, strict), func(t *testing.T) {
				value, issues := (jsonc.Parser{}).Parse([]byte(tc.data), strict)
				if tc.failure == port.None {
					if len(issues) != 0 {
						t.Fatalf("within-budget input refused: %v", issues)
					}
				} else if !reflect.DeepEqual(value, port.Value{}) || !reflect.DeepEqual(issues, []port.Issue{{Kind: tc.failure}}) {
					t.Fatalf("want one %v refusal without partial data, got %v", tc.failure, issues)
				}
			})
		}
	}
}

func TestBudgetIgnoresLiteralAndCommentPunctuation(t *testing.T) {
	literal := strings.Repeat("[{}],:\"\\/*//", 2100)
	encoded, err := json.Marshal(literal)
	if err != nil {
		t.Fatal(err)
	}
	for _, strict := range []bool{false, true} {
		value, issues := (jsonc.Parser{}).Parse(encoded, strict)
		if len(issues) != 0 || value.Kind != port.String || value.Text != literal {
			t.Fatal("literal punctuation consumed structural budget", issues)
		}
	}
	for _, data := range []string{
		"/**/ {}",
		"/**//**/ {}",
		"/*" + strings.Repeat("[\"{,", 3000) + "*/ {}",
		"//" + strings.Repeat("[\"{,", 3000) + "\n {}",
		"{\r\n\t\"x\" /* [ */ : /* { */ [1, /* } */ 2,],}",
	} {
		if _, issues := (jsonc.Parser{}).Parse([]byte(data), false); len(issues) != 0 {
			t.Fatal("comments consumed structural budget", issues)
		}
	}
	for _, data := range []string{"/* unfinished [", "// no value [", "/*/", "{} // trailing comment without newline [\""} {
		if _, issues := (jsonc.Parser{}).Parse([]byte(data), false); !reflect.DeepEqual(issues, []port.Issue{{Kind: port.Syntax}}) {
			t.Fatal("incomplete syntax changed classification", issues)
		}
	}
}
