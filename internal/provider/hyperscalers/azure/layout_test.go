// Tests for the architectural-topology-projection placement mechanisms
// in layout.go: which composites composeResourceGroup builds and only
// from a real document edge, VNet/subnet container geometry, NSG
// boundary-badge placement and multi-association sequencing, and the
// VM/NIC wrapper.
package azure

import (
	"testing"

	"go.osirisjson.org/consumers/internal/topology"
)

// newTestRenderer builds a minimal TopologyRenderer and hierarchyIndex
// directly from hand-written nodes/edges, without a document or graph
// composeResourceGroup and its helpers only ever consult r.model.Edges,
// r.neighbors, r.consumedEdgeIDs and the index's nodeByID/cellID, all
// of which this reproduces exactly as
// NewTopologyRenderer/buildHierarchyIndex would.
func newTestRenderer(nodes []topology.Node, edges []topology.Edge) (*TopologyRenderer, *hierarchyIndex) {
	model := &topology.Topology{Nodes: nodes, Edges: edges}
	neighbors := make(map[string][]string, len(nodes))
	for _, e := range edges {
		neighbors[e.Source] = append(neighbors[e.Source], e.Target)
		neighbors[e.Target] = append(neighbors[e.Target], e.Source)
	}
	nodeType := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nodeType[n.ID] = n.Type
	}
	r := &TopologyRenderer{
		model:           model,
		armType:         map[string]string{},
		nodeType:        nodeType,
		extraLabel:      map[string]string{},
		neighbors:       neighbors,
		consumedEdgeIDs: make(map[string]bool),
	}
	return r, buildHierarchyIndex(model)
}

// TestComposeResourceGroupNoEdgesLeavesEverythingIndependent covers a
// resource group with no VM<->NIC or NSG<->subnet/NIC edges at all:
// alone VNet still gets its own (empty) container box, a VNet is always
// drawn as a container, "contains" only decides which subnets nest
// inside it, the subnet can remain visually empty but every other
// member, including an orphan subnet with no owning VNet in this group,
// stays a plain flat icon; nothing is ever wrapped or badged without
// a real edge.
func TestComposeResourceGroupNoEdgesLeavesEverythingIndependent(t *testing.T) {
	nodes := []topology.Node{
		{ID: "vnet1", Label: "vnet1", Type: "network.vpc"},
		{ID: "subnet1", Label: "subnet1", Type: "network.subnet"},
		{ID: "vm1", Label: "vm1", Type: "compute.vm"},
		{ID: "nic1", Label: "nic1", Type: "network.interface"},
		{ID: "nsg1", Label: "nsg1", Type: "network.security.group"},
		{ID: "disk1", Label: "disk1", Type: "storage.disk"},
	}
	r, idx := newTestRenderer(nodes, nil)
	flat, composites := r.composeResourceGroup(nodes, idx, 4000)

	if len(composites) != 1 || composites[0].groupType != "network.vpc" || len(composites[0].children) != 0 {
		t.Fatalf("composites = %+v, want exactly one empty VNet box (no VM/NSG composite without a real edge)", composites)
	}
	if len(flat) != len(nodes)-1 {
		t.Fatalf("flat = %d, want every non-VNet member (%d) to stay a plain icon", len(flat), len(nodes)-1)
	}
	if len(r.consumedEdgeIDs) != 0 {
		t.Errorf("no edges at all: consumedEdgeIDs = %v, want empty", r.consumedEdgeIDs)
	}
}

func TestComposeResourceGroupNestsSubnetOnlyWithRealContainsEdge(t *testing.T) {
	nodes := []topology.Node{
		{ID: "vnet1", Label: "vnet1", Type: "network.vpc"},
		{ID: "owned", Label: "owned", Type: "network.subnet"},
		{ID: "orphan", Label: "orphan", Type: "network.subnet"},
	}
	edges := []topology.Edge{
		{ID: "e-contains", Source: "owned", Target: "vnet1", Type: "contains", SourceResolved: true, TargetResolved: true},
	}
	r, idx := newTestRenderer(nodes, edges)
	flat, composites := r.composeResourceGroup(nodes, idx, 4000)

	if len(composites) != 1 {
		t.Fatalf("composites = %d, want 1 (the VNet box)", len(composites))
	}
	vnetBox := composites[0]
	if vnetBox.groupType != "network.vpc" || len(vnetBox.children) != 1 {
		t.Fatalf("vnet box = %+v, want exactly one nested subnet", vnetBox)
	}
	if got := vnetBox.children[0].id; got != idx.cellID["owned"] {
		t.Errorf("nested subnet id = %q, want %q", got, idx.cellID["owned"])
	}
	// The orphan subnet no VNet in this resource group claims it via a
	// real "contains" edge must still be shown, as a flat icon, never
	// silently dropped.
	foundOrphan := false
	for _, n := range flat {
		if n.ID == "orphan" {
			foundOrphan = true
		}
		if n.ID == "owned" {
			t.Errorf("the nested subnet must not also appear in the flat list")
		}
	}
	if !foundOrphan {
		t.Errorf("orphan subnet (no contains edge) must still render, flat: %+v", flat)
	}
	if !r.consumedEdgeIDs["e-contains"] {
		t.Errorf("the contains edge must be marked consumed once shown as nesting")
	}
}

func TestComposeResourceGroupWrapsVMOnlyWithRealNICEdge(t *testing.T) {
	nodes := []topology.Node{
		{ID: "vm1", Label: "vm1", Type: "compute.vm"},
		{ID: "nic1", Label: "nic1", Type: "network.interface"},
	}
	edges := []topology.Edge{
		{ID: "e-vm-nic", Source: "vm1", Target: "nic1", Type: "network", SourceResolved: true, TargetResolved: true},
	}
	r, idx := newTestRenderer(nodes, edges)
	flat, composites := r.composeResourceGroup(nodes, idx, 4000)

	if len(composites) != 1 || composites[0].groupType != "azure.vmwrapper" {
		t.Fatalf("composites = %+v, want exactly one VM wrapper", composites)
	}
	if len(flat) != 0 {
		t.Errorf("flat = %+v, want both the VM and its NIC consumed into the wrapper", flat)
	}
	if !r.consumedEdgeIDs["e-vm-nic"] {
		t.Errorf("the VM<->NIC edge must be marked consumed once shown inside the wrapper")
	}
}

// TestComposeResourceGroupNeverWrapsWithoutAnExplicitEdge a lone VM
// and a lone NIC in the same resource group, with no document edge
// between them, must render independently even though wrapping them
// would look plausible.
func TestComposeResourceGroupNeverWrapsWithoutAnExplicitEdge(t *testing.T) {
	nodes := []topology.Node{
		{ID: "vm1", Label: "vm1", Type: "compute.vm"},
		{ID: "nic1", Label: "nic1", Type: "network.interface"},
	}
	r, idx := newTestRenderer(nodes, nil)
	flat, composites := r.composeResourceGroup(nodes, idx, 4000)

	if len(composites) != 0 {
		t.Fatalf("composites = %+v, want none: no cardinality fallback without a real edge", composites)
	}
	if len(flat) != 2 {
		t.Fatalf("flat = %+v, want both the VM and the NIC rendered independently", flat)
	}
}

func TestComposeResourceGroupSQLVMAlsoWrapsFromARealEdge(t *testing.T) {
	nodes := []topology.Node{
		{ID: "sqlvm1", Label: "sqlvm1", Type: "osiris.azure.sqlvm"},
		{ID: "nic1", Label: "nic1", Type: "network.interface"},
	}
	edges := []topology.Edge{
		{ID: "e0", Source: "sqlvm1", Target: "nic1", Type: "network", SourceResolved: true, TargetResolved: true},
	}
	r, idx := newTestRenderer(nodes, edges)
	_, composites := r.composeResourceGroup(nodes, idx, 4000)
	if len(composites) != 1 || composites[0].groupType != "azure.vmwrapper" {
		t.Fatalf("composites = %+v, want a SQL VM wrapper too", composites)
	}
}

func TestNSGBadgeNestsInsideItsSpecificSubnetNeverGenericallyOnTheVNet(t *testing.T) {
	nodes := []topology.Node{
		{ID: "vnet1", Label: "vnet1", Type: "network.vpc"},
		{ID: "subnetA", Label: "subnetA", Type: "network.subnet"},
		{ID: "subnetB", Label: "subnetB", Type: "network.subnet"},
		{ID: "nsg1", Label: "nsg1", Type: "network.security.group"},
	}
	edges := []topology.Edge{
		{ID: "e-a", Source: "subnetA", Target: "vnet1", Type: "contains", SourceResolved: true, TargetResolved: true},
		{ID: "e-b", Source: "subnetB", Target: "vnet1", Type: "contains", SourceResolved: true, TargetResolved: true},
		{ID: "e-nsg", Source: "subnetA", Target: "nsg1", Type: "network", SourceResolved: true, TargetResolved: true},
	}
	r, idx := newTestRenderer(nodes, edges)
	_, composites := r.composeResourceGroup(nodes, idx, 4000)
	vnetBox := composites[0]

	var subnetABox, subnetBBox *placedGroup
	for i := range vnetBox.children {
		switch vnetBox.children[i].id {
		case idx.cellID["subnetA"]:
			subnetABox = &vnetBox.children[i]
		case idx.cellID["subnetB"]:
			subnetBBox = &vnetBox.children[i]
		}
	}
	if subnetABox == nil || subnetBBox == nil {
		t.Fatalf("expected both subnets nested in the VNet, got %+v", vnetBox.children)
	}
	if len(subnetABox.members) != 1 {
		t.Fatalf("subnetA members = %+v, want exactly the one NSG badge it has a real edge to", subnetABox.members)
	}
	badge := subnetABox.members[0]
	if badge.geom != (rect{nsgBadgeX, subnetMinH - nsgBadgeH, nsgBadgeW, nsgBadgeH}) {
		t.Errorf("badge geometry = %+v, want bottom-left corner {%d,%d,%d,%d}", badge.geom, nsgBadgeX, subnetMinH-nsgBadgeH, nsgBadgeW, nsgBadgeH)
	}
	if badge.parent != subnetABox.id {
		t.Errorf("badge parent = %q, want subnetA's own cell id %q (never the VNet)", badge.parent, subnetABox.id)
	}
	if len(subnetBBox.members) != 0 {
		t.Errorf("subnetB has no real NSG edge, so it must get no badge: %+v", subnetBBox.members)
	}
	if !r.consumedEdgeIDs["e-nsg"] {
		t.Errorf("the NSG<->subnet edge must be consumed once shown as a badge")
	}
}

func TestNSGWithMultipleAssociationsGetsOneBadgePerAssociation(t *testing.T) {
	nodes := []topology.Node{
		{ID: "vnet1", Label: "vnet1", Type: "network.vpc"},
		{ID: "subnetA", Label: "subnetA", Type: "network.subnet"},
		{ID: "subnetB", Label: "subnetB", Type: "network.subnet"},
		{ID: "nsg1", Label: "nsg1", Type: "network.security.group"},
	}
	edges := []topology.Edge{
		{ID: "e-a", Source: "subnetA", Target: "vnet1", Type: "contains", SourceResolved: true, TargetResolved: true},
		{ID: "e-b", Source: "subnetB", Target: "vnet1", Type: "contains", SourceResolved: true, TargetResolved: true},
		{ID: "e-nsg-a", Source: "subnetA", Target: "nsg1", Type: "network", SourceResolved: true, TargetResolved: true},
		{ID: "e-nsg-b", Source: "subnetB", Target: "nsg1", Type: "network", SourceResolved: true, TargetResolved: true},
	}
	r, idx := newTestRenderer(nodes, edges)
	_, composites := r.composeResourceGroup(nodes, idx, 4000)
	vnetBox := composites[0]

	var badgeIDs []string
	for _, child := range vnetBox.children {
		for _, m := range child.members {
			badgeIDs = append(badgeIDs, m.id)
		}
	}
	if len(badgeIDs) != 2 {
		t.Fatalf("badges = %v, want exactly 2 (one per real association)", badgeIDs)
	}
	nsgCellID := idx.cellID["nsg1"]
	first, second := badgeIDs[0], badgeIDs[1]
	if first != nsgCellID {
		t.Errorf("first badge id = %q, want the NSG's own canonical cell id %q", first, nsgCellID)
	}
	if second == nsgCellID || second != nsgCellID+"-nsg1" {
		t.Errorf("second badge id = %q, want a suffixed synthetic id distinct from %q", second, nsgCellID)
	}
	if !r.consumedEdgeIDs["e-nsg-a"] || !r.consumedEdgeIDs["e-nsg-b"] {
		t.Errorf("both NSG associations must be consumed")
	}
}

func TestBadgeCellIDSequencing(t *testing.T) {
	if got := badgeCellID("n1", 0); got != "n1" {
		t.Errorf("first badge = %q, want the base cell id unchanged", got)
	}
	if got := badgeCellID("n1", 1); got != "n1-nsg1" {
		t.Errorf("second badge = %q, want a suffixed synthetic id", got)
	}
	if got := badgeCellID("n1", 2); got != "n1-nsg2" {
		t.Errorf("third badge = %q, want its own distinct suffix", got)
	}
}

func TestNSGSiblingBadgeOnAVMWrapperNIC(t *testing.T) {
	nodes := []topology.Node{
		{ID: "vm1", Label: "vm1", Type: "compute.vm"},
		{ID: "nic1", Label: "nic1", Type: "network.interface"},
		{ID: "nsg1", Label: "nsg1", Type: "network.security.group"},
	}
	edges := []topology.Edge{
		{ID: "e-vm-nic", Source: "vm1", Target: "nic1", Type: "network", SourceResolved: true, TargetResolved: true},
		{ID: "e-nsg", Source: "nic1", Target: "nsg1", Type: "network", SourceResolved: true, TargetResolved: true},
	}
	r, idx := newTestRenderer(nodes, edges)
	_, composites := r.composeResourceGroup(nodes, idx, 4000)
	if len(composites) != 1 {
		t.Fatalf("composites = %+v, want the one VM wrapper", composites)
	}
	wrapper := composites[0]
	var nicGeom rect
	var badge *placedNode
	for i := range wrapper.members {
		m := wrapper.members[i]
		if m.id == idx.cellID["nic1"] {
			nicGeom = m.geom
		}
		if m.id == idx.cellID["nsg1"] {
			badge = &wrapper.members[i]
		}
	}
	if badge == nil {
		t.Fatalf("expected a sibling NSG badge inside the wrapper, got members %+v", wrapper.members)
	}
	if badge.parent != wrapper.id {
		t.Errorf("sibling badge parent = %q, want the wrapper's own id %q", badge.parent, wrapper.id)
	}
	want := rect{nicGeom.x + nsgBadgeX, nicGeom.y + nicGeom.h - nsgBadgeH, nsgBadgeW, nsgBadgeH}
	if badge.geom != want {
		t.Errorf("sibling badge geometry = %+v, want %+v (the NIC's own bottom-left corner)", badge.geom, want)
	}
	if !r.consumedEdgeIDs["e-nsg"] {
		t.Errorf("the NSG<->NIC edge must be consumed once shown as a sibling badge")
	}
}

func TestPlaceVNetContainerGeometryMatchesConfirmedSpacing(t *testing.T) {
	nodes := []topology.Node{
		{ID: "vnet1", Label: "vnet1", Type: "network.vpc"},
		{ID: "s1", Label: "s1", Type: "network.subnet"},
		{ID: "s2", Label: "s2", Type: "network.subnet"},
	}
	r, idx := newTestRenderer(nodes, nil)
	box := r.placeVNetContainer(nodes[0], []topology.Node{nodes[1], nodes[2]}, idx, map[string]int{}, 4000)

	if len(box.children) != 2 {
		t.Fatalf("children = %d, want 2 subnets", len(box.children))
	}
	if box.children[0].geom.x != hGap {
		t.Errorf("first subnet x = %d, want the confirmed left margin %d", box.children[0].geom.x, hGap)
	}
	wantSecondX := hGap + subnetMinW + hGap
	if box.children[1].geom.x != wantSecondX {
		t.Errorf("second subnet x = %d, want %d (margin + width + gap)", box.children[1].geom.x, wantSecondX)
	}
	for i, c := range box.children {
		if c.geom.w != subnetMinW || c.geom.h != subnetMinH {
			t.Errorf("subnet %d size = %dx%d, want the fixed %dx%d", i, c.geom.w, c.geom.h, subnetMinW, subnetMinH)
		}
	}
}

func TestPlaceSubnetContainerIsFixedSizeAndVisuallyEmptyWithoutAnNSG(t *testing.T) {
	r, idx := newTestRenderer([]topology.Node{{ID: "s1", Label: "s1", Type: "network.subnet"}}, nil)
	box := r.placeSubnetContainer(topology.Node{ID: "s1", Label: "s1", Type: "network.subnet"}, idx, map[string]int{})
	if box.geom.w != subnetMinW || box.geom.h != subnetMinH {
		t.Errorf("subnet size = %dx%d, want %dx%d", box.geom.w, box.geom.h, subnetMinW, subnetMinH)
	}
	if len(box.members) != 0 {
		t.Errorf("no NSG edge: members = %+v, want none (visually empty)", box.members)
	}
}

func TestHasContainsEdgeIsDirectionSensitive(t *testing.T) {
	r, _ := newTestRenderer(nil, []topology.Edge{
		{ID: "e0", Source: "subnet1", Target: "vnet1", Type: "contains", SourceResolved: true, TargetResolved: true},
	})
	if !r.hasContainsEdge("subnet1", "vnet1") {
		t.Errorf("expected the real subnet -> vnet contains edge to be found")
	}
	if r.hasContainsEdge("vnet1", "subnet1") {
		t.Errorf("reversed direction must not match: OSIRIS JSON connections are directional, never inferred")
	}
	if r.hasContainsEdge("subnet1", "other-vnet") {
		t.Errorf("a different target must not match")
	}
}

func TestMarkEdgeConsumedIsUndirectedAndNoOpWithoutBID(t *testing.T) {
	r, _ := newTestRenderer(nil, []topology.Edge{
		{ID: "e0", Source: "a", Target: "b", Type: "network", SourceResolved: true, TargetResolved: true},
	})
	r.markEdgeConsumed("b", "a") // reversed order from the edge's own Source/Target
	if !r.consumedEdgeIDs["e0"] {
		t.Errorf("markEdgeConsumed must find the edge in either direction")
	}
	r.markEdgeConsumed("x", "")
	if len(r.consumedEdgeIDs) != 1 {
		t.Errorf("an empty idB must be a no-op, got consumedEdgeIDs = %v", r.consumedEdgeIDs)
	}
}
