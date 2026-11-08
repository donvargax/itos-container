package features

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These captured-data validators never invoke the engine fixture driver.
func testFixtureImageStores(t *testing.T, built, other, manifest string) {
	t.Run("image stores", func(t *testing.T) {
		for _, media := range []string{"application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json"} {
			body := strings.ReplaceAll(manifest, "application/vnd.oci.image.manifest.v1+json", media)
			digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(body)))
			for _, tc := range []struct{ name, id, config, store, diagnostic string }{
				{"containerd", digest, built, "containerd", ""},
				{"graph driver", built, built, "graph-driver", ""},
				{"unrelated built ID", other, built, "", "manifest identity"},
				{"containerd missing config", digest, "", "", "daemon config identity"},
				{"containerd invalid config", digest, "bad-id", "", "daemon config identity"},
				{"containerd wrong config", digest, other, "", "manifest validation"},
				{"containerd manifest as config", digest, digest, "", "manifest validation"},
				{"graph driver wrong config", built, other, "", "manifest identity"},
			} {
				t.Run(media+"/"+tc.name, func(t *testing.T) {
					stdout, stderr, err := runFixtureValidator(t, "validate_manifest", fixtureResponseFile(t, "manifest.json", body), fixtureResponseFile(t, "headers", "Docker-Content-Digest: "+digest+"\r\n"), tc.id, tc.config)
					if tc.diagnostic == "" {
						requireFixture(t, err == nil && strings.TrimSpace(stdout) == digest, "valid %s manifest rejected: %v stdout=%s stderr=%s", tc.store, err, stdout, stderr)
						requireFixture(t, strings.Contains(stderr, "store="+tc.store) && strings.Contains(stderr, "daemonConfigID="+built), "store or independent config observation missing: %s", stderr)
						// The cached daemon ID remains the raw built ID on both stores.
						reference := "127.0.0.1:1234/fixture@" + digest
						cached := `[{"Id":"` + tc.id + `","RepoDigests":["` + reference + `"],"Os":"linux","Architecture":"amd64","Config":{"User":"1001:1002"}}]`
						_, stderr, err = runFixtureValidator(t, "validate_cached", fixtureResponseFile(t, "cached.json", cached), reference, tc.id, "1001", "1002")
						requireFixture(t, err == nil, "cached %s ID rejected: %v %s", tc.store, err, stderr)
					} else {
						requireFixture(t, err != nil && stdout == "" && strings.Contains(stderr, "engine-fixture: "+tc.diagnostic), "invalid identity lacks %s diagnostic: %v %s", tc.diagnostic, err, stderr)
					}
				})
			}
		}
		for _, body := range []string{
			strings.ReplaceAll(manifest, `"schemaVersion":2`, `"schemaVersion":1`),
			strings.ReplaceAll(manifest, "application/vnd.oci.image.manifest.v1+json", "application/vnd.oci.image.index.v1+json"),
			strings.ReplaceAll(manifest, `"config":`, `"manifests":[],"config":`),
			strings.ReplaceAll(manifest, `{"digest":"`+built+`"}`, `null`),
		} {
			digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(body)))
			stdout, stderr, err := runFixtureValidator(t, "validate_manifest", fixtureResponseFile(t, "manifest.json", body), fixtureResponseFile(t, "headers", "Docker-Content-Digest: "+digest+"\r\n"), digest, built)
			requireFixture(t, err != nil && stdout == "" && strings.Contains(stderr, "engine-fixture: manifest validation"), "containerd identity bypassed manifest validation: %v %s", err, stderr)
		}
	})

	t.Run("daemon export config", func(t *testing.T) {
		config := `{"architecture":"amd64","os":"linux","config":{"User":"1001:1002"}}` + "\n"
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(config)))
		hex := strings.TrimPrefix(digest, "sha256:")
		for _, member := range []string{hex + ".json", "blobs/sha256/" + hex} {
			exported := `[{"Config":"` + member + `","RepoTags":null,"Layers":[]}]`
			stdout, stderr, err := runFixtureValidator(t, "daemon_config_path", fixtureResponseFile(t, "export.json", exported))
			requireFixture(t, err == nil && strings.TrimSpace(stdout) == member, "valid export rejected: %v %s", err, stderr)
			stdout, stderr, err = runFixtureValidator(t, "daemon_config_digest", fixtureResponseFile(t, "config.json", config), member)
			requireFixture(t, err == nil && strings.TrimSpace(stdout) == digest, "raw config bytes not independently hashed: %v %s %s", err, stdout, stderr)
		}
		for _, body := range []string{"{", "", "[]", "null", "{}", `[{}]`, `[{"Config":false}]`, `[{"Config":"/etc/passwd"}]`, `[{"Config":"../config.json"}]`, `[{"Config":"blobs/sha256/no"}]`, `[{"Config":"` + hex + `.json"},{"Config":"` + hex + `.json"}]`} {
			stdout, stderr, err := runFixtureValidator(t, "daemon_config_path", fixtureResponseFile(t, "export.json", body))
			requireFixture(t, err != nil && stdout == "" && strings.Contains(stderr, "engine-fixture: daemon export"), "invalid export accepted or undiagnosed: %v %s %s", err, stdout, stderr)
		}
		stdout, stderr, err := runFixtureValidator(t, "daemon_config_digest", fixtureResponseFile(t, "config.json", config+" "), "blobs/sha256/"+hex)
		requireFixture(t, err != nil && stdout == "" && strings.Contains(stderr, "engine-fixture: daemon config bytes"), "raw config byte mismatch accepted: %v %s", err, stderr)
		for _, body := range []string{strings.ReplaceAll(config, "amd64", "arm64"), strings.ReplaceAll(config, "linux", "windows"), strings.ReplaceAll(config, `{"User":"1001:1002"}`, `null`), "{", "", config + config} {
			member := fmt.Sprintf("blobs/sha256/%x", sha256.Sum256([]byte(body)))
			stdout, stderr, err := runFixtureValidator(t, "daemon_config_digest", fixtureResponseFile(t, "config.json", body), member)
			requireFixture(t, err != nil && stdout == "" && strings.Contains(stderr, "engine-fixture: daemon config validation"), "invalid raw config accepted or undiagnosed: %v %s %s", err, stdout, stderr)
		}
		missing := filepath.Join(t.TempDir(), "absent")
		for _, tc := range []struct {
			function, diagnostic string
			args                 []string
		}{
			{"daemon_config_path", "daemon export metadata", []string{missing}},
			{"daemon_config_digest", "daemon config hashing", []string{missing, "blobs/sha256/" + hex}},
		} {
			stdout, stderr, err := runFixtureValidator(t, tc.function, tc.args...)
			requireFixture(t, err != nil && stdout == "" && strings.Contains(stderr, "engine-fixture: "+tc.diagnostic), "missing export file lost diagnostic: %v %s", err, stderr)
		}
	})

}

func runFixtureValidator(t *testing.T, function string, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	argv := append([]string{"-euo", "pipefail", "-c", `source "$1"; shift; "$@"`, "T-34", "../tools/lib/engine-fixture-checks.sh", function}, args...)
	cmd := exec.CommandContext(ctx, "bash", argv...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	requireFixture(t, ctx.Err() == nil, "validator exceeded five seconds")
	return stdout.String(), stderr.String(), err
}

func fixtureResponseFile(t *testing.T, name, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	requireFixture(t, os.WriteFile(path, []byte(data), 0o600) == nil, "cannot write %s", path)
	return path
}
