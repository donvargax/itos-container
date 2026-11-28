package features

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

var errCredentialRunObserverUnsupported = errors.New("credential checkout observer requires Linux")

// Inotify wire event kinds, shared with the portable bracket judge.
const (
	credentialModify     = 0x002
	credentialAttrib     = 0x004
	credentialCloseWrite = 0x008
	credentialMoveFrom   = 0x040
	credentialMoveTo     = 0x080
	credentialCreate     = 0x100
	credentialDelete     = 0x200
	credentialDeleteSelf = 0x400
	credentialMoveSelf   = 0x800
	credentialOverflow   = 0x4000
)

type credentialRunEvent struct {
	path string
	kind uint32
}

type credentialRunObserver interface {
	events() ([]credentialRunEvent, error)
	close() error
}

type credentialRunBracketRejection struct{ error }

type credentialRunMutation struct {
	Op, Path string
	At       time.Time
}

type credentialRunBracket struct {
	begin, end, target, op string
	kinds                  []uint32
}

type credentialRunCheckout struct {
	credentialRunObserver
	checkout, control, staging string
	proofs                     map[string]bool
	brackets                   []credentialRunBracket
	mutations                  []credentialRunMutation
}

// The journal attributes the fixture mutation. The event stream detects other
// writers, including writers whose changes disappear before a final file scan.
func judgeCredentialRunEvents(events []credentialRunEvent, checkout, control string, proofs map[string]bool, brackets []credentialRunBracket, mutations []credentialRunMutation) error {
	seenProofs := make(map[string][]uint32)
	completed := make(map[string]bool)
	var active *credentialRunBracket
	var kinds []uint32
	for _, event := range events {
		if event.kind == credentialOverflow {
			return errors.New("checkout observer queue overflow")
		}
		if _, proofPath := proofs[event.path]; proofPath && (active == nil || event.path != active.target) {
			seenProofs[event.path] = append(seenProofs[event.path], event.kind)
			continue
		}
		if i := slices.IndexFunc(brackets, func(b credentialRunBracket) bool { return b.begin == event.path }); i >= 0 {
			if active != nil || completed[event.path] || event.kind != credentialCreate {
				return fmt.Errorf("unexpected bracket begin at %s", event.path)
			}
			active = &brackets[i]
			continue
		}
		if i := slices.IndexFunc(brackets, func(b credentialRunBracket) bool { return b.end == event.path }); i >= 0 {
			if active != &brackets[i] || event.kind != credentialCreate || !slices.Equal(kinds, active.kinds) {
				return fmt.Errorf("policy bracket events differ at %s", brackets[i].target)
			}
			completed[active.begin], active, kinds = true, nil, nil
			continue
		}
		if active != nil && event.path == active.target {
			kinds = append(kinds, event.kind)
			continue
		}
		// Prefer the most specific unexpected path when a directory and its
		// contents both changed. The directory event itself still fails.
		if credentialRunUnder(event.path, checkout) || credentialRunUnder(event.path, control) {
			var unexpected []string
			for _, other := range events {
				if other.path == event.path || credentialRunUnder(other.path, event.path) {
					unexpected = append(unexpected, other.path)
				}
			}
			return fmt.Errorf("unattributed checkout or control event at %v", slices.Compact(unexpected))
		}
		return fmt.Errorf("unknown checkout observer path %s", event.path)
	}
	for path, proof := range proofs {
		want := []uint32{credentialCreate}
		if proof {
			want = append(want, credentialModify, credentialCloseWrite, credentialDelete)
		}
		if !slices.Equal(seenProofs[path], want) {
			return fmt.Errorf("observer proof or calibration setup differed at %s", path)
		}
	}
	if active != nil || len(completed) != len(brackets) {
		return errors.New("missing or unpaired checkout policy bracket")
	}
	var edits []credentialRunBracket
	for _, bracket := range brackets {
		if credentialRunUnder(bracket.target, checkout) {
			edits = append(edits, bracket)
		}
	}
	if len(mutations) != len(edits) {
		return errors.New("policy mutation journal length differs from policy edit count")
	}
	for i, edit := range edits {
		if mutations[i].Op != edit.op || mutations[i].Path != edit.target || mutations[i].At.IsZero() {
			return fmt.Errorf("unexpected journaled mutation for %s", edit.target)
		}
	}
	return nil
}

func credentialRunUnder(path, parent string) bool {
	return credentialRunUnderOrEqual(path, parent) && path != parent
}

func credentialRunUnderOrEqual(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && filepath.IsLocal(rel)
}

func armCredentialRunCheckout(checkout, control, staging string) (*credentialRunCheckout, error) {
	observer, err := newCredentialRunObserver(checkout, control)
	if err != nil {
		return nil, err
	}
	return &credentialRunCheckout{credentialRunObserver: observer, checkout: checkout, control: control, staging: staging, proofs: map[string]bool{}}, nil
}

func (c *credentialRunCheckout) prove() error {
	path := filepath.Join(c.checkout, "credrun-observer-proof-"+egressNonce())
	c.proofs[path] = true
	if err := os.WriteFile(path, []byte("observer control"), 0600); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	events, err := c.events()
	if err != nil {
		return err
	}
	var observed []uint32
	for _, event := range events {
		if event.path == path {
			observed = append(observed, event.kind)
		}
	}
	if !slices.Equal(observed, []uint32{credentialCreate, credentialModify, credentialCloseWrite, credentialDelete}) {
		return fmt.Errorf("checkout observer missed the transient write control at %s", path)
	}
	return nil
}

func (c *credentialRunCheckout) judge() error {
	events, err := c.events()
	if err != nil {
		return err
	}
	return judgeCredentialRunEvents(events, c.checkout, c.control, c.proofs, c.brackets, c.mutations)
}

func credentialRunSentinel(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDONLY, 0600)
	if err != nil {
		return err
	}
	return file.Close()
}

func (c *credentialRunCheckout) bracket(seq int, target, op string, kinds []uint32) credentialRunBracket {
	id := fmt.Sprintf("%d-%s", seq, egressNonce())
	return credentialRunBracket{begin: filepath.Join(c.control, "begin-"+id), end: filepath.Join(c.control, "end-"+id), target: target, op: op, kinds: kinds}
}

func (c *credentialRunCheckout) calibrate(seq int, op string) ([]uint32, error) {
	target := filepath.Join(c.control, fmt.Sprintf("calibrate-target-%d", seq))
	if err := credentialRunSentinel(target); err != nil {
		return nil, err
	}
	c.proofs[target] = false
	stage := filepath.Join(c.staging, fmt.Sprintf("calibrate-stage-%d", seq))
	if op == "rename" {
		if err := os.WriteFile(stage, []byte("calibration"), 0600); err != nil {
			return nil, err
		}
	}
	bracket := c.bracket(seq, target, op, nil)
	if err := credentialRunSentinel(bracket.begin); err != nil {
		return nil, err
	}
	if err := credentialRunPolicyMutation(op, stage, target); err != nil {
		return nil, err
	}
	if err := credentialRunSentinel(bracket.end); err != nil {
		return nil, err
	}
	events, err := c.events()
	if err != nil {
		return nil, err
	}
	inBracket := false
	for _, event := range events {
		if event.path == bracket.begin {
			inBracket = true
		}
		if event.path == bracket.end {
			break
		}
		if inBracket && event.path == target {
			bracket.kinds = append(bracket.kinds, event.kind)
		}
	}
	want := credentialMoveTo
	if op == "unlink" {
		want = credentialDelete
	}
	if !slices.Equal(bracket.kinds, []uint32{uint32(want)}) {
		return nil, errors.New("policy-edit calibration did not see exactly the expected event")
	}
	c.brackets = append(c.brackets, bracket)
	return bracket.kinds, nil
}

func credentialRunPolicyMutation(op, stage, target string) error {
	if op == "rename" {
		return os.Rename(stage, target)
	}
	return os.Remove(target)
}

// between is used by the harness regression to model another writer.
func (c *credentialRunCheckout) editPolicy(op string, data []byte, between func(after bool) error) error {
	target := filepath.Join(c.checkout, "itos-container.yaml")
	seq := len(c.mutations) + 1
	kinds, err := c.calibrate(seq, op)
	if err != nil {
		return err
	}
	stage := filepath.Join(c.staging, "policy-"+egressNonce())
	var staged os.FileInfo
	var stagedDigest [sha256.Size]byte
	if op == "rename" {
		if err = os.WriteFile(stage, data, 0600); err != nil {
			return err
		}
		staged, err = os.Stat(stage)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(stage)
		if err != nil {
			return err
		}
		stagedDigest = sha256.Sum256(contents)
		sameDevice, err := credentialRunSameDevice(stage, c.checkout)
		if err != nil || !sameDevice {
			return errors.New("policy staging and checkout must share a device")
		}
	}
	bracket := c.bracket(seq, target, op, kinds)
	c.brackets = append(c.brackets, bracket)
	if err := credentialRunSentinel(bracket.begin); err != nil {
		return err
	}
	if between != nil {
		if err := between(false); err != nil {
			return err
		}
	}
	c.mutations = append(c.mutations, credentialRunMutation{Op: op, Path: target, At: time.Now()})
	if err := credentialRunPolicyMutation(op, stage, target); err != nil {
		return err
	}
	if between != nil {
		if err := between(true); err != nil {
			return err
		}
	}
	if err := credentialRunSentinel(bracket.end); err != nil {
		return err
	}
	journal, err := json.Marshal(c.mutations)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.staging, "policy-edits.json"), journal, 0600); err != nil {
		return err
	}
	if err := c.judge(); err != nil {
		return credentialRunBracketRejection{err}
	}
	got, err := os.Lstat(target)
	if op == "unlink" && os.IsNotExist(err) {
		return nil
	}
	if err != nil || op == "unlink" || !os.SameFile(staged, got) {
		return credentialRunBracketRejection{errors.New("policy path identity differs after fixture edit")}
	}
	content, err := os.ReadFile(target)
	if err != nil || sha256.Sum256(content) != stagedDigest {
		return credentialRunBracketRejection{errors.New("policy path hash differs after fixture edit")}
	}
	return nil
}
