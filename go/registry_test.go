package eid

import (
	"strings"
	"testing"
)

// allRoles holds the ten role constants in the order of spec/registry.md, beside the text each must carry.
var allRoles = []struct {
	role Role
	text string
}{
	{RoleEntity, "entity"}, {RoleActivation, "activation"}, {RoleCommand, "command"}, {RoleRun, "run"},
	{RoleStream, "stream"}, {RoleStore, "store"}, {RoleDatabase, "database"}, {RoleTransaction, "transaction"},
	{RoleCorrelation, "correlation"}, {RoleRequest, "request"},
}

const rolesSystems = "code\tname\towner\thome\tstate\tserved_by\n" +
	"emq\tEchoMQ\t-\th1\tlive\t-\n" +
	"kan\tKAN\t-\th1\tlive\t-\n"

// TestRegistryRoles: each constant carries the text of the spec, a row naming it loads with that Role, and a role
// outside the list (the sys kind's own role system, or an upper-case entity) is refused with registry-role.
func TestRegistryRoles(t *testing.T) {
	var k strings.Builder
	k.WriteString("system\tkind\trole\tstate\tserved_by\tmeaning\n")
	kinds := []string{"aaa", "bbb", "ccc", "ddd", "eee", "fff", "ggg", "hhh", "iii", "jjj"}
	for i, r := range allRoles {
		if string(r.role) != r.text {
			t.Errorf("role %d is %q, want %q", i, r.role, r.text)
		}
		k.WriteString("kan\t" + kinds[i] + "\t" + r.text + "\tlive\t-\tm\n")
	}
	reg, err := ParseRegistry(rolesSystems, k.String())
	if err != nil {
		t.Fatalf("ten roles: %v", err)
	}
	for i, r := range allRoles {
		row, ok := reg.Kind(MustCode("kan"), MustCode(kinds[i]))
		if !ok || row.Role != r.role {
			t.Errorf("kan %s: role %q (found %v), want %q", kinds[i], row.Role, ok, r.role)
		}
	}
	for _, bad := range []string{"system", "Entity"} {
		_, err := ParseRegistry(rolesSystems, "system\tkind\trole\tstate\tserved_by\tmeaning\nkan\taaa\t"+bad+"\tlive\t-\tm\n")
		if re, ok := err.(*RegistryError); !ok || re.Reason != "registry-role" {
			t.Errorf("role %q: %v, want registry-role", bad, err)
		}
	}
}

// TestRegistryKindsByRole: the kinds of one system with one role, in code order, in every state; none of another
// system, none for a role outside the list, none for a system without a row.
func TestRegistryKindsByRole(t *testing.T) {
	reg, err := ParseRegistry(rolesSystems,
		"system\tkind\trole\tstate\tserved_by\tmeaning\n"+
			"emq\tevt\tstream\tlive\t-\tevent\n"+
			"emq\tjob\tcommand\tlive\t-\tjob\n"+
			"emq\tobj\tentity\tlive\t-\tobject\n"+
			"kan\tdoc\tentity\tlive\t-\tdocument\n"+
			"kan\told\tentity\tretired\t-\told\n"+
			"kan\taus\tentity\tlive\t-\tstory\n"+
			"kan\tcls\tentity\tclosed\t-\tclosed\n"+
			"kan\tses\trun\tlive\t-\tsession\n")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	kan, emq := MustCode("kan"), MustCode("emq")
	names := func(rows []KindRow) string {
		var s []string
		for _, r := range rows {
			s = append(s, r.Kind.String())
		}
		return strings.Join(s, " ")
	}
	cases := []struct {
		name   string
		system Code
		role   Role
		want   string
	}{
		{"entity kinds of kan, every state, code order", kan, RoleEntity, "aus cls doc old"},
		{"run kinds of kan", kan, RoleRun, "ses"},
		{"entity kinds of emq only", emq, RoleEntity, "obj"},
		{"no command kind in kan, though emq has one", kan, RoleCommand, ""},
		{"a role outside the list", kan, Role("thing"), ""},
		{"a system without a row", MustCode("zzz"), RoleEntity, ""},
	}
	for _, c := range cases {
		if got := names(reg.KindsByRole(c.system, c.role)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	for _, row := range reg.KindsByRole(kan, RoleEntity) {
		if row.System != kan || row.Role != RoleEntity {
			t.Errorf("row %v of KindsByRole(kan, entity)", row)
		}
	}
}

// TestRegistryKindsByRoleCoversTheFiles: over registry/, the ten roles split the kinds of every system, so each kind
// row is returned for exactly one role.
func TestRegistryKindsByRoleCoversTheFiles(t *testing.T) {
	reg := repoRegistry(t)
	for _, s := range reg.Systems() {
		n := 0
		for _, r := range allRoles {
			n += len(reg.KindsByRole(s.Code, r.role))
		}
		if all := len(reg.KindsOf(s.Code)); n != all {
			t.Errorf("system %s: the ten roles return %d kinds, KindsOf %d", s.Code, n, all)
		}
	}
}
