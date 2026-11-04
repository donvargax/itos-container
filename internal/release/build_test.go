package release

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/release/releasetest"
)

func TestBinaryMovedReportsAMissingDestinationRevision(t *testing.T) {
	t.Chdir(releasetest.Example(t, true))
	if moved, err := BinaryMoved("HEAD", "missing-destination"); err == nil || !strings.Contains(err.Error(), "missing-destination") || moved != nil {
		t.Fatalf("missing destination = %v, %v", moved, err)
	}
}

func TestBuiltAtReportsAnUnavailableTemporaryDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, missing)
	}
	if _, err := BuiltAt("HEAD"); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unavailable temporary directory = %v, want not-exist", err)
	}
}

// toolOutput supplies completed Go subprocess results; extraction still uses
// the real committed fixture. This exercises malformed output independently
// of whichever Go release is installed on the test runner.
type toolOutput struct {
	list, mod       string
	listErr, modErr error
}

func (o toolOutput) run(_ string, _ string, args ...string) (string, error) {
	if args[0] == "list" {
		return o.list, o.listErr
	}
	return o.mod, o.modErr
}

func TestBuiltAtReportsFailedOrMalformedGoOutput(t *testing.T) {
	t.Chdir(releasetest.Example(t, true))
	for _, tc := range []struct {
		name string
		tool toolOutput
		want string
	}{
		{"list failure", toolOutput{listErr: errors.New("list failed")}, "at HEAD: list failed"},
		{"malformed list", toolOutput{list: "{"}, "at HEAD, go list's output:"},
		{"mod failure", toolOutput{modErr: errors.New("mod failed")}, "at HEAD: mod failed"},
		{"malformed mod", toolOutput{mod: "{"}, "at HEAD, go mod edit's output:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			build, err := builtAt("HEAD", tc.tool.run)
			if err == nil || !strings.Contains(err.Error(), tc.want) || build.Modules != nil || build.Toolchain != "" {
				t.Fatalf("failed tool = %#v, %v; want %q and no build", build, err, tc.want)
			}
		})
	}
}

func TestGoInReportsCommandFailureAndDiagnostic(t *testing.T) {
	out, err := goIn(t.TempDir(), "", "T-8-invalid-subcommand")
	if err == nil || out != "" || !strings.Contains(err.Error(), "go T-8-invalid-subcommand:") || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("failed go command = %q, %v", out, err)
	}
}

func TestExtractReportsPipeAndStartFailures(t *testing.T) {
	t.Run("stdout already assigned", func(t *testing.T) {
		cmd := exec.Command("git", "--version")
		cmd.Stdout = io.Discard
		if err := extractCommand(cmd, "HEAD", t.TempDir()); err == nil || !strings.Contains(err.Error(), "Stdout already set") {
			t.Fatalf("pipe failure = %v", err)
		}
	})
	t.Run("executable missing", func(t *testing.T) {
		cmd := exec.Command(filepath.Join(t.TempDir(), "missing"))
		// Windows resolves executable extensions before starting the process and
		// reports exec.ErrNotFound; Unix reports os.ErrNotExist from process start.
		if err := extractCommand(cmd, "HEAD", t.TempDir()); !errors.Is(err, os.ErrNotExist) && !errors.Is(err, exec.ErrNotFound) {
			t.Fatalf("start failure = %v", err)
		}
	})
}

func TestExtractDrainsRejectedArchiveBeforeWaiting(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestArchiveProcess$")
	cmd.Env = append(os.Environ(), "T8_ARCHIVE_PROCESS=1")
	err = extractCommand(cmd, "HEAD", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), `the archive names "../outside", outside its folder`) {
		t.Fatalf("rejected archive = %v, want the extraction error after draining", err)
	}
}

func TestArchiveProcess(t *testing.T) {
	if os.Getenv("T8_ARCHIVE_PROCESS") != "1" {
		return
	}
	// More than a pipe buffer: waiting without draining cannot finish normally.
	w := tar.NewWriter(os.Stdout)
	data := bytes.Repeat([]byte("x"), 1<<20)
	if err := w.WriteHeader(&tar.Header{Name: "../outside", Mode: 0o600, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func archiveOf(t *testing.T, entries ...tar.Header) *bytes.Reader {
	t.Helper()
	var data bytes.Buffer
	w := tar.NewWriter(&data)
	for _, entry := range entries {
		if err := w.WriteHeader(&entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(data.Bytes())
}

func TestUntarReportsInvalidInputAndFilesystemFailures(t *testing.T) {
	t.Run("reader failure", func(t *testing.T) {
		err := untar(failingReader{}, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "no room left") {
			t.Fatalf("archive read failure = %v", err)
		}
	})
	t.Run("outside name", func(t *testing.T) {
		dir := t.TempDir()
		err := untar(archiveOf(t, tar.Header{Name: "../outside", Typeflag: tar.TypeReg}), dir)
		if err == nil || !strings.Contains(err.Error(), "outside its folder") {
			t.Fatalf("outside name = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "..", "outside")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("archive wrote outside its directory: %v", err)
		}
	})
	for _, kind := range []byte{tar.TypeDir, tar.TypeReg, tar.TypeSymlink} {
		t.Run(string(kind), func(t *testing.T) {
			dir := t.TempDir()
			occupied := filepath.Join(dir, "occupied")
			if err := os.WriteFile(occupied, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := untar(archiveOf(t, tar.Header{Name: "occupied/child", Typeflag: kind, Linkname: "target"}), dir)
			if err == nil {
				t.Fatal("archive entry beneath a file succeeded")
			}
			data, err := os.ReadFile(occupied)
			if err != nil || string(data) != "keep" {
				t.Fatalf("existing file = %q, %v", data, err)
			}
		})
	}
}

func TestWriteReportsAnUnwritableDestination(t *testing.T) {
	dir := t.TempDir()
	if err := write(dir, strings.NewReader("data"), 0o600); err == nil {
		t.Fatal("writing over a directory succeeded")
	}
}
