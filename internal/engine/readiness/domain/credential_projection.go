package domain

import (
	"slices"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

type credentialCoordinate struct {
	device uint64
	path   string
}
type credentialMapping struct {
	coordinate credentialCoordinate
	visible    string
	mount      int
}
type credentialProjection struct {
	ok, match  bool
	overlap    bool
	reserved   []credentialCoordinate
	failure    string
	source     []credentialMapping
	checkout   credentialMapping
	state      []credentialCoordinate
	stateMount []int
}

func sameProjection(a, b credentialProjection) bool {
	return slices.Equal(a.source, b.source) && a.checkout == b.checkout && slices.Equal(a.state, b.state) && slices.Equal(a.stateMount, b.stateMount)
}

// Project boundaries independently of sources, before checkout inventory. A
// checkout bind must not turn protected state into an enumerated directory.
func (s *credentialScan) projectBoundaries(checkout credentialWalk, states []credentialWalk, statePaths []string, mounts []port.CredentialMount, complete bool) credentialProjection {
	p := credentialProjection{}
	if credentialBoundaryIdentityOverlap(checkout, states) {
		p.overlap = true
		return p
	}
	if !complete || s.expired() {
		return p
	}
	rows := credentialMountRows(mounts)
	p.checkout, p.failure = s.mapping(checkout.pins[len(checkout.pins)-1], rows, credentialCheckout)
	if p.failure != "" {
		return p
	}
	for i, state := range states {
		stateMap, failure := s.mapping(state.pins[len(state.pins)-1], rows, credentialSource)
		if failure != "" {
			p.failure = failure
			return p
		}
		coordinate := stateMap.coordinate
		if state.missing != "" {
			coordinate.path = strings.TrimSuffix(coordinate.path, "/") + "/" + state.missing
		}
		p.state = append(p.state, coordinate)
		p.stateMount = append(p.stateMount, stateMap.mount)
		p.reserved = append(p.reserved, coordinate)
		for _, m := range mounts {
			if m.Unprojectable && (credentialDescendant(statePaths[i], m.Point) || credentialDescendant(m.Point, statePaths[i])) {
				p.failure = "credential.unavailable"
				return p
			}
			switch {
			case credentialDescendant(statePaths[i], m.Point):
				// Hidden backing subtrees remain reserved after an overmount.
				p.reserved = append(p.reserved, credentialCoordinate{m.Device, credentialMountCoordinate(m, statePaths[i])})
			case credentialDescendant(m.Point, statePaths[i]):
				p.reserved = append(p.reserved, credentialCoordinate{m.Device, m.Root})
			}
		}
	}
	checkoutTrees := []credentialCoordinate{p.checkout.coordinate}
	for _, m := range mounts {
		if !m.Unprojectable && credentialDescendant(m.Point, p.checkout.visible) {
			checkoutTrees = append(checkoutTrees, credentialCoordinate{m.Device, m.Root})
		}
	}
	for _, tree := range checkoutTrees {
		if s.expired() {
			return p
		}
		for _, state := range p.reserved {
			if tree.device == state.device && (credentialDescendant(tree.path, state.path) || credentialDescendant(state.path, tree.path)) {
				p.overlap = true
				return p
			}
		}
	}
	p.ok = !s.expired()
	return p
}

func credentialBoundaryIdentityOverlap(checkout credentialWalk, states []credentialWalk) bool {
	root := checkout.pins[len(checkout.pins)-1]
	for _, state := range states {
		for _, pin := range state.pins {
			if sameCredentialPin(root, pin) {
				return true
			}
		}
		if state.missing == "" {
			stateRoot := state.pins[len(state.pins)-1]
			for _, pin := range checkout.pins {
				if sameCredentialPin(stateRoot, pin) {
					return true
				}
			}
		}
	}
	return false
}

func credentialMountRows(mounts []port.CredentialMount) map[int]port.CredentialMount {
	rows := make(map[int]port.CredentialMount, len(mounts))
	for _, m := range mounts {
		rows[m.ID] = m
	}
	return rows
}

func (s *credentialScan) project(source credentialWalk, boundary credentialProjection, statePaths []string, mounts []port.CredentialMount) credentialProjection {
	p := boundary
	if !p.ok {
		return p
	}
	p.ok = false
	// Only validated descriptor paths participate in mountpoint comparisons.
	// An empty failed descriptor must not make every absolute mount relevant.
	protected := append([]string{p.checkout.visible}, statePaths...)
	for _, m := range mounts {
		if credentialStackingFilesystem(m) {
			for _, path := range protected {
				if credentialDescendant(path, m.Point) || credentialDescendant(m.Point, path) {
					p.failure = "credential.filesystem"
					return p
				}
			}
		}
	}
	rows := credentialMountRows(mounts)
	for _, pin := range source.pins {
		m, failure := s.mapping(pin, rows, credentialSource)
		if failure != "" {
			p.failure = failure
			return p
		}
		if credentialStackingFilesystem(rows[m.mount]) {
			p.failure = "credential.filesystem"
			return p
		}
		p.source = append(p.source, m)
		for _, tree := range p.reserved {
			if tree.device == m.coordinate.device && credentialDescendant(m.coordinate.path, tree.path) {
				p.match = true
			}
		}
	}
	p.ok = !s.expired()
	return p
}

func (s *credentialScan) mapping(pin credentialPin, rows map[int]port.CredentialMount, kind credentialWalkKind) (credentialMapping, string) {
	var m credentialMapping
	current, ok := s.fs.Stat(pin.handle)
	if !ok || current.Device != pin.node.Device || current.Inode != pin.node.Inode {
		return m, "credential.unavailable"
	}
	d, ok := s.fs.Descriptor(pin.handle)
	if pin.mapped && pin.descriptor.MountID == 0 || ok && d.MountID == 0 {
		return m, "credential.mount_id"
	}
	if !pin.mapped || !ok || d != pin.descriptor || !absoluteCredentialPath(d.Path) {
		return m, "credential.unavailable"
	}
	row, exists := rows[d.MountID]
	if !exists || row.Unprojectable || !credentialDescendant(d.Path, row.Point) {
		return m, "credential.unavailable"
	}
	// A descriptor's path is evidence only after a fresh no-follow rewalk binds
	// that spelling to the held identity and the same mount, including file binds.
	// Only the rewalk owns these temporary pins. Release them on every return,
	// retaining the original source and boundary pins across both observations.
	rewalk := *s
	rewalk.opened = nil
	defer rewalk.close()
	w := rewalk.walk(d.Path, kind)
	if w.rule != "" {
		return m, "credential.unavailable"
	}
	last := w.pins[len(w.pins)-1]
	if last.mapped && last.descriptor.MountID == 0 {
		return m, "credential.mount_id"
	}
	if !last.mapped || !sameCredentialPin(pin, last) || last.descriptor != d {
		return m, "credential.unavailable"
	}
	return credentialMapping{credentialCoordinate{row.Device, credentialMountCoordinate(row, d.Path)}, d.Path, d.MountID}, ""
}

func credentialStackingFilesystem(row port.CredentialMount) bool {
	return row.Filesystem == "overlay" || row.Filesystem == "fuse" || row.Filesystem == "fuseblk" ||
		strings.HasPrefix(row.Filesystem, "fuse.") || strings.HasPrefix(row.Filesystem, "fuseblk.")
}

// Validated root and visible paths are each at most 4096 bytes. The relative
// suffix is at most 4095, so their joined coordinate cannot exceed 8192 bytes.
func credentialMountCoordinate(row port.CredentialMount, path string) string {
	relative := strings.TrimPrefix(strings.TrimPrefix(path, row.Point), "/")
	if relative == "" {
		return row.Root
	}
	return strings.TrimSuffix(row.Root, "/") + "/" + relative
}
