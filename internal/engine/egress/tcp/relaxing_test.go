package tcp

import (
	"go/ast"
	"go/build"
	"go/build/constraint"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// releaseTargets are the platforms .goreleaser.yaml builds.
var releaseTargets = []struct{ goos, goarch string }{
	{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"},
}

// relaxing returns why a production file under egress could make the proxy differ
// by platform, read a setting from outside, or survive a panic. dir is the folder
// the file is in, which MatchFile reads it from.
func relaxing(t testing.TB, dir, name string) []string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	file := parse(t, name, src)
	var why []string
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if constraint.IsGoBuild(comment.Text) || constraint.IsPlusBuild(comment.Text) {
				why = append(why, "holds a build constraint: "+comment.Text)
			}
		}
	}
	for _, target := range releaseTargets {
		context := build.Default
		context.GOOS, context.GOARCH, context.CgoEnabled = target.goos, target.goarch, false
		if ok, err := context.MatchFile(dir, name); err != nil || !ok {
			why = append(why, "is ignored for "+target.goos+"/"+target.goarch)
		}
	}
	have := imports(file)
	for _, path := range []string{"os", "syscall"} {
		if name, ok := have[path]; ok && name != "" {
			why = append(why, "renames "+path+" as "+name)
		}
	}
	allowed := map[string]string{"os": "ErrDeadlineExceeded", "syscall": "RawConn"}
	selectors(file, func(qualifier, selected string) {
		for path, ok := range allowed {
			if _, imported := have[path]; imported && qualifier == path && selected != ok {
				why = append(why, "uses "+path+"."+selected)
			}
		}
		if selected == "GOOS" {
			why = append(why, "reads GOOS")
		}
	})
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "recover" {
				why = append(why, "recovers a panic")
			}
		}
		return true
	})
	return why
}

// TestNoRelaxingCode holds "no flag, variable or build tag" and "recovers no
// panic" over egress's own production sources, files go would ignore included. A
// build constraint, a file name go would ignore on one release target, a rename of
// os or syscall, any os or syscall name but the two the adapter uses, any GOOS and
// any recover call are refused, so neither a build tag nor a file name can make
// egress differ by platform, and neither os.Args, an environment variable nor a
// file can feed it a setting.
func TestNoRelaxingCode(t *testing.T) {
	const header = "package p\n\n"
	offending := []struct{ name, file, src string }{
		{"//go:build", "a.go", "//go:build relaxed\n\n" + header},
		{"// +build", "a.go", "// +build relaxed\n\n" + header},
		{"// +build ignore", "a.go", "// +build ignore\n\n" + header},
		{"a _windows.go name", "table_windows.go", header},
		{"a _darwin.go name", "table_darwin.go", header},
		{"an _amd64.go name", "table_amd64.go", header},
		{"an underscore-prefixed name", "_table.go", header},
		{"a renamed os", "a.go", header + "import o \"os\"\n\nvar _ = o.ErrDeadlineExceeded\n"},
		{"a renamed syscall", "a.go", header + "import s \"syscall\"\n\nvar _ s.RawConn\n"},
		{"os.Args", "a.go", header + "import \"os\"\n\nvar _ = os.Args\n"},
		{"os.Getenv", "a.go", header + "import \"os\"\n\nvar _ = os.Getenv\n"},
		{"os.LookupEnv", "a.go", header + "import \"os\"\n\nvar _ = os.LookupEnv\n"},
		{"os.Environ", "a.go", header + "import \"os\"\n\nvar _ = os.Environ\n"},
		{"os.Open", "a.go", header + "import \"os\"\n\nvar _ = os.Open\n"},
		{"os.ReadFile", "a.go", header + "import \"os\"\n\nvar _ = os.ReadFile\n"},
		{"syscall.Getenv", "a.go", header + "import \"syscall\"\n\nvar _ = syscall.Getenv\n"},
		{"syscall.Open", "a.go", header + "import \"syscall\"\n\nvar _ = syscall.Open\n"},
		{"runtime.GOOS", "a.go", header + "import \"runtime\"\n\nvar _ = runtime.GOOS\n"},
		{"another qualifier's GOOS", "a.go", header + "var _ = goos.GOOS\n"},
		{"recover", "a.go", header + "func f() { _ = recover() }\n"},
	}
	for _, c := range offending {
		t.Run("refuses "+c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, c.file), []byte(c.src), 0o600); err != nil {
				t.Fatal(err)
			}
			if why := relaxing(t, dir, c.file); len(why) == 0 {
				t.Errorf("%s was not refused", c.name)
			}
		})
	}
	t.Run("accepts the two names the adapter uses", func(t *testing.T) {
		dir := t.TempDir()
		src := header + "import (\n\t\"os\"\n\t\"syscall\"\n)\n\nvar _ = os.ErrDeadlineExceeded\nvar _ syscall.RawConn\n"
		if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		if why := relaxing(t, dir, "a.go"); len(why) != 0 {
			t.Errorf("refused %v", why)
		}
	})
	root := repoRoot(t)
	files := productionSources(t, root, "internal/engine/egress")
	read := map[string]bool{}
	for _, rel := range files {
		for _, why := range relaxing(t, filepath.Join(root, filepath.Dir(filepath.FromSlash(rel))), filepath.Base(rel)) {
			t.Errorf("%s %s", rel, why)
		}
		for _, folder := range []string{"domain", "port", "port/porttest", "tcp"} {
			if filepath.ToSlash(filepath.Dir(rel)) == "internal/engine/egress/"+folder {
				read[folder] = true
			}
		}
	}
	for _, folder := range []string{"domain", "port", "port/porttest", "tcp"} {
		if !read[folder] {
			t.Errorf("read no production file in %s; a walk rooted wrongly would pass", folder)
		}
	}
	if len(files) == 0 || !slices.ContainsFunc(files, func(rel string) bool { return strings.HasSuffix(rel, "/proxy.go") }) {
		t.Errorf("read %v; want the adapter's proxy.go among egress's sources", files)
	}
}
