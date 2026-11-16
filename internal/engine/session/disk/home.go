package disk

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type Homes struct {
	Storage   approval.CatalogStorage
	Codec     port.HomeCodec
	Host      approval.Host
	StateRoot string
	Locations approval.Locations
}

func (s Homes) Scope(b port.Binding) string { return s.Codec.Scope(b) }
func (s Homes) List(ctx context.Context) ([]string, port.Outcome) {
	uid, _ := s.Host.Identity()
	ids, failure := s.Storage.List(ctx, uid)
	if failure != "" {
		return nil, storageFailure(failure, false)
	}
	return ids, ""
}
func (s Homes) Open(ctx context.Context, scope string, create bool) (port.HomeTransaction, port.Outcome) {
	uid, _ := s.Host.Identity()
	d, failure := s.Storage.Open(ctx, uid, scope, create)
	if failure != "" {
		return nil, storageFailure(failure, create)
	}
	if d == nil {
		return nil, ""
	}
	t := &homeTransaction{directory: d, codec: s.Codec, uid: uid, scope: scope}
	if _, f := t.Read(); f != "" {
		d.Close()
		return nil, f
	}
	return t, ""
}

type homeTransaction struct {
	directory  approval.Directory
	codec      port.HomeCodec
	uid, scope string
	data       []byte
}

func (t *homeTransaction) Close() { t.directory.Close() }
func (t *homeTransaction) Read() (port.HomeScope, port.Outcome) {
	data, failure := t.directory.Read(t.scope + ".json")
	if failure != "" {
		return port.HomeScope{}, storageFailure(failure, false)
	}
	if t.data != nil && !bytes.Equal(data, t.data) {
		return port.HomeScope{}, port.Refused
	}
	if data == nil {
		return port.HomeScope{}, ""
	}
	s, f := t.codec.DecodeHome(data)
	if s.UID != "" && s.UID != t.uid {
		return port.HomeScope{}, port.Refused
	}
	if f != "" {
		return s, f
	}
	if s.Scope != t.scope || s.UID != t.uid {
		return port.HomeScope{}, port.Refused
	}
	t.data = data
	return s, ""
}
func (t *homeTransaction) Save(s port.HomeScope) port.Outcome {
	if s.Scope != t.scope || s.UID != t.uid {
		return port.Refused
	}
	data := t.codec.EncodeHome(s)
	if _, f := t.codec.DecodeHome(data); f != "" {
		return f
	}
	var f approval.Failure
	if bytes.Equal(data, t.data) {
		f = t.directory.Sync(t.scope+".json", data)
	} else {
		f = t.directory.Publish(t.scope+".json", t.data, data)
	}
	if f != "" {
		// A rename followed by a failed directory sync can make unconfirmed
		// completion visible. Keep the durable pending action authoritative.
		// Publish itself fences writes after the transaction's deadline.
		if t.data != nil && completingHome(t.codec, t.data, s) {
			if current, failure := t.directory.Read(t.scope + ".json"); failure == "" && bytes.Equal(current, data) {
				_ = t.directory.Publish(t.scope+".json", data, t.data)
			}
		}
		return storageFailure(f, true)
	}
	t.data = data
	return ""
}

func completingHome(codec port.HomeCodec, previous []byte, next port.HomeScope) bool {
	old, failure := codec.DecodeHome(previous)
	if failure != "" {
		return false
	}
	for i, g := range old.Generations {
		if i < len(next.Generations) && (g.Stage == "reset-pending" || g.Stage == "delete-pending") && next.Generations[i].Stage != g.Stage {
			return true
		}
	}
	return false
}
func (s Homes) Claim(r port.Record, session string) (port.HomeClaim, port.Outcome) {
	if !filepath.IsAbs(s.StateRoot) || filepath.Clean(s.StateRoot) != s.StateRoot || !port.HexID(session) {
		return port.HomeClaim{}, port.Invalid
	}
	return port.HomeClaim{Session: session, StateRoot: s.StateRoot, Intent: port.Reference(r.Project, r.NameToken())}, ""
}
func (s Homes) CheckClaims(ctx context.Context, scope port.HomeScope, g port.HomeGeneration) port.Outcome {
	for _, claim := range g.Claims {
		if f := s.checkClaim(ctx, scope, g, claim); f != "" {
			return f
		}
	}
	if len(g.Claims) != 0 {
		return port.HomeInUse
	}
	return ""
}

func (s Homes) claimBytes(ctx context.Context, c approval.Candidate, root, collection, name string) ([]byte, port.Outcome) {
	storage, ok := s.Locations.At(root).(approval.OriginalStorage)
	if !ok {
		return nil, port.Incomplete
	}
	data, f := storage.ReadOriginal(ctx, c, collection, name)
	if f != "" {
		return nil, storageFailure(f, false)
	}
	if data == nil {
		return nil, port.Incomplete
	}
	return data, ""
}

// Validate original protected authority without taking a session operation lock.
// An exited worker or a stopped public record never releases a durable claim.
func (s Homes) checkClaim(ctx context.Context, scope port.HomeScope, g port.HomeGeneration, claim port.HomeClaim) port.Outcome {
	project, token, valid := port.ParseReference(claim.Intent)
	if !valid || project != scope.Project || !port.HexID(claim.Session) || !filepath.IsAbs(claim.StateRoot) || filepath.Clean(claim.StateRoot) != claim.StateRoot {
		return port.Invalid
	}
	codec, ok := s.Codec.(port.Codec)
	public, pub := s.Codec.(port.PublicCodec)
	if s.Locations == nil || !ok || !pub {
		return port.Incomplete
	}
	c := approval.Candidate{UID: scope.UID, GID: scope.GID, Project: scope.Project}
	data, failure := s.claimBytes(ctx, c, claim.StateRoot, "intents", token+".json")
	if failure != "" {
		return failure
	}
	r, failure := codec.Decode(data)
	if failure != "" {
		return failure
	}
	data, failure = s.claimBytes(ctx, c, claim.StateRoot, "snapshots", r.Generation+".json")
	if failure != "" {
		return failure
	}
	b, failure := codec.Snapshot(data)
	if failure != "" {
		return failure
	}
	expected := b.Intent(token)
	expected.Schema, expected.Stage, expected.ContainerID = r.Schema, r.Stage, r.ContainerID
	expected.Anchor, expected.Egress, expected.Home = r.Anchor, r.Egress, r.Home
	if r != expected || !r.ValidHome(b) || !r.ValidAnchor(b) || !r.ValidEgress(b) || r.Project != scope.Project || r.UID != scope.UID || r.GID != scope.GID || r.Endpoint != scope.Engine.Endpoint || r.EngineID != scope.Engine.ID || s.Codec.Scope(b) != scope.Scope || r.Home.Scope != scope.Scope || r.Home.ID != g.ID || r.Home.Name != g.Name || r.Home.Created != "" && r.Home.Created != g.Created {
		return port.Invalid
	}
	if originalOverlap(claim.StateRoot, b.Checkout.Root) {
		return port.Unsafe
	}
	c.Root = b.Checkout.Root
	d, f := s.Locations.Inspect(ctx, c, claim.StateRoot)
	if f != "" {
		return storageFailure(f, false)
	}
	if d == nil {
		return port.Incomplete
	}
	defer d.Close()
	records, f := d.Sessions()
	if f != "" {
		return storageFailure(f, false)
	}
	for _, raw := range records {
		if raw.ID != claim.Session || raw.Index != registryKey(b.Checkout.Root, scope.UID) {
			continue
		}
		p, failure := public.DecodePublic(raw.Data)
		if failure != "" {
			return failure
		}
		if p.ID != claim.Session || p.Project != scope.Project || p.Intent != claim.Intent {
			return port.Invalid
		}
		if !d.Unchanged() {
			return port.Unsafe
		}
		return ""
	}
	return port.Incomplete
}

func originalOverlap(state, checkout string) bool {
	within := func(p, root string) bool {
		return p == root || strings.HasPrefix(p, strings.TrimRight(root, string(filepath.Separator))+string(filepath.Separator))
	}
	return !filepath.IsAbs(checkout) || within(state, checkout) || within(checkout, state)
}
