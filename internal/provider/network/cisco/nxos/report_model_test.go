package nxos

import (
	"testing"

	"go.osirisjson.org/consumers/internal/document"
)

func TestBuildReportModelOnRealFixture(t *testing.T) {
	g := fixture(t)
	m := BuildReportModel(g, document.Metadata{}, "deadbeef")

	if m.Switch.Name != "TEST-MXP-SW23" {
		t.Errorf("Switch.Name = %q, want TEST-MXP-SW23", m.Switch.Name)
	}
	if m.Switch.ChassisProductID != "N9K-C93180YC-FX" {
		t.Errorf("Switch.ChassisProductID = %q, want N9K-C93180YC-FX", m.Switch.ChassisProductID)
	}
	if m.Switch.ManagementIP != "192.0.2.22" {
		t.Errorf("Switch.ManagementIP = %q, want 192.0.2.22", m.Switch.ManagementIP)
	}

	// Known counts for this fixture, cross-checked against
	// internal/graph's own producer-fixture test.
	if m.Ports.Total != 55 {
		t.Errorf("Ports.Total = %d, want 55", m.Ports.Total)
	}
	if len(m.Ports.Rows) != 55 {
		t.Errorf("len(Ports.Rows) = %d, want 55", len(m.Ports.Rows))
	}
	if m.Interfaces.Total != 37 {
		t.Errorf("Interfaces.Total = %d, want 37", m.Interfaces.Total)
	}
	if len(m.LAGs) != 3 {
		t.Errorf("len(LAGs) = %d, want 3", len(m.LAGs))
	}
	if m.VLANs.Total != 31 {
		t.Errorf("VLANs.Total = %d, want 31", m.VLANs.Total)
	}
	if len(m.VRFs) != 4 {
		t.Errorf("len(VRFs) = %d, want 4", len(m.VRFs))
	}
	if m.Neighbours.Total != 15 {
		t.Errorf("Neighbours.Total = %d, want 15", m.Neighbours.Total)
	}

	if !m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = false, want true (Ports.Rows exceeds the threshold)")
	}
}

func TestBuildReportModelLAGMembersResolveFromContainsEdges(t *testing.T) {
	g := fixture(t)
	m := BuildReportModel(g, document.Metadata{}, "deadbeef")

	var found bool
	for _, l := range m.LAGs {
		if l.Name == "port-channel10" {
			found = true
			if len(l.Members) == 0 {
				t.Error("port-channel10 has no members, want at least one resolved from a contains connection")
			}
		}
	}
	if !found {
		t.Fatal("fixture sanity: expected a port-channel10 LAG in this fixture")
	}
}

func TestInt64OrAbsent(t *testing.T) {
	if got := int64OrAbsent(nil); got != "(absent)" {
		t.Errorf("int64OrAbsent(nil) = %q, want (absent)", got)
	}
	var zero int64
	if got := int64OrAbsent(&zero); got != "0" {
		t.Errorf("int64OrAbsent(&0) = %q, want \"0\" (a real zero must not read as absent)", got)
	}
}

func TestStringOrAbsentAndLabelOr(t *testing.T) {
	if got := stringOrAbsent(""); got != "(absent)" {
		t.Errorf("stringOrAbsent(\"\") = %q, want (absent)", got)
	}
	if got := stringOrAbsent("x"); got != "x" {
		t.Errorf("stringOrAbsent(x) = %q, want x", got)
	}
	if got := labelOr("", "id-1"); got != "id-1" {
		t.Errorf("labelOr(\"\", id-1) = %q, want id-1", got)
	}
	if got := labelOr("name", "id-1"); got != "name" {
		t.Errorf("labelOr(name, id-1) = %q, want name", got)
	}
}
