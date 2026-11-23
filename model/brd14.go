package main

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ---- the brd14 embedding (spec/registry.md#brd14)

const (
	brd14Len      = 14
	brd14Epoch    = 1704067200000
	brd14Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

var brdSystem = mustCode("brd")

// fromBrd14 embeds a brd14 id: system brd, kind = the namespace in lowercase, time = epoch + ts, writer = node,
// count = seq. The payload's range is checked on its numeric value.
func fromBrd14(t []byte) ([]byte, error) {
	if len(t) != brd14Len {
		return nil, refuse("brd14-length")
	}
	for _, c := range t[:3] {
		if c < 'A' || c > 'Z' {
			return nil, refuse("brd14-namespace")
		}
	}
	n := new(big.Int)
	for _, c := range t[3:] {
		d := strings.IndexByte(brd14Alphabet, c)
		if d < 0 {
			return nil, refuse("brd14-alphabet")
		}
		n.Mul(n, big.NewInt(62))
		n.Add(n, big.NewInt(int64(d)))
	}
	if n.Cmp(new(big.Int).Lsh(big.NewInt(1), 63)) >= 0 {
		return nil, refuse("brd14-range")
	}
	snow := n.Uint64()
	kind := mustCode(strings.ToLower(string(t[:3])))
	return pack(brdSystem, kind, brd14Epoch+snow>>22, (snow>>12)&1023, snow&4095), nil
}

// toBrd14 is the inverse of fromBrd14, for a valid id.
func toBrd14(b []byte) (string, error) {
	f := fields(b)
	assert(kindValid(f.kind), "toBrd14: a valid id")
	if f.system != brdSystem {
		return "", refuse("brd14-system")
	}
	if f.time < brd14Epoch || f.time-brd14Epoch >= 1<<41 {
		return "", refuse("brd14-time")
	}
	if f.writer >= 1<<10 {
		return "", refuse("brd14-writer")
	}
	if f.count >= 1<<12 {
		return "", refuse("brd14-count")
	}
	snow := (f.time-brd14Epoch)<<22 | f.writer<<12 | f.count
	digits := make([]byte, 11)
	for i := 10; i >= 0; i-- {
		digits[i] = brd14Alphabet[snow%62]
		snow /= 62
	}
	return strings.ToUpper(codeText(f.kind)) + string(digits), nil
}

// brdKinds returns the kinds that registry/kinds.tsv gives the system brd.
func brdKinds() map[string]bool {
	text := string(must(os.ReadFile(filepath.Join("..", "registry", "kinds.tsv"))))
	kinds := map[string]bool{}
	header := false
	for _, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if strings.HasPrefix(l, "#") {
			continue
		}
		if !header {
			assert(l == "system\tkind\trole\tstate\tserved_by\tmeaning", "kinds.tsv: the header")
			header = true
			continue
		}
		cells := strings.Split(l, "\t")
		assert(len(cells) == 6, "kinds.tsv: six cells")
		if cells[0] == "brd" {
			kinds[cells[1]] = true
		}
	}
	assert(len(kinds) > 0, "kinds.tsv: brd has kinds")
	return kinds
}

// brd14Ref is a row of brd14's own vectors/valid.tsv (the brd14 reference): the id, unix_ms, node and seq.
type brd14Ref struct {
	id                string
	unixMS, node, seq uint64
}

// brd14References are rows copied from brd14's vectors/valid.tsv: eight of its random corpus and the boundaries of
// the payload, of node and of seq. The model's embedding must give time = unix_ms, writer = node and count = seq.
var brd14References = []brd14Ref{
	{"FTR0tBfKgLQqTm", 1882186635835, 803, 2758},
	{"ZMB5lfLSvvIa1w", 2858434513080, 964, 1640},
	{"EQM4teKEozavkr", 2684096807284, 918, 3885},
	{"VYE5VFPf2mt2ww", 2805444683746, 811, 3350},
	{"CNBAGTYYuKWccK", 3758291363102, 383, 2464},
	{"FCE4OhMgJYV1o6", 2584202746725, 128, 3438},
	{"KKD9a1ME4q776F", 3621268724673, 806, 1959},
	{"JVS86Azq7bG1tE", 3324841141599, 586, 3540},
	{"CRS7BOm9iX2ec9", 3141591343486, 666, 1181},
	{"USR00000000000", 1704067200000, 0, 0},
	{"ABC00000000000", 1704067200000, 0, 0},
	{"SES00000000000", 1704067200000, 0, 0},
	{"USR00000000001", 1704067200000, 0, 1},
	{"USR0000000000z", 1704067200000, 0, 61},
	{"USR00000000010", 1704067200000, 0, 62},
	{"USR00000001000", 1704067200000, 58, 760},
	{"USR0000000Hb84", 1704067200001, 0, 0},
	{"USR0000ciKbTd1", 1704067724287, 1023, 4095},
	{"USR0KHTOWnGLuC", 1769526697641, 0, 0},
	{"USR0NgWEfAEJfs", 1780512970164, 0, 0},
	{"USR5UfZOVH2ZO4", 2803578827776, 0, 0},
	{"USRAzL8n0XnXe4", 3903090455551, 0, 0},
	{"USRAzL8n0Y58m6", 3903090455551, 1023, 4094},
	{"USRAzL8n0Y58m7", 3903090455551, 1023, 4095},
	{"ZZZAzL8n0Y58m7", 3903090455551, 1023, 4095},
}

// brd14RefusalReferences are the first rows of brd14's vectors/invalid.tsv: the input in hex and brd14's class,
// which names the reason: LENGTH, NAMESPACE, CHARSET and RANGE are brd14-length, -namespace, -alphabet and -range.
var brd14RefusalReferences = [][2]string{
	{"5553527a7a7a7a7a7a7a7a7a7a7a", "RANGE"},
	{"757372304b48544f576e474c7543", "NAMESPACE"},
	{"555352304b48544f576e474c75", "LENGTH"},
	{"555352304b48544f576e474c754343", "LENGTH"},
	{"555352417a4c386e305935386d38", "RANGE"},
	{"555352417a4c386e305935386e30", "RANGE"},
	{"5553524230303030303030303030", "RANGE"},
	{"5553526130303030303030303030", "RANGE"},
	{"5553313030303030303030303030", "NAMESPACE"},
	{"550052304b48544f576e474c7543", "NAMESPACE"},
	{"c384c396c39c304b48544f576e47", "NAMESPACE"},
	{"e697a5e69cace8aa9ee381aee38386e382b9e38388", "LENGTH"},
	{"ffffffffffffffffffffffffffff", "NAMESPACE"},
	{"0000000000000000000000000000", "NAMESPACE"},
	{"-", "LENGTH"},
	{"757372304b48544f576e474c2121", "NAMESPACE"},
	{"5553527a7a7a7a7a7a7a7a7a7a21", "CHARSET"},
	{"757372304b48544f576e474c75", "LENGTH"},
	{"555352217a7a7a7a7a7a7a7a7a7a", "CHARSET"},
	{"5553527b30303030303030303030", "CHARSET"},
	{"5553527e30303030303030303030", "CHARSET"},
	{"55", "LENGTH"},
	{"5553", "LENGTH"},
	{"555352", "LENGTH"},
	{"55535230", "LENGTH"},
	{"555352304b", "LENGTH"},
	{"555352304b48", "LENGTH"},
}

var brd14ClassReason = map[string]string{
	"LENGTH": "brd14-length", "NAMESPACE": "brd14-namespace", "CHARSET": "brd14-alphabet", "RANGE": "brd14-range",
}

// brd14Text builds a brd14 id from its namespace and the fields of its snowflake.
func brd14Text(ns string, ts, node, seq uint64) string {
	assert(ts < 1<<41 && node < 1<<10 && seq < 1<<12, "brd14Text: fields in range")
	snow := ts<<22 | node<<12 | seq
	digits := make([]byte, 11)
	for i := 10; i >= 0; i-- {
		digits[i] = brd14Alphabet[snow%62]
		snow /= 62
	}
	return ns + string(digits)
}

func answer(out string, err error) string {
	if err != nil {
		return "!" + reason(err)
	}
	return out
}

func genBrd14() string {
	kinds := brdKinds()
	var out strings.Builder
	out.WriteString(header)
	out.WriteString("# brd14 ids and their embedding into ids of the system brd (spec/registry.md#brd14), both ways\n")
	out.WriteString("# from: input is the brd14 id's bytes in hex; to: input is the Ecd ID's canonical text\n")
	out.WriteString("# expect is the layer's answer: the output, or !reason. pure: from_brd14 or to_brd14, which read no\n")
	out.WriteString("# registry; the registry layer answers the same unless a registry row of the same case and op follows.\n")
	out.WriteString("# registry: the registry layer, against registry/kinds.tsv, refuses a kind that brd has no row for\n")
	out.WriteString("# case\top\tlayer\tinput\texpect\n")
	seen := map[string]bool{}
	// pure writes the pure row of an input and, when the registry layer refuses what the pure layer accepts, the
	// registry row after it.
	pure := func(name, op, input, res string, err error, b []byte) {
		if err != nil {
			seen[reason(err)] = true
		}
		out.WriteString(row(name, op, "pure", input, answer(res, err)))
		if err == nil && !kinds[codeText(fields(b).kind)] {
			seen["brd14-kind"] = true
			out.WriteString(row(name, op, "registry", input, "!brd14-kind"))
		}
	}

	type valid struct{ name, text string }
	var ids []valid
	for i, r := range brd14References {
		b := must(fromBrd14([]byte(r.id)))
		f := fields(b)
		assert(f.time == r.unixMS && f.writer == r.node && f.count == r.seq, "brd14: the reference row "+r.id)
		ids = append(ids, valid{"ref-" + strconv.Itoa(i+1), r.id})
	}
	ids = append(ids,
		valid{"payload-zero", "JOB00000000000"},
		valid{"payload-max", "JOBAzL8n0Y58m7"},
		valid{"node-0-seq-4095", brd14Text("TRC", 1, 0, 4095)},
		valid{"node-1023-seq-0", brd14Text("TRC", 1, 1023, 0)},
		valid{"node-1023-seq-4095", brd14Text("TRC", 1, 1023, 4095)},
		valid{"kind-row-log", brd14Text("LOG", 65459497641, 7, 9)},
		valid{"kind-row-gam", brd14Text("GAM", 65459497641, 7, 9)},
		valid{"kind-no-row", brd14Text("QQQ", 65459497641, 7, 9)},
		valid{"kind-sys", brd14Text("SYS", 65459497641, 7, 9)},
		valid{"kind-msh", brd14Text("MSH", 65459497641, 7, 9)},
		valid{"kind-sdg", brd14Text("SDG", 65459497641, 7, 9)},
		valid{"order-1", brd14Text("EVT", 65459497641, 5, 4095)},
		valid{"order-2", brd14Text("EVT", 65459497641, 6, 0)},
		valid{"order-3", brd14Text("EVT", 65459497642, 0, 0)},
	)
	rng := newMersenne(0xB4D14)
	for i := 0; i < 40; i++ {
		ns := string([]byte{byte('A' + rng.randbelow(26)), byte('A' + rng.randbelow(26)), byte('A' + rng.randbelow(26))})
		ids = append(ids, valid{"random-" + strconv.Itoa(i+1),
			brd14Text(ns, rng.getrandbits(41), rng.getrandbits(10), rng.getrandbits(12))})
	}
	embedded := make([][]byte, len(ids))
	for i, v := range ids {
		b, err := fromBrd14([]byte(v.text))
		back, err2 := toBrd14(b)
		assert(err == nil && err2 == nil && back == v.text, "brd14: "+v.text+" embeds and returns")
		embedded[i] = b
		pure(v.name, "from", hex.EncodeToString([]byte(v.text)), encode(b), nil, b)
		pure(v.name, "to", encode(b), v.text, nil, b)
	}
	for i := range ids {
		for j := range ids {
			want := strings.Compare(ids[i].text, ids[j].text)
			assert(bytes.Compare(embedded[i], embedded[j]) == want, "brd14: byte order equals brd14 text order")
		}
	}

	from := func(name string, raw []byte) {
		b, err := fromBrd14(raw)
		assert(err != nil, "brd14: accepted "+name)
		pure(name, "from", hexOrDash(raw), "", err, b)
	}
	for i, r := range brd14RefusalReferences {
		raw, _ := hex.DecodeString(strings.TrimPrefix(r[0], "-"))
		_, err := fromBrd14(raw)
		assert(err != nil && reason(err) == brd14ClassReason[r[1]], "brd14: the reference refusal "+r[0])
		from("ref-refused-"+strconv.Itoa(i+1), raw)
	}
	for _, c := range []textCase{
		{"empty", ""},
		{"len13", "USR0KHTOWnGLu"},
		{"len15", "USR0KHTOWnGLuC0"},
		{"payload-non-ascii", "USR0KHTOWnGLé"},
		{"ns-lower", "usr0KHTOWnGLuC"},
		{"ns-mixed", "Usr0KHTOWnGLuC"},
		{"ns-digit", "U5R0KHTOWnGLuC"},
		{"ns-and-payload", "usr0KHTOWnGL!!"},
		{"payload-hyphen", "USR0KHTOWn-LuC"},
		{"payload-underscore", "USR0KHTOWn_LuC"},
		{"payload-space", "USR0KHTOWn LuC"},
		{"payload-high-byte", "USR0KHTOWn\xffLuC"},
		{"payload-above-and-off-alphabet", "USR~0000000000"},
		{"range-2^63", "USRAzL8n0Y58m8"},
		{"range-B", "USRB0000000000"},
		{"range-max", "USRzzzzzzzzzzz"},
	} {
		from(c.name, []byte(c.raw))
	}

	brd, usr := brdSystem, mustCode("usr")
	to := func(name string, b []byte) {
		_, err := checkBinary(b)
		assert(err == nil, "brd14: a valid id "+name)
		_, err = toBrd14(b)
		assert(err != nil, "brd14: accepted "+name)
		pure(name, "to", encode(b), "", err, b)
	}
	to("system-emq", must(parse([]byte("emq_job_23obzcbapvupu2272222"))))
	to("system-detached", pack(mustCode("3ab"), usr, brd14Epoch, 0, 0))
	to("system-own-id", must(systemID("brd")))
	to("time-below", pack(brd, usr, brd14Epoch-1, 0, 0))
	to("time-zero", pack(brd, usr, 0, 0, 0))
	to("time-end", pack(brd, usr, brd14Epoch+1<<41, 0, 0))
	to("time-max", pack(brd, usr, timeMax, 0, 0))
	to("writer-1024", pack(brd, usr, brd14Epoch, 1024, 0))
	to("writer-max", pack(brd, usr, brd14Epoch, writerMax, 0))
	to("count-4096", pack(brd, usr, brd14Epoch, 0, 4096))
	to("count-max", pack(brd, usr, brd14Epoch, 0, countLimit-1))
	to("time-writer-count", pack(brd, usr, brd14Epoch-1, 1024, 4096))
	to("writer-count", pack(brd, usr, brd14Epoch, 1024, 4096))
	for _, r := range []string{"brd14-length", "brd14-namespace", "brd14-alphabet", "brd14-range", "brd14-system",
		"brd14-time", "brd14-writer", "brd14-count", "brd14-kind"} {
		assert(seen[r], "brd14: a refusal row for "+r)
	}
	return out.String()
}
