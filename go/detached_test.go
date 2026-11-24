package eid

import "testing"

// The detached code draw of spec/lease.md#detached: one 16-bit draw r; r >= 61440 is drawn again, at most 64 times;
// the code is 1024 + r mod 5120.

// scriptedRandom returns given bytes in order; a read past the end fails with ErrRandom.
type scriptedRandom struct {
	data []byte
	pos  int
}

func (s *scriptedRandom) Fill(buf []byte) error {
	if s.pos+len(buf) > len(s.data) {
		return ErrRandom
	}
	copy(buf, s.data[s.pos:])
	s.pos += len(buf)
	return nil
}

// drawBytes returns the bytes of the given 16-bit draws followed by the four writer bytes 00 00 00 07.
func drawBytes(draws ...uint16) []byte {
	var b []byte
	for _, d := range draws {
		b = append(b, byte(d>>8), byte(d))
	}
	return append(b, 0, 0, 0, 7)
}

func detachedDrawLease(t *testing.T, src *scriptedRandom) Lease {
	t.Helper()
	lease, err := DetachedLease(src, NewManualClock(t0), nil)
	if err != nil {
		t.Fatalf("detached lease: %v", err)
	}
	if lease.System.Class() != Detached || lease.Writer != 7 {
		t.Fatalf("lease %+v: want a detached system and writer 7", lease)
	}
	return lease
}

func TestDetachedDrawOneRejection(t *testing.T) {
	src := &scriptedRandom{data: []byte{0xF0, 0x00, 0x00, 0x00, 0, 0, 0, 7}}
	lease := detachedDrawLease(t, src)
	if got := lease.System.String(); got != "322" || lease.System != 1024 {
		t.Fatalf("code %s (%d), want 322 (1024)", lease.System, uint16(lease.System))
	}
	if src.pos != len(src.data) {
		t.Fatalf("read %d of %d bytes", src.pos, len(src.data))
	}
}

func TestDetachedDrawLastAccepted(t *testing.T) {
	lease := detachedDrawLease(t, &scriptedRandom{data: []byte{0xEF, 0xFF, 0, 0, 0, 7}})
	if got := lease.System.String(); got != "7zz" || lease.System != 6143 {
		t.Fatalf("code %s (%d), want 7zz (6143)", lease.System, uint16(lease.System))
	}
}

func TestDetachedDrawAfterSixtyThreeRejections(t *testing.T) {
	draws := make([]uint16, 0, 64)
	for i := 0; i < 63; i++ {
		draws = append(draws, 0xFFFF)
	}
	draws = append(draws, 5)
	lease := detachedDrawLease(t, &scriptedRandom{data: drawBytes(draws...)})
	if got := lease.System.String(); got != "327" || lease.System != 1029 {
		t.Fatalf("code %s (%d), want 327 (1029)", lease.System, uint16(lease.System))
	}
}

func TestDetachedDrawSixtyFourRejectionsRefused(t *testing.T) {
	draws := make([]uint16, 64)
	for i := range draws {
		draws[i] = 0xFFFF
	}
	src := &scriptedRandom{data: drawBytes(draws...)}
	_, err := DetachedLease(src, NewManualClock(t0), nil)
	if err != ErrRandom {
		t.Fatalf("error %v, want %v", err, ErrRandom)
	}
	if src.pos != 128 {
		t.Fatalf("read %d bytes, want 128 (64 draws, no writer draw)", src.pos)
	}
}

func TestDetachedDrawSourceFailureReturnsAtOnce(t *testing.T) {
	src := &scriptedRandom{data: []byte{0xFF}}
	_, err := DetachedLease(src, NewManualClock(t0), nil)
	if err != ErrRandom || src.pos != 0 {
		t.Fatalf("error %v after %d bytes, want %v after none", err, src.pos, ErrRandom)
	}
}

func TestDetachedDrawEveryCodeTwelveTimes(t *testing.T) {
	var seen [1 << 15]int
	for r := 0; r < 61440; r++ {
		lease := detachedDrawLease(t, &scriptedRandom{data: drawBytes(uint16(r))})
		seen[lease.System]++
	}
	for c := range seen {
		want := 0
		if c >= 1024 && c < 6144 {
			want = 12
		}
		if seen[c] != want {
			t.Fatalf("code %s drawn %d times, want %d", Code(c), seen[c], want)
		}
	}
}

func TestDetachedDrawKeepDrawsOnlyTheWriter(t *testing.T) {
	keep := MustCode("3ab")
	src := &scriptedRandom{data: []byte{0, 0, 0, 9, 0xFF, 0xFF}}
	lease, err := DetachedLease(src, NewManualClock(t0), &keep)
	if err != nil {
		t.Fatalf("detached lease: %v", err)
	}
	if lease.System != keep || lease.Writer != 9 || src.pos != 4 {
		t.Fatalf("lease %+v after %d bytes, want system %s, writer 9 after 4", lease, src.pos, keep)
	}
}
