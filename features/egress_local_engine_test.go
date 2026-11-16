package features

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The proxy image's config ID differs from the worker's (doctorImageID) and
// from the manifest digest in the pinned reference, so swapping any two of the
// three cannot pass an assertion.
const egressProxyConfigID = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
const egressProxyReady = `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready"}`
const egressInternalAddress = "172.28.0.2"

// egressEvent is one engine request the fixture served, in arrival order. Role
// names the product resource it addressed: anchor, internal, egress, proxy or
// worker; an image inspection names the reference or ID it asked for.
type egressEvent struct {
	Kind, Role, Query string
	PID               int
}

// localEgressAPI extends the recorded local Engine API fixture with T-40's two
// networks, the proxy's stdout-ready record and exit, and a second, independent
// image response. It executes no workload, proxy or helper. Every create and
// removal is checked against the protected intent the product must have written
// first; a mismatch is kept as the fixture's problem for the Then steps.
type localEgressAPI struct {
	mu          sync.Mutex
	w           *world
	proxyImage  map[string]any
	proxyStatus int
	defect      string
	networks    map[string]map[string]any
	containers  map[string]map[string]any
	volumes     map[string]map[string]any
	roles       map[string]string         // full ID or volume name to role
	bodies      map[string]map[string]any // role to its create request
	ids         map[string]string         // role to full ID
	starts      map[string]int
	removed     []string
	attached    map[string]net.Conn
	exited      map[string]chan struct{}
	events      []egressEvent
	problem     error
	evidence    map[string]string // protected files naming the proxy config ID at the first create
	observed    bool
}

func (w *world) localEgressFixture() error {
	if w.doctor == nil || w.doctor.engine == nil || w.approval == nil {
		return fmt.Errorf("the egress fixture extends an approval checkout's Engine API fixture")
	}
	w.localOfflineFixture()
	w.runFixture()
	a := &localEgressAPI{w: w, networks: map[string]map[string]any{}, containers: map[string]map[string]any{}, volumes: map[string]map[string]any{},
		roles: map[string]string{}, bodies: map[string]map[string]any{}, ids: map[string]string{}, starts: map[string]int{},
		attached: map[string]net.Conn{}, exited: map[string]chan struct{}{},
		proxyImage: map[string]any{"Id": egressProxyConfigID, "RepoDigests": []any{"busybox@" + egressPinDigest()}, "Os": "linux", "Architecture": "amd64",
			"Config": map[string]any{"Env": []any{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}, "Cmd": []any{"sh"}, "Labels": map[string]any{}}}}
	e := w.doctor.engine
	e.mu.Lock()
	e.egress = a
	// The oldest daemon the isolated bridge exists on. Faults raise or lower it.
	e.version["Version"], e.version["ApiVersion"], e.version["MinAPIVersion"] = "28.0.0", "1.48", "1.24"
	e.version["Components"] = []any{map[string]any{"Name": "Engine", "Version": "28.0.0"}}
	e.info["ServerVersion"] = "28.0.0"
	e.mu.Unlock()
	w.doctor.api = "1.48"
	w.doctor.cleanup = append(w.doctor.cleanup, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, c := range a.attached {
			_ = c.Close()
		}
		for _, done := range a.exited {
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})
	return nil
}

func egressPinDigest() string { return busyboxFixturePin[strings.Index(busyboxFixturePin, "sha256:"):] }

func egressFrame(stream byte, data string) []byte {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	return append(header, data...)
}

func (a *localEgressAPI) fail(out http.ResponseWriter, format string, args ...any) {
	if a.problem == nil {
		a.problem = fmt.Errorf(format, args...)
	}
	http.Error(out, `{"message":"egress fixture refused the request"}`, http.StatusBadRequest)
}

// intent reads the product's protected record for token. Only the members this
// fixture relies on are read, so a later record version may add its own.
func (a *localEgressAPI) intent(token string) (map[string]any, error) {
	path := filepath.Join(a.w.doctor.env["XDG_STATE_HOME"], "itos-container", "intents", a.w.approval.project, token+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no protected intent before an engine effect: %w", err)
	}
	var record map[string]any
	if err = json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	if record["fingerprint"] != a.w.approval.fingerprint || record["project"] != a.w.approval.project || record["name"] != "itos-container-"+token {
		return nil, fmt.Errorf("protected intent names another approval or session")
	}
	return record, nil
}

func egressStage(record map[string]any, role string) (string, string) {
	key := map[string]string{"internal": "internal", "egress": "network", "proxy": "proxy"}[role]
	if key == "" {
		return fmt.Sprint(record["stage"]), fmt.Sprint(record["container_id"])
	}
	resource := object(object(record["egress"])[key])
	return fmt.Sprint(resource["stage"]), fmt.Sprint(resource["id"])
}

// recorded requires the durable stage and full ID for every role given, as
// "role=stage"; an owned or absent stage also requires that role's fixture ID.
func (a *localEgressAPI) recorded(token string, want ...string) error {
	record, err := a.intent(token)
	if err != nil {
		return err
	}
	for _, pair := range want {
		role, stage, _ := strings.Cut(pair, "=")
		got, id := egressStage(record, role)
		if got != stage {
			return fmt.Errorf("protected intent has %s stage %q, want %q", role, got, stage)
		}
		if (stage == "owned" || stage == "absent") && id != a.ids[role] {
			return fmt.Errorf("protected intent lacks the full %s ID", role)
		}
		if stage == "intent" && id != "" {
			return fmt.Errorf("protected %s intent already names an ID", role)
		}
	}
	return nil
}

func egressRole(name string) (role, token string) {
	rest, ok := strings.CutPrefix(name, "itos-container-")
	if !ok {
		return "", ""
	}
	for _, kind := range []string{"internal", "egress", "proxy", "anchor"} {
		if token, ok := strings.CutPrefix(rest, kind+"-"); ok {
			return kind, token
		}
	}
	return "worker", rest
}

func (a *localEgressAPI) token() string {
	for _, role := range []string{"worker", "proxy", "internal", "egress"} {
		if body := a.bodies[role]; body != nil {
			labels := object(body["Labels"])
			if token, _ := labels["io.itos-container.intent"].(string); token != "" {
				return token
			}
		}
	}
	return ""
}

// observeEvidence records, once, which protected files already name the proxy
// image's config ID when the product asks the engine for its first resource.
func (a *localEgressAPI) observeEvidence() {
	if a.observed {
		return
	}
	a.observed = true
	a.evidence = egressEvidence(a.w)
}

func egressEvidence(w *world) map[string]string {
	found := map[string]string{}
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), egressProxyConfigID) {
			found[path] = string(data)
		}
		return nil
	})
	return found
}

func (a *localEgressAPI) serve(out http.ResponseWriter, r *http.Request) bool {
	path := doctorRequestPath(r.URL.Path)
	pid, _ := r.Context().Value(runPeerKey{}).(int)
	a.mu.Lock()
	defer a.mu.Unlock()
	note := func(kind, role string) {
		a.events = append(a.events, egressEvent{Kind: kind, Role: role, Query: r.URL.RawQuery, PID: pid})
	}
	out.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
		reference := strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/json")
		engine := a.w.doctor.engine
		engine.mu.Lock()
		workerID, _ := engine.image["Id"].(string)
		worker := approvalClone(engine.image)
		engine.mu.Unlock()
		switch reference {
		case busyboxFixturePin, egressProxyConfigID:
			note("image", reference)
			if r.Method != http.MethodGet || r.URL.RawQuery != "" {
				a.fail(out, "proxy image request is not a plain inspection")
				return true
			}
			if a.proxyStatus != 0 {
				out.WriteHeader(a.proxyStatus)
				_, _ = out.Write([]byte(`{"message":"fixture failure"}`))
				return true
			}
			_ = json.NewEncoder(out).Encode(a.proxyImage)
			return true
		case workerID:
			note("image", reference)
			_ = json.NewEncoder(out).Encode(worker)
			return true
		}
		return false
	case strings.HasPrefix(path, "/volumes/"):
		a.observeEvidence()
		name := strings.TrimPrefix(path, "/volumes/")
		kind := map[string]string{"POST": "create", "GET": "inspect", "DELETE": "remove"}[r.Method]
		if r.Method == "DELETE" {
			var want []string
			for _, role := range []string{"internal", "egress", "proxy"} {
				if a.ids[role] != "" {
					want = append(want, role+"=absent")
				}
			}
			if _, token := egressRole(name); token != "" && len(want) != 0 {
				if err := a.recorded(token, want...); err != nil {
					a.fail(out, "anchor released before its egress resources: %v", err)
					return true
				}
			}
			a.removed = append(a.removed, "anchor")
		}
		note("volume-"+kind, "anchor")
		serveLocalVolume(out, r, a.volumes)
		return true
	case path == "/networks/create" && r.Method == "POST":
		a.observeEvidence()
		a.createNetwork(out, r, note)
		return true
	case strings.HasPrefix(path, "/networks/"):
		a.network(out, r, strings.Split(strings.TrimPrefix(path, "/networks/"), "/"), note)
		return true
	case path == "/containers/create" && r.Method == "POST":
		a.observeEvidence()
		a.createContainer(out, r, pid, note)
		return true
	case strings.HasPrefix(path, "/containers/"):
		return a.container(out, r, strings.Split(strings.TrimPrefix(path, "/containers/"), "/"), note)
	}
	return false
}

func (a *localEgressAPI) createNetwork(out http.ResponseWriter, r *http.Request, note func(string, string)) {
	var body map[string]any
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		a.fail(out, "undecodable network create")
		return
	}
	name, _ := body["Name"].(string)
	role, token := egressRole(name)
	note("network-create", role)
	if (role != "internal" && role != "egress") || a.bodies[role] != nil {
		a.fail(out, "unexpected or repeated network create %q", name)
		return
	}
	want := []string{"internal=intent"}
	if role == "egress" {
		want = []string{"internal=owned", "egress=intent"}
	}
	a.bodies[role] = body
	if err := a.recorded(token, want...); err != nil {
		a.fail(out, "%s network create: %v", role, err)
		return
	}
	id := strings.Repeat(map[string]string{"internal": "1", "egress": "2"}[role], 64)
	observed := approvalClone(body)
	observed["Id"], observed["Scope"], observed["Containers"] = id, "local", map[string]any{}
	if role == "internal" {
		observed["IPAM"] = map[string]any{"Driver": "default", "Config": []any{map[string]any{"Subnet": "172.28.0.0/16"}}}
	} else {
		observed["IPAM"] = map[string]any{"Driver": "default", "Config": []any{map[string]any{"Subnet": "172.29.0.0/16", "Gateway": "172.29.0.1"}}}
	}
	a.networks[id], a.roles[id], a.ids[role] = observed, role, id
	out.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(out).Encode(map[string]string{"Id": id})
}

func (a *localEgressAPI) network(out http.ResponseWriter, r *http.Request, parts []string, note func(string, string)) {
	id := parts[0]
	observed := a.networks[id]
	if observed == nil {
		for candidate, value := range a.networks {
			if value["Name"] == id {
				id, observed = candidate, value
			}
		}
	}
	role := a.roles[id]
	if len(parts) != 1 || (r.Method != "GET" && r.Method != "DELETE") {
		// Connect, disconnect and prune would each be a route the product must not add.
		note("network-"+strings.ToLower(r.Method)+"-"+strings.Join(parts[1:], "/"), role)
		a.fail(out, "forbidden network request %s %s", r.Method, r.URL.Path)
		return
	}
	if observed == nil {
		note("network-absent", parts[0])
		http.Error(out, `{"message":"absent"}`, http.StatusNotFound)
		return
	}
	if r.Method == "DELETE" {
		note("network-remove", role)
		if len(object(observed["Containers"])) != 0 {
			a.fail(out, "%s network removed while a container is attached", role)
			return
		}
		if err := a.recorded(a.token(), role+"=owned"); err != nil {
			a.fail(out, "%s network removal: %v", role, err)
			return
		}
		delete(a.networks, id)
		a.removed = append(a.removed, role)
		out.WriteHeader(http.StatusNoContent)
		return
	}
	note("network-inspect", role)
	reply := approvalClone(observed)
	if role == "internal" {
		switch a.defect {
		case "internal bridge ignores isolated mode":
			reply["Options"] = map[string]any{}
		case "internal bridge has an IPAM gateway":
			reply["IPAM"] = map[string]any{"Driver": "default", "Config": []any{map[string]any{"Subnet": "172.28.0.0/16", "Gateway": "172.28.0.1"}}}
		case "internal bridge enables IPv6":
			reply["EnableIPv6"] = true
		}
	}
	_ = json.NewEncoder(out).Encode(reply)
}

func (a *localEgressAPI) createContainer(out http.ResponseWriter, r *http.Request, pid int, note func(string, string)) {
	var body map[string]any
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		a.fail(out, "undecodable container create")
		return
	}
	name := r.URL.Query().Get("name")
	role, token := egressRole(name)
	note("container-create", role)
	if (role != "proxy" && role != "worker") || a.bodies[role] != nil {
		a.fail(out, "unexpected or repeated container create %q", name)
		return
	}
	a.bodies[role] = body
	host := object(body["HostConfig"])
	networks := map[string]any{}
	for network, endpoint := range object(object(body["NetworkingConfig"])["EndpointsConfig"]) {
		networks[network] = approvalClone(object(endpoint))
	}
	id := strings.Repeat(map[string]string{"proxy": "3", "worker": "4"}[role], 64)
	var mounts []any
	if role == "proxy" {
		if err := a.recorded(token, "internal=owned", "egress=owned", "proxy=intent"); err != nil {
			a.fail(out, "proxy create: %v", err)
			return
		}
		for _, raw := range anyList(host["Mounts"]) {
			m := object(raw)
			mounts = append(mounts, map[string]any{"Type": m["Type"], "Source": m["Source"], "Destination": m["Target"], "RW": m["ReadOnly"] != true})
		}
	} else {
		expected := a.w.doctor.engine.offline.argv(pid)
		if !sameJSON(body["Cmd"], offlineStrings(expected)) || !sameJSON(body["Entrypoint"], []any{}) {
			a.fail(out, "worker create lost the literal argv")
			return
		}
		if err := a.recorded(token, "worker=intent", "internal=owned", "egress=owned", "proxy=owned"); err != nil {
			a.fail(out, "worker create: %v", err)
			return
		}
		mounts = localAnchorMount(host)
	}
	container := map[string]any{"Id": id, "Name": "/" + name, "Image": body["Image"], "Config": body, "HostConfig": host, "Mounts": mounts,
		"NetworkSettings": map[string]any{"Networks": networks}, "State": map[string]any{"Status": "created", "Running": false, "Pid": 0, "StartedAt": "0001-01-01T00:00:00Z"}}
	a.containers[id], a.roles[id], a.ids[role], a.exited[id] = container, role, id, make(chan struct{})
	out.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(out).Encode(map[string]string{"Id": id})
}

func (a *localEgressAPI) container(out http.ResponseWriter, r *http.Request, parts []string, note func(string, string)) bool {
	id := parts[0]
	container := a.containers[id]
	if container == nil {
		for candidate, value := range a.containers {
			if value["Name"] == "/"+id {
				id, container = candidate, value
			}
		}
	}
	role := a.roles[id]
	if container == nil {
		note("container-absent", parts[0])
		http.Error(out, `{"message":"absent"}`, http.StatusNotFound)
		return true
	}
	token := a.token()
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	switch {
	case len(parts) == 1 && r.Method == "DELETE":
		note("remove", role)
		state := object(container["State"])
		if state["Running"] == true {
			a.fail(out, "%s removed while running", role)
			return true
		}
		if err := a.recorded(token, role+"=owned"); err != nil {
			a.fail(out, "%s removal: %v", role, err)
			return true
		}
		if role == "proxy" && a.starts["proxy"] != 0 && !a.copied() {
			a.fail(out, "proxy removed before its stopped log was read")
			return true
		}
		delete(a.containers, id)
		a.detach(id)
		a.removed = append(a.removed, role)
		if conn := a.attached[id]; conn != nil {
			_ = conn.Close()
		}
		a.finish(id)
		out.WriteHeader(http.StatusNoContent)
	case action == "json" && r.Method == "GET":
		note("inspect", role)
		reply := approvalClone(container)
		a.applyDefect(role, reply)
		_ = json.NewEncoder(out).Encode(reply)
	case action == "attach" && r.Method == "POST" && role == "worker":
		note("attach", role)
		conn, buffer, err := out.(http.Hijacker).Hijack()
		if err != nil {
			a.problem = err
			return true
		}
		a.attached[id] = conn
		_, _ = buffer.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = buffer.Flush()
	case action == "start" && r.Method == "POST":
		note("start", role)
		a.starts[role]++
		if a.starts[role] != 1 {
			a.fail(out, "%s started more than once", role)
			return true
		}
		if err := a.recorded(token, role+"=owned"); err != nil {
			a.fail(out, "%s start: %v", role, err)
			return true
		}
		if role == "worker" && (a.attached[id] == nil || a.starts["proxy"] != 1 || !a.running("proxy")) {
			a.fail(out, "worker started without its attachment or a running proxy")
			return true
		}
		container["State"] = map[string]any{"Status": "running", "Running": true, "Pid": 123, "StartedAt": "2026-11-09T12:00:00Z"}
		a.join(id, container)
		if role == "worker" {
			_ = a.attached[id].Close()
		}
		out.WriteHeader(http.StatusNoContent)
	case action == "logs" && r.Method == "GET" && role == "proxy":
		note("logs", role)
		out.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		if a.starts["proxy"] != 0 {
			_, _ = out.Write(egressFrame(1, egressProxyReady+"\n"))
		}
	case action == "wait" && r.Method == "POST":
		note("wait", role)
		if role == "worker" {
			if a.starts["worker"] == 0 {
				a.fail(out, "wait for a worker that never started")
				return true
			}
			a.stop(id, container)
			_ = json.NewEncoder(out).Encode(map[string]any{"StatusCode": 0})
			return true
		}
		// The proxy runs until the controller's own SIGKILL. A watcher that
		// leaves first ends its subscription, never the proxy.
		done := a.exited[id]
		out.WriteHeader(http.StatusOK)
		out.(http.Flusher).Flush()
		a.mu.Unlock()
		select {
		case <-done:
		case <-r.Context().Done():
		}
		a.mu.Lock()
		if r.Context().Err() == nil {
			_ = json.NewEncoder(out).Encode(map[string]any{"StatusCode": 137})
		}
	case action == "kill" && r.Method == "POST":
		note("kill", role)
		if role == "proxy" && r.URL.Query().Get("signal") != "SIGKILL" {
			a.fail(out, "proxy stopped by another signal than SIGKILL")
			return true
		}
		if role == "proxy" && a.running("worker") {
			a.fail(out, "proxy stopped while the worker was running")
			return true
		}
		a.stop(id, container)
		out.WriteHeader(http.StatusNoContent)
	default:
		// Restart, exec, rename, update, network connect and any other request
		// are effects the specified launch never asks for.
		note("forbidden-"+strings.ToLower(r.Method)+"-"+action, role)
		a.fail(out, "forbidden container request %s %s", r.Method, r.URL.Path)
	}
	return true
}

func (a *localEgressAPI) running(role string) bool {
	container := a.containers[a.ids[role]]
	return container != nil && object(container["State"])["Running"] == true
}

func (a *localEgressAPI) finish(id string) {
	if done := a.exited[id]; done != nil {
		select {
		case <-done:
		default:
			close(done)
		}
	}
}

func (a *localEgressAPI) stop(id string, container map[string]any) {
	container["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-11-09T12:00:00Z", "ExitCode": 0}
	a.detach(id)
	a.finish(id)
}

// join gives a started container its endpoints, as only start allocates them.
func (a *localEgressAPI) join(id string, container map[string]any) {
	for name, raw := range object(object(container["NetworkSettings"])["Networks"]) {
		endpoint := object(raw)
		for networkID, network := range a.networks {
			if network["Name"] != name {
				continue
			}
			address := "172.29.0.2"
			if a.roles[networkID] == "internal" {
				address = egressInternalAddress
				if a.roles[id] == "worker" {
					address = "172.28.0.3"
				}
			}
			endpoint["EndpointID"], endpoint["IPAddress"] = strings.Repeat("e", 64), address
			object(network["Containers"])[id] = map[string]any{"Name": strings.TrimPrefix(fmt.Sprint(container["Name"]), "/")}
		}
	}
}

func (a *localEgressAPI) detach(id string) {
	for _, network := range a.networks {
		delete(object(network["Containers"]), id)
	}
}

// copied is whether the stopped proxy's log was read after its last stop.
func (a *localEgressAPI) copied() bool {
	stopped := -1
	for i, e := range a.events {
		if e.Role == "proxy" && e.Kind == "kill" {
			stopped = i
		}
	}
	for i, e := range a.events {
		if e.Role == "proxy" && e.Kind == "logs" && i > stopped && !strings.Contains(e.Query, "follow=1") && !strings.Contains(e.Query, "follow=true") {
			return true
		}
	}
	return false
}

func (a *localEgressAPI) applyDefect(role string, reply map[string]any) {
	switch {
	case role == "proxy" && a.defect == "proxy has IPv4 forwarding enabled":
		object(reply["HostConfig"])["Sysctls"] = map[string]any{"net.ipv4.ip_forward": "1"}
	case role == "proxy" && a.defect == "proxy executable mount is writable":
		for _, raw := range anyList(reply["Mounts"]) {
			object(raw)["RW"] = true
		}
	case role == "worker" && a.defect == "worker has an additional network":
		object(object(reply["NetworkSettings"])["Networks"])["bridge"] = map[string]any{"NetworkID": strings.Repeat("9", 64)}
	case role == "worker" && a.defect == "worker has a published port":
		object(reply["NetworkSettings"])["Ports"] = map[string]any{"8080/tcp": []any{map[string]any{"HostIp": "0.0.0.0", "HostPort": "8080"}}}
	}
}

func anyList(value any) []any { list, _ := value.([]any); return list }

func sameJSON(a, b any) bool { return string(approvalJSON(a)) == string(approvalJSON(b)) }

// argv is the literal command the harness started the run with, by child PID.
func (a *localOfflineAPI) argv(pid int) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.expected[pid]...)
}
