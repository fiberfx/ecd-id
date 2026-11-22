//go:build linux || darwin || freebsd || netbsd || openbsd

package ecdid

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---- G12: the file store

func openStore(t *testing.T, dir string) *FileStore {
	t.Helper()
	st, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestG12_FileStoreKeepsItsRecordAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	emq, kan := MustCode("emq"), MustCode("kan")
	st := openStore(t, dir)
	if _, ok := st.LastSecond(emq); ok {
		t.Fatal("a new store holds a record")
	}
	if st.CompareAndSet(emq, 0, true, 5) {
		t.Fatal("set over a record that does not exist")
	}
	if !st.CompareAndSet(emq, 0, false, 1_795_271_400) || !st.CompareAndSet(kan, 0, false, 7) {
		t.Fatal("first set refused")
	}
	if st.CompareAndSet(emq, 0, false, 9) || st.CompareAndSet(emq, 3, true, 9) {
		t.Fatal("set with a stale expectation")
	}
	again := openStore(t, dir)
	if v, ok := again.LastSecond(emq); !ok || v != 1_795_271_400 {
		t.Fatalf("emq after reopen: %d %v", v, ok)
	}
	if v, ok := again.LastSecond(kan); !ok || v != 7 {
		t.Fatalf("kan after reopen: %d %v", v, ok)
	}
	b, err := os.ReadFile(filepath.Join(dir, "emq"))
	if err != nil || string(b) != "1795271400\n" {
		t.Fatalf("record on disk: %q %v", b, err)
	}
	fmt.Println("G12: the grant second of each system is on disk after the compare-and-set and after a reopen")
}

func TestG12_FileStoreActivationIsOnePerSystemOnADirectory(t *testing.T) {
	dir := t.TempDir()
	emq, kan := MustCode("emq"), MustCode("kan")
	a, b := openStore(t, dir), openStore(t, dir)
	if !a.Activate(emq) || a.Activate(emq) || b.Activate(emq) {
		t.Fatal("a second activation of one system was accepted")
	}
	if !b.Activate(kan) {
		t.Fatal("another system is refused")
	}
	a.Release(emq)
	if !b.Activate(emq) {
		t.Fatal("activation after release is refused")
	}
	fmt.Println("G12: a second activation of a system on one directory is refused, from the same store and from another")
}

func TestG12_FileStoreUnreadableRecordRefusesTheAllocator(t *testing.T) {
	dir := t.TempDir()
	emq := MustCode("emq")
	if err := os.WriteFile(filepath.Join(dir, "emq"), []byte("12x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := openStore(t, dir)
	if v, ok := st.LastSecond(emq); !ok || v != ^uint64(0) || st.Err() == nil {
		t.Fatalf("a corrupt record reads as %d %v, err %v", v, ok, st.Err())
	}
	reg := homedRegistry(t)
	a, err := StartAllocator(reg, emq, "h1", st, NewManualClock(t0), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Grant(60_000); err != ErrClockBehind {
		t.Fatalf("grant over a corrupt record: %v", err)
	}
	fmt.Println("G12: a record that cannot be parsed refuses every grant (clock-behind) instead of granting again")
}

func TestG12_FileStoreServesTheAllocatorAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	reg := homedRegistry(t)
	emq := MustCode("emq")
	clock := NewManualClock(t0 + 123)
	first := openStore(t, dir)
	a, err := StartAllocator(reg, emq, "h1", first, clock, 5)
	if err != nil {
		t.Fatal(err)
	}
	l1, err := a.Grant(60_000)
	if err != nil || l1.Writer != WriterFor(t0/1000, 5) {
		t.Fatalf("first grant: %v %s", err, l1)
	}
	first.Release(emq)

	second := openStore(t, dir)
	b, err := StartAllocator(reg, emq, "h1", second, clock, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Grant(60_000); err != ErrWait {
		t.Fatalf("restart inside the grant second: %v", err)
	}
	clock.Advance(1000)
	l2, err := b.Grant(60_000)
	if err != nil || l2.Writer == l1.Writer || l2.NotBefore <= l1.NotBefore {
		t.Fatalf("grant after the wait: %v %s then %s", err, l1, l2)
	}
	second.Release(emq)

	clock.Set(t0 - 5000)
	c, err := StartAllocator(reg, emq, "h1", openStore(t, dir), clock, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Grant(60_000); err != ErrClockBehind {
		t.Fatalf("clock behind the stored second: %v", err)
	}
	fmt.Println("G12: an allocator restarted on the same directory waits for the next second and refuses a clock behind it")
}

// The helper process of the two-process tests. The parent starts it with ECDID_G12_HELPER=dir|mode|startAtMs|count:
// it waits for startAtMs, claims count grant seconds on the store by compare-and-set, and prints the writer number
// of each claim.
func TestG12_HelperProcess(t *testing.T) {
	spec := os.Getenv("ECDID_G12_HELPER")
	if spec == "" {
		t.Skip("the helper process of the two-process tests")
	}
	p := strings.Split(spec, "|")
	st := openStore(t, p[0])
	st.noLock = p[1] == "nolock"
	startAt, _ := strconv.ParseInt(p[2], 10, 64)
	n, _ := strconv.Atoi(p[3])
	for time.Now().UnixMilli() < startAt {
		time.Sleep(time.Millisecond)
	}
	emq := MustCode("emq")
	for done := 0; done < n; {
		last, has := st.LastSecond(emq)
		next := last + 1
		if !has {
			next = 1
		}
		if st.CompareAndSet(emq, last, has, next) {
			fmt.Println(WriterFor(next, 0))
			done++
		}
	}
}

// twoProcesses runs two helper processes of the given mode on one new store, each claiming per grants, and returns
// the writer numbers they printed and the second the store holds at the end.
func twoProcesses(t *testing.T, mode string, per int) (writers []uint32, stored uint64) {
	t.Helper()
	dir := t.TempDir()
	startAt := time.Now().Add(500 * time.Millisecond).UnixMilli()
	var outs [2]bytes.Buffer
	var cmds [2]*exec.Cmd
	for i := range cmds {
		c := exec.Command(os.Args[0], "-test.run=^TestG12_HelperProcess$")
		c.Env = append(os.Environ(), fmt.Sprintf("ECDID_G12_HELPER=%s|%s|%d|%d", dir, mode, startAt, per))
		c.Stdout, c.Stderr = &outs[i], os.Stderr
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		cmds[i] = c
	}
	for i, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatalf("helper %d: %v\n%s", i, err, outs[i].String())
		}
	}
	for i := range outs {
		for _, l := range strings.Split(outs[i].String(), "\n") {
			if v, err := strconv.ParseUint(l, 10, 32); err == nil {
				writers = append(writers, uint32(v))
			}
		}
	}
	stored, _ = openStore(t, dir).LastSecond(MustCode("emq"))
	return writers, stored
}

func repeated(writers []uint32) int {
	seen := map[uint32]bool{}
	for _, w := range writers {
		seen[w] = true
	}
	return len(writers) - len(seen)
}

func TestG12_TwoProcessesOneThousandGrantsEachGiveDistinctWriters(t *testing.T) {
	writers, stored := twoProcesses(t, "lock", 1000)
	if len(writers) != 2000 || repeated(writers) != 0 || stored != 2000 {
		t.Fatalf("%d writers, %d repeated, store holds %d", len(writers), repeated(writers), stored)
	}
	fmt.Printf("G12: 2 processes x 1000 grants on one store give %d distinct writers; the store holds %d\n",
		len(writers), stored)
}

func TestG12_ControlWithoutTheLockWritersRepeat(t *testing.T) {
	writers, stored := twoProcesses(t, "nolock", 1000)
	n := repeated(writers)
	if len(writers) != 2000 || n == 0 {
		t.Fatalf("%d writers, %d repeated: the control did not repeat any", len(writers), n)
	}
	fmt.Printf("G12 control (no lock): 2 processes x 1000 grants repeat %d of %d writers; the store holds %d\n",
		n, len(writers), stored)
}
