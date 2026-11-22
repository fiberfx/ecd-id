package eid

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

const (
	t0      = 1_795_271_400_000
	tStream = 1_795_357_800_000
)

// ---- G2: order

func randomIDs(n int, seed uint64) []ID {
	g := &mix{seed}
	out := make([]ID, n)
	for i := range out {
		system := Code(1024 + g.below(1<<15-1024))
		kind := Code((6+g.below(26))<<10 | (6+g.below(26))<<5 | (6 + g.below(26)))
		id, err := FromParts(system, kind, g.next()&TimeMax, uint32(g.next())&WriterMax, uint32(g.next())&(CountLimit-1))
		if err != nil {
			panic(err)
		}
		out[i] = id
	}
	return out
}

func TestG02_TextOrderEqualsByteOrder(t *testing.T) {
	const n = 1_000_000
	ids := randomIDs(n, 0xEC0D0001)
	byBytes := append([]ID(nil), ids...)
	slices.SortFunc(byBytes, func(a, b ID) int { return a.Compare(b) })
	texts := make([]string, n)
	for i, id := range ids {
		texts[i] = id.String()
	}
	sort.Strings(texts)
	for i, s := range texts {
		id, err := ParseString(s)
		if err != nil || id != byBytes[i] {
			t.Fatalf("position %d: %s", i, s)
		}
	}
	fmt.Printf("G2 good: %d random ids sort the same as text and as bytes\n", n)
}

func TestG02_ControlUnsortedAlphabetBreaksOrder(t *testing.T) {
	const n = 1_000_000
	const std = "abcdefghijklmnopqrstuvwxyz234567"
	ids := randomIDs(n, 0xEC0D0002)
	byBytes := append([]ID(nil), ids...)
	slices.SortFunc(byBytes, func(a, b ID) int { return a.Compare(b) })
	type pair struct {
		text string
		id   ID
	}
	pairs := make([]pair, n)
	for i, id := range ids {
		b := []byte(id.String())
		for j, c := range b {
			if c != '_' {
				b[j] = std[strings.IndexByte(Alphabet, c)]
			}
		}
		pairs[i] = pair{string(b), id}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].text < pairs[j].text })
	mismatches := 0
	for i := range pairs {
		if pairs[i].id != byBytes[i] {
			mismatches++
		}
	}
	fmt.Printf("G2 control (a-z2-7): %d of %d positions differ\n", mismatches, n)
	if mismatches == 0 {
		t.Fatal("the unsorted alphabet should not keep byte order")
	}
}

// ---- G3: bursts

func burstLease(t *testing.T, writer uint32) Lease {
	return mustLease(t, "emq", writer, t0, t0+LeaseTermMax)
}

func TestG03_EightLeasesTenMillionIDs(t *testing.T) {
	const total = 10_000_000
	clock := NewManualClock(t0 + 123)
	per := total / 8
	results := make([][][2]uint64, 8)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		m := NewMinter(burstLease(t, uint32(1000+w)), clock, NewManualPid(7))
		go func(w int, m *Minter) {
			defer wg.Done()
			evt := MustCode("evt")
			out := make([][2]uint64, per)
			for i := range out {
				id, err := m.Mint(evt)
				if err != nil {
					panic(err)
				}
				hi, lo := id.halves()
				out[i] = [2]uint64{hi, lo}
			}
			results[w] = out
		}(w, m)
	}
	wg.Wait()
	less := func(a, b [2]uint64) bool { return a[0] < b[0] || (a[0] == b[0] && a[1] < b[1]) }
	all := make([][2]uint64, 0, total)
	for _, r := range results {
		for i := 1; i < len(r); i++ {
			if !less(r[i-1], r[i]) {
				t.Fatalf("a writer's ids must rise strictly at %d", i)
			}
		}
		all = append(all, r...)
	}
	slices.SortFunc(all, func(a, b [2]uint64) int {
		if less(a, b) {
			return -1
		}
		if less(b, a) {
			return 1
		}
		return 0
	})
	distinct := 0
	for i := range all {
		if i == 0 || all[i] != all[i-1] {
			distinct++
		}
	}
	fmt.Printf("G3 good: %d ids from 8 leases, %d distinct; each writer strictly rising\n", len(all), distinct)
	if distinct != total {
		t.Fatal("duplicates")
	}
}

func TestG03_BorrowsPastTheCountLimit(t *testing.T) {
	m := NewMinter(burstLease(t, 5), NewManualClock(t0+123), NewManualPid(7))
	job := MustCode("job")
	var last ID
	for i := uint32(0); i < CountLimit; i++ {
		id, _ := m.Mint(job)
		if id.Time() != t0+123 || id.Count() != i {
			t.Fatalf("id %d: %d %d", i, id.Time(), id.Count())
		}
		last = id
	}
	next, _ := m.Mint(job)
	if next.Time() != t0+124 || next.Count() != 0 || next.Compare(last) <= 0 {
		t.Fatalf("borrow: %s", next)
	}
	fmt.Printf("G3 borrow: id 262,145 in one millisecond is stamped %d count 0\n", next.Time())
}

func TestG03_TwoKindsNeverRepeatATimeCountPair(t *testing.T) {
	clock := NewManualClock(t0 + 123)
	m := NewMinter(burstLease(t, 9), clock, NewManualPid(7))
	a, b := MustCode("job"), MustCode("evt")
	pairs := map[[2]uint64]bool{}
	for i := 0; i < 600_000; i++ {
		if i%1000 == 0 {
			clock.Advance(1)
		}
		k := a
		if i%2 == 1 {
			k = b
		}
		id, _ := m.Mint(k)
		key := [2]uint64{id.Time(), uint64(id.Count())}
		if pairs[key] {
			t.Fatalf("repeated (time, count) at %d", i)
		}
		pairs[key] = true
	}
	fmt.Println("G3 two kinds: 600,000 ids alternating job and evt, no (time, count) pair repeats")
}

func TestG03_ControlTwoMintersOnOneLeaseDuplicate(t *testing.T) {
	clock := NewManualClock(t0 + 123)
	m1 := NewMinter(burstLease(t, 77), clock, NewManualPid(7))
	m2 := NewMinter(burstLease(t, 77), clock, NewManualPid(7))
	seen := map[ID]bool{}
	dups := 0
	for i := 0; i < 1000; i++ {
		for _, m := range []*Minter{m1, m2} {
			id, _ := m.Mint(MustCode("evt"))
			if seen[id] {
				dups++
			}
			seen[id] = true
		}
	}
	fmt.Printf("G3 control (one lease, two minters): %d duplicates in 2,000 ids\n", dups)
	if dups == 0 {
		t.Fatal("two minters on one lease must collide")
	}
}

// ---- G4: stream ids

func streamMinter(t *testing.T, system string, clock Clock) *Minter {
	return NewMinter(mustLease(t, system, 3<<13, tStream-5000, tStream-5000+LeaseTermMax), clock, NewManualPid(1))
}

func twoSystems(t *testing.T, ms int) []ID {
	clock := NewManualClock(tStream)
	a, b := streamMinter(t, "emq", clock), streamMinter(t, "kan", clock)
	evt := MustCode("evt")
	var ids []ID
	for i := 0; i < ms; i++ {
		clock.Set(tStream + uint64(i))
		x, _ := a.Mint(evt)
		y, _ := b.Mint(evt)
		ids = append(ids, x, y)
	}
	return ids
}

func appendAll(entries []StreamID) (refused int) {
	var top StreamID
	has := false
	for _, e := range entries {
		if ValkeyAccepts(top, has, e) {
			top, has = e, true
		} else {
			refused++
		}
	}
	return refused
}

func TestG04_TwoSystemsSameWriterSameMsDistinctOrdered(t *testing.T) {
	ids := twoSystems(t, 1000)
	slices.SortFunc(ids, func(a, b ID) int {
		sa, sb := a.StreamID(), b.StreamID()
		if sa.Less(sb) {
			return -1
		}
		if sb.Less(sa) {
			return 1
		}
		return 0
	})
	distinct := map[StreamID]bool{}
	var sids []StreamID
	for i, id := range ids {
		s := id.StreamID()
		distinct[s] = true
		sids = append(sids, s)
		if i > 0 && sids[i-1].Ms > s.Ms {
			t.Fatal("not in ms order")
		}
		back, err := FromStream("evt", s)
		if err != nil || back != id {
			t.Fatalf("inverse of %s", s)
		}
	}
	if len(distinct) != len(ids) || appendAll(sids) != 0 {
		t.Fatalf("distinct %d of %d", len(distinct), len(ids))
	}
	fmt.Printf("G4 good: %d ids from 2 systems on one writer, same ms: %d distinct stream ids, appended in ms order, inverse exact\n", len(ids), len(distinct))
}

func TestG04_ControlLegacyTailCollides(t *testing.T) {
	ids := twoSystems(t, 1000)
	var legacy []StreamID
	distinct := map[StreamID]bool{}
	for _, id := range ids {
		s := StreamID{id.Time(), id.LegacyTail()}
		legacy = append(legacy, s)
		distinct[s] = true
	}
	sort.Slice(legacy, func(i, j int) bool { return legacy[i].Less(legacy[j]) })
	refused := appendAll(legacy)
	fmt.Printf("G4 control (48-bit tail without the system): %d ids, %d distinct stream ids, %d appends refused\n", len(ids), len(distinct), refused)
	if len(distinct) != len(ids)/2 || refused != len(ids)/2 {
		t.Fatal("the legacy tail should collide once per millisecond")
	}
}

func TestG04_RefusedSameMsAppendSucceedsWhenReminted(t *testing.T) {
	clock := NewManualClock(tStream)
	low, high := streamMinter(t, "emq", clock), streamMinter(t, "kan", clock)
	evt := MustCode("evt")
	h, _ := high.Mint(evt)
	l, _ := low.Mint(evt)
	if !ValkeyAccepts(StreamID{}, false, h.StreamID()) || ValkeyAccepts(h.StreamID(), true, l.StreamID()) {
		t.Fatal("race")
	}
	clock.Advance(1)
	again, _ := low.Mint(evt)
	if !ValkeyAccepts(h.StreamID(), true, again.StreamID()) {
		t.Fatal("re-mint refused")
	}
	fmt.Printf("G4 race: %s refused after %s; re-minted as %s, accepted\n", l.StreamID(), h.StreamID(), again.StreamID())
}

func TestG04_DoorRefusesAheadAndForeignKind(t *testing.T) {
	doorClock := NewManualClock(t0 + 5)
	d := NewDoor(NewMinter(mustLease(t, "emq", 1, t0, t0+LeaseTermMax), doorClock, NewManualPid(1)),
		[]Code{MustCode("job"), MustCode("evt")}, 2000, doorClock)
	fast := NewManualClock(t0 + 5 + 2001)
	ahead, _ := NewMinter(mustLease(t, "emq", 7, t0, t0+LeaseTermMax), fast, NewManualPid(1)).Mint(MustCode("evt"))
	if _, err := d.Admit(ahead, nil); err != ErrAhead {
		t.Fatalf("ahead: %v", err)
	}
	fast.Set(t0 + 5 + 2000)
	edge, _ := NewMinter(mustLease(t, "emq", 7, t0, t0+LeaseTermMax), fast, NewManualPid(1)).Mint(MustCode("evt"))
	if a, err := d.Admit(edge, nil); err != nil || !a.Own {
		t.Fatalf("edge: %v", err)
	}
	msg, _ := NewMinter(mustLease(t, "emq", 7, t0, t0+LeaseTermMax), doorClock, NewManualPid(1)).Mint(MustCode("msg"))
	if _, err := d.Admit(msg, nil); err != ErrKind {
		t.Fatalf("foreign kind: %v", err)
	}
	dd := NewDoor(NewMinter(mustLease(t, "emq", 1, t0, t0+LeaseTermMax), doorClock, NewManualPid(1)),
		[]Code{MustCode("evt")}, DefaultAheadMs, doorClock)
	at := func(ms uint64) ID {
		id, _ := NewMinter(mustLease(t, "emq", 7, t0, t0+LeaseTermMax), NewManualClock(ms), NewManualPid(1)).Mint(MustCode("evt"))
		return id
	}
	if _, err := dd.Admit(at(t0+5+DefaultAheadMs+1), nil); err != ErrAhead {
		t.Fatalf("default limit, ahead: %v", err)
	}
	if a, err := dd.Admit(at(t0+5+DefaultAheadMs), nil); err != nil || !a.Own {
		t.Fatalf("default limit, edge: %v", err)
	}
	fmt.Printf("G4 door: an id 2001 ms ahead is refused; 2000 ms ahead is admitted; with the default limit, %d refused and %d admitted; a kind the stream does not carry is refused\n", DefaultAheadMs+1, DefaultAheadMs)
}
