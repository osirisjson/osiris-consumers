// Tests for the generic index: complete graph for mixed-provider and
// unknown-type input, literal edges, reverse directions, and defensive
// structural anomaly detection.

package graph

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.osirisjson.org/consumers/internal/document"
)

func load(t *testing.T, rel string) *document.Document {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", rel))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	v, err := document.Read(strings.NewReader(string(b)), 0)
	if err != nil {
		t.Fatalf("fixture %s not one JSON value: %v", rel, err)
	}
	doc, err := document.Parse(v)
	if err != nil {
		t.Fatalf("parsing fixture %s: %v", rel, err)
	}
	return doc
}

func TestBuildMixedProviderIsComplete(t *testing.T) {
	g, err := Build(context.Background(), load(t, "valid/mixed-provider.json"))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(g.Anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", g.Anomalies)
	}
	if len(g.Resources) != 4 || len(g.ResourceIDs) != 4 {
		t.Errorf("resources indexed = %d/%d, want 4", len(g.Resources), len(g.ResourceIDs))
	}
	if len(g.Edges) != 3 {
		t.Errorf("edges = %d, want 3", len(g.Edges))
	}
	// Every provider present, verbatim.
	for _, p := range []string{"aws", "azure", "cisco", "hpe", "custom"} {
		if len(g.ByProvider[p]) == 0 {
			t.Errorf("provider %q not bucketed", p)
		}
	}
	// Unknown resource type still bucketed.
	if len(g.ByType["quantum.entangler"]) != 1 {
		t.Errorf("unknown resource type dropped from ByType")
	}
	// Unknown connection type still bucketed.
	if len(g.ConnectionsByType["unknown.linktype"]) != 1 {
		t.Errorf("unknown connection type dropped")
	}
	// Literal edge metadata.
	e := g.Edges[0]
	if e.Source != "aws::vpc-1" || e.Target != "azure::vnet-1" || e.Type != "network.peering" || e.Direction != "bidirectional" {
		t.Errorf("edge[0] = %+v, want literal fields from the connection", e)
	}
	// Reverse adjacency.
	if len(g.InEdges["azure::vnet-1"]) == 0 || len(g.OutEdges["aws::vpc-1"]) == 0 {
		t.Errorf("adjacency not populated")
	}
	// Group children and reverse membership.
	if len(g.GroupChildren["grp-all"]) != 2 {
		t.Errorf("group children not indexed")
	}
	if got := g.ParentOfGroups["grp-cloud"]; len(got) != 1 || got[0] != "grp-all" {
		t.Errorf("reverse group parent = %v, want [grp-all]", got)
	}
	if got := g.MemberOfGroups["aws::vpc-1"]; len(got) != 1 || got[0] != "grp-cloud" {
		t.Errorf("reverse membership = %v, want [grp-cloud]", got)
	}
}

func TestBuildKeepsContainsLiteral(t *testing.T) {
	doc := &document.Document{
		Version: "1.0.0",
		Topology: document.Topology{
			Resources: []document.Resource{
				{ID: "vpc", Type: "network.vpc"},
				{ID: "subnet", Type: "network.subnet"},
			},
			Connections: []document.Connection{
				{ID: "c", Type: "contains", Source: "vpc", Target: "subnet", Direction: "forward"},
			},
		},
	}
	g, err := Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(g.Anomalies) != 0 {
		t.Fatalf("anomalies: %+v", g.Anomalies)
	}
	if g.Edges[0].Type != "contains" {
		t.Errorf("edge type = %q, want the literal \"contains\"", g.Edges[0].Type)
	}
	// No hierarchy field is synthesised from a "contains" type.
	if len(g.GroupChildren) != 0 || len(g.ParentOfGroups) != 0 {
		t.Errorf("a containment hierarchy was inferred from a connection type")
	}
}

func TestBuildFlagsDanglingConnection(t *testing.T) {
	g, err := Build(context.Background(), load(t, "adversarial/dangling-connection.json"))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !hasAnomaly(g, DanglingConnectionTarget, "c-ghost", "ghost") {
		t.Fatalf("dangling target not reported: %+v", g.Anomalies)
	}
	// The edge is still present; nothing is dropped.
	if len(g.Edges) != 1 {
		t.Errorf("edges = %d, want 1 (edge retained despite dangling endpoint)", len(g.Edges))
	}
}

func TestBuildFlagsDuplicateResourceID(t *testing.T) {
	g, err := Build(context.Background(), load(t, "adversarial/duplicate-resource-id.json"))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !hasAnomaly(g, DuplicateResourceID, "dup", "") {
		t.Fatalf("duplicate id not reported: %+v", g.Anomalies)
	}
	// First declaration wins; it is not overwritten.
	if g.Resources["dup"].Type != "compute.host" {
		t.Errorf("duplicate overwrote the first declaration")
	}
}

func TestBuildFlagsSelfConnectionAndDanglingSource(t *testing.T) {
	doc := &document.Document{
		Version: "1.0.0",
		Topology: document.Topology{
			Resources: []document.Resource{{ID: "a", Type: "t.t"}},
			Connections: []document.Connection{
				{ID: "self", Type: "network", Source: "a", Target: "a"},
				{ID: "orphan", Type: "network", Source: "missing", Target: "a"},
			},
		},
	}
	g, _ := Build(context.Background(), doc)
	if !hasAnomaly(g, SelfConnection, "self", "a") {
		t.Errorf("self connection not reported: %+v", g.Anomalies)
	}
	if !hasAnomaly(g, DanglingConnectionSource, "orphan", "missing") {
		t.Errorf("dangling source not reported: %+v", g.Anomalies)
	}
}

func TestBuildFlagsGroupCycle(t *testing.T) {
	doc := &document.Document{
		Version: "1.0.0",
		Topology: document.Topology{
			Groups: []document.Group{
				{ID: "g1", Type: "l.g", Children: []string{"g2"}},
				{ID: "g2", Type: "l.g", Children: []string{"g3"}},
				{ID: "g3", Type: "l.g", Children: []string{"g1"}},
			},
		},
	}
	g, _ := Build(context.Background(), doc)
	for _, id := range []string{"g1", "g2", "g3"} {
		if !hasAnomaly(g, GroupCycle, id, "") {
			t.Errorf("group %s not reported on the cycle: %+v", id, g.Anomalies)
		}
	}
}

func TestBuildFlagsSelfGroupChildWithoutInfiniteLoop(t *testing.T) {
	doc := &document.Document{
		Version: "1.0.0",
		Topology: document.Topology{
			Groups: []document.Group{{ID: "g", Type: "l.g", Children: []string{"g"}}},
		},
	}
	g, _ := Build(context.Background(), doc)
	if !hasAnomaly(g, SelfGroupChild, "g", "g") {
		t.Errorf("self group child not reported: %+v", g.Anomalies)
	}
	if hasAnomalyKind(g, GroupCycle) {
		t.Errorf("self child should be its own anomaly, not a GroupCycle")
	}
}

func TestBuildAnomalyOrderIsDeterministic(t *testing.T) {
	doc := &document.Document{
		Version: "1.0.0",
		Topology: document.Topology{
			Resources: []document.Resource{{ID: "z", Type: "t.t"}, {ID: "z", Type: "t.t"}, {ID: "a", Type: "t.t"}, {ID: "a", Type: "t.t"}},
		},
	}
	g, _ := Build(context.Background(), doc)
	if len(g.Anomalies) != 2 {
		t.Fatalf("anomalies = %d, want 2", len(g.Anomalies))
	}
	if g.Anomalies[0].Subject != "a" || g.Anomalies[1].Subject != "z" {
		t.Errorf("anomalies not sorted by subject: %+v", g.Anomalies)
	}
}

func TestBuildHonoursCancellation(t *testing.T) {
	res := make([]document.Resource, 5000)
	for i := range res {
		res[i] = document.Resource{ID: string(rune('a'+i%26)) + itoa(i), Type: "t.t"}
	}
	doc := &document.Document{Version: "1.0.0", Topology: document.Topology{Resources: res}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, doc); err != context.Canceled {
		t.Fatalf("Build error = %v, want context.Canceled", err)
	}
}

func TestBuildFlagsDanglingGroupMemberAndChild(t *testing.T) {
	doc := &document.Document{
		Version: "1.0.0",
		Topology: document.Topology{
			Resources: []document.Resource{{ID: "r", Type: "t.t"}},
			Groups: []document.Group{
				{ID: "g1", Type: "l.g", Members: []string{"r", "no-such-member"}, Children: []string{"no-such-group"}},
			},
		},
	}
	g, _ := Build(context.Background(), doc)
	if !hasAnomaly(g, DanglingGroupMember, "g1", "no-such-member") {
		t.Errorf("dangling member not reported: %+v", g.Anomalies)
	}
	if !hasAnomaly(g, DanglingGroupChild, "g1", "no-such-group") {
		t.Errorf("dangling child not reported: %+v", g.Anomalies)
	}
	// A member that names a resource is fine.
	if hasAnomaly(g, DanglingGroupMember, "g1", "r") {
		t.Errorf("valid member wrongly flagged")
	}
}

func TestBuildDuplicateConnectionAndGroupIDs(t *testing.T) {
	doc := &document.Document{
		Version: "1.0.0",
		Topology: document.Topology{
			Resources:   []document.Resource{{ID: "a", Type: "t.t"}, {ID: "b", Type: "t.t"}},
			Connections: []document.Connection{{ID: "c", Type: "network", Source: "a", Target: "b"}, {ID: "c", Type: "network", Source: "b", Target: "a"}},
			Groups:      []document.Group{{ID: "g", Type: "l.g"}, {ID: "g", Type: "l.g"}},
		},
	}
	g, _ := Build(context.Background(), doc)
	if !hasAnomaly(g, DuplicateConnectionID, "c", "") {
		t.Errorf("duplicate connection id not reported: %+v", g.Anomalies)
	}
	if !hasAnomaly(g, DuplicateGroupID, "g", "") {
		t.Errorf("duplicate group id not reported: %+v", g.Anomalies)
	}
	// Both connections still produce edges; nothing dropped.
	if len(g.Edges) != 2 {
		t.Errorf("edges = %d, want 2", len(g.Edges))
	}
}

func TestGraphHelpers(t *testing.T) {
	g, err := Build(context.Background(), load(t, "valid/no-provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !g.ResourceExists("node-a") || g.ResourceExists("nope") {
		t.Errorf("ResourceExists wrong")
	}
	if AnomalyKind(99).String() == "" {
		t.Errorf("AnomalyKind.String default branch empty")
	}
	for k := DuplicateResourceID; k <= GroupCycle; k++ {
		if k.String() == "" {
			t.Errorf("kind %d has no slug", k)
		}
	}
}

// TestBuildProducerFixtures indexes the neutralised per-producer
// snapshots. They model the real shape of osiris-producer output (ARM
// and ARN style identifiers, osiris.<vendor>.<kind> types, provider
// source/type fields, contains* and network.* connection types, all
// three edge directions, nested groups) and must index cleanly with no
// anomaly and nothing dropped.
func TestBuildProducerFixtures(t *testing.T) {
	cases := []struct {
		file                     string
		resources, conns, groups int
	}{
		{producerFixtures[0], 126, 129, 29},
		{producerFixtures[1], 84, 107, 4},
		{producerFixtures[2], 17, 16, 1},
		{producerFixtures[3], 17, 11, 0},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			g, err := Build(context.Background(), load(t, tc.file))
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if len(g.Anomalies) != 0 {
				t.Fatalf("anomalies: %+v", g.Anomalies)
			}
			if len(g.ResourceIDs) != tc.resources || len(g.ConnectionIDs) != tc.conns || len(g.GroupIDs) != tc.groups {
				t.Fatalf("counts = %d/%d/%d, want %d/%d/%d",
					len(g.ResourceIDs), len(g.ConnectionIDs), len(g.GroupIDs),
					tc.resources, tc.conns, tc.groups)
			}
			// Every connection produced exactly one literal edge.
			if len(g.Edges) != tc.conns {
				t.Errorf("edges = %d, want %d", len(g.Edges), tc.conns)
			}
			// Every resource is bucketed by provider and type.
			var byProvider int
			for _, ids := range g.ByProvider {
				byProvider += len(ids)
			}
			if byProvider != tc.resources {
				t.Errorf("ByProvider covers %d resources, want %d", byProvider, tc.resources)
			}
		})
	}
}

// producerFixtures is the set of neutralised per-producer snapshots
// under testdata/producer/, by repo-relative path from testdata/.
var producerFixtures = []string{
	"producer/hyperscaler/azure/azure.json",
	"producer/hyperscaler/aws/aws.json",
	"producer/network/cisco/apic/cisco-apic.json",
	"producer/network/cisco/nxos/cisco-nxos.json",
	"producer/network/cisco/vmanage/cisco-vmanage.json",
	"producer/network/hpe/arubacentral/hpe-arubacentral.json",
}

func TestBuildProducerFixturesCoverEdgeDirections(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range producerFixtures {
		g, err := Build(context.Background(), load(t, f))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range g.Edges {
			seen[e.Direction] = true
		}
	}
	for _, d := range []string{"forward", "bidirectional"} {
		if !seen[d] {
			t.Errorf("no producer fixture exercises direction %q", d)
		}
	}
}

func hasAnomaly(g *Graph, k AnomalyKind, subject, ref string) bool {
	for _, a := range g.Anomalies {
		if a.Kind == k && a.Subject == subject && (ref == "" || a.Reference == ref) {
			return true
		}
	}
	return false
}

func hasAnomalyKind(g *Graph, k AnomalyKind) bool {
	for _, a := range g.Anomalies {
		if a.Kind == k {
			return true
		}
	}
	return false
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}
