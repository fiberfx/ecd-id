package eid

import (
	"fmt"
	"strings"
	"testing"
)

var brd14Reasons = []string{"brd14-length", "brd14-namespace", "brd14-alphabet", "brd14-range", "brd14-system",
	"brd14-time", "brd14-writer", "brd14-count", "brd14-kind"}

func brd14Answer(out string, err error) string {
	if err != nil {
		return "!" + err.Error()
	}
	return out
}

func TestG14_Brd14VectorsBothWays(t *testing.T) {
	reg := repoRegistry(t)
	seen := map[string]bool{}
	n := map[string]int{}
	for _, r := range tsv(t, "brd14.tsv") {
		var got, gotReg string
		switch r[1] {
		case "from":
			text := string(unhex(t, r[2]))
			id, err := FromBrd14(text)
			got = brd14Answer(id.String(), err)
			id, err = reg.FromBrd14(text)
			gotReg = brd14Answer(id.String(), err)
		case "to":
			id, err := ParseString(r[2])
			if err != nil {
				t.Fatalf("case %s: %v", r[0], err)
			}
			s, err := ToBrd14(id)
			got = brd14Answer(s, err)
			s, err = reg.ToBrd14(id)
			gotReg = brd14Answer(s, err)
		default:
			t.Fatalf("case %s: op %s", r[0], r[1])
		}
		if got != r[3] || gotReg != r[4] {
			t.Fatalf("case %s %s %s: got %s %s want %s %s", r[0], r[1], r[2], got, gotReg, r[3], r[4])
		}
		for _, a := range r[3:5] {
			if strings.HasPrefix(a, "!") {
				seen[a[1:]] = true
			}
		}
		n[r[1]]++
	}
	for _, want := range brd14Reasons {
		if !seen[want] {
			t.Fatalf("no vector for %s", want)
		}
	}
	fmt.Printf("G14 brd14: %d from and %d to rows agree, pure and registry layer; %d reasons\n", n["from"], n["to"],
		len(seen))
}

func TestG14_Brd14OrderAndInverse(t *testing.T) {
	var texts []string
	var ids []ID
	for _, r := range tsv(t, "brd14.tsv") {
		if r[1] != "to" || strings.HasPrefix(r[3], "!") {
			continue
		}
		id, err := ParseString(r[2])
		if err != nil {
			t.Fatal(err)
		}
		texts, ids = append(texts, r[3]), append(ids, id)
	}
	for i := range ids {
		for j := range ids {
			if strings.Compare(texts[i], texts[j]) != ids[i].Compare(ids[j]) {
				t.Fatalf("order of %s and %s", texts[i], texts[j])
			}
		}
	}
	m := &mix{s: 14}
	var prevText string
	var prev ID
	const n = 100000
	for k := 0; k < n; k++ {
		b := []byte{byte('A' + m.below(26)), byte('A' + m.below(26)), byte('A' + m.below(26))}
		s := m.next() >> 1
		ts, node, seq := s>>22, s>>12&1023, s&4095
		digits := make([]byte, 11)
		for i := 10; i >= 0; i-- {
			digits[i] = brd14Alphabet[s%62]
			s /= 62
		}
		text := string(b) + string(digits)
		id, err := FromBrd14(text)
		if err != nil || id.Time() != Brd14Epoch+ts || uint64(id.Writer()) != node || uint64(id.Count()) != seq {
			t.Fatalf("%s: %v %d %d %d", text, err, id.Time(), id.Writer(), id.Count())
		}
		if back, err := ToBrd14(id); err != nil || back != text {
			t.Fatalf("%s returns %s: %v", text, back, err)
		}
		if k > 0 && strings.Compare(prevText, text) != prev.Compare(id) {
			t.Fatalf("order of %s and %s", prevText, text)
		}
		prevText, prev = text, id
	}
	if _, err := ToBrd14(ID{}); err != ErrSystemReserved {
		t.Fatalf("sixteen zero bytes: %v", err)
	}
	fmt.Printf("G14 brd14: %d vector ids keep brd14 order pairwise; %d random ids return and keep order\n", len(ids), n)
}
