package porttest

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// CredentialEntry describes raw kernel observations, including bind aliases.
// Filesystem identity is independent of the spelling used to open an entry.
type CredentialEntry struct {
	port.CredentialNode
	MountID int
	Path    string
}

// CredentialBatch and CredentialStat let fixtures supply incomplete raw reads.
type CredentialBatch struct {
	Names    []string
	Done, OK bool
}
type CredentialStat struct {
	Node    port.CredentialNode
	OK      bool
	Elapsed time.Duration
}

// CredentialFiles is an I/O-free filesystem/topology fake, not a validator.
// Parser is supplied by assembly so a contract can exercise the native parser.
// Retained history is generated in bounded batches if mistakenly enumerated.
type CredentialFiles struct {
	Nodes, FinalNodes              map[string]CredentialEntry
	MountsText                     []string `json:"Mounts"`
	ReadErrors                     map[string]bool
	MountErrors                    map[int]bool
	DescriptorErrors               map[string]bool
	MountIDUnavailable             map[string]bool
	MountIDsAfterTable             map[string]int
	DescriptorMountIDs             map[string][]int
	Activity                       string
	Retained                       int
	HandleLimit                    int
	Deadline                       bool
	Calls                          map[string]int
	Completed                      string
	Parser                         func([]byte) ([]port.CredentialMount, bool) `json:"-"`
	FDParser                       func([]byte) (int, bool)                    `json:"-"`
	FDInfo                         map[string]string
	Elapsed                        time.Duration
	FinalStats                     map[string]port.CredentialNode
	OpenErrors                     map[string]port.CredentialRead
	StatErrors                     map[string]bool
	CheckoutEntries, CheckoutDepth int
	Batches                        map[string][]CredentialBatch
	StatSequence                   map[string][]CredentialStat
	DescriptorPaths                map[string]string
	ClockTicks                     []time.Duration
	ClockReads                     int
	statReads                      map[string]int
	clock                          time.Time
	next                           port.CredentialHandle
	handles                        map[port.CredentialHandle]CredentialEntry
	cursors                        map[port.CredentialHandle]int
	mountReads                     int
}

func (f *CredentialFiles) record(operation string) {
	if f.Calls == nil {
		f.Calls = map[string]int{}
	}
	f.Calls[operation]++
}

func (f *CredentialFiles) Now() time.Time {
	if f.clock.IsZero() {
		f.clock = time.Unix(1000, 0)
	}
	if len(f.ClockTicks) > 0 {
		i := min(f.ClockReads, len(f.ClockTicks)-1)
		f.ClockReads++
		return f.clock.Add(f.ClockTicks[i])
	}
	if f.Deadline && f.mountReads > 0 {
		return f.clock.Add(10 * time.Second)
	}
	if f.mountReads > 0 {
		return f.clock.Add(f.Elapsed)
	}
	return f.clock
}

func (f *CredentialFiles) Open(parent port.CredentialHandle, name string) (port.CredentialHandle, port.CredentialNode, port.CredentialRead) {
	if f.HandleLimit > 0 && len(f.handles) >= f.HandleLimit {
		return 0, port.CredentialNode{}, port.CredentialReadUnavailable
	}
	path := name
	if parent != 0 {
		p, ok := f.handles[parent]
		if !ok {
			return 0, port.CredentialNode{}, port.CredentialReadUnavailable
		}
		path = strings.TrimSuffix(p.Path, "/") + "/" + name
	}
	if strings.HasPrefix(path, "/state/") {
		f.record("state metadata")
	}
	if path != "/" && path != "/state" && path != "/checkout" && !strings.HasPrefix(path, "/state/") && !strings.HasPrefix(path, "/checkout/") {
		f.record("source metadata")
	}
	n, ok := f.Nodes[path]
	if status := f.OpenErrors[path]; status != port.CredentialReadOK {
		return 0, port.CredentialNode{}, status
	}
	if !ok && strings.HasPrefix(path, "/checkout/entry-") {
		i, err := strconv.Atoi(strings.TrimPrefix(path, "/checkout/entry-"))
		if err == nil && i >= 0 && i < f.CheckoutEntries {
			n = CredentialEntry{CredentialNode: port.CredentialNode{Device: 1, Inode: uint64(i) + 10000, Links: 1, UID: 1000, Mode: 0100600, Size: 32}, Path: path, MountID: 1}
			ok = true
		}
	}
	if !ok && strings.HasPrefix(path, "/checkout/d") && strings.Trim(strings.TrimPrefix(path, "/checkout"), "/d") == "" {
		depth := strings.Count(strings.TrimPrefix(path, "/checkout"), "/")
		if depth >= 0 && depth <= f.CheckoutDepth {
			n = CredentialEntry{CredentialNode: port.CredentialNode{Device: 1, Inode: uint64(depth) + 500, Links: 1, UID: 1000, Mode: 0040700}, Path: path, MountID: 1}
			ok = true
		}
	}
	if !ok && strings.HasPrefix(path, "/state/record-") {
		i, err := strconv.Atoi(strings.TrimPrefix(path, "/state/record-"))
		if err == nil && i >= 0 && i < f.Retained {
			n = CredentialEntry{CredentialNode: port.CredentialNode{Device: 1, Inode: uint64(i) + 100, Links: 1, UID: 1000, Mode: 0100600, Size: 32}, Path: path, MountID: 1}
			ok = true
		}
	}
	if !ok {
		return 0, port.CredentialNode{}, port.CredentialReadMissing
	}
	if f.handles == nil {
		f.handles = map[port.CredentialHandle]CredentialEntry{}
		f.cursors = map[port.CredentialHandle]int{}
	}
	f.next++
	f.handles[f.next] = n
	return f.next, n.CredentialNode, port.CredentialReadOK
}

func (f *CredentialFiles) Stat(h port.CredentialHandle) (port.CredentialNode, bool) {
	n, ok := f.handles[h]
	if sequence := f.StatSequence[n.Path]; len(sequence) > 0 {
		if f.statReads == nil {
			f.statReads = map[string]int{}
		}
		i := min(f.statReads[n.Path], len(sequence)-1)
		f.statReads[n.Path]++
		f.Elapsed = sequence[i].Elapsed
		return sequence[i].Node, ok && sequence[i].OK
	}
	if f.StatErrors[n.Path] {
		return port.CredentialNode{}, false
	}
	if final, exists := f.FinalStats[n.Path]; f.mountReads > 1 && exists {
		return final, ok
	}
	return n.CredentialNode, ok
}

func (f *CredentialFiles) Descriptor(h port.CredentialHandle) (port.CredentialDescriptor, bool) {
	n, ok := f.handles[h]
	key := "descriptor " + n.Path
	f.record(key)
	if id, exists := f.MountIDsAfterTable[n.Path]; f.mountReads > 0 && exists {
		n.MountID = id
	}
	if ids := f.DescriptorMountIDs[n.Path]; len(ids) > 0 {
		n.MountID = ids[min(f.Calls[key]-1, len(ids)-1)]
	}
	if f.MountIDUnavailable[n.Path] {
		return port.CredentialDescriptor{}, ok
	}
	if f.DescriptorErrors[n.Path] {
		return port.CredentialDescriptor{}, false
	}
	if path, exists := f.DescriptorPaths[n.Path]; exists {
		n.Path = path
	}
	if data, exists := f.FDInfo[n.Path]; exists {
		if f.FDParser == nil {
			return port.CredentialDescriptor{}, false
		}
		id, valid := f.FDParser([]byte(data))
		n.MountID = id
		ok = ok && valid
	}
	return port.CredentialDescriptor{Path: n.Path, MountID: n.MountID}, ok
}

func (f *CredentialFiles) Entries(h port.CredentialHandle) ([]string, bool, bool) {
	n, ok := f.handles[h]
	if !ok {
		return nil, false, false
	}
	if n.Path == "/state" || strings.HasPrefix(n.Path, "/state/") {
		f.record("state enumeration")
	} else {
		f.record("checkout traversal")
	}
	if f.ReadErrors[n.Path] {
		return nil, false, false
	}
	if batches := f.Batches[n.Path]; len(batches) > 0 {
		i := min(f.cursors[h], len(batches)-1)
		f.cursors[h]++
		b := batches[i]
		return b.Names, b.Done, b.OK
	}
	names := []string{}
	prefix := strings.TrimSuffix(n.Path, "/") + "/"
	for path := range f.Nodes {
		if tail, found := strings.CutPrefix(path, prefix); found && tail != "" && !strings.Contains(tail, "/") {
			names = append(names, tail)
		}
	}
	slices.Sort(names)
	if n.Path == "/checkout" && f.CheckoutEntries > 0 {
		start := f.cursors[h]
		end := min(start+128, f.CheckoutEntries)
		for i := start; i < end; i++ {
			names = append(names, "entry-"+strconv.Itoa(i))
		}
		f.cursors[h] = end
		return names, end == f.CheckoutEntries, true
	}
	if f.CheckoutDepth > 0 && (n.Path == "/checkout" || strings.HasPrefix(n.Path, "/checkout/d")) {
		depth := strings.Count(strings.TrimPrefix(n.Path, "/checkout"), "/")
		if depth < f.CheckoutDepth {
			names = append(names, "d")
		}
	}
	start := f.cursors[h]
	total := len(names)
	if n.Path == "/state" {
		total += f.Retained
	}
	end := min(start+128, total)
	batch := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		if i < len(names) {
			batch = append(batch, names[i])
		} else {
			batch = append(batch, "record-"+strconv.Itoa(i-len(names)))
		}
	}
	f.cursors[h] = end
	return batch, end == total, true
}

func (f *CredentialFiles) Mounts() ([]port.CredentialMount, bool) {
	data, ok := f.RawMounts()
	if !ok || f.Parser == nil {
		return nil, false
	}
	return f.Parser(data)
}

// RawMounts supports the same observation schedule beneath the disk parser.
func (f *CredentialFiles) RawMounts() ([]byte, bool) {
	f.record("state mount-table")
	i := f.mountReads
	f.mountReads++
	if i == 0 && f.Activity != "" {
		// Raw publication/removal occurs during the first table observation.
		if f.Nodes == nil {
			f.Nodes = map[string]CredentialEntry{}
		}
		temp := CredentialEntry{CredentialNode: port.CredentialNode{Device: 1, Inode: 9999, Links: 1, UID: 1000, Mode: 0100600, Size: 32}, Path: "/state/other-temp", MountID: 1}
		f.Nodes[temp.Path] = temp
		if f.Activity != "session record cleanup" {
			temp.Path = "/state/other-record"
			f.Nodes[temp.Path] = temp
		} else {
			delete(f.Nodes, "/state/other-record")
		}
		delete(f.Nodes, "/state/other-temp")
		f.Completed = f.Activity
	}
	if i == 1 {
		for path, node := range f.FinalNodes {
			f.Nodes[path] = node
		}
	}
	if f.MountErrors[i] || i >= len(f.MountsText) {
		return nil, false
	}
	return []byte(f.MountsText[i]), true
}

func (f *CredentialFiles) Close(h port.CredentialHandle) { delete(f.handles, h); delete(f.cursors, h) }
