package features

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// egressSubnets are the fixture networks' address ranges. The public range lies
// outside the proxy's refused table, as an endpoint a listed name may resolve
// to must; the private range lies inside it.
var egressSubnets = map[string][2]string{"public": {"11.22.33.0/24", "fd27:27::/64"}, "private": {"192.168.227.0/24", ""}}

type egressReceipt struct {
	Kind, Name, Nonce, Remote string
	Bytes                     int64
	Handshake                 bool
}

// egressEndpoint is one trusted TLS service in a fixture container. Its key
// never leaves that process: the harness holds only its public certificates.
type egressEndpoint struct {
	key, network, dir, id, nonce, leaf, redirect string
	names                                        []string
	address, address6                            netip.Addr
	controls                                     []error // independent harness exchanges, in order
	sent                                         []string
}

type egressFixtureNetwork struct {
	name, id string
	labels   map[string]string
}

type egressNative struct {
	w         *world
	cli       *client.Client
	nonce     string
	proxy     string // the pinned proxy image's config ID, as the harness inspected it
	names     []string
	networks  map[string]*egressFixtureNetwork
	endpoints map[string]*egressEndpoint
	order     []string
	authority []byte
	runs      []*egressNativeRun
	rows      []map[string]string
	original  egressOriginal
	consent   string
	cleanup   []func() error
	isolation *egressIsolation // the route or resolver control of ID-EGRESS-15 and -16
}

func egressNonce() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (w *world) egressNativeWorld() (*egressNative, error) {
	if w.egress == nil || w.egress.native == nil {
		return nil, errors.New("missing CI egress checkout")
	}
	return w.egress.native, nil
}

// egressNativeCheckout provisions only trusted controls: a private temporary
// checkout and state, a synthetic approval written from independent canonical
// data, and the worker probe. It asks the product for nothing.
func (w *world) egressNativeCheckout(spellings, canonical []string) error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	if err := w.doctorRealCheckout(false); err != nil {
		return err
	}
	d := w.doctor
	socket, err := filepath.EvalSymlinks("/var/run/docker.sock")
	if err != nil {
		return err
	}
	// The oracle reads the engine's identity from this record; nothing serves it.
	d.engine = newDoctorAPI(d.image)
	d.engine.info["ID"], d.engine.image["Id"] = d.engineID, d.imageID
	d.env["DOCKER_HOST"] = "unix://" + socket
	w.approval = &approvalWorld{uid: strconv.Itoa(os.Geteuid()), gid: strconv.Itoa(os.Getegid()), policyVersion: 2, allow: canonical}
	if err = os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), []byte(egressAllowlistPolicy(spellings, "")), 0600); err != nil {
		return err
	}
	if err = w.approvalSeed("1", false); err != nil {
		return err
	}
	if err = w.seedControlLocations(); err != nil {
		return err
	}
	cli, err := client.New(client.WithHost("unix:///var/run/docker.sock"))
	if err != nil {
		return err
	}
	n := &egressNative{w: w, cli: cli, nonce: egressNonce(), names: canonical, networks: map[string]*egressFixtureNetwork{}, endpoints: map[string]*egressEndpoint{}}
	w.egressFixture().native = n
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pinned, err := cli.ImageInspect(ctx, busyboxFixturePin)
	if err != nil {
		return fmt.Errorf("the pinned proxy image is not cached on the CI engine: %w", err)
	}
	n.proxy = pinned.ID
	if owned, err := n.owned(ctx); err != nil || len(owned) != 0 {
		return fmt.Errorf("the project already owns engine resources: %v %v", owned, err)
	}
	return w.installNativeProbe("slice-27-probe", func(output string) error {
		return egressBuild(w.root, filepath.Join("features", "testdata", "slice-27", "probe"), output)
	})
}

func egressBuild(root, source, output string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", output, "./"+filepath.ToSlash(source))
	cmd.Dir, cmd.Env = root, sessionChildEnvironment(os.Environ())
	for key, value := range map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64", "GOFLAGS": "", "GOWORK": "off"} {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compile slice-27 trusted program %s: %w\n%s", source, err, out)
	}
	return nil
}

func (n *egressNative) fixtureNetwork(ctx context.Context, key string) (*egressFixtureNetwork, error) {
	if existing := n.networks[key]; existing != nil {
		return existing, nil
	}
	ranges, ok := egressSubnets[key]
	if !ok {
		return nil, fmt.Errorf("unknown fixture network %q", key)
	}
	f := &egressFixtureNetwork{name: "slice-27-" + key + "-" + n.nonce, labels: map[string]string{"slice-27.fixture": n.nonce, "slice-27.network": key}}
	n.networks[key] = f // retain the intent even if the create reply is lost
	gateway := func(prefix netip.Prefix) netip.Addr { return prefix.Addr().Next() }
	four := netip.MustParsePrefix(ranges[0])
	ipam := &network.IPAM{Config: []network.IPAMConfig{{Subnet: four, Gateway: gateway(four)}}}
	six := ranges[1] != ""
	if six {
		prefix := netip.MustParsePrefix(ranges[1])
		ipam.Config = append(ipam.Config, network.IPAMConfig{Subnet: prefix, Gateway: gateway(prefix)})
	}
	created, err := n.cli.NetworkCreate(ctx, f.name, client.NetworkCreateOptions{Driver: "bridge", EnableIPv6: &six, IPAM: ipam, Labels: f.labels})
	if err != nil {
		return nil, fmt.Errorf("create the %s fixture network: %w", key, err)
	}
	f.id = created.ID
	return f, nil
}

// endpoint starts one trusted TLS service and waits until it has published its
// certificates. names are both its network aliases and its certificate names.
func (n *egressNative) endpoint(key, networkKey string, names []string, redirect string) (*egressEndpoint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f, err := n.fixtureNetwork(ctx, networkKey)
	if err != nil {
		return nil, err
	}
	w := n.w
	e := &egressEndpoint{key: key, network: networkKey, names: names, nonce: egressNonce(), redirect: redirect}
	host := byte(10 + len(n.order))
	four := netip.MustParsePrefix(egressSubnets[networkKey][0]).Addr().As4()
	four[3] = host
	e.address = netip.AddrFrom4(four)
	if six := egressSubnets[networkKey][1]; six != "" {
		raw := netip.MustParsePrefix(six).Addr().As16()
		raw[15] = host
		e.address6 = netip.AddrFrom16(raw)
	}
	if e.dir, err = os.MkdirTemp("", "slice-27-endpoint-"); err != nil {
		return nil, err
	}
	n.cleanup = append(n.cleanup, func() error { return os.RemoveAll(e.dir) })
	program, err := nativeProbeBytes(probeProduct(w.bin)+"-slice-27-endpoint", func(output string) error {
		return egressBuild(w.root, filepath.Join("features", "testdata", "slice-27", "endpoint"), output)
	})
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(e.dir, "slice-27-endpoint"), program, 0700); err != nil {
		return nil, err
	}
	config := approvalJSON(map[string]any{"Names": names, "Nonce": e.nonce, "Redirect": redirect})
	if err = os.WriteFile(filepath.Join(e.dir, "config.json"), config, 0600); err != nil {
		return nil, err
	}
	n.endpoints[key], n.order = e, append(n.order, key)
	settings := &network.EndpointSettings{Aliases: names, IPAMConfig: &network.EndpointIPAMConfig{IPv4Address: e.address}}
	if e.address6.IsValid() {
		settings.IPAMConfig.IPv6Address = e.address6
	}
	created, err := n.cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: "slice-27-endpoint-" + key + "-" + n.nonce,
		Config: &container.Config{Image: w.doctor.imageID, User: w.approval.uid + ":" + w.approval.gid, Entrypoint: []string{}, Cmd: []string{"/control/slice-27-endpoint", "serve", "/control"},
			Labels: map[string]string{"slice-27.fixture": n.nonce, "slice-27.endpoint": key}, Healthcheck: &container.HealthConfig{Test: []string{"NONE"}}},
		HostConfig: &container.HostConfig{NetworkMode: container.NetworkMode(f.name), ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"},
			// The service binds 443 as the unprivileged fixture user, inside its own namespace only.
			Sysctls:   map[string]string{"net.ipv4.ip_unprivileged_port_start": "0"},
			LogConfig: container.LogConfig{Type: "local"},
			Mounts:    []mount.Mount{{Type: mount.TypeBind, Source: e.dir, Target: "/control"}}},
		NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{f.name: settings}}})
	if err != nil {
		return nil, fmt.Errorf("create the %s endpoint: %w", key, err)
	}
	e.id = created.ID
	if _, err = n.cli.ContainerStart(ctx, e.id, client.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("start the %s endpoint: %w", key, err)
	}
	var ready struct{ Leaf string }
	for {
		data, err := os.ReadFile(filepath.Join(e.dir, "ready.json"))
		if err == nil && json.Unmarshal(data, &ready) == nil && ready.Leaf != "" {
			break
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("the %s endpoint never published its certificate", key)
		case <-time.After(50 * time.Millisecond):
		}
	}
	e.leaf = ready.Leaf
	authority, err := os.ReadFile(filepath.Join(e.dir, "ca.pem"))
	if err != nil {
		return nil, err
	}
	// Only public trust reaches the client fixture.
	n.authority = append(n.authority, authority...)
	return e, os.WriteFile(filepath.Join(w.doctor.checkout, "slice-27-ca.pem"), n.authority, 0600)
}

// control is one independent exchange with an endpoint, made by the harness
// from outside every product network, with a nonce no worker is given.
func (n *egressNative) control(e *egressEndpoint, name string) {
	nonce := "control-" + egressNonce()
	e.sent = append(e.sent, nonce)
	e.controls = append(e.controls, func() error {
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(n.authority)
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(e.address.String(), "443"), &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: name})
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		sum := sha256.Sum256(conn.ConnectionState().PeerCertificates[0].Raw)
		if hex.EncodeToString(sum[:]) != e.leaf {
			return errors.New("control met another certificate than the endpoint published")
		}
		if _, err = fmt.Fprintf(conn, "GET /nonce?n=%s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", nonce, name); err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(conn, 8192))
		if !strings.HasSuffix(string(data), "\r\n\r\n"+e.nonce) {
			return fmt.Errorf("control received no endpoint nonce (%v)", err)
		}
		return nil
	}())
}

func (e *egressEndpoint) receipts() ([]egressReceipt, error) {
	data, err := os.ReadFile(filepath.Join(e.dir, "receipts.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []egressReceipt
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var r egressReceipt
		if line == "" {
			continue
		}
		if err = json.Unmarshal([]byte(line), &r); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	return all, nil
}

// owned lists every engine resource carrying this project's label, by kind and ID.
func (n *egressNative) owned(ctx context.Context) ([]string, error) {
	label := "io.itos-container.project=" + n.w.approval.project
	filter := make(client.Filters).Add("label", label)
	var found []string
	containers, err := n.cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filter})
	if err != nil {
		return nil, err
	}
	for _, c := range containers.Items {
		found = append(found, "container "+c.ID)
	}
	networks, err := n.cli.NetworkList(ctx, client.NetworkListOptions{Filters: filter})
	if err != nil {
		return nil, err
	}
	for _, item := range networks.Items {
		found = append(found, "network "+item.ID)
	}
	volumes, err := n.cli.VolumeList(ctx, client.VolumeListOptions{Filters: filter})
	if err != nil {
		return nil, err
	}
	for _, v := range volumes.Items {
		found = append(found, "volume "+v.Name)
	}
	return found, nil
}

// closeEgressNative is the harness's safety net. It runs after every Then has
// read its evidence and never supplies any: a product resource it has to remove
// was already counted as left behind.
func (w *world) closeEgressNative() error {
	if w.egress == nil || w.egress.native == nil {
		return nil
	}
	n := w.egress.native
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var result error
	for _, run := range n.runs {
		run.stop()
	}
	if owned, err := n.owned(ctx); err == nil {
		for _, kind := range []string{"container ", "network ", "volume "} {
			for _, item := range owned {
				id, ok := strings.CutPrefix(item, kind)
				if !ok {
					continue
				}
				switch kind {
				case "container ":
					_, err = n.cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true})
				case "network ":
					_, err = n.cli.NetworkRemove(ctx, id, client.NetworkRemoveOptions{})
				default:
					_, err = n.cli.VolumeRemove(ctx, id, client.VolumeRemoveOptions{})
				}
				result = errors.Join(result, err)
			}
		}
	}
	for _, e := range n.endpoints {
		if e.id != "" {
			_, err := n.cli.ContainerRemove(ctx, e.id, client.ContainerRemoveOptions{Force: true})
			result = errors.Join(result, err)
		}
	}
	for _, f := range n.networks {
		target := f.id
		if target == "" {
			target = f.name
		}
		if _, err := n.cli.NetworkRemove(ctx, target, client.NetworkRemoveOptions{}); err != nil && f.id != "" {
			result = errors.Join(result, err)
		}
	}
	// After every product container is gone: the control network may have
	// carried the proxy.
	if n.isolation != nil {
		result = errors.Join(result, n.isolation.s.close())
	}
	for i := len(n.cleanup) - 1; i >= 0; i-- {
		result = errors.Join(result, n.cleanup[i]())
	}
	return errors.Join(result, n.cli.Close())
}
