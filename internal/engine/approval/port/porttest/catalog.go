package porttest

import (
	"context"
	"slices"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Catalog holds private scope directories by effective UID, without I/O.
type Catalog struct {
	Directories map[string]map[string]*Directory
	Failure     port.Failure
}

func (s *Catalog) List(ctx context.Context, uid string) ([]string, port.Failure) {
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	if s.Failure != "" {
		return nil, s.Failure
	}
	var keys []string
	for key := range s.Directories[uid] {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys, ""
}
func (s *Catalog) Open(ctx context.Context, uid, key string, create bool) (port.Directory, port.Failure) {
	if len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" {
		return nil, "state.invalid"
	}
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	if s.Failure != "" {
		return nil, s.Failure
	}
	d := s.Directories[uid][key]
	if d == nil && create {
		if s.Directories == nil {
			s.Directories = map[string]map[string]*Directory{}
		}
		if s.Directories[uid] == nil {
			s.Directories[uid] = map[string]*Directory{}
		}
		d = &Directory{Files: map[string][]byte{}}
		s.Directories[uid][key] = d
	}
	if d == nil {
		return nil, ""
	}
	if d.Locked {
		return nil, "state.unavailable"
	}
	d.Locked = true
	return d, ""
}

func CatalogContract(t Reporter, s port.CatalogStorage, uid string) {
	t.Helper()
	ctx := context.Background()
	key := strings.Repeat("a", 64)
	if keys, f := s.List(ctx, uid); f != "" || len(keys) != 0 {
		t.Errorf("empty catalog %v %s", keys, f)
	}
	if d, f := s.Open(ctx, uid, key, false); f != "" || d != nil {
		t.Errorf("missing catalog created %v %s", d, f)
		return
	}
	for _, bad := range []string{"", "../escape", strings.Repeat("A", 64), strings.Repeat("a", 63)} {
		if d, f := s.Open(ctx, uid, bad, true); f != "state.invalid" || d != nil {
			t.Errorf("invalid scope %q: %v %s", bad, d, f)
		}
	}
	d, f := s.Open(ctx, uid, key, true)
	if f != "" || d == nil {
		t.Errorf("create %s", f)
		return
	}
	name := key + ".json"
	if f = d.Publish(name, nil, []byte("original")); f != "" {
		t.Errorf("publication %s", f)
	}
	if f = d.Publish(name, nil, []byte("replacement")); f != "state.conflict" {
		t.Errorf("overwrote authority %s", f)
	}
	data, f := d.Read(name)
	if f != "" || string(data) != "original" {
		t.Errorf("read %s %s", data, f)
	}
	if len(data) > 0 {
		data[0] = 'X'
	}
	if f = d.Sync(name, []byte("original")); f != "" {
		t.Errorf("read aliases persisted authority %s", f)
	}
	d.Close()
	if keys, f := s.List(ctx, uid); f != "" || !slices.Equal(keys, []string{key}) {
		t.Errorf("discovery %v %s", keys, f)
	}
	d, f = s.Open(ctx, uid, key, false)
	if f != "" || d == nil {
		t.Errorf("reopen %s", f)
		return
	}
	defer d.Close()
	if f = d.Publish(name, []byte("original"), []byte("advanced")); f != "" {
		t.Errorf("advance %s", f)
	}
	if f = d.Sync(name, []byte("advanced")); f != "" {
		t.Errorf("sync %s", f)
	}
}

func (s *Storage) ReadOriginal(ctx context.Context, c port.Candidate, collection, name string) ([]byte, port.Failure) {
	d, f := s.Open(ctx, c, collection, false)
	if f != "" || d == nil {
		return nil, f
	}
	defer d.Close()
	return d.Read(name)
}
