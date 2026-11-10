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
	state      credentialCoordinate
	stateMount int
}

func sameProjection(a, b credentialProjection) bool {
	return slices.Equal(a.source, b.source) && a.checkout == b.checkout && a.state == b.state && a.stateMount == b.stateMount
}

func (s *credentialScan) project(source, checkout, state credentialWalk, statePath string, mounts []port.CredentialMount, complete bool) credentialProjection {
	p := credentialProjection{}
	if !complete || s.expired() {
		return p
	}
	rows := make(map[int]port.CredentialMount, len(mounts))
	for _, m := range mounts {
		rows[m.ID] = m
	}
	stateMap, ok := s.mapping(state.pins[len(state.pins)-1], rows)
	if !ok {
		return p
	}
	p.state = stateMap.coordinate
	p.stateMount = stateMap.mount
	if state.missing != "" {
		p.state.path = strings.TrimSuffix(p.state.path, "/") + "/" + state.missing
		if len(p.state.path) > 8192 {
			return p
		}
	}
	reserved := []credentialCoordinate{p.state}
	for _, m := range mounts {
		if credentialDescendant(m.Point, statePath) {
			reserved = append(reserved, credentialCoordinate{m.Device, m.Root})
		}
	}
	p.checkout, ok = s.mapping(checkout.pins[len(checkout.pins)-1], rows)
	if !ok {
		return p
	}
	for _, pin := range source.pins {
		m, ok := s.mapping(pin, rows)
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

func (s *credentialScan) mapping(pin credentialPin, rows map[int]port.CredentialMount) (credentialMapping, bool) {
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
	w := s.walk(d.Path, false, false)
	if w.rule != "" {
		return m, false
	}
	last := w.pins[len(w.pins)-1]
	d2, ok := s.fs.Descriptor(last.handle)
	if !ok || !sameCredentialPin(pin, last) || d2 != d {
		return m, false
	}
	relative := strings.TrimPrefix(strings.TrimPrefix(d.Path, row.Point), "/")
	coordinate := row.Root
	if relative != "" {
		coordinate = strings.TrimSuffix(coordinate, "/") + "/" + relative
	}
	if len(coordinate) > 8192 {
		return m, false
	}
	return credentialMapping{credentialCoordinate{row.Device, coordinate}, d.Path, d.MountID}, true
}
