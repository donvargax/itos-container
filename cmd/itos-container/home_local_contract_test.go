package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/donvargax/itos-container/internal/cli"
	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

type homeLocalFixture struct {
	lastRule       string
	disk           *homeDiskFixture
	sdk            *homeSDKFixture
	m              publicMemory
	homes          *sf.Homes
	homeStore      sp.HomeStore
	engine         *sf.HomeEngine
	next           uint64
	consent        sp.HomeConsent
	ctx            context.Context
	approvalStatus string
	boundary       string
	beforeRun      func(ap.Durable)
}
type homeLocalEntropy struct{ f *homeLocalFixture }

func (e homeLocalEntropy) Token() (string, sp.Outcome) {
	e.f.next++
	return fmt.Sprintf("%064x", e.f.next), ""
}

// Independent wire codec for seeding and observing the test's protected catalog.
// The runtime codec must validate these bytes itself; this codec is no oracle
// for acceptance and does not call the production decoder.
type homeFixtureCodec struct{}

func (homeFixtureCodec) Scope(b sp.Binding) string {
	data, _ := json.Marshal(map[string]any{"schema": 1, "project": b.Frozen.Project, "uid": b.UID, "gid": b.GID, "engine": map[string]string{"endpoint": b.Endpoint, "id": b.EngineID}})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func (homeFixtureCodec) EncodeHome(s sp.HomeScope) []byte { data, _ := json.Marshal(s); return data }
func (homeFixtureCodec) DecodeHome(data []byte) (sp.HomeScope, sp.Outcome) {
	var s sp.HomeScope
	if json.Unmarshal(data, &s) != nil {
		return s, sp.Invalid
	}
	return s, ""
}

func newHomeLocalFixture(t *testing.T, uid, gid string) *homeLocalFixture {
	t.Helper()
	if os.Getenv("SLICE_34_HOME_BACKEND") == "disk" && runtime.GOOS != "linux" {
		t.Skip("protected home storage target is Linux")
	}
	m := publicMemoryFixture(t)
	var settings map[string]any
	if err := json.Unmarshal([]byte(m.c.Canonical), &settings); err != nil {
		t.Fatal(err)
	}
	settings["profile"] = "offline-home-v1"
	settings["home"] = map[string]any{"storage": "volume", "target": "/home/itos", "lifetime": "project", "mode": "0700"}
	settings["host"] = map[string]string{"uid": uid, "gid": gid}
	manifest := settings["manifest"].(map[string]any)
	manifest["containerUser"], manifest["remoteUser"] = uid, uid
	data, _ := json.Marshal(settings)
	m.c.Canonical = string(data)
	m.c.UID, m.c.GID = uid, gid
	m.c.Fingerprint = "sha256:" + fmt.Sprintf("%x", sha256.Sum256(data))
	identity, _ := json.Marshal(map[string]any{"schema": 1, "uid": uid, "checkout": settings["checkout"]})
	oldProject := m.c.Project
	m.c.Project = fmt.Sprintf("%x", sha256.Sum256(identity))
	if oldProject != m.c.Project {
		m.storage.Directories["snapshots/"+m.c.Project] = m.storage.Directories["snapshots/"+oldProject]
		m.storage.Directories["intents/"+m.c.Project] = m.storage.Directories["intents/"+oldProject]
		delete(m.storage.Directories, "snapshots/"+oldProject)
		delete(m.storage.Directories, "intents/"+oldProject)
	}
	m.host.UID, m.host.GID = uid, gid
	m.b.UID, m.b.GID, m.b.Home = uid, gid, true
	m.b.Frozen = ap.Frozen{Project: m.c.Project, Generation: "1", Fingerprint: m.c.Fingerprint, Settings: m.c.Canonical}
	m.storage.Directories["snapshots/"+m.c.Project].Files["1.json"] = canonical.JSON{}.Encode(ap.Record{Candidate: m.c, Generation: "1"})
	f := &homeLocalFixture{m: m, homes: &sf.Homes{Codec: homeFixtureCodec{}}, engine: &sf.HomeEngine{RunEngine: m.e}}
	f.engine.WaitFailure = ""
	f.engine.Homes = map[string]sp.HomeObservation{}
	f.engine.Roots = map[string]sf.HomeRoot{}
	f.engine.Workers = map[string]string{}
	if os.Getenv("SLICE_34_HOME_BACKEND") == "disk" {
		f.sdk = homeSDK(t, f)
		var settings map[string]any
		_ = json.Unmarshal([]byte(f.m.c.Canonical), &settings)
		settings["engine"].(map[string]any)["endpoint"] = f.sdk.endpoint
		f.m.b.Endpoint = f.sdk.endpoint
		f.engine.Endpoint = f.sdk.endpoint
		f.settings(t, settings, true)
		f.disk = homeDisk(t, f)
	}
	return f
}

func (f *homeLocalFixture) runner() domain.Runner {
	homes := f.service()
	return domain.Runner{Homes: &homes, Store: f.m.registry.Store, Registry: f.m.registry, Engine: f.runEngine(), Entropy: homeLocalEntropy{f}, Workspace: sf.MountWorkspace(f.m.b)}
}
func (f *homeLocalFixture) command(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	if f.disk != nil {
		f.disk.seed()
		f.sdk.seed()
	}
	var c commandLine
	var out, diagnostic bytes.Buffer
	parser, err := kong.New(&c, append(cli.Flags(), kong.Writers(&out, &diagnostic), kong.Exit(func(int) {}), kong.Vars{"version": "test"})...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := parser.Parse(runOptions(args))
	if err != nil {
		code := run(args, nil, &out, &diagnostic)
		return code, out.String(), diagnostic.String()
	}
	switch name, _, _ := strings.Cut(ctx.Command(), " "); name {
	case "run":
		c.Run.Context = f.ctx
		if c.Run.Context == nil {
			c.Run.Context = context.Background()
		}
		c.Run.Approval = &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: f.m.c, Record: ap.Record{Candidate: f.m.c, Generation: f.m.b.Frozen.Generation}, Status: f.status()}}
		c.Run.Snapshots = &af.Snapshots{}
		runner := f.runner()
		c.Run.Runner = offlineContractRunner{run: func(ctx context.Context, d ap.Durable, argv []string, streams sp.Streams) sp.RunResult {
			if f.beforeRun != nil {
				f.beforeRun(d)
			}
			return runner.Run(ctx, d, argv, streams)
		}}
		code := c.Run.Run(&cli.UI{Stdout: &out, Stderr: &diagnostic, Classify: runcommand.Classify})
		return code, out.String(), diagnostic.String()
	case "status":
		service := f.service()
		c.Status.Context = f.ctx
		c.Status.HomeService = service
		c.Status.HomePresentation = presentation.UI{Out: &out, Err: &diagnostic}
		c.Status.Service = domain.Recovery{Registry: f.m.registry, Engine: f.runEngine(), Homes: &service}
		c.Status.Presentation = presentation.UI{Out: &out, Err: &diagnostic}
		code := c.Status.Run()
		return code, out.String(), diagnostic.String()
	case "stop":
		service := f.service()
		c.Stop.Context = f.ctx
		c.Stop.Homes = service
		c.Stop.HomePresentation = presentation.UI{Out: &out, Err: &diagnostic}
		c.Stop.Consent = f.consent
		c.Stop.Service = domain.Recovery{Registry: f.m.registry, Engine: f.runEngine(), Homes: &service}
		c.Stop.Presentation = presentation.UI{Out: &out, Err: &diagnostic}
		code := c.Stop.Run()
		return code, out.String(), diagnostic.String()
	default:
		t.Fatal("unknown command", name)
	}
	return 70, "", ""
}

func homeReuseContract(t *testing.T, uid, gid string) {
	f := newHomeLocalFixture(t, uid, gid)
	if code, _, diagnostic := f.command(t, "run", "--", "probe"); code != 0 {
		t.Fatalf("approved home launch must reach its runtime boundary: exit %d; %s", code, diagnostic)
	}
	scope := f.scope()
	if len(scope.Generations) != 1 {
		t.Fatal("one protected home generation must survive cleanup", scope)
	}
	g := scope.Generations[0]
	if g.Stage != "retained" || !g.Initialized || g.Created == "" || len(g.Claims) != 0 || len(f.engine.Homes) != 1 || len(f.engine.Resources) != 0 {
		t.Fatal("home lifetime/ownership differs", g, f.engine.HomeEvents)
	}
	root := f.engine.Roots[g.Name]
	if root.UID != uid || root.GID != gid || root.Mode != 0700 || !root.Directory {
		t.Fatal("wrong initialized owner/mode", root)
	}
	root.Marker = "retained worker data"
	f.engine.Roots[g.Name] = root
	if code, _, d := f.command(t, "run", "--", "probe"); code != 0 {
		t.Fatal("fresh reuse", code, d)
	}
	if f.engine.Roots[g.Name].Marker != root.Marker || len(f.engine.Homes) != 1 {
		t.Fatal("reuse lost marker or allocated another home")
	}
	counts := map[string]int{}
	for _, event := range f.engine.HomeEvents {
		counts[event]++
	}
	if counts["allocate"] != 1 || counts["initialize"] != 1 || counts["remove"] != 0 {
		t.Fatal("reuse or cleanup altered home", counts)
	}
}

func (f *homeLocalFixture) service() domain.Homes {
	var store sp.HomeStore = f.homes
	if f.disk != nil {
		store = f.disk
	}
	if f.homeStore != nil {
		store = f.homeStore
	}
	return domain.Homes{Store: store, Engine: f.runEngine(), Entropy: homeLocalEntropy{f}}
}
func (f *homeLocalFixture) runEngine() sp.RunEngine {
	if f.sdk != nil {
		return f.sdk.engine()
	}
	return f.engine
}

type homeLocalConsent struct {
	yes    bool
	before func(sp.HomeEntry, string)
}

func (c homeLocalConsent) Review(_ context.Context, e sp.HomeEntry, a string) bool {
	if c.before != nil {
		c.before(e, a)
	}
	return c.yes
}
func (f *homeLocalFixture) seedHome(t *testing.T) sp.HomeGeneration {
	t.Helper()
	key := homeFixtureCodec{}.Scope(f.m.b)
	id := strings.Repeat("a", 64)
	g := sp.HomeGeneration{ID: id, Name: "itos-container-home-" + id, Created: "2026-11-16T00:00:00Z", Stage: "retained", Initialized: true, Claims: []sp.HomeClaim{}}
	f.homes.Scopes = map[string]sp.HomeScope{key: {Schema: 1, Scope: key, Project: f.m.c.Project, UID: f.m.b.UID, GID: f.m.b.GID, Engine: sp.HomeEngineIdentity{Endpoint: f.m.b.Endpoint, ID: f.m.b.EngineID}, Current: id, Generations: []sp.HomeGeneration{g}}}
	f.engine.Homes = map[string]sp.HomeObservation{g.Name: {Name: g.Name, Created: g.Created, Driver: "local", Scope: "local", Token: id, Key: key, ExactLabels: true, EmptyOptions: true}}
	f.engine.Roots = map[string]sf.HomeRoot{g.Name: {UID: f.m.b.UID, GID: f.m.b.GID, Mode: 0700, Directory: true, Marker: "original"}}
	if f.disk != nil {
		f.disk.seed()
	}
	return g
}
func (f *homeLocalFixture) retire(t *testing.T, id, action string, want int) sp.HomeEntry {
	t.Helper()
	code, out, errout := f.command(t, "stop", "--home", id, "--"+action, "--json")
	var result struct {
		Schema   int
		OK       bool
		Homes    []sp.HomeEntry
		Problems []presentation.Problem
	}
	if json.Unmarshal([]byte(out), &result) != nil || result.Schema != 1 || result.OK != (want == 0) || code != want || len(result.Homes) != 1 {
		t.Fatalf("retirement = %d %s %s; want %d", code, out, errout, want)
	}
	f.lastRule = ""
	if want == 0 && len(result.Problems) != 0 || want != 0 && len(result.Problems) == 0 {
		t.Fatal("wrong problem set", result.Problems)
	}
	if len(result.Problems) > 0 {
		f.lastRule = result.Problems[0].Rule
	}
	return result.Homes[0]
}
func TestHomeRetirementCommands(t *testing.T) {
	for _, action := range []string{"reset", "delete"} {
		t.Run(action, func(t *testing.T) {
			f := newHomeLocalFixture(t, "1000", "1000")
			g := f.seedHome(t)
			before := homeFixtureCodec{}.EncodeHome(f.homes.Scopes[homeFixtureCodec{}.Scope(f.m.b)])
			f.retire(t, g.ID, action, 1)
			if !bytes.Equal(before, homeFixtureCodec{}.EncodeHome(f.homes.Scopes[homeFixtureCodec{}.Scope(f.m.b)])) || len(f.engine.Homes) != 1 {
				t.Fatal("unconsented action changed storage")
			}
			f.consent = homeLocalConsent{yes: true, before: func(e sp.HomeEntry, a string) {
				if e.ID != g.ID || a != action || e.State != "present" || e.UID != "1000" || e.Project != f.m.c.Project {
					t.Fatal("wrong review", e, a)
				}
			}}
			e := f.retire(t, g.ID, action, 0)
			if e.State != "absent" || len(f.engine.Homes) != 0 || len(f.engine.Resources) != 0 {
				t.Fatal("retirement did not confirm exact absence", e)
			}
			removed := len(f.engine.HomeEvents)
			f.consent = homeLocalConsent{}
			repeated := f.retire(t, g.ID, action, 0)
			if (e.Successor == nil) != (repeated.Successor == nil) || e.Successor != nil && *e.Successor != *repeated.Successor {
				t.Fatal("retry redrew successor")
			}
			for _, event := range f.engine.HomeEvents[removed:] {
				if event == "remove" || event == "allocate" {
					t.Fatal("completed retry mutated storage")
				}
			}
			if action == "delete" {
				if code, _, d := f.command(t, "run", "--", "probe"); code != 1 || !strings.Contains(d, "run.home-refused") {
					t.Fatal("deleted scope relaunched", code, d)
				}
				f.consent = homeLocalConsent{yes: true}
				e = f.retire(t, g.ID, "reset", 0)
			}
			if code, _, d := f.command(t, "run", "--", "probe"); code != 0 {
				t.Fatal("successor launch", code, d)
			}
			if e.Successor == nil || f.engine.Roots["itos-container-home-"+*e.Successor].Marker != "" {
				t.Fatal("successor did not start empty", e)
			}
		})
	}
}
func TestHomeStatusRetainsInspectionFailure(t *testing.T) {
	f := newHomeLocalFixture(t, "1000", "1000")
	if code, _, d := f.command(t, "run", "--", "probe"); code != 0 {
		t.Fatal(code, d)
	}
	f.engine.HomeFault = "inspect"
	if code, out, d := f.command(t, "status", "--json"); code != 75 || !strings.Contains(out, `"state":"unknown"`) || !strings.Contains(d, "recovery.temporary") {
		t.Fatal("home failure hidden by absent worker", code, out, d)
	}
}
func TestHomeManagementChecksOwnershipAfterReview(t *testing.T) {
	for _, defect := range []string{"creation", "labels", "driver", "claim", "generation"} {
		t.Run(defect, func(t *testing.T) {
			f := newHomeLocalFixture(t, "1000", "1000")
			g := f.seedHome(t)
			f.consent = homeLocalConsent{yes: true, before: func(_ sp.HomeEntry, _ string) {
				o := f.engine.Homes[g.Name]
				s := f.homes.Scopes[homeFixtureCodec{}.Scope(f.m.b)]
				switch defect {
				case "creation":
					o.Created = "replacement"
				case "labels":
					o.Key = strings.Repeat("f", 64)
				case "driver":
					o.Driver = "nfs"
				case "claim":
					s.Generations[0].Claims = []sp.HomeClaim{{Session: strings.Repeat("c", 64), Intent: sp.Reference(s.Project, strings.Repeat("d", 64)), StateRoot: "/fixture/state/itos-container"}}
				case "generation":
					s.Generations[0].Created = "replacement"
				}
				f.engine.Homes[g.Name] = o
				f.homes.Scopes[s.Scope] = s
			}}
			f.retire(t, g.ID, "reset", 1)
			for _, event := range f.engine.HomeEvents {
				if event == "remove" {
					t.Fatal("substituted or claimed home removed")
				}
			}
		})
	}
}

func (f *homeLocalFixture) status() string {
	if f.approvalStatus != "" {
		return f.approvalStatus
	}
	return "matching"
}
func TestHomeLocalContract(t *testing.T) {
	id := os.Getenv("SLICE_34_HOME_CASE")
	given := os.Getenv("SLICE_34_HOME_GIVEN")
	cases := homeLocalCases
	if id != "" {
		cases = []homeLocalCase{{id, given}}
	}
	for _, backend := range []string{"memory", "disk"} {
		t.Run(backend, func(t *testing.T) {
			t.Setenv("SLICE_34_HOME_BACKEND", backend)
			for _, row := range cases {
				t.Run(row.id+"/"+row.given, func(t *testing.T) { runHomeLocalCase(t, row.id, row.given) })
			}
		})
	}
}
func runHomeLocalCase(t *testing.T, id, given string) {
	parameter := ""
	if _, rest, ok := strings.Cut(given, string(rune(34))); ok {
		parameter, _, _ = strings.Cut(rest, string(rune(34)))
	}

	switch id {
	case "HOME-03":
		homeApprovalChanges(t)
	case "HOME-04":
		homeFrozenSelection(t)
	case "HOME-05":
		uid, gid := "1000", "1000"
		if match := regexp.MustCompile("UID ([0-9]+) GID ([0-9]+)").FindStringSubmatch(given); len(match) > 2 {
			uid, gid = match[1], match[2]
		}
		homeReuseContract(t, uid, gid)
	case "HOME-06":
		homeStartupFault(t, parameter)
	case "HOME-07":
		homeReuseDefect(t, parameter)
	case "HOME-08":
		homeBoundaryDefect(t, parameter)
	case "HOME-09":
		homeSessionRecovery(t, parameter)
	case "HOME-10":
		homeLateAllocation(t)
	case "HOME-11":
		homeOperationBudget(t, parameter)
	case "HOME-12":
		homeSharedScope(t)
	case "HOME-16":
		homeCatalogDiscovery(t)
	case "HOME-17":
		homeConsentContract(t, parameter)
	case "HOME-18":
		homeResetContract(t, parameter)
	case "HOME-19":
		homeObsoleteDeletion(t, parameter)
	case "HOME-20":
		homeRetirementSubstitution(t, parameter)
	case "HOME-21":
		homeClaimFence(t, parameter)
	case "HOME-22":
		homeRetirementFault(t, parameter)
	case "HOME-28":
		openCodeLaunch(t)
	case "HOME-29":
		// The defect text may hold quotes of its own, so it is the whole span between
		// the row's fixed words, not up to its first closing quote.
		openCodeBoundary(t, strings.TrimSuffix(strings.TrimPrefix(given, "an OpenCode launch fixture whose worker inspection has \""), "\" and an independent correct OpenCode control"))
	case "HOME-24":
		homeInvalidAction(t, parameter)
	default:
		t.Fatal("unbound home contract", id)
	}
}

func (f *homeLocalFixture) settings(t *testing.T, v map[string]any, home bool) {
	t.Helper()
	data, _ := json.Marshal(v)
	f.m.c.Canonical = string(data)
	f.m.c.Fingerprint = "sha256:" + fmt.Sprintf("%x", sha256.Sum256(data))
	n, _ := strconv.Atoi(f.m.b.Frozen.Generation)
	generation := strconv.Itoa(n + 1)
	f.m.b.Home = home
	f.m.b.Frozen = ap.Frozen{Project: f.m.c.Project, Generation: generation, Fingerprint: f.m.c.Fingerprint, Settings: f.m.c.Canonical}
	f.m.storage.Directories["snapshots/"+f.m.c.Project].Files[generation+".json"] = canonical.JSON{}.Encode(ap.Record{Candidate: f.m.c, Generation: generation})
}
