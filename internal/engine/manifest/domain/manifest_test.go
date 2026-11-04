package domain_test

import (
	"reflect"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/manifest/domain"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
)

func str(text string) port.Value { return port.Value{Kind: port.String, Text: text} }
func obj(fields map[string]port.Value) port.Value {
	return port.Value{Kind: port.Object, Fields: fields}
}
func arr(values ...port.Value) port.Value { return port.Value{Kind: port.Array, Elements: values} }

func inspect(fields map[string]port.Value) (port.Result, *porttest.Files, *porttest.Parser, *porttest.References) {
	files := &porttest.Files{Data: map[string][]byte{"config/worker.json": []byte("unmodified")}, Directories: map[string]bool{"src/nested": true}}
	parser := &porttest.Parser{Value: obj(fields)}
	refs := &porttest.References{Images: map[string]bool{"literal-image": true}}
	return domain.Inspect("config/worker.json", files, parser, refs), files, parser, refs
}

func TestDefaultAndExplicitSettings(t *testing.T) {
	got, files, parser, refs := inspect(map[string]port.Value{"image": str("literal-image")})
	want := &port.Settings{Image: "literal-image", WorkspaceFolder: "/workspace", ContainerEnv: map[string]string{}}
	if !reflect.DeepEqual(got.Effective, want) || len(got.Issues) != 0 || got.Path != "config/worker.json" || got.Image != "literal-image" {
		t.Fatalf("defaults: %#v", got)
	}
	if !reflect.DeepEqual(files.Reads, []string{"config/worker.json"}) || len(files.DirectoryReads) != 0 || !reflect.DeepEqual(parser.Calls, []porttest.ParseCall{{Data: "unmodified"}}) || !reflect.DeepEqual(refs.Calls, []string{"literal-image"}) {
		t.Fatal(files, parser, refs)
	}
	if got.Metadata.Checked || got.Metadata.Present != nil || len(got.Metadata.Unused) != 0 {
		t.Fatal("file validation claimed metadata inspection")
	}
	name, uid, remote := "display", "18446744073709551616", "1"
	got, files, _, _ = inspect(map[string]port.Value{
		"image": str("literal-image"), "name": str(name), "workspaceFolder": str("/workspace/src/nested"), "containerUser": str(uid), "remoteUser": str(remote),
		"containerEnv":   obj(map[string]port.Value{"APP_2": str("$HOST $(literal) `literal`\n"), "_lower": str("")}),
		"customizations": obj(map[string]port.Value{"vscode": obj(map[string]port.Value{"extensions": arr(str("example.editor")), "settings": obj(map[string]port.Value{"task": str("${unused}")})})}),
	})
	want = &port.Settings{Name: &name, Image: "literal-image", WorkspaceFolder: "/workspace/src/nested", ContainerEnv: map[string]string{"APP_2": "$HOST $(literal) `literal`\n", "_lower": ""}, ContainerUser: &uid, RemoteUser: &remote}
	if !reflect.DeepEqual(got.Effective, want) || !reflect.DeepEqual(got.Unused, []string{"/customizations/vscode"}) || !reflect.DeepEqual(files.DirectoryReads, []string{"src/nested"}) {
		t.Fatalf("explicit: %#v, %v", got, files.DirectoryReads)
	}
}

func TestRepresentationAndReadFailuresShortCircuit(t *testing.T) {
	for _, failure := range []port.Failure{port.Missing, port.Path} {
		files := &porttest.Files{Failures: map[string]port.Failure{"selected": failure}}
		parser, refs := &porttest.Parser{}, &porttest.References{}
		got := domain.Inspect("selected", files, parser, refs)
		if !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: failure}}) || got.Effective != nil || len(parser.Calls) != 0 || len(refs.Calls) != 0 {
			t.Fatal(got, parser, refs)
		}
	}
	files := &porttest.Files{Data: map[string][]byte{"selected": []byte("raw")}}
	parser := &porttest.Parser{Issues: []port.Issue{{Kind: port.Duplicate, Path: []string{"x"}}}}
	refs := &porttest.References{}
	got := domain.Inspect("selected", files, parser, refs)
	if !reflect.DeepEqual(got.Issues, parser.Issues) || len(refs.Calls) != 0 {
		t.Fatal(got, refs)
	}
	parser.Issues = nil
	got = domain.Inspect("selected", files, parser, refs)
	if !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.InvalidValue}}) || len(refs.Calls) != 0 {
		t.Fatal(got, refs)
	}
	got, _, _, _ = inspect(map[string]port.Value{})
	if !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.Image, Path: []string{"image"}}}) {
		t.Fatal(got)
	}
}

func TestFieldRefusals(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value port.Value
		kind  port.Failure
		path  string
	}{
		{"image", port.Value{}, port.Image, "/image"}, {"image", str("not-pinned"), port.Image, "/image"},
		{"name", str(""), port.InvalidValue, "/name"}, {"name", str("x\x7f"), port.InvalidValue, "/name"}, {"name", str("x\n"), port.InvalidValue, "/name"}, {"name", port.Value{}, port.InvalidValue, "/name"},
		{"containerUser", port.Value{Kind: port.Number, Text: "1"}, port.InvalidValue, "/containerUser"}, {"remoteUser", port.Value{}, port.InvalidValue, "/remoteUser"},
		{"containerUser", str(""), port.Identity, "/containerUser"}, {"containerUser", str("0"), port.Identity, "/containerUser"}, {"containerUser", str("root"), port.Identity, "/containerUser"}, {"remoteUser", str("1:2"), port.Identity, "/remoteUser"},
		{"workspaceFolder", port.Value{}, port.InvalidValue, "/workspaceFolder"}, {"workspaceFolder", str("relative"), port.Workspace, "/workspaceFolder"}, {"workspaceFolder", str("/workspace/a\\b"), port.Workspace, "/workspaceFolder"}, {"workspaceFolder", str("/workspace/a\x00b"), port.Workspace, "/workspaceFolder"}, {"workspaceFolder", str("/workspace/../a"), port.Workspace, "/workspaceFolder"}, {"workspaceFolder", str("/workspace/./a"), port.Workspace, "/workspaceFolder"}, {"workspaceFolder", str("/workspace/a/"), port.Workspace, "/workspaceFolder"}, {"workspaceFolder", str("/workspace/missing"), port.Workspace, "/workspaceFolder"},
		{"containerEnv", port.Value{}, port.InvalidValue, "/containerEnv"}, {"containerEnv", obj(map[string]port.Value{"MODE": {Kind: port.Boolean}}), port.InvalidValue, "/containerEnv/MODE"}, {"containerEnv", obj(map[string]port.Value{"MODE": str("x\x00")}), port.InvalidValue, "/containerEnv/MODE"}, {"containerEnv", obj(map[string]port.Value{"": str("x")}), port.InvalidValue, "/containerEnv/"}, {"containerEnv", obj(map[string]port.Value{"9MODE": str("x")}), port.InvalidValue, "/containerEnv/9MODE"}, {"containerEnv", obj(map[string]port.Value{"APP-MODE": str("x")}), port.InvalidValue, "/containerEnv/APP-MODE"}, {"containerEnv", obj(map[string]port.Value{"http_proxy": str("")}), port.Reserved, "/containerEnv/http_proxy"}, {"containerEnv", obj(map[string]port.Value{"OpEnAi_model": str("")}), port.Reserved, "/containerEnv/OpEnAi_model"},
		{"customizations", arr(), port.InvalidValue, "/customizations"}, {"customizations", obj(map[string]port.Value{"unknown": obj(nil)}), port.Unsupported, "/customizations/unknown"}, {"customizations", obj(map[string]port.Value{"vscode": {}}), port.InvalidValue, "/customizations/vscode"}, {"customizations", obj(map[string]port.Value{"vscode": obj(map[string]port.Value{"command": str("execute")})}), port.Unsupported, "/customizations/vscode/command"}, {"customizations", obj(map[string]port.Value{"vscode": obj(map[string]port.Value{"settings": arr()})}), port.InvalidValue, "/customizations/vscode/settings"}, {"customizations", obj(map[string]port.Value{"vscode": obj(map[string]port.Value{"extensions": str("extension")})}), port.InvalidValue, "/customizations/vscode/extensions"}, {"customizations", obj(map[string]port.Value{"vscode": obj(map[string]port.Value{"extensions": arr(str(""))})}), port.InvalidValue, "/customizations/vscode/extensions/0"},
		{"initializeCommand", str("do not run"), port.Unsupported, "/initializeCommand"},
		{"image", obj(map[string]port.Value{"part": arr(str("${HOST}"))}), port.Interpolation, "/image/part/0"},
		{"containerEnv", str("${HOST}"), port.Interpolation, "/containerEnv"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			fields := map[string]port.Value{"image": str("literal-image"), tc.key: tc.value}
			got, _, _, _ := inspect(fields)
			if got.Effective != nil || len(got.Issues) != 1 || got.Issues[0].Kind != tc.kind || port.Pointer(got.Issues[0].Path) != tc.path {
				t.Fatalf("got %#v; want %v at %s", got, tc.kind, tc.path)
			}
		})
	}
}

func TestIndependentProblemsSurviveInterpolation(t *testing.T) {
	got, _, _, _ := inspect(map[string]port.Value{"image": str("literal-image"), "containerEnv": obj(map[string]port.Value{"APP": str("${HOST}"), "MODE": {}, "PATH": str("")}), "privileged": {}, "remoteUser": str("0")})
	want := []port.Issue{{Kind: port.Interpolation, Path: []string{"containerEnv", "APP"}}, {Kind: port.InvalidValue, Path: []string{"containerEnv", "MODE"}}, {Kind: port.Reserved, Path: []string{"containerEnv", "PATH"}}, {Kind: port.Unsupported, Path: []string{"privileged"}}, {Kind: port.Identity, Path: []string{"remoteUser"}}}
	if !reflect.DeepEqual(got.Issues, want) || got.Effective != nil {
		t.Fatalf("%#v", got)
	}
}
