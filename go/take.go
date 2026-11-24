package eid

import "time"

// The waits of one allocator run.
const (
	// activePoll is the pause between attempts to take an activation that another run holds.
	activePoll = 30 * time.Millisecond
	// DefaultActiveWait is how long a run polls a held activation before it refuses with `active`: one attempt,
	// then 100 more, activePoll apart.
	DefaultActiveWait = 3 * time.Second

	waitTries = 2 // attempts to wait out the grant second of an earlier run
)

// Sleeper is implemented by a Clock that owns its waits. TakeLease waits through it when its clock has one, so a
// manual clock can move itself instead of the wall clock moving; any other clock waits with time.Sleep.
type Sleeper interface {
	Sleep(d time.Duration)
}

// Sleep moves the clock forward by d, which stands for waiting d.
func (c *ManualClock) Sleep(d time.Duration) { c.Advance(uint64(d / time.Millisecond)) }

func pause(c Clock, d time.Duration) {
	if s, ok := c.(Sleeper); ok {
		s.Sleep(d)
		return
	}
	time.Sleep(d)
}

// TakeLease runs one allocator run of system on st under home and returns one lease of termMs. The run takes the
// activation of system, polling a held one for activeWait; asks for the grant and, when it starts in the second of
// an earlier grant, waits for the next second and asks again (twice); and releases the activation before it returns,
// so the next run on the store takes its own lease (spec/lease.md#allocator, spec/cli.md#minting-from-a-store).
// The activation fences runs of other processes only on a FileStore; a MemoryStore fences runs within one process.
func TakeLease(reg *Registry, system Code, home string, st LeaseStore, clock Clock, r RandomSource, termMs uint64,
	activeWait time.Duration) (Lease, error) {
	return takeLease(reg, system, home, st, clock, r, termMs, activeWait, false)
}

// takeLease is TakeLease; skipStartWait is set only by tests, to show what the wait prevents.
func takeLease(reg *Registry, system Code, home string, st LeaseStore, clock Clock, r RandomSource, termMs uint64,
	activeWait time.Duration, skipStartWait bool) (Lease, error) {
	offset, err := RandomOffset(r)
	if err != nil {
		return Lease{}, err
	}
	polls := int(activeWait / activePoll)
	var a *Allocator
	for try := 0; ; try++ {
		a, err = StartAllocator(reg, system, home, st, clock, offset)
		if err != ErrActive || try >= polls {
			break
		}
		pause(clock, activePoll)
	}
	if err != nil {
		return Lease{}, err
	}
	defer st.Release(system)
	a.skipStartWait = skipStartWait
	var l Lease
	for try := 0; ; try++ {
		l, err = a.Grant(termMs)
		if err != ErrWait || try == waitTries {
			break
		}
		pause(clock, time.Duration(1000-clock.NowMs()%1000+1)*time.Millisecond)
	}
	if err != nil {
		return Lease{}, err
	}
	return l, nil
}
