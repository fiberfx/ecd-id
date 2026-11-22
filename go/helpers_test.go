package ecdid

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func root() string { return ".." }

// tsv returns the data rows of a vector file: comment lines dropped, cells split on tabs.
func tsv(t *testing.T, name string) [][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root(), "vectors", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var out [][]string
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, strings.Split(l, "\t"))
	}
	return out
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	if s == "-" {
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex %q: %v", s, err)
	}
	return b
}

func reason(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

// mix is splitmix64, a seeded generator for reproducible random ids.
type mix struct{ s uint64 }

func (m *mix) next() uint64 {
	m.s += 0x9E3779B97F4A7C15
	z := m.s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

func (m *mix) below(n uint64) uint64 { return m.next() % n }

func repoRegistry(t *testing.T) *Registry {
	t.Helper()
	s, err := os.ReadFile(filepath.Join(root(), "registry", "systems.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	k, err := os.ReadFile(filepath.Join(root(), "registry", "kinds.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := ParseRegistry(string(s), string(k))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}

func homedRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := ParseRegistry(
		"code\tname\towner\thome\tstate\tserved_by\n"+
			"emq\tEchoMQ\t-\th1\tlive\t-\n"+
			"kan\tKAN\t-\th1\tlive\t-\n"+
			"brd\tbrd14\t-\t-\tclosed\t-\n",
		"system\tkind\trole\tstate\tserved_by\tmeaning\n"+
			"emq\tevt\tstream\tlive\t-\tevent\n"+
			"emq\tjob\tcommand\tlive\t-\tjob\n"+
			"kan\tdoc\tentity\tlive\t-\tdocument\n")
	if err != nil {
		t.Fatalf("homed registry: %v", err)
	}
	return reg
}

func mustLease(t *testing.T, system string, writer uint32, nb, na uint64) Lease {
	t.Helper()
	l, err := NewLease(MustCode(system), writer, nb, na)
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	return l
}
