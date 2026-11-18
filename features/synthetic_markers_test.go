package features

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const syntheticScanChunk = 32 * 1024

type syntheticMarkerWorld struct {
	values  []string
	trees   map[string]string
	outputs map[string][]byte
}

type syntheticNeedle struct {
	data     []byte
	encoding string
}

type syntheticFound struct{ surface, encoding string }

func (e *syntheticFound) Error() string {
	return fmt.Sprintf("surface %q contains a registered synthetic marker (%s)", e.surface, e.encoding)
}

func syntheticNeedles(markers []string) ([]syntheticNeedle, error) {
	if len(markers) == 0 {
		return nil, errors.New("no registered synthetic markers")
	}
	var needles []syntheticNeedle
	for _, marker := range markers {
		if marker == "" {
			return nil, errors.New("empty synthetic marker")
		}
		for _, c := range []byte(marker) {
			if c > 127 {
				return nil, errors.New("synthetic marker is not ASCII")
			}
		}
		needles = append(needles, syntheticNeedle{[]byte(marker), "raw"})
		for align := range 3 {
			// Keep only sextets wholly inside the marker. The neighbouring
			// bytes, padding, and the outer field's starting offset are unknown.
			// This same window finds padded and unpadded representations.
			start, end := (align*8+5)/6, ((align+len(marker))*8)/6
			for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
				encoded := encoding.EncodeToString(append(make([]byte, align), []byte(marker)...))
				if end > start {
					needles = append(needles, syntheticNeedle{[]byte(encoded[start:end]), "base64"})
				}
			}
		}
	}
	return needles, nil
}

// syntheticAbsent streams with enough overlap for the longest encoded marker.
// Error diagnostics name only the declared surface and representation, never
// the marker, file name, matching bytes, or an underlying reader's error text.
func syntheticAbsent(surface string, input io.Reader, markers []string) error {
	needles, err := syntheticNeedles(markers)
	if err != nil {
		return err
	}
	keep := 0
	for _, needle := range needles {
		keep = max(keep, len(needle.data)-1)
	}
	buffer := make([]byte, syntheticScanChunk+keep)
	tail := 0
	for {
		n, err := input.Read(buffer[tail : tail+syntheticScanChunk])
		data := buffer[:tail+n]
		for _, needle := range needles {
			if bytes.Contains(data, needle.data) {
				return &syntheticFound{surface, needle.encoding}
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil || n == 0 {
			return fmt.Errorf("surface %q could not be completely read", surface)
		}
		tail = min(keep, len(data))
		copy(buffer, data[len(data)-tail:])
	}
}

func syntheticTreeAbsent(surface, root string, markers []string) error {
	if _, err := syntheticNeedles(markers); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("surface %q is not a readable tree", surface)
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("surface %q is not a readable tree", surface)
	}
	defer func() { _ = directory.Close() }()
	err = fs.WalkDir(directory.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("surface %q could not be completely walked", surface)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("surface %q contains an unexpected nonregular entry", surface)
		}
		file, err := directory.Open(path)
		if err != nil {
			return fmt.Errorf("surface %q has an unreadable file", surface)
		}
		defer func() { _ = file.Close() }()
		return syntheticAbsent(surface, file, markers)
	})
	return err
}

// syntheticPositiveControl exercises the actual assertions in a separate
// disposable tree before a scenario judges any product surface. Each encoded
// form crosses a read boundary, and neither planted bytes nor paths enter a
// product tree or a diagnostic.
func syntheticPositiveControl(markers []string) error {
	if _, err := syntheticNeedles(markers); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "slice-28-marker-control-")
	if err != nil {
		return errors.New("cannot create synthetic positive control")
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, ".hidden")
	for _, marker := range markers {
		forms := [][]byte{[]byte(marker)}
		for align := range 3 {
			for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
				forms = append(forms, []byte(encoding.EncodeToString(append(bytes.Repeat([]byte{0xff}, align), []byte(marker)...))))
			}
		}
		for _, form := range forms {
			data := append(bytes.Repeat([]byte{0}, syntheticScanChunk-1), form...)
			var detected *syntheticFound
			if !errors.As(syntheticAbsent("positive control output", bytes.NewReader(data), markers), &detected) {
				return errors.New("synthetic output positive control was missed")
			}
			if os.WriteFile(path, data, 0600) != nil {
				return errors.New("cannot plant synthetic tree positive control")
			}
			if !errors.As(syntheticTreeAbsent("positive control tree", dir, markers), &detected) {
				return errors.New("synthetic tree positive control was missed")
			}
		}
	}
	if os.WriteFile(path, nil, 0600) != nil {
		return errors.New("cannot clear synthetic positive control")
	}
	return errors.Join(syntheticAbsent("clean control output", strings.NewReader(""), markers), syntheticTreeAbsent("clean control tree", dir, markers))
}

func (w *world) syntheticMarkerSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^the synthetic markers are absent from tree "([^"]*)"$`, w.syntheticTree)
	sc.Step(`^the synthetic markers are absent from output "([^"]*)"$`, w.syntheticOutput)
}

func (w *world) syntheticOutput(name string) error {
	if w.markers == nil {
		return errors.New("no registered synthetic markers")
	}
	data, ok := w.markers.outputs[name]
	if !ok {
		return fmt.Errorf("output surface %q was not captured", name)
	}
	if err := syntheticPositiveControl(w.markers.values); err != nil {
		return err
	}
	return syntheticAbsent(name, bytes.NewReader(data), w.markers.values)
}

func (w *world) syntheticTree(name string) error {
	if w.markers == nil {
		return errors.New("no registered synthetic markers")
	}
	root, ok := w.markers.trees[name]
	if !ok {
		return fmt.Errorf("tree surface %q was not captured", name)
	}
	if err := syntheticPositiveControl(w.markers.values); err != nil {
		return err
	}
	return syntheticTreeAbsent(name, root, w.markers.values)
}
