package vmanage

import (
	"testing"

	"go.osirisjson.org/consumers/internal/document"
)

func TestBuildReportModelOnRealFixture(t *testing.T) {
	g := fixture(t)
	m := BuildReportModel(g, document.Metadata{}, "deadbeef")

	if len(m.Routers) != 1 {
		t.Fatalf("len(Routers) = %d, want 1", len(m.Routers))
	}
	r := m.Routers[0]
	if r.Name != "TEST-ROUTER01" {
		t.Errorf("Routers[0].Name = %q, want TEST-ROUTER01", r.Name)
	}
	if r.ManagementIP != "192.0.2.10" {
		t.Errorf("Routers[0].ManagementIP = %q, want 192.0.2.10", r.ManagementIP)
	}
	if r.Health != "green" {
		t.Errorf("Routers[0].Health = %q, want green", r.Health)
	}

	// Known counts for this fixture, cross-checked against
	// internal/graph's own producer-fixture test.
	if m.Ports.Total != 16 {
		t.Errorf("Ports.Total = %d, want 16", m.Ports.Total)
	}
	if len(m.Ports.Rows) != 16 {
		t.Errorf("len(Ports.Rows) = %d, want 16", len(m.Ports.Rows))
	}
	if len(m.Sites) != 1 {
		t.Fatalf("len(Sites) = %d, want 1", len(m.Sites))
	}
	if m.Sites[0].Name != "OFFICE-01" {
		t.Errorf("Sites[0].Name = %q, want OFFICE-01", m.Sites[0].Name)
	}
	if m.Sites[0].Members != 1 {
		t.Errorf("Sites[0].Members = %d, want 1", m.Sites[0].Members)
	}

	if m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = true, want false (this fixture stays under the row threshold)")
	}
}

func TestBuildReportModelPortsResolveTheirOwnRouter(t *testing.T) {
	g := fixture(t)
	m := BuildReportModel(g, document.Metadata{}, "deadbeef")

	for _, p := range m.Ports.Rows {
		if p.RouterName != "TEST-ROUTER01" {
			t.Errorf("port %q RouterName = %q, want TEST-ROUTER01 (resolved from the document's own contains connection)",
				p.Name, p.RouterName)
		}
	}
}

func TestBuildReportModelPortStatusBreakdown(t *testing.T) {
	g := fixture(t)
	m := BuildReportModel(g, document.Metadata{}, "deadbeef")

	byStatus := map[string]int{}
	for _, kv := range m.Ports.ByStatus {
		byStatus[kv.Key] = kv.Count
	}
	if byStatus["active"] != 14 {
		t.Errorf("ByStatus[active] = %d, want 14", byStatus["active"])
	}
	if byStatus["degraded"] != 2 {
		t.Errorf("ByStatus[degraded] = %d, want 2", byStatus["degraded"])
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
