package features

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type doctorReal struct {
	client        *http.Client
	transport     *http.Transport
	resources     map[string]string
	serverVersion string
}

func (w *world) doctorRuntimeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a CI-only doctor checkout and a preloaded native Docker worker fixture$`, func() error { return w.doctorRealCheckout(false) })
	sc.Step(`^a CI-only doctor checkout requesting a digest absent from native Docker$`, func() error { return w.doctorRealCheckout(true) })
	sc.Step(`^a read-only engine request recorder and a snapshot of engine resources$`, w.doctorRealRecorder)
	sc.Step(`^its JSON inspection identities agree with the CI engine and cached image observations$`, w.doctorRealIdentifies)
	sc.Step(`^the engine recorder observed only inspection requests$`, w.doctorJournal)
	sc.Step(`^engine resources, checkout files and host control state are unchanged$`, w.doctorRealUnchanged)
}

func (w *world) doctorSentinelsPortable() error {
	if runtime.GOOS == "linux" {
		return w.doctorSentinels()
	}
	return nil
}

// This guard uses the actual process context, independently of selection. No
// ordinary fixture hook reads engine inputs or creates a real transport.
func doctorRealGuard() error {
	if os.Getenv("ITOS_CONTAINER_ENGINE_TESTS") != "1" || os.Getenv("CI") != "true" || os.Getenv("GITHUB_ACTIONS") != "true" || runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("real doctor fixture requires the CI engine tier on native Linux amd64")
	}
	return nil
}

func (w *world) doctorRealCheckout(absent bool) error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	image := os.Getenv("ITOS_CONTAINER_ENGINE_FIXTURE_IMAGE")
	id := os.Getenv("ITOS_CONTAINER_ENGINE_FIXTURE_ID")
	uid, gid := os.Getenv("ITOS_CONTAINER_ENGINE_FIXTURE_UID"), os.Getenv("ITOS_CONTAINER_ENGINE_FIXTURE_GID")
	if !regexp.MustCompile(`^127\.0\.0\.1:[1-9][0-9]*/[a-z0-9][a-z0-9/_-]*@sha256:[0-9a-f]{64}$`).MatchString(image) || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(id) || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(uid) || !regexp.MustCompile(`^[0-9]+$`).MatchString(gid) {
		return fmt.Errorf("absent or malformed CI cached-image fixture inputs")
	}
	if err := w.doctorCheckout(); err != nil {
		return err
	}
	transport := doctorTransport("/var/run/docker.sock")
	real := &doctorReal{transport: transport, client: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	w.doctor.real = real
	w.doctor.cleanup = append(w.doctor.cleanup, transport.CloseIdleConnections)
	var version struct {
		Version, APIVersion, MinAPIVersion, Os, Arch string
		Platform                                     struct{ Name string }
	}
	if err := real.read("/version", &version, 200); err != nil {
		return err
	}
	if version.Os != "linux" || version.Arch != "amd64" || (version.Platform.Name != "Docker Engine - Community" && version.Platform.Name != "Docker Engine - Enterprise") {
		return fmt.Errorf("CI fixture is not native Linux amd64 Docker")
	}
	real.serverVersion = version.Version
	var info struct{ ID, OSType, Architecture, OperatingSystem string }
	if err := real.read("/info", &info, 200); err != nil {
		return err
	}
	if info.ID == "" || info.OSType != "linux" || (info.Architecture != "x86_64" && info.Architecture != "amd64") || strings.Contains(strings.ToLower(info.OperatingSystem), "desktop") {
		return fmt.Errorf("CI daemon identity/platform is unsupported")
	}
	var cached struct {
		ID, Os, Architecture string
		RepoDigests          []string
		Config               *struct {
			User       string
			Entrypoint []string
			Volumes    map[string]any
			Labels     map[string]string
		}
	}
	if err := real.read("/images/"+image+"/json", &cached, 200); err != nil {
		return err
	}
	if cached.ID != id || !slices.Contains(cached.RepoDigests, image) || cached.Os != "linux" || cached.Architecture != "amd64" || cached.Config == nil || cached.Config.User != uid+":"+gid || len(cached.Config.Entrypoint) != 0 || len(cached.Config.Volumes) != 0 {
		return fmt.Errorf("cached image disagrees with the CI fixture protocol")
	}
	if _, present := cached.Config.Labels["devcontainer.metadata"]; present {
		return fmt.Errorf("CI fixture unexpectedly carries devcontainer.metadata")
	}
	api, err := doctorNegotiatedAPI(version.APIVersion, version.MinAPIVersion)
	if err != nil {
		return err
	}
	d := w.doctor
	d.image, d.imageID, d.engineID, d.api = image, id, info.ID, api
	if absent {
		repository, _, _ := strings.Cut(image, "@")
		digest := strings.Repeat("c", 64)
		if strings.HasSuffix(image, digest) {
			digest = strings.Repeat("d", 64)
		}
		d.image = repository + "@sha256:" + digest
		if err := real.read("/images/"+d.image+"/json", nil, 404); err != nil {
			return fmt.Errorf("absent digest was not independently confirmed: %w", err)
		}
	}
	d.manifest["image"] = d.image
	return w.doctorUsers(uid, uid)
}

func doctorNegotiatedAPI(maximum, minimum string) (string, error) {
	parse := func(value string) (int, error) {
		major, minor, ok := strings.Cut(value, ".")
		if !ok || major != "1" {
			return 0, fmt.Errorf("invalid or unsupported API range")
		}
		n, err := strconv.Atoi(minor)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid API version")
		}
		return n, nil
	}
	maxAPI, err := parse(maximum)
	if err != nil {
		return "", err
	}
	minAPI, err := parse(minimum)
	if err != nil {
		return "", err
	}
	negotiated := min(maxAPI, 56)
	if negotiated < 44 || minAPI > negotiated {
		return "", fmt.Errorf("CI server has incompatible SDK API range")
	}
	return fmt.Sprintf("1.%d", negotiated), nil
}

func (r *doctorReal) read(path string, value any, status int) error {
	// Fixed local transport; no proxy, environment lookup, redirect or acquisition.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://engine"+path, nil)
	if err != nil {
		return err
	}
	response, err := r.client.Do(request)
	if err != nil {
		return fmt.Errorf("CI read-only engine observation failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != status {
		return fmt.Errorf("CI inspection %s returned HTTP %d, expected %d", path, response.StatusCode, status)
	}
	if value == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(value); err != nil {
		return fmt.Errorf("CI inspection JSON unavailable: %w", err)
	}
	return nil
}

func (w *world) doctorRealRecorder() error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	if w.doctor.real == nil {
		return fmt.Errorf("CI engine fixture not initialized")
	}
	a := newDoctorAPI(w.doctor.image)
	a.forward = w.doctor.real.transport
	if err := w.startDoctorAPI(a); err != nil {
		return err
	}
	w.doctor.engine = a
	w.doctor.env["DOCKER_HOST"] = "unix://" + a.socket
	var err error
	w.doctor.real.resources, err = w.doctor.real.snapshotResources()
	return err
}

func (a *doctorAPI) forwardInspection(out http.ResponseWriter, r *http.Request) {
	// serve journals and checks every request BEFORE forwarding. Only the selected
	// read endpoints reach Docker; refused writes still fail the journal assertion.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	request := r.Clone(ctx)
	request.URL = &url.URL{Scheme: "http", Host: "engine", Path: r.URL.Path}
	request.Host = "engine"
	request.RequestURI = ""
	request.Header = make(http.Header)
	response, err := a.forward.RoundTrip(request)
	if err != nil {
		http.Error(out, "CI recorder could not inspect engine", http.StatusBadGateway)
		return
	}
	defer func() { _ = response.Body.Close() }()
	for _, name := range []string{"Content-Type", "API-Version", "Docker-Experimental", "OSType"} {
		if value := response.Header.Get(name); value != "" {
			out.Header().Set(name, value)
		}
	}
	out.WriteHeader(response.StatusCode)
	_, _ = io.Copy(out, io.LimitReader(response.Body, 4<<20))
}

func (r *doctorReal) snapshotResources() (map[string]string, error) {
	result := map[string]string{}
	// Compare stable resource identity/configuration, not humanized uptime strings.
	for _, spec := range []struct {
		path, wrapper string
		fields        []string
	}{
		{"/containers/json?all=1", "", []string{"Id", "Names", "ImageID", "State", "Labels", "HostConfig", "Mounts", "Ports"}},
		{"/images/json?all=1", "", []string{"Id", "RepoTags", "RepoDigests", "Labels"}},
		{"/networks", "", []string{"Id", "Name", "Driver", "Scope", "Containers", "Options", "Labels", "IPAM", "Internal"}},
		{"/volumes", "Volumes", []string{"Name", "Driver", "Mountpoint", "Labels", "Options", "Scope"}},
	} {
		var raw json.RawMessage
		if err := r.read(spec.path, &raw, 200); err != nil {
			return nil, err
		}
		if spec.wrapper != "" {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(raw, &object); err != nil {
				return nil, err
			}
			raw = object[spec.wrapper]
		}
		var entries []map[string]any
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, err
		}
		var encoded []string
		for _, entry := range entries {
			selected := map[string]any{}
			for _, field := range spec.fields {
				selected[field] = entry[field]
			}
			data, err := json.Marshal(selected)
			if err != nil {
				return nil, err
			}
			encoded = append(encoded, string(data))
		}
		slices.Sort(encoded)
		result[spec.path] = strings.Join(encoded, "\n")
	}
	return result, nil
}

func (w *world) doctorRealIdentifies() error {
	if err := w.doctorIdentifies(); err != nil {
		return err
	}
	// Only selected identity fields enter CI logs, never raw response content.
	_, _ = fmt.Fprintf(os.Stdout, "doctor CI observed host=%s/%s server=%s api=%s image=%s image_id=%s; actual identities match independent observations\n", runtime.GOOS, runtime.GOARCH, w.doctor.real.serverVersion, w.doctor.api, w.doctor.image, w.doctor.imageID)
	return nil
}

func (w *world) doctorRealUnchanged() error {
	if err := w.doctorUnchanged(); err != nil {
		return err
	}
	current, err := w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	for key, before := range w.doctor.real.resources {
		if current[key] != before {
			return fmt.Errorf("doctor changed engine resources at %s", key)
		}
	}
	return nil
}
