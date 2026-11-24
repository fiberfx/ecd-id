package eid

import (
	"crypto/rand"
	"encoding/binary"
	"os"
	"sync/atomic"
)

// PidSource is a source of the current process id.
type PidSource interface {
	Pid() uint32
}

// ProcessPid is the operating system's process id.
type ProcessPid struct{}

// Pid returns os.Getpid().
func (ProcessPid) Pid() uint32 { return uint32(os.Getpid()) }

// ManualPid is a process id that changes only when told to; for tests of the pid guard.
type ManualPid struct{ v atomic.Uint32 }

// NewManualPid returns a source reporting pid.
func NewManualPid(pid uint32) *ManualPid {
	p := &ManualPid{}
	p.v.Store(pid)
	return p
}

// Pid returns the reported pid.
func (p *ManualPid) Pid() uint32 { return p.v.Load() }

// Set changes the reported pid.
func (p *ManualPid) Set(pid uint32) { p.v.Store(pid) }

// Minter mints ids inside one lease with no round trip. One lease feeds one minter; a minter is not shared between
// goroutines without a lock.
type Minter struct {
	lease   Lease
	hasLast bool
	lastT   uint64
	count   uint32
	clock   Clock
	pids    PidSource
	pid     uint32
}

// NewMinter returns a minter for lease, guarded by the process id pids reports now.
func NewMinter(lease Lease, clock Clock, pids PidSource) *Minter {
	return &Minter{lease: lease, clock: clock, pids: pids, pid: pids.Pid()}
}

// Lease returns the lease in force, with NotBefore as raised by the last switch.
func (m *Minter) Lease() Lease { return m.lease }

// Clone returns a copy of the minter's whole state, as a forked process holds.
func (m *Minter) Clone() *Minter {
	c := *m
	return &c
}

// Mint mints one id of kind.
func (m *Minter) Mint(kind Code) (ID, error) {
	if m.pids.Pid() != m.pid {
		return ID{}, ErrFork
	}
	if !kind.IsKind() {
		return ID{}, ErrKind
	}
	t := m.clock.NowMs()
	if t < m.lease.NotBefore {
		t = m.lease.NotBefore
	}
	if m.hasLast && t < m.lastT {
		t = m.lastT
	}
	var count uint32
	if m.hasLast && t == m.lastT {
		count = m.count + 1
	}
	if count == CountLimit {
		t, count = t+1, 0
	}
	if t > m.lease.NotAfter {
		return ID{}, ErrLeaseEnded
	}
	m.hasLast, m.lastT, m.count = true, t, count
	return pack(m.lease.System, kind, t, m.lease.Writer, count), nil
}

// MintKind mints one id of the kind named by kind.
func (m *Minter) MintKind(kind string) (ID, error) {
	k, ok := ParseCode(kind)
	if !ok {
		return ID{}, ErrKind
	}
	return m.Mint(k)
}

// Switch switches to a fresh lease of the same system, raising its NotBefore past the last id minted.
func (m *Minter) Switch(lease Lease) error {
	if m.pids.Pid() != m.pid {
		return ErrFork
	}
	if lease.System != m.lease.System {
		return ErrLeaseSystem
	}
	next := lease
	if m.hasLast && next.NotBefore < m.lastT+1 {
		next.NotBefore = m.lastT + 1
	}
	if next.NotBefore > next.NotAfter {
		return ErrLeaseEnded
	}
	m.lease = next
	return nil
}

// RandomSource is a source of random bytes. A failure is never replaced by a weaker source.
type RandomSource interface {
	Fill(buf []byte) error
}

// OsRandom is the operating system's random source (crypto/rand).
type OsRandom struct{}

// Fill implements RandomSource.
func (OsRandom) Fill(buf []byte) error {
	if _, err := rand.Read(buf); err != nil {
		return ErrRandom
	}
	return nil
}

// RandomOffset draws a random 13-bit index offset for an allocator start.
func RandomOffset(r RandomSource) (uint32, error) {
	var b [2]byte
	if err := r.Fill(b[:]); err != nil {
		return 0, err
	}
	return uint32(binary.BigEndian.Uint16(b[:])) & 0x1fff, nil
}

// DetachedLease returns a detached lease: a detached system code (keep, when keep is non-nil; drawn otherwise), a
// random writer, and a window of 24 hours from the clock.
func DetachedLease(r RandomSource, clock Clock, keep *Code) (Lease, error) {
	var system Code
	if keep != nil {
		if keep.Class() != Detached {
			return Lease{}, ErrLeaseSystem
		}
		system = *keep
	} else {
		// Each draw reads one 16-bit value v from the source. A value of 61440 (12 x 5120) or more is drawn again,
		// at most 64 times in all; otherwise the code is 1024 + v mod 5120, so each of the 5,120 detached codes
		// (3 to 7 as first character) is equally likely and a draw is rejected with probability 1/16.
		found := false
		for i := 0; i < 64 && !found; i++ {
			var b [2]byte
			if err := r.Fill(b[:]); err != nil {
				return Lease{}, err
			}
			v := binary.BigEndian.Uint16(b[:])
			if v < 61440 {
				system = Code(1024 + v%5120)
				found = true
			}
		}
		if !found {
			return Lease{}, ErrRandom
		}
	}
	var w [4]byte
	if err := r.Fill(w[:]); err != nil {
		return Lease{}, err
	}
	notBefore := clock.NowMs()
	if notBefore < 1 {
		notBefore = 1
	}
	if notBefore > TimeMax-LeaseTermMax {
		notBefore = TimeMax - LeaseTermMax
	}
	return NewLease(system, binary.BigEndian.Uint32(w[:])&WriterMax, notBefore, notBefore+LeaseTermMax)
}
