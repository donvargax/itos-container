package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

type publicMemoryUnavailable struct{ failure sp.Outcome }

func (e publicMemoryUnavailable) OpenRun(context.Context, sp.Binding) (sp.RunConnection, sp.Outcome) {
	return nil, e.failure
}

func publicFakeMaterial(t *testing.T) {
	for _, fault := range []string{"unsafe", "schema", "duplicate", "unknown", "missing", "exit", "trailing", "oversized", "overdeep", "id", "project", "reference", "intent", "snapshot", "corrupt snapshot"} {
		t.Run(fault, func(t *testing.T) {
			m := publicMemoryFixture(t)
			id, _ := m.seed("a")
			files := m.files.Sessions[m.c.Project+"/"+id]
			data := string(files.Data)
			want := 2
			switch fault {
			case "unsafe":
				files.Unsafe = true
				want = 1
			case "schema":
				data = strings.Replace(data, `"schema":1`, `"schema":2`, 1)
			case "duplicate":
				data = `{"schema":1,` + data[1:]
			case "unknown":
				data = `{"extra":true,` + data[1:]
			case "missing":
				data = strings.Replace(data, `,"last_problem":null`, "", 1)
			case "exit":
				data = strings.Replace(data, `"workload_exit":null`, `"workload_exit":"7"`, 1)
			case "trailing":
				data += " {}"
			case "oversized":
				data = strings.Repeat(" ", 65537)
			case "overdeep":
				data = strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17)
			case "id":
				data = strings.Replace(data, id, strings.Repeat("f", 64), 1)
			case "project":
				data = strings.Replace(data, m.c.Project, strings.Repeat("f", 64), 1)
			case "reference":
				data = strings.Replace(data, "intents/"+m.c.Project, "intents/"+strings.Repeat("f", 64), 1)
			case "intent":
				delete(m.storage.Directories["intents/"+m.c.Project].Files, strings.Repeat("a", 63)+"1.json")
			case "snapshot":
				delete(m.storage.Directories["snapshots/"+m.c.Project].Files, "1.json")
			case "corrupt snapshot":
				m.storage.Directories["snapshots/"+m.c.Project].Files["1.json"] = []byte("{invalid")
			}
			files.Data = []byte(data)
			before := bytes.Clone(files.Data)
			for _, command := range []string{"status", "stop"} {
				r := publicMemoryCommand(t, m.service(), command, id, want)
				rule := "recovery.invalid-state"
				if want == 1 {
					rule = "recovery.unsafe-state"
				}
				if len(r.Problems) != 1 || r.Problems[0].Rule != rule || len(r.Sessions) != 0 {
					t.Fatal(r)
				}
			}
			if !bytes.Equal(files.Data, before) || len(m.files.Events) != 0 || len(m.e.Events) != 0 {
				t.Fatal("invalid authority reached engine or rewrote records")
			}
		})
	}
}

func publicFakeOwnership(t *testing.T) {
	for _, defect := range []string{"no registration", "other project", "host UID", "checkout identity", "daemon", "full ID", "name", "token", "project", "fingerprint", "image", "name and ID"} {
		t.Run(defect, func(t *testing.T) {
			m := publicMemoryFixture(t)
			id, worker := m.seed("a")
			files := m.files.Sessions[m.c.Project+"/"+id]
			o := m.e.Resources[worker]
			switch defect {
			case "no registration":
				delete(m.files.Sessions, m.c.Project+"/"+id)
			case "other project":
				delete(m.files.Sessions, m.c.Project+"/"+id)
				m.files.Sessions[strings.Repeat("f", 64)+"/"+id] = files
			case "host UID":
				m.host.UID = "1001"
			case "checkout identity":
				m.checkout.Identity.GitInode = "99"
			case "daemon":
				m.e.DaemonID = "foreign"
			case "full ID":
				o.ID = strings.Repeat("f", 64)
			case "name":
				o.Name = "foreign"
			case "token":
				o.Token = "foreign"
			case "project":
				o.Project = strings.Repeat("f", 64)
			case "fingerprint":
				o.Fingerprint = "sha256:" + strings.Repeat("f", 64)
			case "image":
				o.ImageID = "sha256:" + strings.Repeat("f", 64)
			case "name and ID":
				i := m.b.Intent(strings.Repeat("a", 63) + "1")
				m.storage.Directories["intents/"+m.c.Project].Files[i.NameToken()+".json"] = m.registry.Store.Codec.Encode(i)
				m.e.Inspection, m.e.DisagreeName = "different full container ID", true
			}
			m.e.Resources[worker] = o
			before := bytes.Clone(files.Data)
			r := publicMemoryCommand(t, m.service(), "stop", id, 1)
			if len(r.Problems) != 1 || r.Problems[0].Rule != "recovery.not-owned" || !bytes.Equal(files.Data, before) || len(m.files.Events) != 0 {
				t.Fatal(r)
			}
			for _, e := range m.e.Engine.Events {
				if e.Method == "REMOVE" || e.Method == "CREATE" {
					t.Fatal("foreign mutation", e)
				}
			}
			for _, e := range m.e.Events {
				if e == "SIGTERM" || e == "SIGKILL" || e == "start" {
					t.Fatal("foreign signal", e)
				}
			}
		})
	}
	// No mutable policy, manifest or approval dependency exists in this assembly.
	// Valid original records still authorize cleanup in the same command route.
	m := publicMemoryFixture(t)
	id, _ := m.seed("a")
	r := publicMemoryCommand(t, m.service(), "status", "", 0)
	if len(r.Sessions) != 1 || r.Sessions[0].Fingerprint != m.c.Fingerprint {
		t.Fatal(r)
	}
	r = publicMemoryCommand(t, m.service(), "stop", id, 0)
	if len(r.Sessions) != 1 || r.Sessions[0].Cleanup != "complete" || r.Sessions[0].WorkloadExit != nil {
		t.Fatal(r)
	}
	for _, outcome := range []sp.Outcome{sp.Environment, sp.Internal, sp.Incomplete} {
		service := m.service()
		service.Engine = publicMemoryUnavailable{outcome}
		for _, command := range []string{"status", "stop"} {
			code := map[sp.Outcome]int{sp.Environment: 3, sp.Internal: 70, sp.Incomplete: 75}[outcome]
			r := publicMemoryCommand(t, service, command, id, code)
			if len(r.Sessions) != 1 || r.Sessions[0].Cleanup != "unknown" || r.Sessions[0].Resource != "unknown" || r.Sessions[0].WorkloadExit != nil {
				t.Fatal(r)
			}
		}
	}
}
