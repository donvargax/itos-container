package domain

import (
	"strings"
	"time"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Credentials observes sources afresh for each command. Handles stay pinned
// throughout both namespace observations and final metadata revalidation.
type Credentials struct {
	FS    port.CredentialFS
	Input port.CredentialInput
}

type credentialPin struct {
	handle     port.CredentialHandle
	node       port.CredentialNode
	descriptor port.CredentialDescriptor
	mapped     bool
}

type credentialWalk struct {
	pins    []credentialPin
	missing string
	rule    string
}

type credentialScan struct {
	fs        port.CredentialFS
	uid       int64
	deadline  time.Time
	remaining int
	opened    []port.CredentialHandle
}

func (c Credentials) Inspect(grants []policy.Credential) port.CredentialResult {
	result := port.CredentialResult{}
	if len(grants) == 0 {
		return result
	}
	boundary := func(rule string) port.CredentialResult {
		issue := port.CredentialIssue{Rule: rule, Index: -1}
		if strings.HasPrefix(rule, "checkout.") {
			issue.Subject = c.Input.Root
		}
		return port.CredentialResult{Issues: []port.CredentialIssue{issue}}
	}
	if c.Input.OS != "linux" || c.Input.Architecture != "amd64" {
		return boundary("host.unsupported")
	}
	if !absoluteCredentialPath(c.Input.Root) {
		return boundary("checkout.root")
	}
	if !absoluteCredentialPath(c.Input.State) {
		return boundary("state.location")
	}
	if !absoluteCredentialPath(c.Input.AccountState) {
		return boundary("state.unavailable")
	}
	statePaths := []string{c.Input.State}
	if c.Input.AccountState != c.Input.State {
		statePaths = append(statePaths, c.Input.AccountState)
	}
	for _, path := range statePaths {
		if credentialDescendant(path, c.Input.Root) || credentialDescendant(c.Input.Root, path) {
			return boundary("state.path")
		}
	}
	s := &credentialScan{fs: c.FS, uid: c.Input.UID, deadline: c.FS.Now().Add(port.MetadataBudget), remaining: port.MetadataEntryLimit}
	defer s.close()
	tables := [2][]port.CredentialMount{}
	complete := [2]bool{}
	checkout := s.walk(c.Input.Root, credentialCheckout)
	if checkout.rule != "" {
		return boundary(checkout.rule)
	}
	states := make([]credentialWalk, len(statePaths))
	for i, path := range statePaths {
		states[i] = s.walk(path, credentialState)
		if states[i].rule != "" {
			return boundary(states[i].rule)
		}
	}
	sources := make([]credentialWalk, len(grants))
	for i, grant := range grants {
		sources[i] = s.walk(grant.Source, credentialSource)
	}
	// Each walk has captured its descriptors' mount IDs before mountinfo.
	// Projection re-reads those held descriptors after the table, without retry.
	tables[0], complete[0] = s.fs.Mounts()
	firstBoundary := s.projectBoundaries(checkout, states, statePaths, tables[0], complete[0])
	if firstBoundary.overlap {
		return boundary("state.path")
	}
	observations := make([][2]credentialProjection, len(grants))
	for i, source := range sources {
		if source.rule == "" {
			observations[i][0] = s.project(source, firstBoundary, statePaths, tables[0])
		}
	}
	identities := map[[2]uint64]bool{}
	scanOK := firstBoundary.ok && s.inventory(checkout.pins[len(checkout.pins)-1], 0, identities)
	tables[1], complete[1] = s.fs.Mounts()
	finalStates := make([]credentialWalk, len(states))
	for i, state := range states {
		finalStates[i] = state
		if !s.expired() {
			finalStates[i] = s.walk(statePaths[i], credentialState)
		}
		if finalStates[i].rule != "" {
			return boundary(finalStates[i].rule)
		}
		if state.missing == "" && !sameCredentialPin(state.pins[len(state.pins)-1], finalStates[i].pins[len(finalStates[i].pins)-1]) {
			return boundary("state.path")
		}
		if state.missing == "" {
			// Existing boundaries retain their original descriptors and mount-ID
			// baseline across both tables, not just a newly opened equivalent pin.
			finalStates[i] = state
		}
	}
	lastBoundary := s.projectBoundaries(checkout, finalStates, statePaths, tables[1], complete[1])
	if lastBoundary.overlap {
		return boundary("state.path")
	}
	seen := map[[2]uint64]bool{}
	for i, source := range sources {
		rule := source.rule
		if rule == "" {
			observations[i][1] = s.project(source, lastBoundary, statePaths, tables[1])
			first, last := observations[i][0], observations[i][1]
			switch {
			case sourceMatches(source, checkout, states, identities) || first.ok && first.match || last.ok && last.match:
				rule = "credential.location"
			case first.failure == "credential.mount_id" || last.failure == "credential.mount_id":
				rule = "credential.mount_id"
			case first.failure == "credential.filesystem" || last.failure == "credential.filesystem":
				rule = "credential.filesystem"
			case !scanOK || !first.ok || !last.ok || !sameProjection(first, last) || s.expired():
				rule = "credential.unavailable"
			default:
				pin := source.pins[len(source.pins)-1]
				if rule = credentialFile(pin.node, s.uid); rule != "" {
					break
				}
				final, ok := s.fs.Stat(pin.handle)
				if !ok || final.Device != pin.node.Device || final.Inode != pin.node.Inode || s.expired() {
					rule = "credential.unavailable"
				} else {
					rule = credentialFile(final, s.uid)
					key := [2]uint64{final.Device, final.Inode}
					if rule == "" && seen[key] {
						rule = "credential.duplicate"
					}
					if rule == "" {
						seen[key] = true
					}
				}
			}
		}
		if rule != "" {
			result.Issues = append(result.Issues, port.CredentialIssue{Rule: rule, Index: i})
		}
	}
	return result
}

func (s *credentialScan) expired() bool { return !s.fs.Now().Before(s.deadline) }

func (s *credentialScan) open(parent port.CredentialHandle, name string) (credentialPin, port.CredentialRead) {
	if s.expired() {
		return credentialPin{}, port.CredentialReadUnavailable
	}
	h, node, status := s.fs.Open(parent, name)
	if status == port.CredentialReadOK {
		s.opened = append(s.opened, h)
	}
	return credentialPin{handle: h, node: node}, status
}

func (s *credentialScan) close() {
	for i := len(s.opened) - 1; i >= 0; i-- {
		s.fs.Close(s.opened[i])
	}
}

type credentialWalkKind uint8

const (
	credentialSource credentialWalkKind = iota
	credentialState
	credentialCheckout
)

// Missing state is reserved under its nearest pinned safe ancestor. Neither
// walking it nor revalidating its suffix creates a directory or reads a record.
func (s *credentialScan) walk(path string, kind credentialWalkKind) credentialWalk {
	state := kind == credentialState
	w := credentialWalk{}
	parts := append([]string{"/"}, strings.Split(strings.TrimPrefix(path, "/"), "/")...)
	if path == "/" {
		parts = parts[:1]
	}
	var parent port.CredentialHandle
	for i, name := range parts {
		pin, status := s.open(parent, name)
		if status != port.CredentialReadOK {
			if status == port.CredentialReadMissing && state && i > 0 {
				w.missing = strings.Join(parts[i:], "/")
				break
			}
			w.rule = "credential.unavailable"
			if status == port.CredentialReadMissing {
				w.rule = "credential.missing"
			}
			break
		}
		w.pins = append(w.pins, pin)
		if pin.node.Mode&0170000 == 0120000 {
			w.rule = "credential.symlink"
			break
		}
		if i < len(parts)-1 || kind != credentialSource {
			if pin.node.Mode&0170000 != 0040000 {
				w.rule = "credential.unavailable"
				if state {
					w.rule = "state.unsafe"
				}
				break
			}
		}
		if state && i == len(parts)-1 && (int64(pin.node.UID) != s.uid || pin.node.Mode&07777 != 0700) {
			w.rule = "state.unsafe"
			break
		}
		parent = pin.handle
	}
	if w.rule == "" && kind != credentialCheckout {
		ancestors := w.pins[:len(w.pins)-1]
		if state {
			ancestors = w.pins
		}
		for _, pin := range ancestors {
			if !credentialAncestor(pin.node, s.uid) {
				w.rule = "credential.ancestor"
				break
			}
		}
	}
	if state && w.rule != "" && !strings.HasPrefix(w.rule, "state.") {
		switch w.rule {
		case "credential.symlink":
			w.rule = "state.path"
		case "credential.ancestor":
			w.rule = "state.unsafe"
		default:
			w.rule = "state.unavailable"
		}
	}
	if kind == credentialCheckout && w.rule != "" {
		if w.rule == "credential.symlink" {
			w.rule = "checkout.path"
		} else {
			w.rule = "checkout.unavailable"
		}
	}
	if w.rule == "" {
		for i := range w.pins {
			if s.expired() {
				break
			}
			w.pins[i].descriptor, w.pins[i].mapped = s.fs.Descriptor(w.pins[i].handle)
		}
	}
	return w
}

func credentialAncestor(n port.CredentialNode, uid int64) bool {
	return n.Mode&0170000 == 0040000 && (n.UID == 0 || int64(n.UID) == uid) && (n.Mode&0022 == 0 || (n.UID == 0 && n.Mode&01000 != 0))
}

func credentialFile(n port.CredentialNode, uid int64) string {
	switch {
	case n.Mode&0170000 != 0100000:
		return "credential.regular"
	case int64(n.UID) != uid:
		return "credential.owner"
	case n.Mode&0077 != 0:
		return "credential.permissions"
	case n.Links != 1:
		return "credential.links"
	case n.Size < 1 || n.Size > 65536:
		return "credential.size"
	default:
		return ""
	}
}

func absoluteCredentialPath(path string) bool {
	if path == "/" {
		return true
	}
	if !strings.HasPrefix(path, "/") || len(path) > 4096 {
		return false
	}
	for _, part := range strings.Split(path[1:], "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func credentialDescendant(path, root string) bool {
	return path == root || strings.HasPrefix(path, strings.TrimSuffix(root, "/")+"/")
}

func sameCredentialPin(a, b credentialPin) bool {
	return a.node.Device == b.node.Device && a.node.Inode == b.node.Inode
}

func sourceMatches(source, checkout credentialWalk, states []credentialWalk, inventory map[[2]uint64]bool) bool {
	root := checkout.pins[len(checkout.pins)-1]
	for _, pin := range source.pins {
		if sameCredentialPin(pin, root) || inventory[[2]uint64{pin.node.Device, pin.node.Inode}] {
			return true
		}
		for _, state := range states {
			if state.missing == "" && sameCredentialPin(pin, state.pins[len(state.pins)-1]) {
				return true
			}
		}
	}
	return false
}

func (s *credentialScan) inventory(dir credentialPin, depth int, identities map[[2]uint64]bool) bool {
	if depth > port.MetadataDepthLimit || s.expired() {
		return false
	}
	ok := true
	for {
		if s.expired() {
			return false
		}
		names, done, readOK := s.fs.Entries(dir.handle)
		if !readOK || len(names) > port.MetadataBatchSize {
			return false
		}
		for _, name := range names {
			if s.remaining == 0 || s.expired() {
				return false
			}
			s.remaining--
			if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
				ok = false
				continue
			}
			child, status := s.open(dir.handle, name)
			if status != port.CredentialReadOK {
				ok = false
				continue
			}
			identities[[2]uint64{child.node.Device, child.node.Inode}] = true
			if child.node.Mode&0170000 == 0040000 && !s.inventory(child, depth+1, identities) {
				ok = false
			}
			// Source and boundary pins stay held; inventory pins need only outlive
			// their descendants, keeping descriptor use bounded by depth.
			s.fs.Close(child.handle)
		}
		if done {
			return ok && !s.expired()
		}
		if len(names) == 0 {
			return false
		}
	}
}
