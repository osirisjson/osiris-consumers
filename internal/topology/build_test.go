// Tests for the topology model builder: one node per resource, one edge
// per connection with literal metadata, groups represented, unresolved
// endpoints disclosed not dropped, total ordering, and the counts
// invariant a later provider enrichment must preserve.

package topology

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

func load(t *testing.T, rel string) (*document.Document, *graph.Graph) {
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
	return doc, g
}

func TestBuildOneNodePerResourceOneEdgePerConnection(t *testing.T) {
	doc, g := load(t, "valid/mixed-provider.json")
	top, err := Build(context.Background(), doc, g)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(top.Nodes) != len(doc.Topology.Resources) {
		t.Errorf("nodes = %d, want one per resource (%d)", len(top.Nodes), len(doc.Topology.Resources))
	}
	if len(top.Edges) != len(doc.Topology.Connections) {
		t.Errorf("edges = %d, want one per connection (%d)", len(top.Edges), len(doc.Topology.Connections))
	}
	if len(top.Groups) != len(doc.Topology.Groups) {
		t.Errorf("groups = %d, want one per group (%d)", len(top.Groups), len(doc.Topology.Groups))
	}
	c := top.Counts()
	if c.Nodes != len(top.Nodes) || c.Edges != len(top.Edges) || c.Groups != len(top.Groups) {
		t.Errorf("Counts() = %+v, out of step with the slices", c)
	}
}

func TestBuildLiteralEdgeMetadata(t *testing.T) {
	doc, g := load(t, "valid/mixed-provider.json")
	top, _ := Build(context.Background(), doc, g)

	var got *Edge
	for i := range top.Edges {
		if top.Edges[i].ID == "c-aws-azure" {
			got = &top.Edges[i]
		}
	}
	if got == nil {
		t.Fatal("edge c-aws-azure not built")
	}
	if got.Source != "aws::vpc-1" || got.Target != "azure::vnet-1" ||
		got.Type != "network.peering" || got.Direction != "bidirectional" {
		t.Errorf("edge = %+v, want the connection's literal fields", *got)
	}
	if !got.SourceResolved || !got.TargetResolved {
		t.Errorf("both endpoints are resources; resolved flags = %v/%v", got.SourceResolved, got.TargetResolved)
	}
}

func TestBuildTotalOrdering(t *testing.T) {
	doc, g := load(t, "valid/mixed-provider.json")
	top, _ := Build(context.Background(), doc, g)

	if !sort.SliceIsSorted(top.Nodes, func(i, j int) bool { return top.Nodes[i].ID < top.Nodes[j].ID }) {
		t.Errorf("nodes not sorted by ID")
	}
	if !sort.SliceIsSorted(top.Groups, func(i, j int) bool { return top.Groups[i].ID < top.Groups[j].ID }) {
		t.Errorf("groups not sorted by ID")
	}
	if !sort.StringsAreSorted(top.Unresolved) {
		t.Errorf("unresolved not sorted")
	}
	// Edge order: ID, then source, then target, then type.
	for i := 1; i < len(top.Edges); i++ {
		a, b := top.Edges[i-1], top.Edges[i]
		if a.ID > b.ID {
			t.Errorf("edges not sorted by ID at %d: %q > %q", i, a.ID, b.ID)
		}
	}
}

func TestBuildUnresolvedEndpointDisclosedNotDropped(t *testing.T) {
	doc, g := load(t, "adversarial/dangling-connection.json")
	top, err := Build(context.Background(), doc, g)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// The connection still becomes an edge.
	if len(top.Edges) != len(doc.Topology.Connections) {
		t.Fatalf("edges = %d, want %d (a dangling connection is still an edge)",
			len(top.Edges), len(doc.Topology.Connections))
	}
	if len(top.Unresolved) == 0 {
		t.Fatalf("dangling endpoint not recorded in Unresolved")
	}
	// An unresolved id is not counted as a node.
	for _, u := range top.Unresolved {
		for _, n := range top.Nodes {
			if n.ID == u {
				t.Errorf("unresolved id %q leaked into Nodes", u)
			}
		}
	}
	var dangling *Edge
	for i := range top.Edges {
		if !top.Edges[i].SourceResolved || !top.Edges[i].TargetResolved {
			dangling = &top.Edges[i]
		}
	}
	if dangling == nil {
		t.Fatal("no edge flagged with an unresolved endpoint")
	}
}

func TestBuildPrimaryGroupIsDeepestClaimingGroup(t *testing.T) {
	// mixed-provider: aws::vpc-1 is a member of grp-cloud only.
	doc, g := load(t, "valid/mixed-provider.json")
	top, _ := Build(context.Background(), doc, g)
	for _, n := range top.Nodes {
		if n.ID == "aws::vpc-1" && n.PrimaryGroup != "grp-cloud" {
			t.Errorf("aws::vpc-1 PrimaryGroup = %q, want grp-cloud", n.PrimaryGroup)
		}
	}
}

func TestBuildGenericNodesAreNotMarkedKnown(t *testing.T) {
	doc, g := load(t, "valid/mixed-provider.json")
	top, _ := Build(context.Background(), doc, g)
	for _, n := range top.Nodes {
		if n.Known {
			t.Errorf("generic build marked node %q Known; enrichment is the only thing that may", n.ID)
		}
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	doc, g := load(t, "valid/mixed-provider.json")
	a, _ := Build(context.Background(), doc, g)
	b, _ := Build(context.Background(), doc, g)
	if a.Counts() != b.Counts() {
		t.Fatalf("counts differ between runs: %+v vs %+v", a.Counts(), b.Counts())
	}
	for i := range a.Nodes {
		if a.Nodes[i] != b.Nodes[i] {
			t.Errorf("node %d differs between runs", i)
		}
	}
	for i := range a.Edges {
		if a.Edges[i] != b.Edges[i] {
			t.Errorf("edge %d differs between runs", i)
		}
	}
}

func TestBuildCancellation(t *testing.T) {
	doc, g := load(t, "valid/mixed-provider.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, doc, g); err == nil {
		t.Fatal("Build returned nil error on a cancelled context")
	}
}
