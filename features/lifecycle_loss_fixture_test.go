package features

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type lifecycleLoss struct {
	setuid, root *isolationControl
	rounds       []*lossRound
	before       map[string]string
	dead         time.Time
	stopBegin    int
	err          error
}

type lossRound struct {
	nonce               string
	released, collected time.Time
	reports             []isolationReport
	boundary            map[string]any
	anchor              map[string]any
	intent              sessionRecord
	pid                 int
	start               string
	snapshot            sessionRecord
	challenge           sessionRecord
	before, after       lossControls
	err                 error
}

type lossControls struct {
	network, dns     []isolationReport
	host             map[string]string
	setuid           string
	privileged, root bool
	err              error
}

func (w *world) lifecycleLossFixture() error {
	// This is the first call, before state allocation, compilation or I/O.
	if err := doctorRealGuard(); err != nil {
		return err
	}
	if err := w.isolationFixture("identity"); err != nil {
		return err
	}
	s := w.offline.isolation
	w.loss = &lifecycleLoss{setuid: s.control}
	w.recovery = &recoveryNative{mode: "loss"}
	w.offline.api.recovery = true
	if _, err := w.recoveryTraceBinary(); err != nil {
		return err
	}
	if err := w.isolationIdentityInputs(); err != nil {
		return err
	}
	var err error
	w.loss.root, err = s.controlCreate(w.doctor.checkout, []string{"/workspace/slice-14-probe", "child", s.config.Nonce}, "0:0", "none", nil, nil, true, func(h *container.HostConfig) { h.ReadonlyRootfs = false })
	if err != nil {
		return err
	}
	if err = s.prepareHost(); err != nil {
		return err
	}
	s.config.HostNonce = s.config.Nonce
	s.mode, s.config.Mode = "loss", "loss"
	if err = s.prepareNetwork(); err != nil {
		return err
	}
	// Inert initial input: publication paths and the first nonce are supplied
	// only after the real run has published its own registration and snapshot.
	if err = os.WriteFile(w.lossChallengePath(), []byte("{}"), 0600); err != nil {
		return err
	}
	w.loss.before, err = w.lossProtected()
	if err != nil {
		return err
	}
	s.resourcesBefore, err = w.doctor.real.snapshotResources()
	return err
}

func (s *isolationWorld) lossRoutes() error {
	if err := s.networkRoutes(); err != nil {
		return err
	}
	networkPlan := s.config
	networkPlan.Mode = "network"
	if err := s.dnsRoutes(); err != nil {
		return err
	}
	dnsPlan := s.config
	dnsPlan.Mode = "dns"
	s.config.Network, s.config.DNS = &networkPlan, &dnsPlan
	s.config.Routes = append(append([]isolationRoute{}, networkPlan.Routes...), dnsPlan.Routes...)
	return nil
}

func (w *world) lossChallengePath() string {
	return filepath.Join(w.doctor.checkout, "slice-15-challenge.json")
}

// Docker's real embedded resolver needs aliases for the newly generated nonce.
// Reconnect only the independently identity-checked CONTROL at its original IP.
// The subject, its resolver file and its network never change. Every route must
// work again after this operation before a challenge is released to the worker.
func (s *isolationWorld) lossAliases(nonce string) error {
	if _, err := s.controlInspect(s.control); err != nil {
		return err
	}
	var aliases []string
	for _, suffix := range []string{"", "b", "a"} {
		for _, phase := range []string{"first", "running"} {
			for _, route := range s.config.DNS.Routes {
				if route.Protocol == "docker" {
					aliases = append(aliases, nonce+suffix+"-"+phase+"-"+route.Name)
				}
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n, err := s.cli.NetworkInspect(ctx, s.networkID, client.NetworkInspectOptions{})
	if err != nil {
		return err
	}
	if n.Network.Name != s.networkName || !reflect.DeepEqual(n.Network.Labels, s.networkLabels) {
		return errors.New("control network identity changed")
	}
	if _, err = s.cli.NetworkDisconnect(ctx, s.networkID, client.NetworkDisconnectOptions{Container: s.control.id}); err != nil {
		return err
	}
	_, err = s.cli.NetworkConnect(ctx, s.networkID, client.NetworkConnectOptions{Container: s.control.id, EndpointConfig: &network.EndpointSettings{
		Aliases: aliases, IPAMConfig: &network.EndpointIPAMConfig{IPv4Address: netip.MustParseAddr("172.30.14.2"), IPv6Address: netip.MustParseAddr("fd14:14::2")},
	}})
	return err
}

func (w *world) lossControls(nonce string) lossControls {
	s, l := w.offline.isolation, w.loss
	c := lossControls{}
	for _, plan := range []*isolationConfig{s.config.Network, s.config.DNS} {
		for _, route := range plan.Routes {
			if plan.Mode == "network" && strings.HasPrefix(route.Name, "host-loopback") {
				c.err = errors.Join(c.err, isolationHostExchange(route, nonce+"-host-control-"+route.Name))
			}
		}
		var reports []isolationReport
		for _, phase := range []string{"first", "running"} {
			r, err := s.controlExec(s.control, "0:0", "/workspace/slice-14-probe", "control", "/workspace/config.json", nonce, phase, plan.Mode)
			c.err = errors.Join(c.err, err)
			reports = append(reports, r...)
		}
		if plan.Mode == "network" {
			c.network = reports
		} else {
			c.dns = reports
		}
	}
	var err error
	c.host, err = s.hostControls()
	c.err = errors.Join(c.err, err)
	// Reuse slice-14's controls without replacing the live route server handle.
	view := &isolationWorld{w: w, cli: s.cli, control: l.setuid, privilegedControl: s.privilegedControl}
	c.setuid, err = view.setuidControl()
	c.err = errors.Join(c.err, err)
	c.privileged, err = view.privilegeControl()
	c.err = errors.Join(c.err, err)
	view.control = l.root
	c.root, err = view.rootWriteControl()
	c.err = errors.Join(c.err, err)
	return c
}

func (w *world) lossProtected() (map[string]string, error) {
	all, err := w.recoveryProtected()
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"slice-15-challenge.json", "slice-15-report"} {
		delete(all, filepath.Join(w.doctor.checkout, name))
	}
	for _, round := range w.loss.rounds {
		delete(all, filepath.Join(w.doctor.checkout, "slice-14-edit-"+round.nonce))
	}
	for path, value := range all {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		all[path] = value + fmt.Sprint(":gid=", stat.FieldByName("Gid"))
	}
	return all, nil
}

func (w *world) lossConfig(nonce string) isolationConfig {
	c := w.offline.isolation.config
	c.Nonce, c.Files, c.Sockets = nonce, maps.Clone(c.Files), maps.Clone(c.Sockets)
	return c
}
