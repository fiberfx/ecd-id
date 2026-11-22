package ecdid

import (
	"fmt"
	"sort"
	"testing"
)

// ---- G5: restarts

func overlapping(leases []Lease) int {
	byWriter := map[uint32][][2]uint64{}
	for _, l := range leases {
		byWriter[l.Writer] = append(byWriter[l.Writer], [2]uint64{l.NotBefore, l.NotAfter})
	}
	repeats := 0
	for _, w := range byWriter {
		sort.Slice(w, func(i, j int) bool { return w[i][0] < w[j][0] })
		for i := 1; i < len(w); i++ {
			if w[i][0] <= w[i-1][1] {
				repeats++
			}
		}
	}
	return repeats
}

func restartCycles(t *testing.T, restarts int, skipWait bool) ([]Lease, int) {
	reg := homedRegistry(t)
	emq := MustCode("emq")
	store := NewMemoryStore()
	clock := NewManualClock(t0)
	var leases []Lease
	sameSecond := 0
	x := uint64(0x9E3779B97F4A7C15)
	for r := 0; r < restarts; r++ {
		a, err := StartAllocator(reg, emq, "h1", store, clock, 0)
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		a.skipStartWait = skipWait
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		grants := 1 + int(x%3)
		for g := 0; g < grants; {
			l, err := a.Grant(LeaseTermMax)
			switch err {
			case nil:
				leases = append(leases, l)
				g++
			case ErrWait:
				sameSecond++
				now := clock.NowMs()
				clock.Set((now/1000+1)*1000 + now%7)
			default:
				t.Fatalf("grant: %v", err)
			}
		}
		store.Release(emq) // the process is killed; its supervisor ends the activation
		clock.Advance(x % 700)
	}
	return leases, sameSecond
}

func TestG05_RestartNeverReissuesALiveWriter(t *testing.T) {
	leases, same := restartCycles(t, 1000, false)
	repeats := overlapping(leases)
	fmt.Printf("G5 good: %d leases over 1000 restarts, %d restarts inside one second, repeats %d\n", len(leases), same, repeats)
	if len(leases) < 1000 || same == 0 || repeats != 0 {
		t.Fatal("restart property")
	}
}

func TestG05_ControlWithoutTheWaitAWriterRepeats(t *testing.T) {
	leases, _ := restartCycles(t, 1000, true)
	repeats := overlapping(leases)
	fmt.Printf("G5 control (wait removed): %d leases, repeats %d\n", len(leases), repeats)
	if repeats == 0 {
		t.Fatal("removing the wait should reissue a live writer")
	}
}

func TestG05_SecondFullAfter8192Grants(t *testing.T) {
	reg := homedRegistry(t)
	emq := MustCode("emq")
	store := NewMemoryStore()
	clock := NewManualClock(t0 + 500)
	a, _ := StartAllocator(reg, emq, "h1", store, clock, 8190)
	writers := map[uint32]bool{}
	for i := 0; i < IndexSlots; i++ {
		l, err := a.Grant(LeaseTermMax)
		if err != nil || writers[l.Writer] {
			t.Fatalf("grant %d: %v", i, err)
		}
		writers[l.Writer] = true
	}
	if _, err := a.Grant(LeaseTermMax); err != ErrSecondFull {
		t.Fatalf("second full: %v", err)
	}
	clock.Advance(1000)
	if l, err := a.Grant(LeaseTermMax); err != nil || writers[l.Writer] {
		t.Fatalf("next second: %v", err)
	}
}

// ---- G6: restore and clocks set back

func grantSeconds(t *testing.T, a *Allocator, clock *ManualClock, seconds int) []Lease {
	var out []Lease
	for len(out) < seconds {
		l, err := a.Grant(LeaseTermMax)
		switch err {
		case nil:
			out = append(out, l)
			clock.Advance(1000)
		case ErrWait:
			clock.Advance(1000)
		default:
			t.Fatalf("grant: %v", err)
		}
	}
	return out
}

func mintTogether(leases []Lease, fromMs uint64) int {
	clock := NewManualClock(fromMs)
	var minters []*Minter
	for _, l := range leases {
		minters = append(minters, NewMinter(l, clock, NewManualPid(1)))
	}
	seen := map[ID]bool{}
	repeats := 0
	for ms := uint64(0); ms < 200; ms++ {
		clock.Set(fromMs + ms)
		for _, m := range minters {
			if id, err := m.Mint(MustCode("evt")); err == nil {
				if seen[id] {
					repeats++
				}
				seen[id] = true
			}
		}
	}
	return repeats
}

// scenario: an allocator grants for 10 seconds; the store is copied after the 4th grant; 6 more grants are lost
// when the store is restored from the copy.
func scenario(t *testing.T, offset uint32) (lost []Lease, restored *MemoryStore) {
	reg := homedRegistry(t)
	store := NewMemoryStore()
	clock := NewManualClock(t0)
	a, err := StartAllocator(reg, MustCode("emq"), "h1", store, clock, offset)
	if err != nil {
		t.Fatal(err)
	}
	grantSeconds(t, a, clock, 4)
	restored = store.Snapshot()
	lost = grantSeconds(t, a, clock, 6)
	restored.Release(MustCode("emq"))
	return lost, restored
}

func TestG06_RestoreWithACorrectClockRepeatsNothing(t *testing.T) {
	reg := homedRegistry(t)
	lost, restored := scenario(t, 17)
	clock := NewManualClock((lost[len(lost)-1].NotBefore/1000+3)*1000 + 250)
	b, err := StartAllocator(reg, MustCode("emq"), "h1", restored, clock, 17)
	if err != nil {
		t.Fatal(err)
	}
	fresh := grantSeconds(t, b, clock, 6)
	lostWriters := map[uint32]bool{}
	for _, l := range lost {
		lostWriters[l.Writer] = true
	}
	shared := 0
	for _, l := range fresh {
		if lostWriters[l.Writer] {
			shared++
		}
	}
	repeats := mintTogether(append(append([]Lease(nil), lost...), fresh...), fresh[len(fresh)-1].NotBefore)
	fmt.Printf("G6 good: %d lost leases still minting beside %d fresh ones after a restore with a correct clock: %d shared writers, %d repeated ids\n", len(lost), len(fresh), shared, repeats)
	if shared != 0 || repeats != 0 {
		t.Fatal("restore with a correct clock repeated")
	}
}

func TestG06_ClockBehindTheStoredSecondIsRefused(t *testing.T) {
	reg := homedRegistry(t)
	store := NewMemoryStore()
	clock := NewManualClock(t0)
	a, _ := StartAllocator(reg, MustCode("emq"), "h1", store, clock, 0)
	grantSeconds(t, a, clock, 3)
	store.Release(MustCode("emq"))
	clock.Set(t0 - 5000)
	b, _ := StartAllocator(reg, MustCode("emq"), "h1", store, clock, 0)
	if _, err := b.Grant(LeaseTermMax); err != ErrClockBehind {
		t.Fatalf("clock behind: %v", err)
	}
	fmt.Println("G6: an allocator whose clock is behind the stored grant second refuses")
}

func restoreAndClockBack(t *testing.T, runs int, sameOffset bool) (runsWithRepeat, repeatedIDs int) {
	reg := homedRegistry(t)
	g := &mix{0xEC0D0006}
	for r := 0; r < runs; r++ {
		offset := uint32(g.next() % 8192)
		lost, restored := scenario(t, offset)
		clock := NewManualClock(lost[0].NotBefore + 100)
		restart := offset
		if !sameOffset {
			restart = uint32(g.next() % 8192)
		}
		b, err := StartAllocator(reg, MustCode("emq"), "h1", restored, clock, restart)
		if err != nil {
			t.Fatal(err)
		}
		fresh := grantSeconds(t, b, clock, 6)
		lostWriters := map[uint32]bool{}
		for _, l := range lost {
			lostWriters[l.Writer] = true
		}
		for _, l := range fresh {
			if lostWriters[l.Writer] {
				runsWithRepeat++
				break
			}
		}
		repeatedIDs += mintTogether(append(append([]Lease(nil), lost...), fresh...), fresh[len(fresh)-1].NotBefore)
	}
	return runsWithRepeat, repeatedIDs
}

func TestG06_ViolatingRestoreAndClockBackRepeatsAreCounted(t *testing.T) {
	rolled, rolledIDs := restoreAndClockBack(t, 20, true)
	fresh, freshIDs := restoreAndClockBack(t, 20, false)
	fmt.Printf("G6 COUNT restore + clock set back, allocator memory rolled back too (same offset): %d/20 runs repeat writers, %d repeated ids\n", rolled, rolledIDs)
	fmt.Printf("G6 COUNT restore + clock set back, allocator restarted (fresh random offset): %d/20 runs repeat writers, %d repeated ids\n", fresh, freshIDs)
	if rolled != 20 || rolledIDs == 0 {
		t.Fatal("with memory and clock rolled back the lost grants repeat exactly")
	}
}

// ---- G7: fenced clone and the pid guard

func TestG07_CopyUnderAnotherHomeIsRefused(t *testing.T) {
	reg := homedRegistry(t)
	store := NewMemoryStore()
	clock := NewManualClock(t0)
	if _, err := StartAllocator(reg, MustCode("emq"), "h1", store, clock, 1); err != nil {
		t.Fatal(err)
	}
	cp := store.Snapshot()
	cp.Release(MustCode("emq"))
	if _, err := StartAllocator(reg, MustCode("emq"), "h2", cp, clock, 1); err != ErrNotHome {
		t.Fatalf("other home: %v", err)
	}
	if _, err := StartAllocator(reg, MustCode("brd"), "h1", NewMemoryStore(), clock, 1); err != ErrState {
		t.Fatalf("closed: %v", err)
	}
	if _, err := StartAllocator(reg, MustCode("3ab"), "h1", NewMemoryStore(), clock, 1); err != ErrClass {
		t.Fatalf("detached: %v", err)
	}
	fmt.Println("G7: a copy of the store started under another home is refused; a closed system and a detached code too")
}

func TestG07_SecondActivationIsRefused(t *testing.T) {
	reg := homedRegistry(t)
	store := NewMemoryStore()
	clock := NewManualClock(t0)
	if _, err := StartAllocator(reg, MustCode("emq"), "h1", store, clock, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := StartAllocator(reg, MustCode("emq"), "h1", store, clock, 2); err != ErrActive {
		t.Fatalf("second activation: %v", err)
	}
	fmt.Println("G7: a second activation of the same code on the same store is refused")
}

func misconfiguredHome(t *testing.T, oa, ob uint32) int {
	reg := homedRegistry(t)
	store := NewMemoryStore()
	cp := store.Snapshot()
	clock := NewManualClock(t0)
	a, _ := StartAllocator(reg, MustCode("emq"), "h1", store, clock, oa)
	b, _ := StartAllocator(reg, MustCode("emq"), "h1", cp, clock, ob)
	wa := map[uint32]bool{}
	shared := 0
	for i := 0; i < 20; i++ {
		la, _ := a.Grant(LeaseTermMax)
		wa[la.Writer] = true
		lb, _ := b.Grant(LeaseTermMax)
		if wa[lb.Writer] {
			shared++
		}
		clock.Advance(1000)
	}
	return shared
}

func TestG07_ControlWithoutTheHomeCheckBothCopiesGrant(t *testing.T) {
	same, different := misconfiguredHome(t, 5, 5), misconfiguredHome(t, 5, 900)
	fmt.Printf("G7 control (home check bypassed): cloned allocator memory grants %d/20 shared writers; distinct offsets %d/20\n", same, different)
	if same != 20 || different != 0 {
		t.Fatal("control")
	}
}

// fork copies the parent's minter whole, as a forked process does; the child process reports its own pid.
func fork(parent *Minter, childPid uint32) *Minter {
	child := parent.Clone()
	child.pids = NewManualPid(childPid)
	return child
}

func TestG07_ForkedChildCannotMintOnTheParentLease(t *testing.T) {
	clock := NewManualClock(t0 + 10)
	parent := NewMinter(mustLease(t, "emq", 4242, t0, t0+LeaseTermMax), clock, NewManualPid(100))
	if _, err := parent.Mint(MustCode("evt")); err != nil {
		t.Fatal(err)
	}
	child := fork(parent, 101)
	for i := 0; i < 5; i++ {
		if _, err := child.Mint(MustCode("evt")); err != ErrFork {
			t.Fatalf("child mint: %v", err)
		}
	}
	if err := child.Switch(mustLease(t, "emq", 9, t0, t0+LeaseTermMax)); err != ErrFork {
		t.Fatalf("child switch: %v", err)
	}
	if _, err := parent.Mint(MustCode("evt")); err != nil {
		t.Fatal(err)
	}
	fmt.Println("G7 pid guard: the forked child's 5 mints and its switch are refused; the parent mints on")
}

func TestG07_ControlWithoutThePidGuardParentAndChildCollide(t *testing.T) {
	clock := NewManualClock(t0 + 10)
	parent := NewMinter(mustLease(t, "emq", 4242, t0, t0+LeaseTermMax), clock, NewManualPid(100))
	child := fork(parent, 100) // a child that reports its parent's pid stands for a minter without the guard
	seen := map[ID]bool{}
	for i := 0; i < 1000; i++ {
		id, _ := parent.Mint(MustCode("evt"))
		seen[id] = true
	}
	dups := 0
	for i := 0; i < 1000; i++ {
		if id, err := child.Mint(MustCode("evt")); err == nil && seen[id] {
			dups++
		}
	}
	fmt.Printf("G7 control (pid guard blind): child and parent mint %d equal ids of 1,000\n", dups)
	if dups == 0 {
		t.Fatal("control")
	}
}

// ---- G10: the door

func doorTraffic(t *testing.T, clock *ManualClock) ([]ID, Lease) {
	leased := NewMinter(mustLease(t, "emq", 7, t0, t0+LeaseTermMax), clock, NewManualPid(1))
	var ids []ID
	for p := 0; p < 50; p++ {
		code := "3a" + string(Alphabet[p%32])
		d := NewMinter(mustLease(t, code, uint32(100+p), t0, t0+LeaseTermMax), clock, NewManualPid(1))
		for i := 0; i < 4; i++ {
			x, _ := d.Mint(MustCode("job"))
			y, _ := leased.Mint(MustCode("job"))
			ids = append(ids, x, y)
			clock.Advance(1)
		}
	}
	return ids, leased.Lease()
}

func newDoor(t *testing.T, clock Clock) *Door {
	return NewDoor(NewMinter(mustLease(t, "emq", 1, t0, t0+LeaseTermMax), clock, NewManualPid(1)),
		[]Code{MustCode("job"), MustCode("evt")}, 2000, clock)
}

func TestG10_DetachedIDsAreBoundNeverStoredAsOwn(t *testing.T) {
	clock := NewManualClock(t0 + 5)
	d := newDoor(t, clock)
	ids, lease := doorTraffic(t, clock)
	bound, detachedStored := 0, 0
	for _, id := range ids {
		var l *Lease
		if id.Class() != Detached {
			l = &lease
		}
		a, err := d.Admit(id, l)
		if err != nil {
			t.Fatalf("admit %s: %v", id, err)
		}
		if !a.Own {
			if a.Token != id || a.ID.System() != MustCode("emq") || a.ID.Kind() != id.Kind() {
				t.Fatalf("binding of %s", id)
			}
			bound++
		}
		if a.ID.Class() == Detached {
			detachedStored++
		}
	}
	fmt.Printf("G10 good: %d ids admitted, %d detached ids bound as command tokens, %d detached ids stored as own\n", len(ids), bound, detachedStored)
	if bound != 200 || detachedStored != 0 {
		t.Fatal("door")
	}
}

func TestG10_AdmittingADetachedIDAgainReturnsTheSameBinding(t *testing.T) {
	clock := NewManualClock(t0 + 5)
	d := newDoor(t, clock)
	lease, err := DetachedLease(OsRandom{}, clock, nil)
	if err != nil || lease.System.Class() != Detached {
		t.Fatalf("detached lease: %v", err)
	}
	token, _ := NewMinter(lease, clock, NewManualPid(1)).Mint(MustCode("job"))
	first, _ := d.Admit(token, nil)
	again, _ := d.Admit(token, nil)
	if first != again || d.Bound() != 1 {
		t.Fatal("retry bound twice")
	}
	fmt.Printf("G10: a retried detached id %s binds to the same id %s\n", token, first.ID)
}

func TestG10_ControlWithoutTheDoorDetachedIDsBecomeFacts(t *testing.T) {
	ids, _ := doorTraffic(t, NewManualClock(t0+5))
	stored := map[ID]bool{}
	detached := 0
	for _, id := range ids {
		if !stored[id] && id.Class() == Detached {
			detached++
		}
		stored[id] = true
	}
	fmt.Printf("G10 control (door off): %d of %d stored facts are detached ids that name no system\n", detached, len(stored))
	if detached == 0 {
		t.Fatal("control")
	}
}

func TestG10_ForeignAndOutOfLeaseIDsRefused(t *testing.T) {
	clock := NewManualClock(t0 + 5)
	d := newDoor(t, clock)
	foreign, _ := NewMinter(mustLease(t, "kan", 7, t0, t0+LeaseTermMax), clock, NewManualPid(1)).Mint(MustCode("job"))
	if _, err := d.Admit(foreign, nil); err != ErrForeign {
		t.Fatalf("foreign: %v", err)
	}
	m := NewMinter(mustLease(t, "emq", 7, t0, t0+LeaseTermMax), clock, NewManualPid(1))
	id, _ := m.Mint(MustCode("job"))
	other := mustLease(t, "emq", 8, t0, t0+LeaseTermMax)
	if _, err := d.Admit(id, &other); err != ErrLease {
		t.Fatalf("lease: %v", err)
	}
	own := m.Lease()
	if a, err := d.Admit(id, &own); err != nil || !a.Own {
		t.Fatalf("own: %v", err)
	}
	fmt.Println("G10: an id of another system and an id outside the session's lease are refused")
}
