//go:build linux || darwin || freebsd || netbsd || openbsd

package eid

import (
	"fmt"
	"testing"
	"time"
)

// ---- TakeLease: one allocator run of the file store

// zeroRandom draws the offset 0 every time, so only the wait of the allocator separates two runs.
type zeroRandom struct{}

func (zeroRandom) Fill(buf []byte) error {
	for i := range buf {
		buf[i] = 0
	}
	return nil
}

func takeCalls(t *testing.T, calls int, skipStartWait bool) ([]Lease, *FileStore) {
	t.Helper()
	reg := homedRegistry(t)
	st := openStore(t, t.TempDir())
	clock := NewManualClock(t0 + 500)
	var out []Lease
	for i := 0; i < calls; i++ {
		l, err := takeLease(reg, MustCode("emq"), "h1", st, clock, zeroRandom{}, LeaseTermMax, DefaultActiveWait, skipStartWait)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		out = append(out, l)
		clock.Advance(100)
	}
	return out, st
}

func TestLeaseTakeLeaseGivesEachCallItsOwnWriter(t *testing.T) {
	leases, st := takeCalls(t, 3, false)
	writers := map[uint32]bool{}
	for _, l := range leases {
		writers[l.Writer] = true
		if l.System != MustCode("emq") || l.NotAfter-l.NotBefore != LeaseTermMax {
			t.Fatalf("lease %s", l)
		}
	}
	if len(writers) != 3 || overlapping(leases) != 0 {
		t.Fatalf("%d writers for 3 calls, %d repeats", len(writers), overlapping(leases))
	}
	if !st.Activate(MustCode("emq")) {
		t.Fatal("TakeLease kept the activation after it returned")
	}
	st.Release(MustCode("emq"))
	fmt.Println("G13 TakeLease: 3 calls inside one second give 3 distinct writers, and each releases the activation on return")
}

func TestLeaseTakeLeaseControlWithoutTheStartWaitAWriterRepeats(t *testing.T) {
	leases, _ := takeCalls(t, 3, true)
	repeats := overlapping(leases)
	fmt.Printf("G13 TakeLease control (start-wait removed): %d leases, repeats %d\n", len(leases), repeats)
	if repeats == 0 {
		t.Fatal("removing the start-wait should reissue a live writer")
	}
}

func TestLeaseTakeLeaseRefusals(t *testing.T) {
	reg := homedRegistry(t)
	emq, kan := MustCode("emq"), MustCode("kan")
	dir := t.TempDir()
	st := openStore(t, dir)
	clock := NewManualClock(t0 + 500)
	take := func(system Code, home string, term uint64, wait time.Duration) error {
		_, err := TakeLease(reg, system, home, st, clock, zeroRandom{}, term, wait)
		return err
	}
	if err := take(emq, "h2", LeaseTermMax, 0); err != ErrNotHome {
		t.Fatalf("other home: %v", err)
	}
	if err := take(emq, "h1", 0, 0); err != ErrTerm {
		t.Fatalf("term 0: %v", err)
	}
	if err := take(emq, "h1", LeaseTermMax+1, 0); err != ErrTerm {
		t.Fatalf("term over: %v", err)
	}
	if err := take(MustCode("brd"), "h1", LeaseTermMax, 0); err != ErrState {
		t.Fatalf("retired: %v", err)
	}
	// Every refusal above left the activation free.
	other := openStore(t, dir)
	if !other.Activate(emq) {
		t.Fatal("a refusal kept the activation")
	}
	before := clock.NowMs()
	if err := take(emq, "h1", LeaseTermMax, 90*time.Millisecond); err != ErrActive || clock.NowMs()-before != 90 {
		t.Fatalf("held, 90 ms: %v after %d ms", err, clock.NowMs()-before)
	}
	before = clock.NowMs()
	if err := take(emq, "h1", LeaseTermMax, 0); err != ErrActive || clock.NowMs() != before {
		t.Fatalf("held, no wait: %v after %d ms", err, clock.NowMs()-before)
	}
	other.Release(emq)
	if !st.CompareAndSet(kan, 0, false, (t0+500)/1000+100) {
		t.Fatal("store set")
	}
	if err := take(kan, "h1", LeaseTermMax, 0); err != ErrClockBehind {
		t.Fatalf("stored second ahead: %v", err)
	}
	fmt.Println("G13 TakeLease: not-home, term, state and active (polled for the wait given, not at 0) and clock-behind are refused; none keeps the activation")
}
