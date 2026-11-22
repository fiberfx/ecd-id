// Command eid parses, validates and mints Ecd IDs, version 1 (spec/cli.md). It uses the standard library and the
// package eid only.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"eid"
)

const usage = `usage:
  eid parse ID
  eid valid ID
  eid mint SYSTEM KIND [-n N] --store DIR [--home NAME] [--registry DIR]
  eid mint SYSTEM KIND [-n N] --lease LEASE [--registry DIR]
  eid detached KIND [-n N]
`

// Exit statuses.
const (
	exitOK      = 0
	exitRefused = 1 // a refusal, named by its reason on standard error
	exitUsage   = 2
	exitFailed  = 3 // the registry or the store could not be read or written
)

const (
	maxCount     = 1_000_000 // the most ids one call mints
	leaseTermMs  = 3_600_000 // the term of a lease taken from a store
	activeTries  = 100       // attempts to take the activation while another process holds it
	activeWaitMs = 30        // milliseconds between those attempts
	waitTries    = 2         // attempts to wait out the grant second of an earlier run
)

// env is what the command reads from outside: the environment, the host, the clock, the random source, the process
// id and the sleep that waits for the next second.
type env struct {
	getenv   func(string) string
	hostname func() (string, error)
	clock    eid.Clock
	random   eid.RandomSource
	pids     eid.PidSource
	sleep    func(time.Duration)
}

func main() {
	e := env{getenv: os.Getenv, hostname: os.Hostname, clock: eid.SystemClock{}, random: eid.OsRandom{},
		pids: eid.ProcessPid{}, sleep: time.Sleep}
	os.Exit(run(os.Args[1:], e, os.Stdout, os.Stderr))
}

type usageError string

func (u usageError) Error() string { return string(u) }

// run executes one command line. Standard output carries the result and only on success; a refusal is one line,
// "refused: REASON", on standard error.
func run(args []string, e env, stdout, stderr io.Writer) int {
	out, err := dispatch(args, e)
	var refusal eid.Refusal
	var bad usageError
	switch {
	case err == nil:
		io.WriteString(stdout, out)
		return exitOK
	case errors.As(err, &bad):
		fmt.Fprintf(stderr, "eid: %s\n%s", bad, usage)
		return exitUsage
	case errors.As(err, &refusal):
		fmt.Fprintf(stderr, "refused: %s\n", refusal)
		return exitRefused
	default:
		fmt.Fprintf(stderr, "eid: %v\n", err)
		return exitFailed
	}
}

type options struct {
	pos                          []string
	count                        int
	store, home, registry, lease string
}

func parseOptions(args []string) (options, error) {
	o := options{count: 1}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			o.pos = append(o.pos, a)
			continue
		}
		if i+1 >= len(args) {
			return o, usageError(a + " needs a value")
		}
		i++
		v := args[i]
		switch a {
		case "-n":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > maxCount {
				return o, usageError("-n is a count from 1 to " + strconv.Itoa(maxCount))
			}
			o.count = n
		case "--store":
			o.store = v
		case "--home":
			o.home = v
		case "--registry":
			o.registry = v
		case "--lease":
			o.lease = v
		default:
			return o, usageError("unknown flag " + a)
		}
	}
	return o, nil
}

func dispatch(args []string, e env) (string, error) {
	if len(args) == 0 {
		return "", usageError("no command")
	}
	o, err := parseOptions(args[1:])
	if err != nil {
		return "", err
	}
	switch args[0] {
	case "parse", "valid":
		if len(o.pos) != 1 || o.count != 1 || o.store != "" || o.home != "" || o.registry != "" || o.lease != "" {
			return "", usageError(args[0] + " takes one ID")
		}
		id, err := eid.ParseString(o.pos[0])
		if err != nil {
			return "", err
		}
		if args[0] == "valid" {
			return "ok\n", nil
		}
		return describe(id), nil
	case "mint":
		if len(o.pos) != 2 {
			return "", usageError("mint takes a system and a kind")
		}
		if (o.store == "") == (o.lease == "") {
			return "", usageError("mint takes one of --store and --lease")
		}
		return mint(o, e)
	case "detached":
		if len(o.pos) != 1 || o.store != "" || o.home != "" || o.registry != "" || o.lease != "" {
			return "", usageError("detached takes a kind")
		}
		return detached(o, e)
	}
	return "", usageError("unknown command " + args[0])
}

// describe is the JSON line of the fields of id.
func describe(id eid.ID) string {
	return fmt.Sprintf(`{"text":"%s","hex":"%x","system":"%s","kind":"%s","time":%d,"writer":%d,"count":%d,"class":"%s","stream":"%s"}`+"\n",
		id, id[:], id.System(), id.Kind(), id.Time(), id.Writer(), id.Count(), id.Class(), id.StreamID())
}

func parseKind(s string) (eid.Code, error) {
	k, ok := eid.ParseCode(s)
	if !ok || !k.IsKind() {
		return 0, eid.ErrKind
	}
	return k, nil
}

func loadRegistry(o options, e env) (*eid.Registry, error) {
	dir := o.registry
	if dir == "" {
		dir = e.getenv("EID_REGISTRY")
	}
	if dir == "" {
		return nil, errors.New("no registry: give --registry DIR or set EID_REGISTRY")
	}
	systems, err := os.ReadFile(filepath.Join(dir, "systems.tsv"))
	if err != nil {
		return nil, err
	}
	kinds, err := os.ReadFile(filepath.Join(dir, "kinds.tsv"))
	if err != nil {
		return nil, err
	}
	return eid.ParseRegistry(string(systems), string(kinds))
}

func mint(o options, e env) (string, error) {
	kind, err := parseKind(o.pos[1])
	if err != nil {
		return "", err
	}
	system, ok := eid.ParseCode(o.pos[0])
	if !ok {
		return "", usageError(o.pos[0] + " is not a system code")
	}
	switch system.Class() {
	case eid.Reserved:
		return "", eid.ErrSystemReserved
	case eid.Detached:
		if o.store != "" {
			return "", eid.ErrClass
		}
	case eid.Registered:
		reg, err := loadRegistry(o, e)
		if err != nil {
			return "", err
		}
		row, ok := reg.System(system)
		switch {
		case !ok:
			return "", eid.ErrUnknown
		case row.State != eid.Live:
			return "", eid.ErrState
		}
		if k, ok := reg.Kind(system, kind); !ok || k.State != eid.Live {
			return "", eid.ErrKind
		}
		if o.store != "" {
			return mintStored(o, e, reg, system, kind)
		}
	}
	l, err := eid.ParseLease(o.lease)
	if err != nil {
		return "", err
	}
	if l.System != system {
		return "", eid.ErrLeaseSystem
	}
	return mintIn(l, kind, o.count, e)
}

// mintIn mints n ids of kind inside lease l.
func mintIn(l eid.Lease, kind eid.Code, n int, e env) (string, error) {
	m := eid.NewMinter(l, e.clock, e.pids)
	var b strings.Builder
	for i := 0; i < n; i++ {
		id, err := m.Mint(kind)
		if err != nil {
			return "", err
		}
		b.WriteString(id.String())
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func homeOf(o options, e env) (string, error) {
	if o.home != "" {
		return o.home, nil
	}
	if h := e.getenv("EID_HOME"); h != "" {
		return h, nil
	}
	h, err := e.hostname()
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.SplitN(h, ".", 2)[0]), nil
}

// mintStored takes a lease from the store in o.store, as the allocator of system, and mints n ids inside it. Two
// calls that overlap take the activation in turn; a call that starts in the second of the previous grant waits for
// the next second.
func mintStored(o options, e env, reg *eid.Registry, system, kind eid.Code) (string, error) {
	home, err := homeOf(o, e)
	if err != nil {
		return "", err
	}
	st, err := eid.OpenFileStore(o.store)
	if err != nil {
		return "", err
	}
	offset, err := eid.RandomOffset(e.random)
	if err != nil {
		return "", err
	}
	var a *eid.Allocator
	for try := 0; ; try++ {
		a, err = eid.StartAllocator(reg, system, home, st, e.clock, offset)
		if err != eid.ErrActive || try == activeTries {
			break
		}
		e.sleep(activeWaitMs * time.Millisecond)
	}
	if err != nil {
		return "", storeError(err, st)
	}
	defer st.Release(system)
	var l eid.Lease
	for try := 0; ; try++ {
		l, err = a.Grant(leaseTermMs)
		if err != eid.ErrWait || try == waitTries {
			break
		}
		e.sleep(time.Duration(1000-e.clock.NowMs()%1000+1) * time.Millisecond)
	}
	if err != nil {
		return "", storeError(err, st)
	}
	return mintIn(l, kind, o.count, e)
}

// storeError names a failure of the store's own input or output instead of the refusal that follows from it.
func storeError(err error, st *eid.FileStore) error {
	if ioErr := st.Err(); ioErr != nil {
		return fmt.Errorf("store %s: %w", st.Dir(), ioErr)
	}
	return err
}

func detached(o options, e env) (string, error) {
	kind, err := parseKind(o.pos[0])
	if err != nil {
		return "", err
	}
	l, err := eid.DetachedLease(e.random, e.clock, nil)
	if err != nil {
		return "", err
	}
	return mintIn(l, kind, o.count, e)
}
