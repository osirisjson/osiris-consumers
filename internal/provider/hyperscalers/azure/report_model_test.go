package azure

import (
	"testing"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

func loadGraph(t *testing.T, rel string) (*document.Document, *graph.Graph) {
	t.Helper()
	doc, g, _ := loadFixture(t, rel)
	return doc, g
}

func TestBuildReportModelCategoriesAndHierarchy(t *testing.T) {
	doc, g := loadGraph(t, azureFixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())

	if len(m.Other.Rows) != 0 {
		t.Errorf("fixture sanity: every resource type in this fixture is a confirmed category; got %d unrecognized rows: %+v", len(m.Other.Rows), m.Other.Rows)
	}

	byLabel := make(map[string]int, len(m.Categories))
	for _, c := range m.Categories {
		byLabel[c.Label] = len(c.Rows)
	}
	want := map[string]int{
		"Virtual Machines":        1,
		"Storage Accounts":        1,
		"Disks":                   1,
		"Virtual Networks":        1,
		"Subnets":                 1,
		"Network Security Groups": 1,
		"Public IP Addresses":     1,
		"Load Balancers":          1,
		"Network Interfaces":      1,
		"Bastion Hosts":           1,
		"SQL Virtual Machines":    1,
	}
	for label, count := range want {
		if got := byLabel[label]; got != count {
			t.Errorf("category %q has %d rows, want %d", label, got, count)
		}
	}

	if len(m.ResourceGroups) != 2 {
		t.Fatalf("expected 2 resource groups (populated + the deliberately empty one), got %d", len(m.ResourceGroups))
	}
	var populated, empty *ResourceGroupRow
	for i := range m.ResourceGroups {
		rg := &m.ResourceGroups[i]
		switch rg.Name {
		case "FACTORY-00-rgp":
			populated = rg
		case "FACTORY-00-rgp-empty":
			empty = rg
		}
	}
	if populated == nil || populated.ResourceCount != 12 {
		t.Errorf("populated resource group count = %+v, want ResourceCount 12", populated)
	}
	if empty == nil || empty.ResourceCount != 1 {
		t.Errorf("empty resource group count = %+v, want ResourceCount 1 (its own self-entry)", empty)
	}

	if len(m.Hierarchy) != 2 {
		t.Fatalf("expected 2 hierarchy rows (1 management group + 1 subscription), got %d: %+v", len(m.Hierarchy), m.Hierarchy)
	}
}

func TestBuildReportModelSignature(t *testing.T) {
	doc, g := loadGraph(t, azureFixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	s := m.Signature
	if s.InputSHA256 == "" {
		t.Error("InputSHA256 must not be empty")
	}
	if s.GeneratorName != "osirisjson-producer-azure" {
		t.Errorf("GeneratorName = %q", s.GeneratorName)
	}
	if s.ScopeName != "00000000-0000-0000-0000-000000000000 - FACTORY-00-sub" {
		t.Errorf("ScopeName = %q", s.ScopeName)
	}
	if s.ScopePurpose != "documentation" {
		t.Errorf("ScopePurpose = %q", s.ScopePurpose)
	}
}

func TestNeedsXLSXCompanionThreshold(t *testing.T) {
	small := ReportModel{Categories: []ResourceCategory{{Label: "x", Rows: make([]ResourceRow, azureXLSXRowThreshold)}}}
	if small.NeedsXLSXCompanion() {
		t.Error("exactly at threshold must not trigger the companion")
	}
	big := ReportModel{Categories: []ResourceCategory{{Label: "x", Rows: make([]ResourceRow, azureXLSXRowThreshold+1)}}}
	if !big.NeedsXLSXCompanion() {
		t.Error("one row over threshold must trigger the companion")
	}
	bigRG := ReportModel{ResourceGroups: make([]ResourceGroupRow, azureXLSXRowThreshold+1)}
	if !bigRG.NeedsXLSXCompanion() {
		t.Error("resource groups over threshold must also trigger the companion")
	}
	bigOther := ReportModel{Other: ResourceCategory{Rows: make([]ResourceRow, azureXLSXRowThreshold+1)}}
	if !bigOther.NeedsXLSXCompanion() {
		t.Error("'Other resources over threshold must also trigger the companion")
	}
}

// TestBuildReportModelDoesNotHideAnythingFromTopologyPage proves the
// report is a complete inventory, unlike the curated topology page:
// storage.volume (hidden from the topology) and the resource group's
// own container.resourcegroup self-entry (also hidden there) both show
// up here the disk in its own category, the self-entry folded into
// the resource-group summary's own ResourceCount rather than dropped.
func TestBuildReportModelDoesNotHideAnythingFromTopologyPage(t *testing.T) {
	doc, g := loadGraph(t, azureFixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	for _, c := range m.Categories {
		if c.Label == "Disks" && len(c.Rows) != 1 {
			t.Errorf("Disks category = %d rows, want 1 (the report never hides storage.volume)", len(c.Rows))
		}
	}
}
