package main

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	egressdomain "github.com/donvargax/itos-container/internal/engine/egress/domain"
	policydomain "github.com/donvargax/itos-container/internal/engine/policy/domain"
	policyidna "github.com/donvargax/itos-container/internal/engine/policy/idna"
	policyport "github.com/donvargax/itos-container/internal/engine/policy/port"
	policyporttest "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

// The proxy's allowlist check restates the shape of a name the policy engine
// produces, because one engine may not import another. Only this package may
// import both, so the two are held together here: whatever the real policy
// inspection emits, with its real IDNA adapter, the proxy's list must accept
// whole and admit name by name. Without this a change to either side would show
// only as a refused list when a run starts, in the real-engine scenarios.

// allowlistPolicy is a version 2 allowlist request as a policy source reads it.
func allowlistPolicy(spellings ...string) *policyporttest.Source {
	items := make([]policyport.Value, len(spellings))
	for i, spelling := range spellings {
		items[i] = policyport.Value{Kind: policyport.String, Text: spelling}
	}
	text := func(value string) policyport.Value { return policyport.Value{Kind: policyport.String, Text: value} }
	return &policyporttest.Source{Value: policyport.Value{Kind: policyport.Object, Fields: map[string]policyport.Value{
		"version":  {Kind: policyport.Number, Number: policyport.Decimal{Coefficient: "2", Exponent: "0"}},
		"manifest": text("a.json"),
		"network": {Kind: policyport.Object, Fields: map[string]policyport.Value{
			"mode": text("allowlist"), "allow": {Kind: policyport.Array, Items: items},
		}},
	}}}
}

// policyNames is the canonical list the real inspection gives for a request,
// or nil when it refuses the request.
func policyNames(spellings ...string) []string {
	result := policydomain.Inspect(allowlistPolicy(spellings...), policyidna.Names{})
	if result.Policy == nil {
		return nil
	}
	return result.Policy.Network.Allow
}

func connectHead(target string) []byte {
	return []byte("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n")
}

// admitted requires the proxy to accept the whole list and to pass a CONNECT
// to each of its names on 443, exactly as written.
func admitted(t *testing.T, names []string) {
	t.Helper()
	list, ok := egressdomain.NewAllowlist(names)
	if !ok {
		t.Errorf("the proxy refuses the policy's list %q", names)
		return
	}
	for _, name := range names {
		if head := list.Judge(connectHead(name + ":443")); !head.Pass {
			t.Errorf("the proxy refuses CONNECT %s:443 from its own list (reason %v)", name, head.Reason)
		}
	}
}

var (
	label63 = strings.Repeat("a", 63)
	name253 = strings.Join([]string{label63, strings.Repeat("b", 63), strings.Repeat("c", 63), strings.Repeat("d", 61)}, ".")
)

// respellings pairs each spelling the policy accepts with the one canonical
// name it stands for. The expected names are written out here, independent of
// both engines, so that skipping an input or mapping it twice cannot agree by
// accident. They are the accepted vectors of the policy's own tests and
// scenarios: case, trailing ASCII and ideographic dots, A-labels and U-labels,
// mapped and ignored code points, digits and hyphens, and the length limits.
var respellings = []struct{ spelling, canonical string }{
	{"api.example.com", "api.example.com"},
	{"API.Example.COM", "api.example.com"},
	{"api.example.com.", "api.example.com"},
	{"API.Example.COM.", "api.example.com"},
	{"api.example.com。", "api.example.com"},
	{"example。com", "example.com"},
	{"ＡＰＩ.example.com", "api.example.com"},
	{"exa\u00ADmple.com", "example.com"}, // a soft hyphen, which mapping removes
	{"bücher.example", "xn--bcher-kva.example"},
	{"BÜCHER.Example.", "xn--bcher-kva.example"},
	{"BÜCHER.example", "xn--bcher-kva.example"},
	{"xn--bcher-kva.example", "xn--bcher-kva.example"},
	{"XN--BCHER-KVA.example", "xn--bcher-kva.example"},
	{"日本。example。", "xn--wgv71a.example"},
	{"faß.example", "xn--fa-hia.example"},
	{"\u0430pple.example", "xn--pple-43d.example"}, // a Cyrillic first letter
	{"xn--pi-6kc.example.com", "xn--pi-6kc.example.com"},
	{"registry.example.org", "registry.example.org"},
	{"123.example", "123.example"},
	{"a-b.example", "a-b.example"},
	{"a.b", "a.b"},
	{"0x1f.example", "0x1f.example"},
	{"a.0xg", "a.0xg"},
	{"example.0xg1", "example.0xg1"},
	{"a.b0x", "a.b0x"},
	{"a.1x", "a.1x"},
	{"a.x1", "a.x1"},
	{label63 + ".example", label63 + ".example"},
	{strings.ToUpper(label63) + ".Example.", label63 + ".example"},
	{name253, name253},
	{name253 + ".", name253},
}

func TestEveryPolicyRespellingIsOneNameTheProxyAdmits(t *testing.T) {
	for _, tc := range respellings {
		names := policyNames(tc.spelling)
		if !slices.Equal(names, []string{tc.canonical}) {
			t.Errorf("policy reads %q as %q, not %q", tc.spelling, names, tc.canonical)
			continue
		}
		admitted(t, names)
		if tc.spelling == tc.canonical {
			continue
		}
		// The proxy never maps: a spelling that is not canonical is neither a
		// list entry nor a target it passes, even when its canonical form is listed.
		if _, ok := egressdomain.NewAllowlist([]string{tc.spelling}); ok {
			t.Errorf("the proxy lists the raw spelling %q", tc.spelling)
		}
		list, _ := egressdomain.NewAllowlist(names)
		if list.Judge(connectHead(tc.spelling + ":443")).Pass {
			t.Errorf("the proxy passes the raw spelling %q", tc.spelling)
		}
	}
}

func TestThePolicysWholeListIsTheProxysList(t *testing.T) {
	// One spelling of each canonical name, the last written: a name repeated
	// after canonicalising is the policy's own refusal, not a list.
	chosen := map[string]string{}
	for _, tc := range respellings {
		chosen[tc.canonical] = tc.spelling
	}
	var spellings, want []string
	for canonical, spelling := range chosen {
		spellings, want = append(spellings, spelling), append(want, canonical)
	}
	slices.Sort(spellings)
	slices.Sort(want)
	names := policyNames(spellings...)
	if !slices.Equal(names, want) {
		t.Fatalf("policy reads the whole request as %q, not %q", names, want)
	}
	admitted(t, names)
	list, _ := egressdomain.NewAllowlist(names)
	for _, target := range []string{"other.example:443", "example.com.:443", "com:443", names[0] + ":80", "sub." + names[0] + ":443"} {
		if list.Judge(connectHead(target)).Pass {
			t.Errorf("the proxy passes %s, which the list does not name on 443", target)
		}
	}
}

// corpusName draws one candidate spelling. Most are near a valid host name, so
// both verdicts are well represented: labels of ordinary and limit lengths over
// letters, digits and hyphens, with capitals, mapped and non-ASCII characters,
// address-like final labels, and ASCII or ideographic dots, doubled at times.
func corpusName(r *rand.Rand) string {
	alphabets := []string{
		"abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyz0123456789", "abcdefghijklmnopqrstuvwxyz0123456789-",
		"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef0123456789-", "0123456789", "0123456789abcdefx", "abcü日ßＡ-", "ab_ *",
	}
	lengths := []int{1, 1, 2, 3, 5, 8, 20, 62, 63, 64}
	labels := make([]string, 1+r.IntN(5))
	for i := range labels {
		alphabet := []rune(alphabets[r.IntN(len(alphabets))])
		label := make([]rune, lengths[r.IntN(len(lengths))])
		for j := range label {
			label[j] = alphabet[r.IntN(len(alphabet))]
		}
		labels[i] = string(label)
		if r.IntN(12) == 0 {
			labels[i] = "xn--" + labels[i]
		}
	}
	dots := []string{".", ".", ".", ".", "。", ".."}
	name := labels[0]
	for _, label := range labels[1:] {
		name += dots[r.IntN(len(dots))] + label
	}
	return name + []string{"", "", "", ".", "。", ".."}[r.IntN(6)]
}

// A fixed corpus, the same on every run: no fuzzing happens here. For every
// request the policy accepts, the proxy accepts the list and admits its name.
// The stored shape is compared in both directions as well, through the check
// the policy engine exports for stored names, so that a name only one side
// would take is seen whichever side changed.
func TestGeneratedPolicyNamesAreNamesTheProxyAdmits(t *testing.T) {
	r := rand.New(rand.NewPCG(27, 20261113))
	accepted, refused, canonical := 0, 0, map[string]bool{}
	for range 20000 {
		spelling := corpusName(r)
		names := policyNames(spelling)
		if names == nil {
			refused++
		} else {
			accepted++
			if len(names) != 1 || !policydomain.ValidCanonicalHost(names[0]) {
				t.Fatalf("policy reads %q as %q", spelling, names)
			}
			admitted(t, names)
			canonical[names[0]] = true
		}
		for _, stored := range []string{spelling, strings.ToLower(spelling), strings.TrimSuffix(strings.ToLower(spelling), ".")} {
			_, listed := egressdomain.NewAllowlist([]string{stored})
			if listed != policydomain.ValidCanonicalHost(stored) {
				t.Errorf("stored name %q: the proxy lists it %v, the policy's stored shape says %v", stored, listed, !listed)
			}
		}
	}
	// Neither verdict may be rare, or the corpus proves little.
	if accepted < 2000 || refused < 2000 || len(canonical) < 1500 {
		t.Fatalf("corpus too one-sided: %d accepted (%d distinct), %d refused", accepted, len(canonical), refused)
	}
	// The distinct names again as lists the size a request may have: sorted
	// and unique as the policy emits them, accepted whole.
	all := make([]string, 0, len(canonical))
	for name := range canonical {
		all = append(all, name)
	}
	slices.Sort(all)
	for chunk := range slices.Chunk(all, 64) {
		names := policyNames(chunk...)
		if !slices.Equal(names, chunk) {
			t.Fatalf("policy does not read its own canonical names back unchanged")
		}
		admitted(t, names)
	}
}
