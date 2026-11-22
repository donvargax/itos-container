package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// Each local row runs a command assembly with independent raw fixtures. Capture
// early refusal as data in When; the first Then owns the behavior assertion.
func (w *world) homeLocalSteps(sc *godog.ScenarioContext) {
	var id, given string
	var result doctorResult
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		id, given, result = "", "", doctorResult{}
		for _, tag := range s.Tags {
			if strings.HasPrefix(tag.Name, "@ID-HOME-") {
				id = strings.TrimPrefix(tag.Name, "@ID-")
			}
		}
		return ctx, nil
	})
	bind := func(phrases []string, action func(string) error) {
		for _, phrase := range phrases {
			pattern := regexp.QuoteMeta(phrase)
			pattern = regexp.MustCompile("<[^>]+>").ReplaceAllString(pattern, ".+")
			sc.Step("^("+pattern+")$", action)
		}
	}
	bind([]string{
		"home approval mismatch fixtures for adding and removing the home request",
		"a home launch fixture with approved home settings A and independently approved omitted-home settings B",
		"a home launch fixture with synthetic approved UID <uid> GID <gid> and no home record or volume and two sequential runs",
		"a home launch fixture with fault \"<fault>\" and an independent successful home control",
		"a retained home fixture with defect \"<defect>\" and an independent matching home control",
		"a home launch fixture whose worker inspection has \"<defect>\" and an independent correct home control",
		"an independently seeded home-bearing session with state \"<state>\" and changed current requests and a legacy session control",
		"an OpenCode launch fixture with conflicting image OPENCODE and XDG defaults, host OPENCODE XDG and HOME sentinels, host-home OpenCode configuration and login sentinels and two sequential runs of \"opencode --standalone run --format json --model fake/fake-model say-ok\"",
		"an OpenCode launch fixture whose worker inspection has \"<defect>\" and an independent correct OpenCode control",
		"a home-bearing session with a durable allocation intent and an independently delayed successful volume create and a separate replacement-volume control",
		"a home lifecycle fixture delaying \"<operation>\" and an independent timely home control",
		"two home launch fixtures for the same checkout and daemon with different XDG state roots and an allocation barrier",
		"a home catalog fixture with current obsolete deleted and other-user homes across state roots and an independent empty catalog",
		"independent owned home management fixtures with response \"<response>\" and a terminal consenting control for each action",
		"an independently recorded home with management request condition \"<condition>\" and an independent valid consenting control",
		"an independently recorded unreferenced home that is \"<condition>\" and an independently approved matching run",
		"independent old and current home generations differing by \"<identity>\" with no claims and missing-volume and current-scope deletion controls",
		"a home management fixture with substitution \"<defect>\" and an independent valid owned control",
		"a home shared across state roots with reference \"<reference>\" and barriers at consent and durable retirement intent",
		"an unreferenced owned home with management fault \"<fault>\" and independent timely and replacement-volume controls",
	}, func(text string) error { given = text; return nil })
	bind([]string{
		"run attempts each changed request then each independently approved matching request and finally an independently approved re-enabled home",
		"run selects A and the fixture publishes B before create and then invokes a fresh run through both local backends",
		"fresh run controllers execute both runs through both local backends",
		"both home launches finish through both local backends and explicit recovery follows after the fault is cleared",
		"fresh run controllers attempt home reuse through both local backends",
		"the home boundary launches are attempted through both local backends",
		"fresh status and stop processes inspect and clean that session twice through both local backends",
		"the OpenCode boundary launches are attempted through the disk backend's recording engine endpoint",
		"stop observes name-only absence and later fresh stop observes the matching volume through both local backends",
		"the home operations exhaust their remaining aggregate budget through both local backends",
		"concurrent run controllers contend and a later run follows after the first worker is gone through both local backends",
		"fresh plain and JSON status --homes inspect both catalogs outside the recorded checkouts through both local backends",
		"reset and delete are requested in plain and JSON modes through both local backends",
		"home management is attempted through both local backends",
		"run is attempted before consent then terminal reset completes and fresh run and repeated reset of the old ID follow through both local backends",
		"the person selects each original daemon deletes only the chosen home with consent and retries through both local backends",
		"reset and delete receive terminal consent and the substitution is exercised after review and immediately before removal through both local backends",
		"reset and delete contend with launch then ordinary stop recovers the referenced session and fresh consented management is retried through both local backends",
		"consented reset and delete encounter the fault and fresh status and consented retries follow through both local backends",
	}, func(string) error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			result = doctorResult{exit: 1, stdout: err.Error()}
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestHomeLocalContract$", "-test.count=1", "-test.v"}
		if cover := w.coverageDir(); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		command := exec.CommandContext(ctx, bin, args...)
		command.Env = append(w.coverageEnv(os.Environ()), "SLICE_34_HOME_CASE="+id, "SLICE_34_HOME_GIVEN="+given)
		data, err := command.CombinedOutput()
		result = doctorResult{stdout: string(data)}
		if err != nil {
			result.exit = 1
		}
		return nil
	})
	bind([]string{
		"each mismatch reports changed approval with exit 1 before resource mutation and each matching control reaches its requested runtime boundary",
		"disabling home preserves its retained volume and re-enabling selects the original volume",
		"the first worker uses A's exact home volume and environment and the fresh worker uses B's legacy tmpfs home",
		"A's frozen snapshot and retained home remain unchanged with no post-selection request reads",
		"one volume is allocated after durable intent and owned before mounting and initialized before the first worker starts",
		"both workers use the same inspected volume and approved UID GID with HOME at /home/itos and TMPDIR at /tmp",
		"all four XDG home directories resolve inside the private home despite conflicting image defaults",
		"the second run never initializes or overwrites existing home contents and cleanup removes only session resources",
		"the faulted launch reports run.home-unavailable with exit 75 and no start while its home control succeeds",
		"owned session resources are cleaned or retained with exact recovery guidance and no home is deleted or allocated twice",
		"the defective home is refused before start with run.home-refused and exit 1 while the matching home control succeeds",
		"no foreign or missing home is adopted repaired replaced removed or read for contents",
		"the defective launch reports run.isolation-unverified with exit 1 and no start while the correct home control succeeds",
		"the owned unstarted worker is removed and the retained home and unrelated resources remain intact",
		"each worker's command is exactly the supplied argv and on the disk backend its created and inspected environment holds exactly the opencode-v1 values and the home mappings over the image defaults",
		"no worker environment, mount or archive copy carries a host sentinel or any OpenCode file",
		"the second run reuses the same home without writing into it and host-home sentinels are unchanged",
		"the defective launch, whose inspected environment differs from the approved opencode-v1 bytes, reports run.isolation-unverified with exit 1 and no start while the correct OpenCode control's created and inspected environments each hold exactly the opencode-v1 bytes and it succeeds",
		"status reports the original retained home and stop confirms session cleanup while preserving the original workload outcome",
		"the home bytes and ownership record remain intact with no home removal initialization or new worker",
		"repeated stop makes no destructive engine call and legacy sessions keep their existing output",
		"the first stop reports temporary uncertainty with its exact retry and the later stop durably accounts for the retained home and completes session cleanup",
		"neither stop allocates or starts anything or deletes the home and a same-named replacement is refused by a separate control",
		"the delayed home operation reports temporary failure within its original budget and the timely home control completes",
		"no late observation causes start mutation or completion publication after expiry and recovery retains the original home intent",
		"only one home is allocated and only one worker is admitted at a time and the later run reuses that home",
		"no state-root change resets ownership or initializes existing home contents",
		"the owned catalog reports the exact sorted home metadata and the empty catalog succeeds without creating state",
		"obsolete GID checkout and daemon bindings remain discoverable while other-user entries and all home contents stay private",
		"another daemon is unknown with recovery.environment and neither catalog is changed or used to discover engine resources by labels",
		"each unconsented request reports recovery.consent-required with exit 1 and changes no home record claim or engine resource while its consenting control completes",
		"each consenting review names the exact action home and original binding and warns of permanent loss before accepting consent",
		"JSON stdout contains only the home result and no prompt and no approval record or checkout data changes",
		"the request reports <rule> with exit <exit> without volume removal or successor publication while the control completes",
		"usage errors perform no state or engine access and failed publication or lock expiry preserves original ownership within the mutation budget",
		"the first run reports \"<before>\" without replacement and reset durably retires exactly that generation before any new allocation",
		"only the later approved run initializes one fresh successor with no old marker and repeated reset leaves the successor marker unchanged",
		"original records remain as tombstones and the reset command creates no volume worker or approval and changes no unrelated state",
		"exactly the selected owned storage is absent and its scope is durably disabled while the other home retains its marker and remains reusable",
		"repeated deletion and deletion of confirmed missing storage issue no volume removal and preserve the original ownership tombstone",
		"an approved run for a deleted scope is refused despite policy toggling until a separate consented reset permits one fresh home",
		"the defective request reports <rule> with exit <exit> before removal or retirement while the valid control completes",
		"no substituted resource or protected binding is adopted repaired deleted or read for contents",
		"the referenced home is refused with recovery.home-in-use before any home mutation and no management attempt stops a worker",
		"a launch claiming during consent prevents retirement while a durable pending action prevents a later mount or start",
		"only confirmed session cleanup releases the claim so fresh consent can retire the exact home without changing workload knowledge",
		"the first action reports recovery.temporary within its original budget or loses its controller without claiming completion and the timely control completes",
		"fresh status retains the original action ownership and exact home retry and pending retirement refuses new run claims",
		"after the fault clears retry confirms absence and durable completion without repeated successful removal or more than one successor",
		"the replacement-volume control is refused before deletion and no expiry permits a late mutation or completion publication",
	}, func(string) error {
		if result.exit != 0 || !strings.Contains(result.stdout, "--- PASS: TestHomeLocalContract ") {
			return fmt.Errorf("%s command contract failed at its behavior assertion: exit %d\n%s", id, result.exit, result.stdout)
		}
		return nil
	})
}
