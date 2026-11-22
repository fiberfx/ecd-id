package ecdid

import (
	"sort"
	"strconv"
	"strings"
)

const (
	systemsHeader = "code\tname\towner\thome\tstate\tserved_by"
	kindsHeader   = "system\tkind\trole\tstate\tserved_by\tmeaning"
)

// State is the state of a system or a kind.
type State int

// The states.
const (
	Live State = iota
	Closed
	Retired
)

// Role is the role of a kind; a typed field checks it.
type Role string

var roles = map[string]bool{
	"entity": true, "activation": true, "command": true, "run": true, "stream": true,
	"store": true, "database": true, "transaction": true, "correlation": true, "request": true,
}

// SystemRow is a row of registry/systems.tsv. ServedBy is nil when the row names none.
type SystemRow struct {
	Code     Code
	Name     string
	Owner    string
	Home     string
	State    State
	ServedBy *Code
}

// KindRow is a row of registry/kinds.tsv.
type KindRow struct {
	System   Code
	Kind     Code
	Role     Role
	State    State
	ServedBy *Code
	Meaning  string
}

// RegistryError is a refusal to load a registry file, with the 1-based line number in that file.
type RegistryError struct {
	Reason string
	Line   int
}

func (e *RegistryError) Error() string { return e.Reason + " at line " + strconv.Itoa(e.Line) }

// Registry holds both registries, loaded and checked.
type Registry struct {
	systems map[Code]SystemRow
	kinds   map[[2]Code]KindRow
}

type tsvRow struct {
	line  int
	cells []string
}

func registryRows(text, header string, width int) ([]tsvRow, error) {
	var out []tsvRow
	seen := false
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, line := range lines {
		n := i + 1
		if strings.HasPrefix(line, "#") {
			continue
		}
		if !seen {
			if line != header {
				return nil, &RegistryError{"registry-header", n}
			}
			seen = true
			continue
		}
		cells := strings.Split(line, "\t")
		if len(cells) != width {
			return nil, &RegistryError{"registry-row", n}
		}
		for _, c := range cells {
			if c == "" {
				return nil, &RegistryError{"registry-row", n}
			}
		}
		out = append(out, tsvRow{n, cells})
	}
	if !seen {
		return nil, &RegistryError{"registry-header", 1}
	}
	return out, nil
}

func registryState(s string, line int) (State, error) {
	switch s {
	case "live":
		return Live, nil
	case "closed":
		return Closed, nil
	case "retired":
		return Retired, nil
	}
	return 0, &RegistryError{"registry-state", line}
}

func servedBy(s string) (*Code, bool) {
	if s == "-" {
		return nil, true
	}
	c, ok := ParseCode(s)
	if !ok {
		return nil, false
	}
	return &c, true
}

// ParseRegistry loads the text of systems.tsv and kinds.tsv.
func ParseRegistry(systems, kinds string) (*Registry, error) {
	reg := &Registry{systems: map[Code]SystemRow{}, kinds: map[[2]Code]KindRow{}}
	srows, err := registryRows(systems, systemsHeader, 6)
	if err != nil {
		return nil, err
	}
	type pending struct {
		line int
		code Code
	}
	var served []pending
	for _, r := range srows {
		c := r.cells
		code, ok := ParseCode(c[0])
		if !ok || code.Class() != Registered {
			return nil, &RegistryError{"registry-code", r.line}
		}
		st, err := registryState(c[4], r.line)
		if err != nil {
			return nil, err
		}
		sb, ok := servedBy(c[5])
		if !ok {
			return nil, &RegistryError{"registry-served-by", r.line}
		}
		if sb != nil {
			served = append(served, pending{r.line, *sb})
		}
		if _, dup := reg.systems[code]; dup {
			return nil, &RegistryError{"registry-duplicate", r.line}
		}
		reg.systems[code] = SystemRow{code, c[1], c[2], c[3], st, sb}
	}
	for _, p := range served {
		if _, ok := reg.systems[p.code]; !ok {
			return nil, &RegistryError{"registry-served-by", p.line}
		}
	}
	krows, err := registryRows(kinds, kindsHeader, 6)
	if err != nil {
		return nil, err
	}
	for _, r := range krows {
		c := r.cells
		system, ok := ParseCode(c[0])
		if !ok {
			return nil, &RegistryError{"registry-code", r.line}
		}
		if _, ok := reg.systems[system]; !ok {
			return nil, &RegistryError{"registry-system", r.line}
		}
		kind, ok := ParseCode(c[1])
		if !ok || !kind.IsKind() || kind == Sys {
			return nil, &RegistryError{"registry-code", r.line}
		}
		if !roles[c[2]] {
			return nil, &RegistryError{"registry-role", r.line}
		}
		st, err := registryState(c[3], r.line)
		if err != nil {
			return nil, err
		}
		sb, ok := servedBy(c[4])
		if !ok {
			return nil, &RegistryError{"registry-served-by", r.line}
		}
		if sb != nil {
			if _, ok := reg.systems[*sb]; !ok {
				return nil, &RegistryError{"registry-served-by", r.line}
			}
		}
		key := [2]Code{system, kind}
		if _, dup := reg.kinds[key]; dup {
			return nil, &RegistryError{"registry-duplicate", r.line}
		}
		reg.kinds[key] = KindRow{system, kind, Role(c[2]), st, sb, c[5]}
	}
	return reg, nil
}

// System returns the row of a system.
func (r *Registry) System(code Code) (SystemRow, bool) {
	row, ok := r.systems[code]
	return row, ok
}

// Kind returns the row of a kind of a system.
func (r *Registry) Kind(system, kind Code) (KindRow, bool) {
	row, ok := r.kinds[[2]Code{system, kind}]
	return row, ok
}

// Systems returns all systems in code order.
func (r *Registry) Systems() []SystemRow {
	out := make([]SystemRow, 0, len(r.systems))
	for _, row := range r.systems {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// KindsOf returns the kinds of one system in code order.
func (r *Registry) KindsOf(system Code) []KindRow {
	var out []KindRow
	for k, row := range r.kinds {
		if k[0] == system {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}
