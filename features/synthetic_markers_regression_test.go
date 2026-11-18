package features

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyntheticMarkerAbsenceAssertions(t *testing.T) {
	// '?' and '~' make the standard and URL alphabets observably different.
	marker := "SYNTHETIC_?~_MARKER_" + egressNonce()
	forms := map[string][]byte{"raw": []byte(marker)}
	for align := range 3 {
		for i, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			input := append(bytes.Repeat([]byte{0xfb}, align), []byte(marker)...)
			// A suffix changes the last partial sextet: matching an encoding of
			// the bare marker or of its old fixed prefix is not sufficient.
			input = append(input, 0xff, 0xfe)
			forms[fmt.Sprintf("base64-%d-%d", align, i)] = []byte(encoding.EncodeToString(input))
		}
	}
	for name, form := range forms {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".git", "hidden")
			if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			data := append(bytes.Repeat([]byte{0}, syntheticScanChunk-3), form...)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			w := &world{markers: &syntheticMarkerWorld{values: []string{marker}, trees: map[string]string{"protected": root}, outputs: map[string][]byte{"captured": data}}}
			for _, err := range []error{w.syntheticOutput("captured"), w.syntheticTree("protected")} {
				var found *syntheticFound
				if !errors.As(err, &found) {
					t.Fatalf("planted control was not detected: %v", err)
				}
				if strings.Contains(err.Error(), marker) || strings.Contains(err.Error(), string(form)) || strings.Contains(err.Error(), path) {
					t.Fatal("diagnostic disclosed planted data")
				}
				want := "base64"
				if name == "raw" {
					want = "raw"
				}
				if found.encoding != want {
					t.Fatalf("encoding = %s, want %s", found.encoding, want)
				}
			}
			w.markers.outputs["captured"] = nil // explicitly captured empty bytes are valid
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := errors.Join(w.syntheticOutput("captured"), w.syntheticTree("protected")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type syntheticErrorReader struct{ data []byte }

func (r syntheticErrorReader) Read(p []byte) (int, error) {
	return copy(p, r.data), errors.New("sensitive reader detail")
}

func TestSyntheticMarkerAbsenceFailsClosed(t *testing.T) {
	marker := "SYNTHETIC_MARKER_" + egressNonce()
	for _, markers := range [][]string{nil, {""}, {"not-ASCII-\u00e9"}} {
		if syntheticAbsent("invalid markers", strings.NewReader(""), markers) == nil {
			t.Fatal("invalid marker registration passed")
		}
	}
	err := syntheticAbsent("unreadable", syntheticErrorReader{[]byte("clean prefix")}, []string{marker})
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("reader failure lost or disclosed: %v", err)
	}
	var found *syntheticFound
	if err = syntheticAbsent("last read", syntheticErrorReader{[]byte(marker)}, []string{marker}); !errors.As(err, &found) {
		t.Fatalf("data accompanying an error was not scanned: %v", err)
	}
	w := &world{markers: &syntheticMarkerWorld{values: []string{marker}}}
	if w.syntheticOutput("missing") == nil || w.syntheticTree("missing") == nil {
		t.Fatal("uncaptured surface passed")
	}
	if (&world{}).syntheticOutput("missing") == nil || (&world{}).syntheticTree("missing") == nil {
		t.Fatal("unregistered markers passed")
	}
	root := t.TempDir()
	if syntheticTreeAbsent("missing", filepath.Join(root, "missing"), []string{marker}) == nil {
		t.Fatal("missing tree passed")
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
		t.Skip("symlink control unavailable")
	}
	if syntheticTreeAbsent("linked", root, []string{marker}) == nil {
		t.Fatal("out-of-tree symlink was silently skipped")
	}
}

type syntheticBoundedReader struct{ remaining, largest int }

func (r *syntheticBoundedReader) Read(p []byte) (int, error) {
	r.largest = max(r.largest, len(p))
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.remaining)
	clear(p[:n])
	r.remaining -= n
	return n, nil
}

func TestSyntheticMarkerScanStreamsLargeInput(t *testing.T) {
	r := &syntheticBoundedReader{remaining: 40 * 1024 * 1024}
	if err := syntheticAbsent("large", r, []string{"SYNTHETIC_MARKER_" + egressNonce()}); err != nil {
		t.Fatal(err)
	}
	if r.remaining != 0 || r.largest > syntheticScanChunk {
		t.Fatalf("stream was truncated or read without a bound: %+v", r)
	}
}
