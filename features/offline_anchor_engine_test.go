package features

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// The original pathname request must also pass the recorder. Its unsafe
// resolution is the red subject, not something the test proxy may repair.
func (a *offlineAPI) anchorStart(r *http.Request, entry offlineRequest) bool {
	a.mu.Lock()
	s := a.w.anchor
	if s == nil || entry.Method != "POST" || !strings.HasSuffix(entry.Path, "/start") {
		a.mu.Unlock()
		return true
	}
	s.before = entry.Observation
	hold := s.release != nil && !s.held
	if hold {
		s.held = true
		close(s.ready)
	}
	a.mu.Unlock()
	if hold {
		select {
		case <-s.release:
		case <-r.Context().Done():
			return false
		}
	}
	return true
}

func (a *offlineAPI) anchorVolumeAllowed(r *http.Request, entry *offlineRequest) bool {
	s, w := a.w.anchor, a.w
	if r.Method == "POST" && entry.Path == "/volumes/create" {
		if w.anchors != nil {
			data, err := os.ReadFile(w.anchors.pidPath)
			if err != nil {
				return false
			}
			s.pid, err = strconv.Atoi(string(data))
			if err != nil {
				return false
			}
		}
		name, _ := entry.Body["Name"].(string)
		labels, options := object(entry.Body["Labels"]), object(entry.Body["DriverOpts"])
		prefix, token, _ := strings.Cut(name, "itos-container-")
		// Permit a role prefix without authorizing arbitrary volume names.
		parts := strings.Split(token, "-")
		if prefix != "" || !strings.HasPrefix(name, "itos-container-") || len(parts) == 0 || !offlineHex(parts[len(parts)-1]) ||
			strings.ContainsAny(name, "/.\\") ||
			len(s.volumes) != 0 || len(a.created) != 0 || r.URL.RawQuery != "" || entry.Body["Driver"] != "local" ||
			labels["io.itos-container.project"] != w.approval.project || labels["io.itos-container.fingerprint"] != w.approval.fingerprint ||
			len(options) != 3 || options["type"] != "none" || options["o"] != "bind" {
			return false
		}
		device, _ := options["device"].(string)
		fd, ok := strings.CutPrefix(device, fmt.Sprintf("/proc/%d/fd/", s.pid))
		n, err := strconv.Atoi(fd)
		identity, statErr := anchorIdentity(device)
		if s.pid <= 0 || !ok || err != nil || n < 0 || fd != strconv.Itoa(n) || statErr != nil || identity != s.originalID {
			return false
		}
		s.volumes[name] = &anchorVolume{name: name, labels: labels, options: options}
		return true
	}
	name := strings.TrimPrefix(entry.Path, "/volumes/")
	v := s.volumes[name]
	if v == nil {
		return false
	}
	if r.Method == "GET" {
		return r.URL.RawQuery == ""
	}
	if r.Method != "DELETE" || (r.URL.RawQuery != "" && r.URL.RawQuery != "force=false") {
		return false
	}
	observed, err := a.anchorInspect(v)
	if err != nil || !anchorVolumeMatches(v, observed) {
		return false
	}
	// A volume cannot be released while any possibly created worker remains.
	for _, id := range a.created {
		if a.w.doctor.real.read("/containers/"+id+"/json", nil, 404) != nil {
			return false
		}
	}
	entry.Observation = observed
	return true
}

func anchorVolumeMatches(v *anchorVolume, observed map[string]any) bool {
	return observed != nil && observed["Name"] == v.name && observed["Driver"] == "local" &&
		reflect.DeepEqual(observed["Labels"], v.labels) && reflect.DeepEqual(observed["Options"], v.options)
}

func (a *offlineAPI) anchorInspect(v *anchorVolume) (map[string]any, error) {
	var observed map[string]any
	if err := a.w.doctor.real.read("/volumes/"+url.PathEscape(v.name), &observed, 200); err != nil {
		return nil, err
	}
	if !anchorVolumeMatches(v, observed) {
		return nil, errors.New("native anchor volume identity changed")
	}
	return observed, nil
}

func (a *offlineAPI) anchorMount(m map[string]any) bool {
	if a.w.anchor == nil || m["Target"] != "/workspace" || m["ReadOnly"] == true {
		return false
	}
	name, _ := m["Source"].(string)
	v := a.w.anchor.volumes[name]
	options := object(m["VolumeOptions"])
	if v == nil || options["NoCopy"] != true || (options["Subpath"] != nil && options["Subpath"] != "") || !offlineEmpty(options["DriverConfig"]) {
		return false
	}
	_, err := a.anchorInspect(v)
	return err == nil
}

// Called under the recorder lock, only after identity-checked worker cleanup.
// These operations are harness cleanup and never added to product observations.
func (a *offlineAPI) cleanupAnchorVolumes(ctx context.Context) error {
	if a.w.anchor == nil {
		return nil
	}
	var result error
	for _, v := range a.w.anchor.volumes {
		path := "/volumes/" + url.PathEscape(v.name)
		if a.w.doctor.real.read(path, nil, 404) == nil {
			continue
		}
		if _, err := a.anchorInspect(v); err != nil {
			result = errors.Join(result, err)
			continue
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://engine"+path, nil)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		response, err := a.w.doctor.real.client.Do(request)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			result = errors.Join(result, fmt.Errorf("native anchor harness cleanup HTTP %d", response.StatusCode))
			continue
		}
		result = errors.Join(result, a.w.doctor.real.read(path, nil, 404))
	}
	return result
}
