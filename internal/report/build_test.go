// Tests for the report model builder: exact counts, per-provider and
// per-type breakdowns, total ordering, evidence-state disclosure,
// loss-aware flags, and bounded-inventory truncation notices.

package report

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/validation"
)

func build(t *testing.T, rel string, opts Options) (*Report, *document.Document) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", rel))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", rel, err)
	}
	doc, err := document.Parse(b)
	if err != nil {
		t.Fatalf("parsing %s: %v", rel, err)
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing %s: %v", rel, err)
	}
	receipt := validation.Receipt{
		InputSHA256:          doc.SHA256(),
		EngineName:           validation.PinnedEngineName,
		EngineVersion:        validation.PinnedEngineVersion,
		Profile:              validation.DefaultProfile,
		SpecificationVersion: validation.PinnedSpecVersion,
	}
	r, err := Build(context.Background(), doc, g, receipt, opts)
	if err != nil {
		t.Fatalf("Build %s: %v", rel, err)
	}
	return r, doc
}

func TestBuildExactCounts(t *testing.T) {
	r, doc := build(t, "valid/mixed-provider.json", Options{})
	if r.Counts.Resources != len(doc.Topology.Resources) {
		t.Errorf("Resources = %d, want %d", r.Counts.Resources, len(doc.Topology.Resources))
	}
	if r.Counts.Connections != len(doc.Topology.Connections) {
		t.Errorf("Connections = %d, want %d", r.Counts.Connections, len(doc.Topology.Connections))
	}
	if r.Counts.Groups != len(doc.Topology.Groups) {
		t.Errorf("Groups = %d, want %d", r.Counts.Groups, len(doc.Topology.Groups))
	}
	if r.Counts.Edges != r.Counts.Connections {
		t.Errorf("Edges = %d, want one per connection (%d)", r.Counts.Edges, r.Counts.Connections)
	}
	if len(r.Resources) != r.Counts.Resources {
		t.Errorf("Resources list has %d rows, count says %d", len(r.Resources), r.Counts.Resources)
	}
}

func TestBuildProviderAndTypeBreakdown(t *testing.T) {
	r, _ := build(t, "valid/mixed-provider.json", Options{})

	if !sort.SliceIsSorted(r.Providers, func(i, j int) bool { return r.Providers[i].Name < r.Providers[j].Name }) {
		t.Errorf("providers not sorted by name")
	}
	byName := map[string]ProviderBreakdown{}
	total := 0
	for _, p := range r.Providers {
		byName[p.Name] = p
		total += p.ResourceCount
		if !sort.StringsAreSorted(p.ResourceTypes) {
			t.Errorf("provider %q resource types not sorted: %v", p.Name, p.ResourceTypes)
		}
	}
	if total != r.Counts.Resources {
		t.Errorf("provider rows cover %d resources, want %d", total, r.Counts.Resources)
	}
	if byName["custom"].ResourceCount != 2 {
		t.Errorf("custom provider = %d resources, want 2", byName["custom"].ResourceCount)
	}

	// The novel resource type survives in the type breakdown.
	var sawUnknownType bool
	for _, ty := range r.ResourceTypes {
		if ty.Type == "quantum.entangler" {
			sawUnknownType = ty.Count == 1
		}
	}
	if !sawUnknownType {
		t.Errorf("unknown resource type dropped from the breakdown: %+v", r.ResourceTypes)
	}
}

func TestBuildTotalOrdering(t *testing.T) {
	r, _ := build(t, "valid/mixed-provider.json", Options{})
	if !sort.SliceIsSorted(r.Resources, func(i, j int) bool {
		if r.Resources[i].ID != r.Resources[j].ID {
			return r.Resources[i].ID < r.Resources[j].ID
		}
		return r.Resources[i].Type < r.Resources[j].Type
	}) {
		t.Errorf("resources not sorted by ID then type")
	}
	if !sort.SliceIsSorted(r.Groups, func(i, j int) bool { return r.Groups[i].ID < r.Groups[j].ID }) {
		t.Errorf("groups not sorted by ID")
	}
	for _, e := range r.Resources {
		if !sort.SliceIsSorted(e.Tags, func(i, j int) bool { return e.Tags[i].Key < e.Tags[j].Key }) {
			t.Errorf("resource %q tags not sorted by key", e.ID)
		}
		if !sort.StringsAreSorted(e.GroupIDs) {
			t.Errorf("resource %q group ids not sorted", e.ID)
		}
	}
}

func TestBuildEvidenceStateDisclosure(t *testing.T) {
	// no-provider fixture: has scope + generator, declares no groups.
	r, _ := build(t, "valid/no-provider.json", Options{})
	if !r.Disclosure.Generator {
		t.Errorf("generator present in fixture but disclosed absent")
	}
	if !r.Disclosure.Scope {
		t.Errorf("scope present in fixture but disclosed absent")
	}

	// unknown-fields fixture carries top-level members outside the core.
	r2, _ := build(t, "valid/unknown-fields.json", Options{})
	if !r2.Disclosure.UnknownTopLevelFields {
		t.Errorf("unknown top-level fields not disclosed")
	}
}

func TestBuildLossAwareFlags(t *testing.T) {
	r, _ := build(t, "valid/mixed-provider.json", Options{})
	var withProps int
	for _, e := range r.Resources {
		if e.HasProperties {
			withProps++
		}
	}
	if withProps == 0 {
		t.Errorf("no resource reported carrying a properties payload; fixture has one")
	}
}

func TestBuildBoundedInventoryEmitsTruncationNotice(t *testing.T) {
	full, _ := build(t, "valid/mixed-provider.json", Options{})
	bounded, _ := build(t, "valid/mixed-provider.json", Options{MaxRows: 2})

	if len(bounded.Resources) != 2 {
		t.Fatalf("bounded Resources = %d, want 2", len(bounded.Resources))
	}
	// The kept rows are the first two in the full sort order.
	for i := 0; i < 2; i++ {
		if bounded.Resources[i].ID != full.Resources[i].ID {
			t.Errorf("bounded row %d = %q, want %q (first rows in sort order)", i, bounded.Resources[i].ID, full.Resources[i].ID)
		}
	}
	var note *Truncation
	for i := range bounded.Truncations {
		if bounded.Truncations[i].Section == "resources" {
			note = &bounded.Truncations[i]
		}
	}
	if note == nil {
		t.Fatal("no truncation notice for the bounded resources section")
	}
	if note.Shown != 2 || note.Total != full.Counts.Resources {
		t.Errorf("truncation notice = %+v, want shown 2 of %d", *note, full.Counts.Resources)
	}
	// Counts still report the true totals, never the bounded length.
	if bounded.Counts.Resources != full.Counts.Resources {
		t.Errorf("bounded Counts.Resources = %d, want the true total %d", bounded.Counts.Resources, full.Counts.Resources)
	}
}

func TestBuildAnomaliesCarriedVerbatim(t *testing.T) {
	r, _ := build(t, "adversarial/dangling-connection.json", Options{})
	if len(r.Anomalies) == 0 || r.Counts.Anomalies != len(r.Anomalies) {
		t.Fatalf("anomalies = %+v, count = %d", r.Anomalies, r.Counts.Anomalies)
	}
	if r.Anomalies[0].Kind == "" || r.Anomalies[0].Detail == "" {
		t.Errorf("anomaly entry missing fields: %+v", r.Anomalies[0])
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	a, _ := build(t, "valid/mixed-provider.json", Options{})
	b, _ := build(t, "valid/mixed-provider.json", Options{})
	if a.Counts != b.Counts {
		t.Fatalf("counts differ between runs")
	}
	for i := range a.Resources {
		if a.Resources[i].ID != b.Resources[i].ID {
			t.Errorf("resource order differs between runs at %d", i)
		}
	}
}

func TestBuildCancellation(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "valid/mixed-provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := document.Parse(b)
	g, _ := graph.Build(context.Background(), doc)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, doc, g, validation.Receipt{}, Options{}); err == nil {
		t.Fatal("Build returned nil error on a cancelled context")
	}
}
