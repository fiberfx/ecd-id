package eid

import (
	"fmt"
	"testing"
)

// The lease-free rule of spec/registry.md#lease-free: a detached id is checked by kind only; the registry is
// consulted for registered systems.

func TestRegistryEcdOwner(t *testing.T) {
	row, ok := repoRegistry(t).System(MustCode("ecd"))
	if !ok || row.Owner != "ecd-core" || row.Home != "-" {
		t.Fatalf("ecd row: %+v", row)
	}
}

func detachedMinter(t *testing.T, clock Clock) *Minter {
	return NewMinter(mustLease(t, "3ab", 123456789, t0, t0+LeaseTermMax), clock, NewManualPid(1))
}

func TestRegistryLeaseFreeKindNamedByNoRowPasses(t *testing.T) {
	reg := repoRegistry(t)
	if _, ok := reg.System(MustCode("3ab")); ok {
		t.Fatal("a detached code has a row")
	}
	id, err := detachedMinter(t, NewManualClock(t0+5)).MintKind("zzz")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	back, err := ParseString(id.String())
	if err != nil || back != id || back.Class() != Detached || back.Kind() != MustCode("zzz") {
		t.Fatalf("detached id %s: %v", id, err)
	}
	fmt.Printf("registry: detached %s passes with no row for system 3ab or kind zzz\n", id)
}

func TestRegistryLeaseFreeKindWithADigitIsRefused(t *testing.T) {
	if _, err := detachedMinter(t, NewManualClock(t0+5)).MintKind("j2b"); err != ErrKind {
		t.Fatalf("mint: %v", err)
	}
	if _, err := ParseString("3ab_j2b_25rgqgbq225prncp2222"); err != ErrKind {
		t.Fatalf("parse: %v", err)
	}
	fmt.Println("registry: detached kind j2b refused: kind")
}

func TestRegistryRegisteredKindWithoutARowIsRefused(t *testing.T) {
	reg := repoRegistry(t)
	emq := MustCode("emq")
	if _, ok := reg.Kind(emq, MustCode("zzz")); ok {
		t.Fatal("emq has a row for zzz")
	}
	if _, ok := reg.Kind(emq, MustCode("job")); !ok {
		t.Fatal("emq has no row for job")
	}
	clock := NewManualClock(t0 + 5)
	var kinds []Code
	for _, k := range reg.KindsOf(emq) {
		kinds = append(kinds, k.Kind)
	}
	d := NewDoor(NewMinter(mustLease(t, "emq", 1, t0, t0+LeaseTermMax), clock, NewManualPid(1)), kinds, 2000, clock)
	own, err := NewMinter(mustLease(t, "emq", 7, t0, t0+LeaseTermMax), clock, NewManualPid(1)).Mint(MustCode("zzz"))
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if _, err := d.Admit(own, nil); err != ErrKind {
		t.Fatalf("registered id of a kind with no row: %v", err)
	}
	// The same kind name stands detached: it is compared by name against the door's kinds, and the registry is not
	// consulted for system 3ab.
	det := detachedMinter(t, clock)
	zzz, _ := det.MintKind("zzz")
	if _, err := d.Admit(zzz, nil); err != ErrKind {
		t.Fatalf("detached id of a kind the door does not accept: %v", err)
	}
	job, _ := det.MintKind("job")
	a, err := d.Admit(job, nil)
	if err != nil || a.Own || a.ID.System() != emq || a.ID.Kind() != MustCode("job") {
		t.Fatalf("detached id of an accepted kind: %+v %v", a, err)
	}
	fmt.Println("registry: emq/zzz refused: kind; detached zzz refused by name at the door; detached job bound")
}
