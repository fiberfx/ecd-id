package ecdid

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestG01_ValidBothWays(t *testing.T) {
	rows := tsv(t, "valid.tsv")
	for _, r := range rows {
		id, err := ParseString(r[0])
		if err != nil {
			t.Fatalf("%s: %v", r[0], err)
		}
		if hex.EncodeToString(id[:]) != r[1] {
			t.Fatalf("bytes of %s: %x", r[0], id[:])
		}
		back, err := FromBytes(unhex(t, r[1]))
		if err != nil || back != id {
			t.Fatalf("binary ingress %s: %v", r[1], err)
		}
		got := []string{id.String(), id.System().String(), id.Kind().String(), strconv.FormatUint(id.Time(), 10),
			strconv.FormatUint(uint64(id.Writer()), 10), strconv.FormatUint(uint64(id.Count()), 10),
			id.Class().String(), id.StreamID().String()}
		want := []string{r[0], r[2], r[3], r[4], r[5], r[6], r[7], r[8]}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("fields of %s: got %v want %v", r[0], got, want)
		}
		rebuilt, err := FromParts(id.System(), id.Kind(), id.Time(), id.Writer(), id.Count())
		if err != nil || rebuilt != id {
			t.Fatalf("from parts %s: %v", r[0], err)
		}
	}
	fmt.Printf("G1 valid: %d ids agree both ways\n", len(rows))
}

func TestG01_InvalidTextsRefusedWithReason(t *testing.T) {
	rows := tsv(t, "invalid.tsv")
	seen := map[string]bool{}
	for _, r := range rows {
		_, err := Parse(unhex(t, r[1]))
		if reason(err) != r[2] {
			t.Fatalf("case %s: got %s want %s", r[0], reason(err), r[2])
		}
		seen[r[2]] = true
	}
	for _, want := range []string{"length", "separator", "alphabet", "pad", "reserved", "system-reserved", "kind"} {
		if !seen[want] {
			t.Fatalf("no vector for %s", want)
		}
	}
	fmt.Printf("G1 invalid: %d texts refused with their reasons\n", len(rows))
}

func TestG01_BinaryInvalidRefused(t *testing.T) {
	rows := tsv(t, "binary_invalid.tsv")
	for _, r := range rows {
		_, err := FromBytes(unhex(t, r[1]))
		if reason(err) != r[2] {
			t.Fatalf("case %s: got %s want %s", r[0], reason(err), r[2])
		}
	}
	fmt.Printf("G1 binary invalid: %d inputs refused\n", len(rows))
}

func TestG01_LeaseVectors(t *testing.T) {
	n := 0
	for _, r := range tsv(t, "lease.tsv") {
		n++
		if r[0] == "writer" {
			s, _ := strconv.ParseUint(r[1], 10, 64)
			i, _ := strconv.ParseUint(r[2], 10, 32)
			if got := strconv.FormatUint(uint64(WriterFor(s, uint32(i))), 10); got != r[3] {
				t.Fatalf("writer for %s %s: %s want %s", r[1], r[2], got, r[3])
			}
			continue
		}
		l, err := ParseLease(r[1])
		got := "ok"
		if err != nil {
			got = "!" + err.Error()
		} else if l.String() != r[1] {
			t.Fatalf("lease text %s: %s", r[1], l)
		}
		if got != r[2] {
			t.Fatalf("case %s: got %s want %s", r[0], got, r[2])
		}
	}
	fmt.Printf("G1 lease: %d rows\n", n)
}

func TestG01_MintVectors(t *testing.T) {
	clock := NewManualClock(0)
	pid := NewManualPid(1)
	var m *Minter
	n := 0
	for _, r := range tsv(t, "mint.tsv") {
		op, arg, expect := r[1], r[3], r[4]
		var got string
		show := func(id ID, err error) string {
			if err != nil {
				return "!" + err.Error()
			}
			return id.String()
		}
		switch op {
		case "lease":
			l, err := ParseLease(arg)
			if err != nil {
				t.Fatal(err)
			}
			m = NewMinter(l, clock, pid)
			got = "ok"
		case "mint":
			ms, _ := strconv.ParseUint(r[2], 10, 64)
			clock.Set(ms)
			got = show(m.MintKind(arg))
		case "burst":
			ms, _ := strconv.ParseUint(r[2], 10, 64)
			clock.Set(ms)
			kind, count, _ := strings.Cut(arg, "*")
			c, _ := strconv.Atoi(count)
			for i := 0; i < c; i++ {
				got = show(m.MintKind(kind))
			}
		case "switch":
			l, err := ParseLease(arg)
			if err != nil {
				t.Fatal(err)
			}
			got = "ok"
			if err := m.Switch(l); err != nil {
				got = "!" + err.Error()
			}
		default:
			t.Fatalf("op %s", op)
		}
		if got != expect {
			t.Fatalf("scenario %s op %s %s: got %s want %s", r[0], op, arg, got, expect)
		}
		n++
	}
	fmt.Printf("G1 mint: %d steps\n", n)
}

func TestG04_StreamVectorsAndInverse(t *testing.T) {
	rows := tsv(t, "stream.tsv")
	for _, r := range rows {
		id, err := ParseString(r[0])
		if err != nil {
			t.Fatal(err)
		}
		sid := id.StreamID()
		if sid.String() != r[2] || strconv.FormatUint(id.LegacyTail(), 10) != r[3] || sid.Seq >= 1<<63 {
			t.Fatalf("stream of %s: %s %d", r[0], sid, id.LegacyTail())
		}
		p, err := ParseStreamID(r[2])
		if err != nil {
			t.Fatal(err)
		}
		back, err := FromStream(r[1], p)
		if err != nil || back != id {
			t.Fatalf("inverse of %s: %v", r[0], err)
		}
	}
	sids, tails := map[string]bool{}, map[string]bool{}
	for _, r := range rows[:4] {
		sids[r[2]], tails[r[3]] = true, true
	}
	if len(sids) != 4 || len(tails) != 1 {
		t.Fatalf("four systems on one writer: %d stream ids, %d tails", len(sids), len(tails))
	}
	fmt.Printf("G4 stream vectors: %d ids map and return; 4 systems on one writer give 4 stream ids, 1 legacy tail\n", len(rows))
}

func TestG04_StreamInvalidRefused(t *testing.T) {
	rows := tsv(t, "stream_invalid.tsv")
	for _, r := range rows {
		text := r[2]
		if text == "-" {
			text = ""
		}
		sid, err := ParseStreamID(text)
		if err == nil {
			_, err = FromStream(r[1], sid)
		}
		if reason(err) != r[3] {
			t.Fatalf("case %s: got %s want %s", r[0], reason(err), r[3])
		}
	}
	fmt.Printf("G4 stream invalid: %d refused\n", len(rows))
}

func TestG09_MxdbIdentities(t *testing.T) {
	n := 0
	kinds := map[string]string{"owner": "sys", "store": "sto", "database": "dbs", "transaction": "txn"}
	for _, r := range tsv(t, "mxdb.tsv") {
		n++
		b := unhex(t, r[2])
		if strings.HasPrefix(r[3], "!") {
			if _, err := FromBytes(b); reason(err) != r[3][1:] {
				t.Fatalf("%s %s: got %s", r[0], r[2], reason(err))
			}
			continue
		}
		id, err := ParseString(r[1])
		if err != nil || hex.EncodeToString(id[:]) != r[2] || id.Kind().String() != kinds[r[0]] {
			t.Fatalf("%s %s: %v", r[0], r[1], err)
		}
		if r[0] == "owner" {
			own, _ := SystemID(id.System())
			if own != id || id.Time() != 0 || id.Writer() != 0 || id.Count() != 0 {
				t.Fatalf("owner %s", r[1])
			}
		} else if id.Time() == 0 {
			t.Fatalf("minted %s at time 0", r[1])
		}
	}
	own, _ := SystemID(MustCode("emq"))
	if hex.EncodeToString(own[:]) != "54ad8f60000000000000000000000000" {
		t.Fatalf("emq system id %x", own[:])
	}
	fmt.Printf("G9 mxdb: %d rows\n", n)
}

func TestG09_NoLeaseMintsTimeZero(t *testing.T) {
	emq := MustCode("emq")
	if _, err := NewLease(emq, 5, 0, 1000); err != ErrLeaseWindow {
		t.Fatalf("lease from 0: %v", err)
	}
	m := NewMinter(mustLease(t, "emq", 5, 1, 1000), NewManualClock(0), NewManualPid(1))
	id, err := m.Mint(Sys)
	own, _ := SystemID(emq)
	if err != nil || id.Time() != 1 || id == own {
		t.Fatalf("mint at clock 0: %v %s", err, id)
	}
	fmt.Println("G9: a lease starting at 0 is refused; a clock at 0 still stamps time 1, never the system's own id")
}

func TestG11_CanonicalTextIsOneCase(t *testing.T) {
	if strings.ToLower(Alphabet) != Alphabet {
		t.Fatal("alphabet has uppercase")
	}
	var texts []string
	for _, r := range tsv(t, "valid.tsv") {
		texts = append(texts, r[0])
	}
	exact, folded := map[string]bool{}, map[string]bool{}
	for _, s := range texts {
		if strings.ToLower(s) != s {
			t.Fatalf("uppercase in %s", s)
		}
		exact[s], folded[strings.ToLower(s)] = true, true
	}
	if len(exact) != len(folded) {
		t.Fatal("two texts differ only by case")
	}
	if _, err := ParseString(strings.ToUpper("emq_job_23obzcbapvupu2272222")); err != ErrAlphabet {
		t.Fatalf("uppercase: %v", err)
	}
	fmt.Printf("G11 one case: %d distinct texts, %d after case folding; uppercase refused\n", len(exact), len(folded))
}

func TestRegistryFilesLoad(t *testing.T) {
	reg := repoRegistry(t)
	brd, _ := reg.System(MustCode("brd"))
	if brd.State != Retired || len(reg.KindsOf(MustCode("emq"))) < 5 || len(reg.KindsOf(MustCode("brd"))) < 36 {
		t.Fatal("registry content")
	}
	fmt.Printf("registry: %d systems load\n", len(reg.Systems()))
}

func TestRegistryRefusals(t *testing.T) {
	s := "code\tname\towner\thome\tstate\tserved_by\n"
	k := "system\tkind\trole\tstate\tserved_by\tmeaning\n"
	one := s + "emq\tx\t-\t-\tlive\t-\n"
	cases := []struct{ sys, kinds, reason string }{
		{"code\tname\n", k, "registry-header"},
		{s + "emq\tEchoMQ\t-\t-\tlive\n", k, "registry-row"},
		{s + "3mq\tx\t-\t-\tlive\t-\n", k, "registry-code"},
		{one + "emq\ty\t-\t-\tlive\t-\n", k, "registry-duplicate"},
		{s + "emq\tx\t-\t-\tgone\t-\n", k, "registry-state"},
		{s + "emq\tx\t-\t-\tlive\tzzz\n", k, "registry-served-by"},
		{one, k + "kan\tdoc\tentity\tlive\t-\td\n", "registry-system"},
		{one, k + "emq\tsys\tentity\tlive\t-\td\n", "registry-code"},
		{one, k + "emq\tj2b\tentity\tlive\t-\td\n", "registry-code"},
		{one, k + "emq\tjob\tthing\tlive\t-\td\n", "registry-role"},
	}
	for _, c := range cases {
		_, err := ParseRegistry(c.sys, c.kinds)
		re, ok := err.(*RegistryError)
		if !ok || re.Reason != c.reason {
			t.Fatalf("%q %q: got %v want %s", c.sys, c.kinds, err, c.reason)
		}
	}
	fmt.Printf("registry: %d faults refused\n", len(cases))
}
