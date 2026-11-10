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
	failure    string
	source     []credentialMapping
	checkout   credentialMapping
	state      []credentialCoordinate
	stateMount []int
}

func sameProjection(a, b credentialProjection) bool {
	return slices.Equal(a.source, b.source) && a.checkout == b.checkout && slices.Equal(a.state, b.state) && slices.Equal(a.stateMount, b.stateMount)
}

func (s *credentialScan) project(source, checkout credentialWalk, states []credentialWalk, statePaths []string, mounts []port.CredentialMount, complete bool) credentialProjection {
	p := credentialProjection{}
	if !complete || s.expired() {
		return p
	}
	rows := make(map[int]port.CredentialMount, len(mounts))
	protected := append([]string{checkout.pins[len(checkout.pins)-1].descriptor.Path}, statePaths...)
	for _, m := range mounts {
		rows[m.ID] = m
		if credentialStackingFilesystem(m) {
			for _, path := range protected {
				// Include hidden covering rows and descendant file mounts. Their
				// backing aliases need not share the visible inode's device.
				if credentialDescendant(path, m.Point) || credentialDescendant(m.Point, path) {
					p.failure = "credential.filesystem"
					return p
				}
			}
		}
	}
	var reserved []credentialCoordinate
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
		reserved = append(reserved, coordinate)
		for _, m := range mounts {
			if m.Unprojectable && (credentialDescendant(statePaths[i], m.Point) || credentialDescendant(m.Point, statePaths[i])) {
				p.failure = "credential.unavailable"
				return p
			}
			switch {
			case credentialDescendant(statePaths[i], m.Point):
				// Every covering row reserves its backing coordinate, including hidden
				// parents of an overmounted root or ancestor. An overmount cannot free it.
				reserved = append(reserved, credentialCoordinate{m.Device, credentialMountCoordinate(m, statePaths[i])})
			case credentialDescendant(m.Point, statePaths[i]):
				reserved = append(reserved, credentialCoordinate{m.Device, m.Root})
			}
		}
	}
	p.checkout, p.failure = s.mapping(checkout.pins[len(checkout.pins)-1], rows, credentialCheckout)
	if p.failure != "" {
		return p
	}
	for _, pin := range source.pins {
		m, failure := s.mapping(pin, rows, credentialSource)
		if failure != "" {
			p.failure = failure
			return p
		}
		p.source = append(p.source, m)
		for _, tree := range reserved {
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
	if credentialStackingFilesystem(row) {
		return m, "credential.filesystem"
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
	return row.Filesystem == "overlay" || strings.HasPrefix(row.Filesystem, "fuse.")
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
