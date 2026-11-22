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
	"time"

	"ecdid"
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

// testEnv is an environment with a manual clock whose sleep moves the clock, so a wait costs no time.
func testEnv(clock *ecdid.ManualClock, vars map[string]string) env {
	return env{
		getenv:   func(k string) string { return vars[k] },
		hostname: func() (string, error) { return "box.local", nil },
		clock:    clock,
		random:   ecdid.OsRandom{},
		pids:     ecdid.NewManualPid(1),
		sleep:    func(d time.Duration) { clock.Advance(uint64(d / time.Millisecond)) },
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
	e := testEnv(ecdid.NewManualClock(now), map[string]string{"ECD_ID_REGISTRY": registryDir()})
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

// The same rows against a built binary, when ECDID_BIN names one.
func TestG13_BinaryAgainstVectors(t *testing.T) {
	bin := os.Getenv("ECDID_BIN")
	if bin == "" {
		t.Skip("ECDID_BIN names the binary to check")
	}
	rows := cliVectors(t)
	for _, r := range rows {
		cmd := exec.Command(bin, vectorArgs(r)...)
		cmd.Env = append(os.Environ(), "ECD_ID_REGISTRY="+registryDir())
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

func idsOf(t *testing.T, out string) []ecdid.ID {
	t.Helper()
	var ids []ecdid.ID
	for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		id, err := ecdid.ParseString(l)
		if err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestG13_MintFromAStoreTakesALeasePerCall(t *testing.T) {
	store := t.TempDir()
	clock := ecdid.NewManualClock(now)
	e := testEnv(clock, map[string]string{"ECD_ID_REGISTRY": registryDir(), "ECD_ID_HOME": "m5"})
	kan := ecdid.MustCode("kan")
	var all []ecdid.ID
	writers := map[uint32]bool{}
	for call := 0; call < 3; call++ {
		ids := idsOf(t, mustRun(t, e, "mint", "kan", "doc", "-n", "50", "--store", store))
		if len(ids) != 50 {
			t.Fatalf("call %d: %d ids", call, len(ids))
		}
		for _, id := range ids {
			if id.System() != kan || id.Kind() != ecdid.MustCode("doc") {
				t.Fatalf("call %d: %s", call, id)
			}
		}
		writers[ids[0].Writer()] = true
		all = append(all, ids...)
	}
	seen := map[ecdid.ID]bool{}
	for i, id := range all {
		if seen[id] || (i > 0 && id.Compare(all[i-1]) <= 0) {
			t.Fatalf("id %d (%s) repeats or goes back", i, id)
		}
		seen[id] = true
	}
	if len(writers) != 3 {
		t.Fatalf("%d writers for 3 calls", len(writers))
	}
	fmt.Printf("G13: 3 calls on one store (2 of them inside one grant second) mint %d distinct ids, rising, on 3 writers\n",
		len(all))
}

func TestG13_MintRefusals(t *testing.T) {
	store := t.TempDir()
	clock := ecdid.NewManualClock(now)
	vars := map[string]string{"ECD_ID_REGISTRY": registryDir(), "ECD_ID_HOME": "m5"}
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
	holder, err := ecdid.OpenFileStore(store)
	if err != nil {
		t.Fatal(err)
	}
	if !holder.Activate(ecdid.MustCode("kan")) {
		t.Fatal("holder activation")
	}
	var out, errb bytes.Buffer
	if code := run([]string{"mint", "kan", "doc", "--store", store}, e, &out, &errb); code != exitRefused ||
		errb.String() != "refused: active\n" {
		t.Fatalf("held store: exit %d stderr %q", code, errb.String())
	}
	holder.Release(ecdid.MustCode("kan"))
	mustRun(t, e, "mint", "kan", "doc", "--store", store)
	fmt.Printf("G13: %d refusals before or at the store, and `active` while another holder keeps the activation\n",
		len(cases)+1)
}

func TestG13_DetachedMintsDetachedIDs(t *testing.T) {
	e := testEnv(ecdid.NewManualClock(now), nil)
	ids := idsOf(t, mustRun(t, e, "detached", "evt", "-n", "20"))
	for i, id := range ids {
		if id.Class() != ecdid.Detached || id.Kind() != ecdid.MustCode("evt") || id.System() != ids[0].System() ||
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
