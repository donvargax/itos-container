package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"time"
)

func (w *world) egressLifeCreateFixture(resource string) error {
	kind, ok := map[string]string{"internal network": "internal", "egress network": "egress", "proxy": "proxy"}[resource]
	if !ok {
		return errors.New("unknown egress create barrier")
	}
	if err := w.egressLifeStartupBase(); err != nil {
		return err
	}
	l := w.egress.lifeStartup
	l.resource, l.barrier = kind, newEgressLifeBarrier("create-"+kind)
	l.recorder.after = l.barrier.after
	w.egress.native.cleanup = append(w.egress.native.cleanup, func() error { l.barrier.open(); return nil })
	return nil
}

func (w *world) egressLifeCreateRun() error {
	l, n := w.egress.lifeStartup, w.egress.native
	l.run = n.newRun([]egressStep{n.canonicalControl()}, 0)
	if err := l.run.start(); err != nil {
		return err
	}
	select {
	case <-l.barrier.ready:
		l.reached = true
	case <-l.run.done:
	case <-time.After(150 * time.Second):
	}
	if l.reached {
		// Kill immediately at the accepted reply. Independent inspection comes
		// after death so it cannot consume the shortened call budget and let
		// the controller time out and tear the resource down before the kill.
		l.run.err = errors.Join(l.run.err, l.run.cmd.Process.Kill())
	}
	l.run.finish(10 * time.Second)
	l.barrier.open()
	if l.reached {
		l.run.observe("create")
		l.original = l.run.observed["create"]
	}
	l.journal = l.recorder.snapshot()
	l.id = egressLifeJournalSession(l.journal)
	l.path = w.egressStatePath("sessions", w.recoveryIndexKey(), l.id, "egress-refusals.jsonl")
	if l.id == "" {
		return nil
	}
	if err := w.egressInvoke([]string{"stop", l.id, "--json"}, []string{"status", "--json"}); err != nil {
		return err
	}
	l.journal = l.recorder.snapshot()
	n.controlAll()
	return nil
}

func (w *world) egressLifeCreateAuthority() error {
	l := w.egress.lifeStartup
	if !l.reached {
		return fmt.Errorf("product missed accepted create barrier\n%s", l.run.describe())
	}
	if l.run.err != nil || l.run.timedOut || l.run.cmd.ProcessState == nil {
		return fmt.Errorf("controller loss was not observed at the accepted create\n%s", l.run.describe())
	}
	status, ok := l.run.cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		return errors.New("controller exited before the harness's SIGKILL")
	}
	l.barrier.mu.Lock()
	event, err := l.barrier.request, l.barrier.err
	l.barrier.mu.Unlock()
	if err != nil || event.intentErr != nil {
		return errors.Join(err, event.intentErr)
	}
	key := map[string]string{"internal": "internal", "egress": "network", "proxy": "proxy"}[l.resource]
	intent := object(object(event.intent["egress"])[key])
	var request, response map[string]any
	if json.Unmarshal(event.body, &request) != nil || json.Unmarshal(event.response, &response) != nil {
		return errors.New("accepted create lacks original request/reply")
	}
	name := request["Name"]
	if l.resource == "proxy" {
		q, _ := url.ParseQuery(event.query)
		name = q.Get("name")
	}
	if intent["stage"] != "intent" || intent["id"] != "" || intent["name"] != name {
		return errors.New("durable original intent did not precede create")
	}
	id, _ := response["Id"].(string)
	if len(id) != 64 {
		return errors.New("accepted create has no full ID")
	}
	if l.original == nil || l.original.err != nil {
		return errors.New("accepted native resource was not independently inspected")
	}
	if l.resource == "proxy" {
		proxy := l.original.proxy
		if proxy == nil || proxy.ID != id || proxy.Name != "/"+fmt.Sprint(name) || proxy.Config == nil || proxy.Config.Labels["io.itos-container.session"] != l.id || proxy.Image != w.egress.native.proxy {
			return errors.New("accepted proxy differs from its independent full-ID observation")
		}
	} else {
		network := l.original.networks[l.resource+"-network"]
		if network.ID != id || network.Name != name || network.Labels["io.itos-container.session"] != l.id {
			return errors.New("accepted network differs from its independent full-ID observation")
		}
	}
	path := "/networks/" + id
	if l.resource == "proxy" {
		path = "/containers/" + id
	}
	for at, e := range l.journal {
		if e.method != "DELETE" || e.path != path {
			continue
		}
		observed := object(object(e.intent["egress"])[key])
		if e.intentErr != nil || observed["id"] != id || observed["stage"] != "owned" || observed["name"] != name {
			return errors.New("recovery removed before saving the matching full ID")
		}
		inspect := path
		if l.resource == "proxy" {
			inspect += "/json"
		}
		if !slices.ContainsFunc(l.journal[:at], func(prior egressLifeRequest) bool {
			if prior.method != "GET" || prior.path != inspect || prior.status != 200 {
				return false
			}
			var native struct {
				ID string `json:"Id"`
			}
			return json.Unmarshal(prior.response, &native) == nil && native.ID == id
		}) {
			return errors.New("recovery did not revalidate the original full ID before removing it")
		}
		return nil
	}
	return errors.New("recovery never removed the accepted original resource")
}

func (w *world) egressLifeCreateNoStart() error {
	l := w.egress.lifeStartup
	if l.original == nil || l.original.err != nil || l.original.worker != nil {
		return errors.New("create barrier lacks worker-absence observation")
	}
	counts := map[string]int{}
	anchors := 0
	for _, e := range l.journal {
		if e.method == "POST" && e.path == "/volumes/create" {
			anchors++
		}
		if e.method == "POST" && strings.HasSuffix(e.path, "/start") {
			return errors.New("controller-loss recovery started a container")
		}
		if e.method == "POST" && e.path == "/containers/create" && egressLifeCreateKind(&e) != "proxy" {
			return errors.New("controller-loss recovery created a worker")
		}
		if kind := egressLifeCreateKind(&e); kind != "" {
			counts[kind]++
		}
	}
	want := map[string]int{"internal": 1}
	if l.resource != "internal" {
		want["egress"] = 1
	}
	if l.resource == "proxy" {
		want["proxy"] = 1
	}
	if anchors != 1 || !reflect.DeepEqual(counts, want) {
		return errors.New("recovery replaced or widened the accepted create inventory")
	}
	if err := egressLifeStartupRoutes(l.journal); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(w.doctor.checkout, "slice-27-report.json")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("worker instructions ran after controller death during create")
	}
	return nil
}

func (w *world) egressLifeCreateStatus(state string) error {
	l := w.egress.lifeStartup
	if len(w.egress.results) != 2 {
		return errors.New("no original public identity was recovered")
	}
	var count any
	if state == "stored" {
		count = 0
	}
	for _, result := range w.egress.results {
		if err := egressLifeStoredStatus(result, l.id, l.path, state, count); err != nil {
			return err
		}
		e, _, err := egressLifeEntry(result, l.id)
		if err != nil || e["workload_exit"] != nil || e["resource"] != "absent" || e["anchor"] != "absent" {
			return errors.New("unstarted workload acquired an exit or recovery lost confirmed absence")
		}
	}
	return nil
}

func (w *world) egressLifeCreateCleanup() error {
	l, n := w.egress.lifeStartup, w.egress.native
	if err := egressLifeReverseCreated(l.journal); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	remaining, err := n.owned(ctx)
	if err != nil || len(remaining) != 0 {
		return errors.New("fresh stop left original owned resources")
	}
	return errors.Join(w.approvalRecordUnchanged(), n.endpointsIntact())
}
