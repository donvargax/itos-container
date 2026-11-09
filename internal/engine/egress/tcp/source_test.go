package tcp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// repoRoot is the module root, found from this package's directory.
func repoRoot(t testing.TB) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root %s: %v", root, err)
	}
	return root
}

// productionSources lists every .go file under the roots that is neither a
// _test.go file nor under testdata, files go would ignore on this machine
// included, as slash paths relative to the module root.
func productionSources(t testing.TB, root string, roots ...string) []string {
	t.Helper()
	var paths []string
	for _, start := range roots {
		err := filepath.WalkDir(filepath.Join(root, start), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			if !entry.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				rel, err := filepath.Rel(root, path)
				paths = append(paths, filepath.ToSlash(rel))
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

// parse reads one source with go/parser, which evaluates no build constraint.
func parse(t testing.TB, name string, src []byte) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return file
}

// imports maps each import path of a file to its rename, "" for none.
func imports(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		name := ""
		if spec.Name != nil {
			name = spec.Name.Name
		}
		out[path] = name
	}
	return out
}

// selectors calls visit for every pkg.Name selector with a plain identifier on the left.
func selectors(file *ast.File, visit func(qualifier, name string)) {
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				visit(id.Name, sel.Sel.Name)
			}
		}
		return true
	})
}

// fieldName is the key of a keyed composite literal element, or "" for any other.
func fieldName(element ast.Expr) string {
	if keyed, ok := element.(*ast.KeyValueExpr); ok {
		if name, ok := keyed.Key.(*ast.Ident); ok {
			return name.Name
		}
	}
	return ""
}

// interception returns why a production file holds code that could intercept TLS.
// rel is its slash path from the module root.
func interception(t testing.TB, rel string, src []byte) []string {
	t.Helper()
	file := parse(t, rel, src)
	var why []string
	have := imports(file)
	for path, name := range have {
		switch {
		case path == "crypto/x509" || strings.HasPrefix(path, "crypto/x509/"):
			why = append(why, "imports "+path)
		case path == "crypto/tls" && !strings.HasPrefix(rel, "internal/engine/egress/tcp/"):
			why = append(why, "imports crypto/tls outside the egress adapter")
		case path == "crypto/tls" && name != "":
			why = append(why, "renames crypto/tls as "+name)
		case path == "net/http/httptest":
			why = append(why, "imports net/http/httptest, whose servers hold a built-in certificate")
		}
	}
	if _, ok := have["crypto/tls"]; ok {
		selectors(file, func(qualifier, name string) {
			if qualifier == "tls" && !slices.Contains([]string{"Config", "ClientHelloInfo", "Server"}, name) {
				why = append(why, "uses tls."+name)
			}
		})
		ast.Inspect(file, func(n ast.Node) bool {
			literal, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if sel, ok := literal.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "Config" {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "tls" {
					for _, element := range literal.Elts {
						if fieldName(element) != "GetConfigForClient" {
							why = append(why, "sets a tls.Config field other than GetConfigForClient")
						}
					}
				}
			}
			return true
		})
	}
	if _, ok := have["net/http"]; ok {
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && (sel.Sel.Name == "ListenAndServeTLS" || sel.Sel.Name == "ServeTLS") {
				why = append(why, "uses "+sel.Sel.Name+", which terminates TLS from files")
			}
			return true
		})
	}
	return why
}

// TestNoInterceptionCode is the record's second way to show no interception code
// exists in the tree: the production sources under cmd and internal, files go
// ignores on this machine included (depguard cannot see those), hold none of the
// names that build a certificate, a listener or a client handshake.
func TestNoInterceptionCode(t *testing.T) {
	const prelude = "package p\n\nimport (\n"
	offending := []struct{ name, rel, src string }{
		{"crypto/x509", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/x509\"\n)\n\nvar _ = x509.NewCertPool\n"},
		{"crypto/x509/pkix", "cmd/itos-container/a.go", prelude + "\t\"crypto/x509/pkix\"\n)\n\nvar _ pkix.Name\n"},
		{"crypto/tls outside the adapter", "internal/engine/session/docker/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ tls.Config\n"},
		{"crypto/tls under cmd", "cmd/itos-container/probe_windows.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = tls.Certificate{}\n"},
		{"a renamed crypto/tls", "internal/engine/egress/tcp/a.go", prelude + "\tt \"crypto/tls\"\n)\n\nvar _ t.Config\n"},
		{"a blank crypto/tls", "internal/engine/egress/tcp/a.go", prelude + "\t_ \"crypto/tls\"\n)\n"},
		{"a dot crypto/tls", "internal/engine/egress/tcp/a.go", prelude + "\t. \"crypto/tls\"\n)\n"},
		{"tls.Certificate", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ tls.Certificate\n"},
		{"tls.LoadX509KeyPair", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = tls.LoadX509KeyPair\n"},
		{"tls.X509KeyPair", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = tls.X509KeyPair\n"},
		{"tls.Listen", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = tls.Listen\n"},
		{"tls.NewListener", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = tls.NewListener\n"},
		{"tls.Client", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = tls.Client\n"},
		{"GetCertificate", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = &tls.Config{GetCertificate: nil}\n"},
		{"Certificates beside GetConfigForClient", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = &tls.Config{GetConfigForClient: nil, Certificates: nil}\n"},
		{"an unkeyed tls.Config", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = tls.Config{nil}\n"},
		{"ListenAndServeTLS", "internal/engine/readiness/docker/a.go", prelude + "\t\"net/http\"\n)\n\nvar _ = (&http.Server{}).ListenAndServeTLS\n"},
		{"http.ServeTLS", "internal/engine/readiness/docker/a.go", prelude + "\t\"net/http\"\n)\n\nvar _ = http.ServeTLS\n"},
		{"httptest", "internal/engine/readiness/docker/a.go", prelude + "\t\"net/http/httptest\"\n)\n\nvar _ = httptest.NewTLSServer\n"},
	}
	for _, c := range offending {
		t.Run("refuses "+c.name, func(t *testing.T) {
			if why := interception(t, c.rel, []byte(c.src)); len(why) == 0 {
				t.Errorf("%s was not refused", c.name)
			}
		})
	}
	for _, c := range []struct{ name, rel, src string }{
		{"the peek", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ = tls.Server(nil, &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) { return nil, nil }})\n"},
		{"another package's Config literal", "internal/engine/egress/tcp/a.go", prelude + "\t\"crypto/tls\"\n)\n\nvar _ tls.Config\nvar _ = container.Config{Image: \"x\"}\n"},
		{"plain net/http", "internal/engine/readiness/docker/a.go", prelude + "\t\"net/http\"\n)\n\nvar _ = http.NewRequest\n"},
	} {
		t.Run("accepts "+c.name, func(t *testing.T) {
			if why := interception(t, c.rel, []byte(c.src)); len(why) != 0 {
				t.Errorf("%s was refused: %v", c.name, why)
			}
		})
	}
	root := repoRoot(t)
	files := productionSources(t, root, "cmd", "internal")
	var underCmd, underTCP bool
	for _, rel := range files {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		for _, why := range interception(t, rel, src) {
			t.Errorf("%s: %s", rel, why)
		}
		underCmd = underCmd || strings.HasPrefix(rel, "cmd/")
		underTCP = underTCP || strings.HasPrefix(rel, "internal/engine/egress/tcp/")
	}
	if len(files) < 100 || !underCmd || !underTCP {
		t.Fatalf("read %d files (cmd %v, egress adapter %v); a walk rooted wrongly would pass a bare count: want at least 100, one under cmd and one under the adapter",
			len(files), underCmd, underTCP)
	}
}
