// Command ecdid parses, validates and mints Ecd IDs, version 1 (spec/cli.md). It uses the standard library and the
// package ecdid only.
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

	"ecdid"
)

const usage = `usage:
  ecdid parse ID
  ecdid valid ID
  ecdid mint SYSTEM KIND [-n N] --store DIR [--home NAME] [--registry DIR]
  ecdid mint SYSTEM KIND [-n N] --lease LEASE [--registry DIR]
  ecdid detached KIND [-n N]
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
	clock    ecdid.Clock
	random   ecdid.RandomSource
	pids     ecdid.PidSource
	sleep    func(time.Duration)
}

func main() {
	e := env{getenv: os.Getenv, hostname: os.Hostname, clock: ecdid.SystemClock{}, random: ecdid.OsRandom{},
		pids: ecdid.ProcessPid{}, sleep: time.Sleep}
	os.Exit(run(os.Args[1:], e, os.Stdout, os.Stderr))
}

type usageError string

func (u usageError) Error() string { return string(u) }

// run executes one command line. Standard output carries the result and only on success; a refusal is one line,
// "refused: REASON", on standard error.
func run(args []string, e env, stdout, stderr io.Writer) int {
	out, err := dispatch(args, e)
	var refusal ecdid.Refusal
	var bad usageError
	switch {
	case err == nil:
		io.WriteString(stdout, out)
		return exitOK
	case errors.As(err, &bad):
		fmt.Fprintf(stderr, "ecdid: %s\n%s", bad, usage)
		return exitUsage
	case errors.As(err, &refusal):
		fmt.Fprintf(stderr, "refused: %s\n", refusal)
		return exitRefused
	default:
		fmt.Fprintf(stderr, "ecdid: %v\n", err)
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
		id, err := ecdid.ParseString(o.pos[0])
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
func describe(id ecdid.ID) string {
	return fmt.Sprintf(`{"text":"%s","hex":"%x","system":"%s","kind":"%s","time":%d,"writer":%d,"count":%d,"class":"%s","stream":"%s"}`+"\n",
		id, id[:], id.System(), id.Kind(), id.Time(), id.Writer(), id.Count(), id.Class(), id.StreamID())
}

func parseKind(s string) (ecdid.Code, error) {
	k, ok := ecdid.ParseCode(s)
	if !ok || !k.IsKind() {
		return 0, ecdid.ErrKind
	}
	return k, nil
}

func loadRegistry(o options, e env) (*ecdid.Registry, error) {
	dir := o.registry
	if dir == "" {
		dir = e.getenv("ECD_ID_REGISTRY")
	}
	if dir == "" {
		return nil, errors.New("no registry: give --registry DIR or set ECD_ID_REGISTRY")
	}
	systems, err := os.ReadFile(filepath.Join(dir, "systems.tsv"))
	if err != nil {
		return nil, err
	}
	kinds, err := os.ReadFile(filepath.Join(dir, "kinds.tsv"))
	if err != nil {
		return nil, err
	}
	return ecdid.ParseRegistry(string(systems), string(kinds))
}

func mint(o options, e env) (string, error) {
	kind, err := parseKind(o.pos[1])
	if err != nil {
		return "", err
	}
	system, ok := ecdid.ParseCode(o.pos[0])
	if !ok {
		return "", usageError(o.pos[0] + " is not a system code")
	}
	switch system.Class() {
	case ecdid.Reserved:
		return "", ecdid.ErrSystemReserved
	case ecdid.Detached:
		if o.store != "" {
			return "", ecdid.ErrClass
		}
	case ecdid.Registered:
		reg, err := loadRegistry(o, e)
		if err != nil {
			return "", err
		}
		row, ok := reg.System(system)
		switch {
		case !ok:
			return "", ecdid.ErrUnknown
		case row.State != ecdid.Live:
			return "", ecdid.ErrState
		}
		if k, ok := reg.Kind(system, kind); !ok || k.State != ecdid.Live {
			return "", ecdid.ErrKind
		}
		if o.store != "" {
			return mintStored(o, e, reg, system, kind)
		}
	}
	l, err := ecdid.ParseLease(o.lease)
	if err != nil {
		return "", err
	}
	if l.System != system {
		return "", ecdid.ErrLeaseSystem
	}
	return mintIn(l, kind, o.count, e)
}

// mintIn mints n ids of kind inside lease l.
func mintIn(l ecdid.Lease, kind ecdid.Code, n int, e env) (string, error) {
	m := ecdid.NewMinter(l, e.clock, e.pids)
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
	if h := e.getenv("ECD_ID_HOME"); h != "" {
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
func mintStored(o options, e env, reg *ecdid.Registry, system, kind ecdid.Code) (string, error) {
	home, err := homeOf(o, e)
	if err != nil {
		return "", err
	}
	st, err := ecdid.OpenFileStore(o.store)
	if err != nil {
		return "", err
	}
	offset, err := ecdid.RandomOffset(e.random)
	if err != nil {
		return "", err
	}
	var a *ecdid.Allocator
	for try := 0; ; try++ {
		a, err = ecdid.StartAllocator(reg, system, home, st, e.clock, offset)
		if err != ecdid.ErrActive || try == activeTries {
			break
		}
		e.sleep(activeWaitMs * time.Millisecond)
	}
	if err != nil {
		return "", storeError(err, st)
	}
	defer st.Release(system)
	var l ecdid.Lease
	for try := 0; ; try++ {
		l, err = a.Grant(leaseTermMs)
		if err != ecdid.ErrWait || try == waitTries {
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
func storeError(err error, st *ecdid.FileStore) error {
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
	l, err := ecdid.DetachedLease(e.random, e.clock, nil)
	if err != nil {
		return "", err
	}
	return mintIn(l, kind, o.count, e)
}
