package features

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	codexMemberName  = "codex-x86_64-unknown-linux-musl"
	codexArchiveName = codexMemberName + ".tar.gz"
	codexMemberBody  = "T-81 fake codex\n"
)

type codexTarMember struct{ name, data string }

func codexHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// writeCodexArchive writes a gzip tar of regular 0755 members and returns the archive bytes.
func writeCodexArchive(t *testing.T, path string, members []codexTarMember) []byte {
	t.Helper()
	var raw bytes.Buffer
	zipped := gzip.NewWriter(&raw)
	archive := tar.NewWriter(zipped)
	for _, member := range members {
		header := &tar.Header{Name: member.name, Mode: 0o755, Size: int64(len(member.data)), Typeflag: tar.TypeReg}
		requireFixture(t, archive.WriteHeader(header) == nil, "cannot write tar header")
		_, err := archive.Write([]byte(member.data))
		requireFixture(t, err == nil, "cannot write tar member")
	}
	requireFixture(t, archive.Close() == nil && zipped.Close() == nil, "cannot close archive")
	requireFixture(t, os.WriteFile(path, raw.Bytes(), 0o600) == nil, "cannot write archive")
	return raw.Bytes()
}

// runCodexArchiveVerify gives the function the `||` context production gives it, where set -e is
// off inside it, so a comparison without its own `|| fail` lets the function run on.
func runCodexArchiveVerify(t *testing.T, archive, sums, contextDir string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", `source "$1"; shift; "$@" || exit "$?"`,
		"T-81", "../tools/lib/engine-fixture-checks.sh", "codex_archive_verify", archive, sums, contextDir)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	requireFixture(t, ctx.Err() == nil, "archive verification exceeded five seconds")
	return stdout.String(), stderr.String(), err
}

func testCodexArchiveVerify(t *testing.T) {
	other := codexHash([]byte("another value"))
	cases := []struct {
		name       string
		members    []codexTarMember
		archive    string // replaces line 1's hash when not empty
		member     string // replaces line 2's hash when not empty
		diagnostic string
	}{
		{name: "match", members: []codexTarMember{{codexMemberName, codexMemberBody}}},
		{name: "archive_hash", members: []codexTarMember{{codexMemberName, codexMemberBody}}, archive: other, diagnostic: "codex archive hash:"},
		{name: "member_hash", members: []codexTarMember{{codexMemberName, codexMemberBody}}, member: other, diagnostic: "codex member hash:"},
		{name: "member_name", members: []codexTarMember{{"codex", codexMemberBody}}, diagnostic: "codex archive members:"},
		{name: "second_member", members: []codexTarMember{{codexMemberName, codexMemberBody}, {"extra", "extra\n"}}, diagnostic: "codex archive members:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, contextDir := t.TempDir(), t.TempDir()
			archive := filepath.Join(dir, codexArchiveName)
			raw := writeCodexArchive(t, archive, tc.members)
			first, second := codexHash(raw), codexHash([]byte(codexMemberBody))
			if tc.archive != "" {
				first = tc.archive
			}
			if tc.member != "" {
				second = tc.member
			}
			sums := filepath.Join(dir, "SHA256SUMS")
			text := first + "  " + codexArchiveName + "\n" + second + "  " + codexMemberName + "\n"
			requireFixture(t, os.WriteFile(sums, []byte(text), 0o600) == nil, "cannot write sums")
			stdout, stderr, err := runCodexArchiveVerify(t, archive, sums, contextDir)
			entries, readErr := os.ReadDir(contextDir)
			requireFixture(t, readErr == nil, "cannot read context: %v", readErr)
			if tc.diagnostic == "" {
				requireFixture(t, err == nil && len(entries) == 1 && entries[0].Name() == "codex", "match failed: %v entries=%v stdout=%s stderr=%s", err, entries, stdout, stderr)
				info, statErr := os.Lstat(filepath.Join(contextDir, "codex"))
				data, dataErr := os.ReadFile(filepath.Join(contextDir, "codex"))
				requireFixture(t, statErr == nil && dataErr == nil && info.Mode() == 0o555 && string(data) == codexMemberBody, "wrong codex file: %v %v mode=%v data=%q", statErr, dataErr, info, data)
				return
			}
			requireFixture(t, err != nil && strings.Contains(stderr, "engine-fixture: "+tc.diagnostic), "accepted or wrong diagnostic: %v stderr=%s", err, stderr)
			requireFixture(t, len(entries) == 0, "failure left entries in the context: %v", entries)
		})
	}
}
