package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
)

const (
	repoRoot = "../../.."
	now      = 1_795_271_400_123 // the clock of the vector rows
)

func registryDir() string { return filepath.Join(repoRoot, "registry") }

// cell reverses the encoding of a vector cell: "-" is empty, and "\n" stands for a newline.
func cell(s string) string {
	if s == "-" {
		return ""
	}
	return strings.ReplaceAll(s, `\n`, "\n")
}

type vectorRow struct {
	name, args, stdout, stderr string
	exit                       string
}

func cliVectors(t *testing.T) []vectorRow {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "vectors", "cli.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []vectorRow
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		c := strings.Split(l, "\t")
		if len(c) != 5 {
			t.Fatalf("cli.tsv row of %d cells: %q", len(c), l)
		}
		rows = append(rows, vectorRow{c[0], c[1], cell(c[3]), c[4], c[2]})
	}
	return rows
}

func vectorArgs(r vectorRow) []string {
	if r.args == "-" {
		return nil
	}
	return strings.Split(r.args, " ")
}

// testEnv is an environment with a manual clock. The clock owns the waits of an allocator run (eid.Sleeper), so a
// wait moves it and costs no time.
func testEnv(clock *eid.ManualClock, vars map[string]string) env {
	return env{
		getenv:   func(k string) string { return vars[k] },
		hostname: func() (string, error) { return "box.local", nil },
		clock:    clock,
		random:   eid.OsRandom{},
		pids:     eid.NewManualPid(1),
	}
}

func mustRun(t *testing.T, e env, args ...string) string {
	t.Helper()
	var out, errb bytes.Buffer
	if code := run(args, e, &out, &errb); code != exitOK {
		t.Fatalf("%v: exit %d: %s", args, code, errb.String())
	}
	return out.String()
}

func TestG13_VectorsThroughRun(t *testing.T) {
	rows := cliVectors(t)
	e := testEnv(eid.NewManualClock(now), map[string]string{"EID_REGISTRY": registryDir()})
	for _, r := range rows {
		var out, errb bytes.Buffer
		code := run(vectorArgs(r), e, &out, &errb)
		if fmt.Sprint(code) != r.exit || out.String() != r.stdout || (r.stderr != "*" && errb.String() != cell(r.stderr)) {
			t.Fatalf("%s %s: exit %d stdout %q stderr %q; want %s %q %q", r.name, r.args, code, out.String(), errb.String(),
				r.exit, r.stdout, r.stderr)
		}
	}
	fmt.Printf("G13: %d command lines agree with vectors/cli.tsv: output, exit status and refusal\n", len(rows))
}

// The same rows against a built binary, when EID_BIN names one.
func TestG13_BinaryAgainstVectors(t *testing.T) {
	bin := os.Getenv("EID_BIN")
	if bin == "" {
		t.Skip("EID_BIN names the binary to check")
	}
	rows := cliVectors(t)
	for _, r := range rows {
		cmd := exec.Command(bin, vectorArgs(r)...)
		cmd.Env = append(os.Environ(), "EID_REGISTRY="+registryDir())
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		code := 0
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				t.Fatal(err)
			}
			code = ee.ExitCode()
		}
		if fmt.Sprint(code) != r.exit || out.String() != r.stdout || (r.stderr != "*" && errb.String() != cell(r.stderr)) {
			t.Fatalf("%s %s: exit %d stdout %q stderr %q; want %s %q %q", r.name, r.args, code, out.String(), errb.String(),
				r.exit, r.stdout, r.stderr)
		}
	}
	fmt.Printf("G13 binary %s: %d command lines agree with vectors/cli.tsv\n", filepath.Base(bin), len(rows))
}

func idsOf(t *testing.T, out string) []eid.ID {
	t.Helper()
	var ids []eid.ID
	for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		id, err := eid.ParseString(l)
		if err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestG13_MintFromAStoreTakesALeasePerCall(t *testing.T) {
	store := t.TempDir()
	clock := eid.NewManualClock(now)
	e := testEnv(clock, map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "m5"})
	kan := eid.MustCode("kan")
	var all []eid.ID
	writers := map[uint32]bool{}
	for call := 0; call < 3; call++ {
		ids := idsOf(t, mustRun(t, e, "mint", "kan", "doc", "-n", "50", "--store", store))
		if len(ids) != 50 {
			t.Fatalf("call %d: %d ids", call, len(ids))
		}
		for _, id := range ids {
			if id.System() != kan || id.Kind() != eid.MustCode("doc") {
				t.Fatalf("call %d: %s", call, id)
			}
		}
		writers[ids[0].Writer()] = true
		all = append(all, ids...)
	}
	seen := map[eid.ID]bool{}
	for i, id := range all {
		if seen[id] || (i > 0 && id.Compare(all[i-1]) <= 0) {
			t.Fatalf("id %d (%s) repeats or goes back", i, id)
		}
		seen[id] = true
	}
	if len(writers) != 3 {
		t.Fatalf("%d writers for 3 calls", len(writers))
	}
	fmt.Printf("G13: 3 calls on one store (2 of them in the last grant's second) mint %d distinct ids, rising, on 3 writers\n",
		len(all))
}

func TestG13_MintRefusals(t *testing.T) {
	store := t.TempDir()
	clock := eid.NewManualClock(now)
	vars := map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "m5"}
	e := testEnv(clock, vars)
	cases := []struct {
		name, reason string
		args         []string
	}{
		{"kind not in the registry", "kind", []string{"mint", "kan", "job", "--store", store}},
		{"system not in the registry", "unknown", []string{"mint", "zzz", "job", "--store", store}},
		{"retired system", "state", []string{"mint", "brd", "agt", "--store", store}},
		{"detached system from a store", "class", []string{"mint", "3ab", "run", "--store", store}},
		{"reserved system", "system-reserved", []string{"mint", "2ab", "run", "--store", store}},
		{"other home", "not-home", []string{"mint", "kan", "doc", "--store", store, "--home", "box"}},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		if code := run(c.args, e, &out, &errb); code != exitRefused || out.Len() != 0 || errb.String() != "refused: "+c.reason+"\n" {
			t.Fatalf("%s: exit %d stdout %q stderr %q", c.name, code, out.String(), errb.String())
		}
	}
	// A store whose activation another holder keeps refuses with `active` once the retries are spent.
	holder, err := eid.OpenFileStore(store)
	if err != nil {
		t.Fatal(err)
	}
	if !holder.Activate(eid.MustCode("kan")) {
		t.Fatal("holder activation")
	}
	var out, errb bytes.Buffer
	if code := run([]string{"mint", "kan", "doc", "--store", store}, e, &out, &errb); code != exitRefused ||
		errb.String() != "refused: active\n" {
		t.Fatalf("held store: exit %d stderr %q", code, errb.String())
	}
	holder.Release(eid.MustCode("kan"))
	mustRun(t, e, "mint", "kan", "doc", "--store", store)
	fmt.Printf("G13: %d refusals before or at the store, and `active` while another holder keeps the activation\n",
		len(cases)+1)
}

func TestG13_DetachedMintsDetachedIDs(t *testing.T) {
	e := testEnv(eid.NewManualClock(now), nil)
	ids := idsOf(t, mustRun(t, e, "detached", "evt", "-n", "20"))
	for i, id := range ids {
		if id.Class() != eid.Detached || id.Kind() != eid.MustCode("evt") || id.System() != ids[0].System() ||
			id.Writer() != ids[0].Writer() || (i > 0 && id.Compare(ids[i-1]) <= 0) {
			t.Fatalf("id %d: %s", i, id)
		}
	}
	other := idsOf(t, mustRun(t, e, "detached", "evt"))
	if other[0].Writer() == ids[0].Writer() && other[0].System() == ids[0].System() {
		t.Fatal("two calls drew one code and one writer")
	}
	var out, errb bytes.Buffer
	if code := run([]string{"detached", "EVT"}, e, &out, &errb); code != exitRefused || errb.String() != "refused: kind\n" {
		t.Fatalf("detached EVT: exit %d %q", code, errb.String())
	}
	fmt.Println("G13: detached mints 20 rising detached ids of one code and one writer, and a second call draws another writer")
}

func leaseOf(t *testing.T, out string) eid.Lease {
	t.Helper()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("lease output %q is not one line", out)
	}
	l, err := eid.ParseLease(strings.TrimSuffix(out, "\n"))
	if err != nil {
		t.Fatalf("%q: %v", out, err)
	}
	return l
}

func TestG13_LeaseTakesALeasePerCallAndMintsInsideIt(t *testing.T) {
	store := t.TempDir()
	clock := eid.NewManualClock(now)
	e := testEnv(clock, map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "m5"})
	kan := eid.MustCode("kan")
	writers := map[uint32]bool{}
	for call := 0; call < 3; call++ {
		l := leaseOf(t, mustRun(t, e, "lease", "kan", "--store", store))
		if l.System != kan || l.NotAfter-l.NotBefore != eid.LeaseTermMax {
			t.Fatalf("call %d: %s", call, l)
		}
		writers[l.Writer] = true
		for _, id := range idsOf(t, mustRun(t, e, "mint", "kan", "doc", "-n", "5", "--lease", l.String())) {
			if id.System() != kan || id.Writer() != l.Writer {
				t.Fatalf("call %d: %s is not inside %s", call, id, l)
			}
		}
	}
	if len(writers) != 3 {
		t.Fatalf("%d writers for 3 calls", len(writers))
	}
	short := leaseOf(t, mustRun(t, e, "lease", "kan", "--store", store, "--term", "5000"))
	if short.NotAfter-short.NotBefore != 5000 || writers[short.Writer] {
		t.Fatalf("--term 5000: %s", short)
	}
	fmt.Println("G13: 3 lease calls on one store (2 of them in the last grant's second) print 3 distinct writers of 24 hours, " +
		"ids mint inside each, and --term sets the window")
}

func TestG13_LeaseGrantsMxeAtItsHomeAndRefusesElsewhere(t *testing.T) {
	clock := eid.NewManualClock(now)
	e := testEnv(clock, map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "devbox"})
	store := t.TempDir()
	l := leaseOf(t, mustRun(t, e, "lease", "mxe", "--home", "m5", "--store", store, "--registry", registryDir()))
	mxe := eid.MustCode("mxe")
	if l.System != mxe || l.NotAfter-l.NotBefore != eid.LeaseTermMax {
		t.Fatalf("lease %s is not a 24 hour lease of mxe", l)
	}
	for _, kind := range []string{"itm", "sto"} {
		ids := idsOf(t, mustRun(t, e, "mint", "mxe", kind, "-n", "3", "--lease", l.String()))
		if len(ids) != 3 {
			t.Fatalf("mint mxe %s: %d ids", kind, len(ids))
		}
		for _, id := range ids {
			if id.System() != mxe || id.Writer() != l.Writer {
				t.Fatalf("%s is not inside %s", id, l)
			}
		}
	}
	absent := filepath.Join(t.TempDir(), "absent")
	var out, errb bytes.Buffer
	if code := run([]string{"lease", "mxe", "--home", "devbox", "--store", absent}, e, &out, &errb); code != exitRefused ||
		out.Len() != 0 || errb.String() != "refused: not-home\n" {
		t.Fatalf("mxe at devbox: exit %d stdout %q stderr %q", code, out.String(), errb.String())
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatalf("a refusal before the store opened created %s (%v)", absent, err)
	}
	fmt.Println("G13: mxe is leased at home m5 and mints itm and sto inside the lease; at home devbox it is refused and the store stays absent")
}

func TestG13_MintsTheSpecKindsOfKan(t *testing.T) {
	clock := eid.NewManualClock(now)
	e := testEnv(clock, map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "m5"})
	store := t.TempDir()
	l := leaseOf(t, mustRun(t, e, "lease", "kan", "--store", store))
	kan := eid.MustCode("kan")
	for _, kind := range []string{"aus", "fea", "spc", "epc", "rng", "pgm", "prj", "trk", "doc"} {
		ids := idsOf(t, mustRun(t, e, "mint", "kan", kind, "-n", "2", "--lease", l.String()))
		if len(ids) != 2 {
			t.Fatalf("mint kan %s: %d ids", kind, len(ids))
		}
		for _, id := range ids {
			if id.System() != kan || id.Kind() != eid.MustCode(kind) || id.Writer() != l.Writer {
				t.Fatalf("%s is not a kan %s inside %s", id, kind, l)
			}
		}
	}
	var out, errb bytes.Buffer
	if code := run([]string{"mint", "kan", "spe", "--lease", l.String()}, e, &out, &errb); code != exitRefused ||
		out.Len() != 0 || errb.String() != "refused: kind\n" {
		t.Fatalf("mint kan spe: exit %d stdout %q stderr %q", code, out.String(), errb.String())
	}
	fmt.Println("G13: kan mints aus, fea, spc, epc, rng, pgm, prj and trk beside doc inside one lease; the unregistered kind spe is refused")
}

func TestG13_LeaseGrantsAppAtItsHomeAndRefusesElsewhere(t *testing.T) {
	clock := eid.NewManualClock(now)
	e := testEnv(clock, map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "devbox"})
	store := t.TempDir()
	l := leaseOf(t, mustRun(t, e, "lease", "app", "--home", "echd-os", "--store", store, "--registry", registryDir()))
	app := eid.MustCode("app")
	if l.System != app || l.NotAfter-l.NotBefore != eid.LeaseTermMax {
		t.Fatalf("lease %s is not a 24 hour lease of app", l)
	}
	for _, kind := range []string{"ech", "mxs"} {
		ids := idsOf(t, mustRun(t, e, "mint", "app", kind, "-n", "3", "--lease", l.String()))
		if len(ids) != 3 {
			t.Fatalf("mint app %s: %d ids", kind, len(ids))
		}
		for _, id := range ids {
			if id.System() != app || id.Kind() != eid.MustCode(kind) || id.Writer() != l.Writer {
				t.Fatalf("%s is not an app %s inside %s", id, kind, l)
			}
			if got := mustRun(t, e, "valid", id.String()); got != "ok\n" {
				t.Fatalf("valid %s: %q", id, got)
			}
		}
	}
	var out, errb bytes.Buffer
	// zzz has no row, ecd is closed (reserved for ecd-core) and the four content kinds are retired: none mints.
	for _, kind := range []string{"zzz", "ecd", "crs", "lsn", "lpt", "chg"} {
		out.Reset()
		errb.Reset()
		if code := run([]string{"mint", "app", kind, "--lease", l.String()}, e, &out, &errb); code != exitRefused ||
			out.Len() != 0 || errb.String() != "refused: kind\n" {
			t.Fatalf("mint app %s: exit %d stdout %q stderr %q", kind, code, out.String(), errb.String())
		}
	}
	absent := filepath.Join(t.TempDir(), "absent")
	out.Reset()
	errb.Reset()
	if code := run([]string{"lease", "app", "--home", "m5", "--store", absent}, e, &out, &errb); code != exitRefused ||
		out.Len() != 0 || errb.String() != "refused: not-home\n" {
		t.Fatalf("app at m5: exit %d stdout %q stderr %q", code, out.String(), errb.String())
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatalf("a refusal before the store opened created %s (%v)", absent, err)
	}
	fmt.Println("G13: app is leased at home echd-os and mints ech and mxs inside the lease, each valid; the unregistered zzz, the closed ecd and the retired crs, lsn, lpt and chg are refused; at home m5 it is refused and the store stays absent")
}

func TestG13_LeaseGrantsLlmAtItsHomeAndRefusesElsewhere(t *testing.T) {
	clock := eid.NewManualClock(now)
	e := testEnv(clock, map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "devbox"})
	store := t.TempDir()
	l := leaseOf(t, mustRun(t, e, "lease", "llm", "--home", "m5", "--store", store, "--registry", registryDir()))
	llm := eid.MustCode("llm")
	if l.System != llm || l.NotAfter-l.NotBefore != eid.LeaseTermMax {
		t.Fatalf("lease %s is not a 24 hour lease of llm", l)
	}
	for _, kind := range []string{"ses", "cnv", "arc"} {
		ids := idsOf(t, mustRun(t, e, "mint", "llm", kind, "-n", "3", "--lease", l.String()))
		if len(ids) != 3 {
			t.Fatalf("mint llm %s: %d ids", kind, len(ids))
		}
		for _, id := range ids {
			if id.System() != llm || id.Kind() != eid.MustCode(kind) || id.Writer() != l.Writer {
				t.Fatalf("%s is not an llm %s inside %s", id, kind, l)
			}
			if got := mustRun(t, e, "valid", id.String()); got != "ok\n" {
				t.Fatalf("valid %s: %q", id, got)
			}
		}
	}
	var out, errb bytes.Buffer
	// zzz is no kind of any system; aus is a kind of kan, not of llm.
	for _, kind := range []string{"zzz", "aus"} {
		out.Reset()
		errb.Reset()
		if code := run([]string{"mint", "llm", kind, "--lease", l.String()}, e, &out, &errb); code != exitRefused ||
			out.Len() != 0 || errb.String() != "refused: kind\n" {
			t.Fatalf("mint llm %s: exit %d stdout %q stderr %q", kind, code, out.String(), errb.String())
		}
	}
	absent := filepath.Join(t.TempDir(), "absent")
	out.Reset()
	errb.Reset()
	if code := run([]string{"lease", "llm", "--home", "devbox", "--store", absent}, e, &out, &errb); code != exitRefused ||
		out.Len() != 0 || errb.String() != "refused: not-home\n" {
		t.Fatalf("llm at devbox: exit %d stdout %q stderr %q", code, out.String(), errb.String())
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatalf("a refusal before the store opened created %s (%v)", absent, err)
	}
	fmt.Println("G13: llm is leased at home m5 and mints ses, cnv and arc inside the lease, each valid; the unregistered zzz and kan's aus are refused; at home devbox it is refused and the store stays absent")
}

func TestG13_LeaseRefusalsBeforeTheStoreLeaveItUntouched(t *testing.T) {
	clock := eid.NewManualClock(now)
	e := testEnv(clock, map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "m5"})
	absent := filepath.Join(t.TempDir(), "absent")
	cases := []struct {
		name, reason string
		args         []string
	}{
		{"reserved system", "system-reserved", []string{"lease", "2ab", "--store", absent}},
		{"detached system", "class", []string{"lease", "3ab", "--store", absent}},
		{"system not in the registry", "unknown", []string{"lease", "zzz", "--store", absent}},
		{"retired system", "state", []string{"lease", "brd", "--store", absent}},
		{"system without a home", "not-home", []string{"lease", "emq", "--store", absent}},
		{"other home", "not-home", []string{"lease", "kan", "--store", absent, "--home", "box"}},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		if code := run(c.args, e, &out, &errb); code != exitRefused || out.Len() != 0 || errb.String() != "refused: "+c.reason+"\n" {
			t.Fatalf("%s: exit %d stdout %q stderr %q", c.name, code, out.String(), errb.String())
		}
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatalf("a refusal before the store opened created %s (%v)", absent, err)
	}
	fmt.Printf("G13: %d lease refusals are made before the store opens and leave its directory absent\n", len(cases))
}

func TestG13_LeaseHeldActivationAndClockBehind(t *testing.T) {
	store := t.TempDir()
	clock := eid.NewManualClock(now)
	e := testEnv(clock, map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "m5"})
	kan := eid.MustCode("kan")
	holder, err := eid.OpenFileStore(store)
	if err != nil {
		t.Fatal(err)
	}
	if !holder.Activate(kan) {
		t.Fatal("holder activation")
	}
	var out, errb bytes.Buffer
	before := clock.NowMs()
	if code := run([]string{"lease", "kan", "--store", store, "--active-wait", "90"}, e, &out, &errb); code != exitRefused ||
		out.Len() != 0 || errb.String() != "refused: active\n" || clock.NowMs()-before != 90 {
		t.Fatalf("held store, 90 ms: exit %d stdout %q stderr %q, waited %d ms", code, out.String(), errb.String(),
			clock.NowMs()-before)
	}
	out.Reset()
	errb.Reset()
	before = clock.NowMs()
	if code := run([]string{"lease", "kan", "--store", store, "--active-wait", "0"}, e, &out, &errb); code != exitRefused ||
		errb.String() != "refused: active\n" || clock.NowMs() != before {
		t.Fatalf("held store, no wait: exit %d stderr %q, waited %d ms", code, errb.String(), clock.NowMs()-before)
	}
	holder.Release(kan)
	l := leaseOf(t, mustRun(t, e, "lease", "kan", "--store", store))
	if l.System != kan {
		t.Fatalf("after the release: %s", l)
	}
	// A stored grant second ahead of the clock refuses with clock-behind.
	ahead := t.TempDir()
	st, err := eid.OpenFileStore(ahead)
	if err != nil {
		t.Fatal(err)
	}
	if !st.CompareAndSet(kan, 0, false, now/1000+100) {
		t.Fatal("store set")
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"lease", "kan", "--store", ahead}, e, &out, &errb); code != exitRefused || out.Len() != 0 ||
		errb.String() != "refused: clock-behind\n" {
		t.Fatalf("stored second ahead: exit %d stdout %q stderr %q", code, out.String(), errb.String())
	}
	fmt.Println("G13: a held activation gives `active` after --active-wait (90 ms polled, 0 ms not), then a lease once released; " +
		"a stored second ahead gives `clock-behind`")
}

func TestG13_LeaseFlagsBelongToLease(t *testing.T) {
	store := t.TempDir()
	e := testEnv(eid.NewManualClock(now), map[string]string{"EID_REGISTRY": registryDir(), "EID_HOME": "m5"})
	for _, args := range [][]string{
		{"mint", "kan", "doc", "--store", store, "--term", "5"},
		{"mint", "kan", "doc", "--store", store, "--active-wait", "5"},
		{"parse", "emq_job_23obzcbapvupu2272222", "--term", "5"},
		{"detached", "evt", "--active-wait", "0"},
	} {
		var out, errb bytes.Buffer
		if code := run(args, e, &out, &errb); code != exitUsage || out.Len() != 0 {
			t.Fatalf("%v: exit %d stdout %q", args, code, out.String())
		}
	}
	fmt.Println("G13: --term and --active-wait are refused as usage errors by every command but lease")
}
