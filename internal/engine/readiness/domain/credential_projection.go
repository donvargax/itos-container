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
	for _, m := range mounts {
		rows[m.ID] = m
	}
	var reserved []credentialCoordinate
	for i, state := range states {
		stateMap, ok := s.mapping(state.pins[len(state.pins)-1], rows, credentialSource)
		if !ok {
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
	var ok bool
	p.checkout, ok = s.mapping(checkout.pins[len(checkout.pins)-1], rows, credentialCheckout)
	if !ok {
		return p
	}
	for _, pin := range source.pins {
		m, ok := s.mapping(pin, rows, credentialSource)
		if !ok {
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

func (s *credentialScan) mapping(pin credentialPin, rows map[int]port.CredentialMount, kind credentialWalkKind) (credentialMapping, bool) {
	var m credentialMapping
	current, ok := s.fs.Stat(pin.handle)
	if !ok || current.Device != pin.node.Device || current.Inode != pin.node.Inode {
		return m, false
	}
	d, ok := s.fs.Descriptor(pin.handle)
	if !ok || !absoluteCredentialPath(d.Path) {
		return m, false
	}
	row, exists := rows[d.MountID]
	if !exists || row.Device != current.Device || !credentialDescendant(d.Path, row.Point) {
		return m, false
	}
	// A descriptor's path is evidence only after a fresh no-follow rewalk binds
	// that spelling to the held identity and the same mount, including file binds.
	w := s.walk(d.Path, kind)
	if w.rule != "" {
		return m, false
	}
	last := w.pins[len(w.pins)-1]
	d2, ok := s.fs.Descriptor(last.handle)
	if !ok || !sameCredentialPin(pin, last) || d2 != d {
		return m, false
	}
	return credentialMapping{credentialCoordinate{row.Device, credentialMountCoordinate(row, d.Path)}, d.Path, d.MountID}, true
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
