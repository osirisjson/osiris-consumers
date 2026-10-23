// Tests for the Cisco APIC enrichment trigger: a whole-document check,
// never a first-resource inference.

package apic

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

const fixture = "producer/network/cisco/apic/cisco-apic.json"

func build(t *testing.T, rel string) *graph.Graph {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "testdata", rel))
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
	return g
}

func TestAppliesToAHomogeneousAPICDocument(t *testing.T) {
	g := build(t, fixture)
	if !Applies(g) {
		t.Error("Applies = false, want true for an all-cisco.apic document")
	}
}

func TestAppliesRejectsMixedProvider(t *testing.T) {
	g := build(t, "valid/mixed-provider.json")
	if Applies(g) {
		t.Error("Applies = true, want false for a mixed-provider document")
	}
}

func TestAppliesRejectsUnrelatedProvider(t *testing.T) {
	g := build(t, "producer/network/hpe/arubacentral/hpe-arubacentral-2026-08-06T16-47-25Z-BRANCH-01.json")
	if Applies(g) {
		t.Error("Applies = true, want false for an hpe.arubacentral-only document")
	}
}

func TestAppliesRejectsEmptyDocument(t *testing.T) {
	g := build(t, "valid/no-provider.json")
	if Applies(g) {
		t.Error("Applies = true, want false when there is nothing to detect a provider from")
	}
}
