package main

import (
	"fmt"
	"strings"
)

// cliClock is the clock reading, in milliseconds, that the command-line vectors assume. Every lease the rows pass
// begins after it, so a minter stamps the lease's own not_before and the output does not depend on the real clock.
const cliClock = 1795271400123

// cliCell encodes a standard output or error text for a vector cell: "-" is empty and \n stands for a newline.
func cliCell(s string) string {
	if s == "" {
		return "-"
	}
	return strings.ReplaceAll(s, "\n", `\n`)
}

func cliRow(name, args string, exit int, stdout, stderr string) string {
	return row(name, args, exit, cliCell(stdout), cliCell(stderr))
}

// describe is the JSON line that `eid parse` prints for an id.
func describe(b []byte) string {
	f := fields(b)
	return fmt.Sprintf(`{"text":"%s","hex":"%x","system":"%s","kind":"%s","time":%d,"writer":%d,"count":%d,"class":"%s","stream":"%s"}`+"\n",
		encode(b), b, codeText(f.system), codeText(f.kind), f.time, f.writer, f.count, systemClass(f.system), streamText(b))
}

func invalidText(name string) string {
	for _, c := range invalidCases() {
		if c.name == name {
			return c.raw
		}
	}
	panic("no invalid case " + name)
}

func genCLI() string {
	var out strings.Builder
	out.WriteString(header)
	out.WriteString("# command lines of eid: arguments (- for none), exit status, standard output and standard error;\n")
	out.WriteString("# \\n is a newline, - is empty and * is any text; EID_REGISTRY names registry/ and the clock reads 1795271400123\n")
	out.WriteString("# case\targs\texit\tstdout\tstderr\n")

	ids := []struct {
		name string
		b    []byte
	}{
		{"job", pack(mustCode("emq"), mustCode("job"), 1795271400123, 895287301, 0)},
		{"system-id", must(systemID("emq"))},
		{"detached", pack(mustCode("3ab"), mustCode("run"), 1795271400000, 123456789, 7)},
		{"max", pack(mustCode("zzz"), mustCode("zzz"), timeMax, writerMax, countLimit-1)},
	}
	for _, id := range ids {
		out.WriteString(cliRow("parse-"+id.name, "parse "+encode(id.b), 0, describe(id.b), ""))
		out.WriteString(cliRow("valid-"+id.name, "valid "+encode(id.b), 0, "ok\n", ""))
	}
	for _, name := range []string{"upper", "len27", "len29", "sep-hyphen", "sep-moved", "sep-second", "zero", "pad",
		"pad-z", "reserved", "system-2", "all-zero", "kind-digit-1"} {
		raw := invalidText(name)
		_, err := parse([]byte(raw))
		assert(err != nil, "accepted "+name)
		out.WriteString(cliRow("valid-"+name, "valid "+raw, 1, "", "refused: "+reason(err)+"\n"))
	}
	for _, name := range []string{"len29", "reserved", "kind-digit-2"} {
		raw := invalidText(name)
		_, err := parse([]byte(raw))
		assert(err != nil, "accepted "+name)
		out.WriteString(cliRow("parse-"+name, "parse "+raw, 1, "", "refused: "+reason(err)+"\n"))
	}

	const (
		emqLease = "emq:895287301:4102444800000:4102531200000"
		kanLease = "kan:7:4102444800000:4102531200000"
		detLease = "3ab:123456789:4102444800000:4102531200000"
		mxeLease = "mxe:5:4102444800000:4102531200000"
	)
	minted := func(leaseStr, kind string, n int) string {
		mt := newMinter(must(parseLease(leaseStr)))
		var s strings.Builder
		for i := 0; i < n; i++ {
			s.WriteString(encode(must(mt.mint(cliClock, kind))) + "\n")
		}
		return s.String()
	}
	mintRow := func(name, system, kind string, n int, leaseStr string) {
		args := fmt.Sprintf("mint %s %s -n %d --lease %s", system, kind, n, leaseStr)
		out.WriteString(cliRow(name, args, 0, minted(leaseStr, kind, n), ""))
	}
	mintRow("mint-one", "emq", "job", 1, emqLease)
	mintRow("mint-three", "emq", "job", 3, emqLease)
	mintRow("mint-other-kind", "emq", "evt", 2, emqLease)
	mintRow("mint-kan-doc", "kan", "doc", 2, kanLease)
	mintRow("mint-mxe-itm", "mxe", "itm", 2, mxeLease)
	mintRow("mint-mxe-sto", "mxe", "sto", 1, mxeLease)
	mintRow("mint-detached-lease", "3ab", "run", 2, detLease)
	// A detached system has no registry rows: a kind that no row names mints (spec/registry.md#lease-free).
	mintRow("mint-detached-kind-unregistered", "3ab", "zzz", 2, detLease)
	out.WriteString(cliRow("mint-default-count", "mint emq job --lease "+emqLease, 0, minted(emqLease, "job", 1), ""))

	refused := func(name, args, why string) {
		out.WriteString(cliRow(name, args, 1, "", "refused: "+why+"\n"))
	}
	// Refusals that follow from registry/: emq has no kind zzz, zzz is no system and brd is retired.
	refused("mint-kind-not-registered", "mint emq zzz --lease "+emqLease, "kind")
	refused("mint-mxe-kind-not-registered", "mint mxe zzz --lease "+mxeLease, "kind")
	refused("mint-system-unknown", "mint zzz job --lease zzz:5:4102444800000:4102531200000", "unknown")
	refused("mint-system-retired", "mint brd agt --lease brd:5:4102444800000:4102531200000", "state")
	// Refusals that follow from the format.
	refused("mint-kind-upper", "mint emq JOB --lease "+emqLease, "kind")
	refused("mint-kind-short", "mint emq jo --lease "+emqLease, "kind")
	refused("mint-kind-digit", "mint emq j2b --lease "+emqLease, "kind")
	refused("mint-detached-kind-digit", "mint 3ab j2b --lease "+detLease, "kind")
	refused("mint-system-reserved", "mint 2ab job --lease 2ab:5:4102444800000:4102531200000", "system-reserved")
	for _, c := range []struct{ name, lease string }{
		{"mint-lease-syntax", "emq:05:1:2"},
		{"mint-lease-writer", "emq:1073741824:1:2"},
		{"mint-lease-window", "emq:5:0:10"},
	} {
		_, err := parseLease(c.lease)
		assert(err != nil, "accepted "+c.lease)
		refused(c.name, "mint emq job --lease "+c.lease, reason(err))
	}
	refused("mint-lease-system", "mint emq job --lease "+kanLease, "lease-system")
	past := "emq:5:1000:2000"
	_, err := newMinter(must(parseLease(past))).mint(cliClock, "job")
	assert(err != nil, "a lease in the past minted")
	refused("mint-lease-ended", "mint emq job --lease "+past, reason(err))
	// `eid lease` refuses in this order before it opens a store; no row names a granted lease, whose writer depends on
	// the clock and a random offset (the G13 tests take those).
	const leaseStore = " --store /tmp/eid-store"
	refused("lease-reserved", "lease 2ab"+leaseStore, "system-reserved")
	refused("lease-detached", "lease 3ab"+leaseStore, "class")
	refused("lease-unknown", "lease zzz"+leaseStore, "unknown")
	refused("lease-retired", "lease brd"+leaseStore, "state")
	refused("lease-no-home", "lease emq"+leaseStore, "not-home")
	refused("lease-other-home", "lease kan"+leaseStore+" --home box", "not-home")
	refused("lease-mxe-other-home", "lease mxe"+leaseStore+" --home devbox", "not-home")

	usage := func(name, args string) { out.WriteString(cliRow(name, args, 2, "", "*")) }
	usage("usage-no-command", "-")
	usage("usage-unknown-command", "bogus")
	usage("usage-parse-no-id", "parse")
	usage("usage-valid-two-ids", "valid a b")
	usage("usage-mint-no-kind", "mint emq")
	usage("usage-mint-no-source", "mint emq job")
	usage("usage-mint-both-sources", "mint emq job --lease "+emqLease+" --store /tmp/eid-store")
	usage("usage-count-zero", "mint emq job -n 0 --lease "+emqLease)
	usage("usage-count-over", "mint emq job -n 1000001 --lease "+emqLease)
	usage("usage-flag-no-value", "mint emq job --lease")
	usage("usage-unknown-flag", "mint emq job --lease "+emqLease+" --bogus x")
	usage("usage-system-not-a-code", "mint EMQ job --lease "+emqLease)
	usage("usage-detached-with-lease", "detached evt --lease "+emqLease)
	usage("usage-lease-no-system", "lease")
	usage("usage-lease-no-store", "lease kan")
	usage("usage-lease-two-systems", "lease kan emq"+leaseStore)
	usage("usage-lease-with-lease", "lease kan --lease kan:7:1:2"+leaseStore)
	usage("usage-lease-count", "lease kan -n 2"+leaseStore)
	usage("usage-lease-term-zero", "lease kan --term 0"+leaseStore)
	usage("usage-lease-term-over", "lease kan --term 86400001"+leaseStore)
	usage("usage-lease-wait-over", "lease kan --active-wait 60001"+leaseStore)
	usage("usage-lease-system-upper", "lease KAN"+leaseStore)
	return out.String()
}
