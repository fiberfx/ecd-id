package eid

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func encodingSample(t *testing.T) ID {
	t.Helper()
	id, err := ParseString(tsv(t, "valid.tsv")[0][0])
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestEncodingValidVectorsRoundTrip(t *testing.T) {
	rows := tsv(t, "valid.tsv")
	for _, r := range rows {
		want, err := ParseString(r[0])
		if err != nil {
			t.Fatalf("%s: %v", r[0], err)
		}
		text, err := want.MarshalText()
		if err != nil || string(text) != r[0] {
			t.Fatalf("MarshalText %s: %q, %v", r[0], text, err)
		}
		var back ID
		if err := back.UnmarshalText(text); err != nil || back != want {
			t.Fatalf("UnmarshalText %s: %v, %v", r[0], back, err)
		}
		v, err := want.Value()
		if err != nil {
			t.Fatalf("Value %s: %v", r[0], err)
		}
		raw, ok := v.([]byte)
		if !ok || !bytes.Equal(raw, unhex(t, r[1])) {
			t.Fatalf("Value %s: %T %x", r[0], v, v)
		}
		var scanned ID
		if err := scanned.Scan(v); err != nil || scanned != want {
			t.Fatalf("Scan %s: %v, %v", r[0], scanned, err)
		}
		var null NullID
		nv, err := NullID{ID: want, Valid: true}.Value()
		if err != nil || !bytes.Equal(nv.([]byte), raw) {
			t.Fatalf("NullID.Value %s: %v, %v", r[0], nv, err)
		}
		if err := null.Scan(nv); err != nil || !null.Valid || null.ID != want {
			t.Fatalf("NullID.Scan %s: %v, %v", r[0], null, err)
		}
	}
	fmt.Printf("encodings: %d valid ids round-trip as text and as SQL values\n", len(rows))
}

func TestEncodingValueIsACopy(t *testing.T) {
	id := encodingSample(t)
	v, err := id.Value()
	if err != nil {
		t.Fatal(err)
	}
	raw := v.([]byte)
	raw[0] ^= 0xff
	if id != encodingSample(t) {
		t.Fatal("changing the returned bytes changed the id")
	}
}

func TestEncodingInvalidTextsRefusedAsParseRefuses(t *testing.T) {
	keep := encodingSample(t)
	rows := tsv(t, "invalid.tsv")
	for _, r := range rows {
		in := unhex(t, r[1])
		_, want := Parse(in)
		id := keep
		err := id.UnmarshalText(in)
		if err == nil || err != want || reason(err) != r[2] {
			t.Fatalf("case %s: got %v, Parse %v, want %s", r[0], err, want, r[2])
		}
		if id != keep {
			t.Fatalf("case %s: the id changed on a refusal", r[0])
		}
	}
	fmt.Printf("encodings: %d invalid texts refused as Parse refuses them\n", len(rows))
}

func TestEncodingZeroRefused(t *testing.T) {
	var zero ID
	if _, err := zero.MarshalText(); err != ErrZero {
		t.Fatalf("MarshalText of the zero id: %v", err)
	}
	if _, err := zero.Value(); err != ErrZero {
		t.Fatalf("Value of the zero id: %v", err)
	}
	if _, err := (NullID{Valid: true}).Value(); err != ErrZero {
		t.Fatalf("Value of a valid NullID holding the zero id: %v", err)
	}
	if _, err := (NullID{Valid: true}).MarshalJSON(); err != ErrZero {
		t.Fatalf("MarshalJSON of a valid NullID holding the zero id: %v", err)
	}
	if err := zero.Scan(make([]byte, 16)); err == nil {
		t.Fatal("Scan accepted 16 zero bytes")
	}
}

func TestEncodingScanRefusals(t *testing.T) {
	keep := encodingSample(t)
	cases := []struct {
		name string
		src  any
		want error
	}{
		{"text as string", keep.String(), ErrScanType},
		{"hex as string", fmt.Sprintf("%x", keep[:]), ErrScanType},
		{"28 text bytes", []byte(keep.String()), ErrLength},
		{"15 bytes", keep[:15], ErrLength},
		{"17 bytes", append(keep.Bytes(), 0), ErrLength},
		{"empty bytes", []byte{}, ErrLength},
		{"nil", nil, ErrScanType},
		{"int64", int64(1), ErrScanType},
		{"array", [16]byte(keep), ErrScanType},
	}
	for _, c := range cases {
		id := keep
		if err := id.Scan(c.src); err != c.want {
			t.Fatalf("%s: got %v want %v", c.name, err, c.want)
		}
		if id != keep {
			t.Fatalf("%s: the id changed on a refusal", c.name)
		}
		n := NullID{ID: keep, Valid: true}
		if c.src == nil {
			continue
		}
		if err := n.Scan(c.src); err != c.want || n != (NullID{ID: keep, Valid: true}) {
			t.Fatalf("NullID %s: got %v want %v, value %v", c.name, err, c.want, n)
		}
	}
}

type encodingRecord struct {
	ID ID `json:"id"`
}

type encodingNullable struct {
	ID NullID `json:"id"`
}

func TestEncodingJSON(t *testing.T) {
	id := encodingSample(t)
	b, err := json.Marshal(encodingRecord{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"id":"` + id.String() + `"}`; string(b) != want || len(id.String()) != 28 {
		t.Fatalf("marshal: %s want %s", b, want)
	}
	var back encodingRecord
	if err := json.Unmarshal(b, &back); err != nil || back.ID != id {
		t.Fatalf("unmarshal: %v, %v", back, err)
	}
	if _, err := json.Marshal(encodingRecord{}); err == nil || !errors.Is(err, ErrZero) {
		t.Fatalf("marshal of a struct with the zero id: %v", err)
	}
	for _, bad := range []string{`{"id":"EMQ_JOB_23OBZCBAPVUPU2272222"}`, `{"id":"short"}`, `{"id":12}`, `{"id":""}`} {
		got := encodingRecord{ID: id}
		if err := json.Unmarshal([]byte(bad), &got); err == nil {
			t.Fatalf("unmarshal accepted %s", bad)
		}
		if got.ID != id {
			t.Fatalf("unmarshal of %s changed the id", bad)
		}
	}
}

func TestEncodingJSONNullAndType(t *testing.T) {
	id := encodingSample(t)
	got := encodingRecord{ID: id}
	if err := json.Unmarshal([]byte(`{"id":null}`), &got); !errors.Is(err, ErrZero) {
		t.Fatalf("null into an ID: %v", err)
	}
	if got.ID != id {
		t.Fatal("a refused null changed the id")
	}
	if err := json.Unmarshal([]byte(`{"id":5}`), &got); !errors.Is(err, ErrJSONType) {
		t.Fatalf("a number into an ID: %v", err)
	}
	if got.ID != id {
		t.Fatal("a refused number changed the id")
	}
	text := `{"id":"` + id.String() + `"}`
	var quoted encodingRecord
	if err := json.Unmarshal([]byte(text), &quoted); err != nil || quoted.ID != id {
		t.Fatalf("a quoted text into an ID: %v, %v", quoted, err)
	}
	var missing encodingRecord
	missing.ID = id
	if err := json.Unmarshal([]byte(`{}`), &missing); err != nil || missing.ID != id {
		t.Fatalf("a missing field: %v, %v", missing, err)
	}
}

func TestEncodingNullIDJSON(t *testing.T) {
	id := encodingSample(t)
	b, err := json.Marshal(encodingNullable{})
	if err != nil || string(b) != `{"id":null}` {
		t.Fatalf("absent: %s, %v", b, err)
	}
	var absent encodingNullable
	if err := json.Unmarshal(b, &absent); err != nil || absent.ID.Valid {
		t.Fatalf("absent back: %v, %v", absent, err)
	}
	b, err = json.Marshal(encodingNullable{ID: NullID{ID: id, Valid: true}})
	if want := `{"id":"` + id.String() + `"}`; err != nil || string(b) != want {
		t.Fatalf("present: %s, %v want %s", b, err, want)
	}
	var present encodingNullable
	if err := json.Unmarshal(b, &present); err != nil || !present.ID.Valid || present.ID.ID != id {
		t.Fatalf("present back: %v, %v", present, err)
	}
	over := encodingNullable{ID: NullID{ID: id, Valid: true}}
	if err := json.Unmarshal([]byte(`{"id":null}`), &over); err != nil || over.ID.Valid {
		t.Fatalf("null over a valid id through encoding/json: %v, %v", over, err)
	}
	n := NullID{ID: id, Valid: true}
	if err := n.UnmarshalJSON([]byte(" null ")); err != nil || n.Valid {
		t.Fatalf("null over a valid id: %v, %v", n, err)
	}
	keep := NullID{ID: id, Valid: true}
	for _, bad := range []string{`12`, `true`, `{}`, `[]`, `""`, `"short"`, `"EMQ_JOB_23OBZCBAPVUPU2272222"`, ``, `"`} {
		got := keep
		if err := got.UnmarshalJSON([]byte(bad)); err == nil {
			t.Fatalf("UnmarshalJSON accepted %q", bad)
		}
		if got != keep {
			t.Fatalf("UnmarshalJSON of %q changed the value", bad)
		}
	}
	if _, err := (NullID{ID: id}).Value(); err != nil {
		t.Fatalf("an absent NullID with a set id: %v", err)
	}
	if v, err := (NullID{ID: id}).Value(); v != nil || err != nil {
		t.Fatalf("absent Value: %v, %v", v, err)
	}
	var s NullID
	if err := s.Scan(nil); err != nil || s.Valid {
		t.Fatalf("Scan of nil: %v, %v", s, err)
	}
}
