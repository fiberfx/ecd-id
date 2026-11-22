package eid

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Lease limits.
const (
	LeaseTermMax = 86_400_000 // the longest lease window, 24 hours in milliseconds
	SecondSlots  = 1 << 17    // a writer number's second comes round after this many seconds (36.4 hours)
	IndexSlots   = 1 << 13    // grants per second per system
)

// Clock is a source of Unix milliseconds.
type Clock interface {
	NowMs() uint64
}

// SystemClock is the host's wall clock.
type SystemClock struct{}

// NowMs returns the current Unix time in milliseconds.
func (SystemClock) NowMs() uint64 { return uint64(time.Now().UnixMilli()) }

// ManualClock moves only when told to; for tests of restarts, clocks set back and bursts.
type ManualClock struct{ v atomic.Uint64 }

// NewManualClock returns a clock reading ms.
func NewManualClock(ms uint64) *ManualClock {
	c := &ManualClock{}
	c.v.Store(ms)
	return c
}

// NowMs returns the reading.
func (c *ManualClock) NowMs() uint64 { return c.v.Load() }

// Set sets the reading.
func (c *ManualClock) Set(ms uint64) { c.v.Store(ms) }

// Advance moves the reading forward by ms.
func (c *ManualClock) Advance(ms uint64) { c.v.Add(ms) }

// Lease is an exclusive right to one writer number and the id-times from NotBefore to NotAfter, inclusive.
type Lease struct {
	System    Code
	Writer    uint32
	NotBefore uint64
	NotAfter  uint64
}

// NewLease returns a lease, checked as the text form is checked.
func NewLease(system Code, writer uint32, notBefore, notAfter uint64) (Lease, error) {
	if system.Class() == Reserved {
		return Lease{}, ErrLeaseSystem
	}
	if writer > WriterMax {
		return Lease{}, ErrLeaseWriter
	}
	if notBefore == 0 || notAfter < notBefore || notAfter > TimeMax || notAfter-notBefore > LeaseTermMax {
		return Lease{}, ErrLeaseWindow
	}
	return Lease{system, writer, notBefore, notAfter}, nil
}

func leaseDecimal(p string) (uint64, error) {
	if p == "" || len(p) > 20 || (len(p) > 1 && p[0] == '0') {
		return 0, ErrLeaseSyntax
	}
	for i := 0; i < len(p); i++ {
		if p[i] < '0' || p[i] > '9' {
			return 0, ErrLeaseSyntax
		}
	}
	v, err := strconv.ParseUint(p, 10, 64)
	if err != nil {
		return 0, ErrLeaseSyntax
	}
	return v, nil
}

// ParseLease parses system:writer:not_before:not_after.
func ParseLease(s string) (Lease, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 4 {
		return Lease{}, ErrLeaseSyntax
	}
	system, ok := ParseCode(parts[0])
	if !ok {
		return Lease{}, ErrLeaseSyntax
	}
	var n [3]uint64
	for i := range n {
		v, err := leaseDecimal(parts[i+1])
		if err != nil {
			return Lease{}, err
		}
		n[i] = v
	}
	if system.Class() == Reserved {
		return Lease{}, ErrLeaseSystem
	}
	if n[0] > WriterMax {
		return Lease{}, ErrLeaseWriter
	}
	return NewLease(system, uint32(n[0]), n[1], n[2])
}

func (l Lease) String() string {
	return l.System.String() + ":" + strconv.FormatUint(uint64(l.Writer), 10) + ":" +
		strconv.FormatUint(l.NotBefore, 10) + ":" + strconv.FormatUint(l.NotAfter, 10)
}

// WriterFor returns (second mod 2^17) * 2^13 + index.
func WriterFor(second uint64, index uint32) uint32 {
	return uint32(second%SecondSlots)<<13 | index%IndexSlots
}

// LeaseStore is the allocator's durable state in a system's own store: one number per system, and the live
// activation.
type LeaseStore interface {
	// LastSecond returns the stored grant second of system, if any.
	LastSecond(system Code) (uint64, bool)
	// CompareAndSet sets the stored grant second to next if it equals (expected, exists); it reports whether it did.
	CompareAndSet(system Code, expected uint64, exists bool, next uint64) bool
	// Activate records a live activation of system; it returns false if one is already live.
	Activate(system Code) bool
	// Release ends the live activation of system; the supervisor calls it when the allocator's process ends.
	Release(system Code)
}

// MemoryStore is a lease store in memory. Snapshot copies it, which stands for a backup that is later restored or
// cloned.
type MemoryStore struct {
	mu     sync.Mutex
	last   map[Code]uint64
	active map[Code]bool
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{last: map[Code]uint64{}, active: map[Code]bool{}}
}

// Snapshot returns an independent copy of the store as it is now.
func (m *MemoryStore) Snapshot() *MemoryStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := NewMemoryStore()
	for k, v := range m.last {
		c.last[k] = v
	}
	for k, v := range m.active {
		c.active[k] = v
	}
	return c
}

// LastSecond implements LeaseStore.
func (m *MemoryStore) LastSecond(system Code) (uint64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.last[system]
	return v, ok
}

// CompareAndSet implements LeaseStore.
func (m *MemoryStore) CompareAndSet(system Code, expected uint64, exists bool, next uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.last[system]
	if ok != exists || (ok && v != expected) {
		return false
	}
	m.last[system] = next
	return true
}

// Activate implements LeaseStore.
func (m *MemoryStore) Activate(system Code) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[system] {
		return false
	}
	m.active[system] = true
	return true
}

// Release implements LeaseStore.
func (m *MemoryStore) Release(system Code) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.active, system)
}

// Allocator grants leases of one system from the clock second and one stored number (spec/lease.md#allocator).
type Allocator struct {
	system    Code
	store     LeaseStore
	clock     Clock
	offset    uint32
	hasRun    bool
	runSecond uint64
	granted   uint32

	skipStartWait bool // set only by tests, to show what the wait prevents
}

// StartAllocator starts the allocator of system under the supervisor home. offset is the random 13-bit index
// offset, drawn once per start (see RandomOffset).
func StartAllocator(reg *Registry, system Code, home string, store LeaseStore, clock Clock, offset uint32) (*Allocator, error) {
	if system.Class() != Registered {
		return nil, ErrClass
	}
	row, ok := reg.System(system)
	if !ok {
		return nil, ErrUnknown
	}
	if row.State != Live {
		return nil, ErrState
	}
	if row.Home == "-" || row.Home != home {
		return nil, ErrNotHome
	}
	if !store.Activate(system) {
		return nil, ErrActive
	}
	return &Allocator{system: system, store: store, clock: clock, offset: offset % IndexSlots}, nil
}

// System returns the system this allocator serves.
func (a *Allocator) System() Code { return a.system }

// Grant grants a lease of termMs milliseconds (at most 24 hours) from the current clock second.
func (a *Allocator) Grant(termMs uint64) (Lease, error) {
	if termMs == 0 || termMs > LeaseTermMax {
		return Lease{}, ErrTerm
	}
	s := a.clock.NowMs() / 1000
	if s == 0 {
		return Lease{}, ErrClockZero
	}
	last, has := a.store.LastSecond(a.system)
	if !a.hasRun {
		if has {
			if s < last {
				return Lease{}, ErrClockBehind
			}
			if s == last && !a.skipStartWait {
				return Lease{}, ErrWait
			}
		}
		if !(has && last == s) && !a.store.CompareAndSet(a.system, last, has, s) {
			return Lease{}, ErrContended
		}
		a.hasRun, a.runSecond, a.granted = true, s, 0
	} else {
		if !has || last != a.runSecond {
			return Lease{}, ErrContended
		}
		switch {
		case s < a.runSecond:
			return Lease{}, ErrClockBehind
		case s == a.runSecond:
			if a.granted+1 >= IndexSlots {
				return Lease{}, ErrSecondFull
			}
			a.granted++
		default:
			if !a.store.CompareAndSet(a.system, a.runSecond, true, s) {
				return Lease{}, ErrContended
			}
			a.runSecond, a.granted = s, 0
		}
	}
	notBefore := s * 1000
	notAfter := notBefore + termMs
	if notAfter > TimeMax {
		notAfter = TimeMax
	}
	return NewLease(a.system, WriterFor(s, a.offset+a.granted), notBefore, notAfter)
}
