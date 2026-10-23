package vmanage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// fixture reads and indexes the shared Cisco vManage testdata fixture.
func fixture(t *testing.T) *graph.Graph {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "testdata", "producer", "network", "cisco", "vmanage",
		"cisco-vmanage-2026-08-06T16-53-27Z-OFFICE-01.json")
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

// fixtureWithModel builds a minimal single-router document with the
// given "properties.model" string, for tests that only care about
// model-based dispatch (ConfirmedTopologyApplies).
func fixtureWithModel(t *testing.T, model string) *graph.Graph {
	t.Helper()
	doc := &document.Document{
		Topology: document.Topology{
			Resources: []document.Resource{
				{
					ID: "test-router01", Type: "network.router", Name: "TEST-ROUTER01", Provider: document.Provider{Name: ProviderName},
					Properties: []byte(`{"model":"` + model + `"}`),
				},
			},
		},
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	return g
}

func TestAppliesOnRealFixture(t *testing.T) {
	g := fixture(t)
	if !Applies(g) {
		t.Fatal("expected the Cisco vManage fixture to trigger enrichment")
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
				{ID: "a", Type: "network.router", Provider: document.Provider{Name: ProviderName}},
				{ID: "b", Type: "network.router", Provider: document.Provider{Name: "cisco.nxos"}},
			},
		},
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	if Applies(g) {
		t.Error("Applies fired on a document mixing Cisco vManage with a genuinely different provider")
	}
}

func TestAppliesRejectsMixedUnknownProvider(t *testing.T) {
	// Unlike Cisco NX-OS, vManage own fixtures carry no CDP/LLDP-style
	// discovery stub under an "unknown"/absent provider name so,
	// unlike nxos.Applies, this package's Applies has no such carve-out
	// and must reject a document mixing ProviderName with one.
	doc := &document.Document{
		Topology: document.Topology{
			Resources: []document.Resource{
				{ID: "a", Type: "network.router", Provider: document.Provider{Name: ProviderName}},
				{ID: "b", Type: "network.router.port", Provider: document.Provider{Name: "unknown"}},
			},
		},
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	if Applies(g) {
		t.Error("Applies fired on a document mixing Cisco vManage with an unknown-provider resource")
	}
}
