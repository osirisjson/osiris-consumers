package nxos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// fixture reads and indexes the shared Cisco NX-OS testdata fixture.
func fixture(t *testing.T) *graph.Graph {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "testdata", "producer", "network", "cisco", "nxos",
		"cisco-nxos.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	doc, err := document.Parse(raw)
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing fixture: %v", err)
	}
	return g
}

func TestAppliesOnRealFixture(t *testing.T) {
	g := fixture(t)
	if !Applies(g) {
		t.Fatal("expected the Cisco NX-OS fixture to trigger enrichment")
	}
}

func TestAppliesRejectsEmptyDocument(t *testing.T) {
	g, err := graph.Build(context.Background(), &document.Document{})
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	if Applies(g) {
		t.Error("Applies fired on a document with no resources")
	}
}

func TestAppliesRejectsThirdProvider(t *testing.T) {
	doc := &document.Document{
		Topology: document.Topology{
			Resources: []document.Resource{
				{ID: "a", Type: "network.switch", Provider: document.Provider{Name: ProviderName}},
				{ID: "b", Type: "network.switch", Provider: document.Provider{Name: "hpe.arubacentral"}},
			},
		},
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	if Applies(g) {
		t.Error("Applies fired on a document mixing Cisco NX-OS with a genuinely different provider")
	}
}

func TestAppliesRejectsWithoutTheRealProvider(t *testing.T) {
	// A document made entirely of "unknown"-provider resources (no real
	// Cisco NX-OS resource at all) must not qualify, Applies requires
	// at least one ProviderName resource, not merely the absence of a
	// conflicting one.
	doc := &document.Document{
		Topology: document.Topology{
			Resources: []document.Resource{
				{ID: "a", Type: "network.interface", Provider: document.Provider{Name: "unknown"}},
			},
		},
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	if Applies(g) {
		t.Error("Applies fired on a document with no ProviderName resource at all")
	}
}

func TestAppliesAcceptsProviderPlusUnknownNeighbours(t *testing.T) {
	doc := &document.Document{
		Topology: document.Topology{
			Resources: []document.Resource{
				{ID: "a", Type: "network.switch", Provider: document.Provider{Name: ProviderName}},
				{ID: "b", Type: "network.interface", Provider: document.Provider{Name: "unknown"}},
				{ID: "c", Type: "network.interface", Provider: document.Provider{}},
			},
		},
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	if !Applies(g) {
		t.Error("Applies did not fire on ProviderName plus unknown/absent-provider discovery stubs")
	}
}
