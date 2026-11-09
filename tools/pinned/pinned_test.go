package pinned_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise the actual downloader with curl and a local release server. Only
// platform/release metadata is replaced in the disposable launcher; repository
// pins, the real tool cache and external release services are never touched.
func TestPinnedDownload(t *testing.T) {
	for _, tc := range []struct {
		name      string
		statuses  []int
		corrupt   bool
		wantExit  int
		wantCalls int32
		wantText  string
	}{
		{"transient gateway timeout", []int{504, 200}, false, 7, 2, "an argument with spaces\n"},
		{"gateway outage outlasts early retries", []int{504, 504, 504, 504, 200}, false, 7, 5, "an argument with spaces\n"},
		{"exhausted gateway timeout", []int{504}, false, 2, 6, "pinned: cannot fetch"},
		{"missing release", []int{404}, false, 2, 1, "pinned: cannot fetch"},
		{"bad checksum after retry", []int{504, 200}, true, 2, 2, "SHA-256 did not match the pinned release"},
		{"bad checksum after extended outage", []int{504, 504, 504, 504, 200}, true, 2, 5, "SHA-256 did not match the pinned release"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			archive := toolArchive(t)
			body := archive
			if tc.corrupt {
				body = append(bytes.Clone(archive), []byte("corrupt release")...)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				i := int(calls.Add(1)) - 1
				status := tc.statuses[min(i, len(tc.statuses)-1)]
				// Keep outage tests short while exercising curl's real retry logic.
				// Production releases need not supply this header.
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write(body)
				}
			}))
			defer server.Close()
			root, launcher := fixtureLauncher(t, server.URL, archive)
			run := func() (int, string) {
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "sh", launcher, "golangci-lint", "an argument with spaces")
				// The fixture server must remain local even on a host with proxies.
				cmd.Env = append(os.Environ(), "NO_PROXY=*", "no_proxy=*")
				output, err := cmd.CombinedOutput()
				if ctx.Err() != nil || cmd.ProcessState == nil {
					t.Fatalf("launcher did not finish: %v\n%s", err, output)
				}
				return cmd.ProcessState.ExitCode(), string(output)
			}
			exit, output := run()
			if exit != tc.wantExit || !strings.Contains(output, tc.wantText) {
				t.Fatalf("exit %d, want %d and %q:\n%s", exit, tc.wantExit, tc.wantText, output)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Fatalf("download attempts = %d, want %d", got, tc.wantCalls)
			}
			if tc.wantExit == 7 {
				// The tool's nonzero exit and exact argv survive both the first
				// download and a cache hit with the release server unavailable.
				server.Close()
				exit, output = run()
				if exit != 7 || output != tc.wantText || calls.Load() != tc.wantCalls {
					t.Fatalf("cached tool: exit %d, calls %d, output %q", exit, calls.Load(), output)
				}
			} else if _, err := os.Stat(filepath.Join(root, ".tools")); !os.IsNotExist(err) {
				t.Fatalf("failed download published a tool cache: %v", err)
			}
		})
	}
}

func TestPinnedAssetRedirect(t *testing.T) {
	archive := toolArchive(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			if r.Header.Get("Accept") != "application/octet-stream" {
				http.Error(w, "binary asset media type required", http.StatusNotAcceptable)
				return
			}
			if calls.Add(1) == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusGatewayTimeout)
				return
			}
			http.Redirect(w, r, "/archive", http.StatusFound)
		case "/archive":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	_, launcher := fixtureLauncher(t, server.URL+"/asset", archive)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", launcher, "--path", "golangci-lint")
	cmd.Env = append(os.Environ(), "NO_PROXY=*", "no_proxy=*")
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("asset download failed: %v\n%s", err, &diagnostics)
	}
	if calls.Load() != 2 || !strings.Contains(diagnostics.String(), "504") {
		t.Fatalf("--path mixed streams or failed to retry: stdout %q, stderr %q, calls %d", output, diagnostics.String(), calls.Load())
	}
	// Consume the returned path as the caller's shell would, including on Git
	// for Windows where shell paths differ from Go's native filesystem paths.
	run := exec.CommandContext(ctx, "sh", "-c", `exec "$1" "$2"`, "fixture", strings.TrimSpace(string(output)), "an argument with spaces")
	result, err := run.CombinedOutput()
	if run.ProcessState == nil || run.ProcessState.ExitCode() != 7 || string(result) != "an argument with spaces\n" {
		t.Fatalf("returned tool path is unusable: %v, output %q", err, result)
	}
}

func fixtureLauncher(t *testing.T, url string, archive []byte) (string, string) {
	t.Helper()
	source, err := os.ReadFile("../bin/pinned")
	if err != nil {
		t.Fatal(err)
	}
	script := string(source)
	start := strings.Index(script, `case "$(uname -s)-$(uname -m)" in`)
	end := strings.Index(script, "\ndir=$root/.tools/$tool-$version")
	if start < 0 || end <= start {
		t.Fatal("cannot locate launcher release metadata")
	}
	metadata := fmt.Sprintf("version=fixture\nasset=fixture.tar.gz\nmember=golangci-lint\nexe=\nurl=%s\nsum=%x\n", url, sha256.Sum256(archive))
	script = script[:start] + metadata + script[end:]
	root := t.TempDir()
	dir := filepath.Join(root, "tools", "bin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(dir, "pinned")
	if err := os.WriteFile(launcher, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, launcher
}

func toolArchive(t *testing.T) []byte {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\nprintf '%s\\n' \"$1\"\nexit 7\n")
	if err := tw.WriteHeader(&tar.Header{Name: "golangci-lint", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

// Read the actual launcher's selected metadata before its cache/download branch.
// Only uname's value is substituted; every release field comes from the script.
func TestGitCliffPin(t *testing.T) {
	source, err := os.ReadFile("../bin/pinned")
	if err != nil {
		t.Fatal(err)
	}
	script := string(source)
	end := strings.Index(script, "\ndir=$root/.tools/$tool-$version")
	if end < 0 {
		t.Fatal("cannot locate launcher metadata boundary")
	}
	script = strings.Replace(script[:end], `"$(uname -s)-$(uname -m)"`, `"$T25_PLATFORM"`, 1)
	script += `
printf '%s\n' "$version" "$target" "$asset" "$member" "$sum" "$asset_id" "$url" "$exe"
`
	for _, tc := range []struct{ platform, target, sum, assetID string }{
		{"Linux-x86_64", "x86_64-unknown-linux-gnu", "24f397c733add5390fdceee3a2088588ab0d5f944ce00d34cb7029b888cf2db4", "573483509"},
		{"Linux-aarch64", "aarch64-unknown-linux-gnu", "69e208f6e4baef3a0f9b759d5f44223aa9336b86c691072a3d0c98d35a816e69", "573483780"},
		{"Darwin-x86_64", "x86_64-apple-darwin", "4e647d2deb152aca1e987f225cf42b080b991dd98f3bce4027ef95446d982ac3", "573484837"},
		{"Darwin-arm64", "aarch64-apple-darwin", "a0be97e237d440d34c5e508ec21d070b2c85cbb8be48a46a56f9a4e8bd247f05", "573487203"},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			cmd := exec.Command("sh", "-s", "--", "git-cliff")
			cmd.Stdin = strings.NewReader(script)
			cmd.Env = append(os.Environ(), "T25_PLATFORM="+tc.platform)
			got, err := cmd.CombinedOutput()
			want := strings.Join([]string{"2.14.2", tc.target,
				"git-cliff-2.14.2-" + tc.target + ".tar.gz", "git-cliff-2.14.2/git-cliff",
				tc.sum, tc.assetID, "https://api.github.com/repos/orhun/git-cliff/releases/assets/" + tc.assetID, "", ""}, "\n")
			if err != nil || string(got) != want {
				t.Fatalf("git-cliff metadata missing or incorrect: %v\ngot %q\nwant %q", err, got, want)
			}
		})
	}
}
