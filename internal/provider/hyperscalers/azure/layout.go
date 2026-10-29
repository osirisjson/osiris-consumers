// This file computes the deterministic placement of an Azure topology,
// a management group, subscription, resource group are drawn as
// containers; a "container.region" grouping (a second, non-hierarchical
// axis some documents also carry) is not rendered as a container at
// all, so a resource's placement always follows its resource-group
// membership never whichever group happen to sort first alphabetically.
package azure

import (
	"sort"
	"strconv"

	"go.osirisjson.org/consumers/internal/topology"
)

// Layout geometry in mxGraph user units to improve overall readability.
const (
	startSize = 30 // swimlane header-bar height, every container level
	tenantPad = 10 // margin on every side of a nested management-group/subscription container

	rgOffsetX    = 40 // resource groups start this far from their subscription's left edge
	rgOffsetY    = 40 // resource groups start this far below the subscription's own header bar
	rgGap        = 40 // gap between resource-group boxes, both axes, and the subscription's own closing (right/bottom) margin the same 40pt as rgOffsetX/rgOffsetY, a uniform inset on every side
	rgContentPad = 20 // padding around a populated resource group's own member-icon grid

	emptyRGHeight       = 78  // a resource-group box with no visible member: header bar only
	emptyRGMinW         = 160 // approximate; real text-width measurement is not implemented
	emptyRGMaxW         = 320
	emptyRGWidthPerChar = 7
	emptyRGWidthBase    = 60

	hGap = 40 // gap between member icons in a populated resource group's own grid
	vGap = 40

	// Architectural-topology-projection geometry.
	subnetMinW = 120 // a subnet container's own fixed size
	subnetMinH = 78  // visually empty

	nsgBadgeW = 18 // the small boundary badge marking that an NSG applies
	nsgBadgeH = 20 // to a specific subnet or NIC
	nsgBadgeX = -8 // NSG straddles the anchor's own left edge

	vmWrapperPadX = 80  // left/right margin inside a "Virtual Machine <name>" wrapper
	vmWrapperPadY = 40  // top/bottom margin inside the same wrapper
	vmWrapperGapX = 120 // gap between the VM icon and each attached NIC icon (leaving room for each icon's own label lines)
)

// rect is an axis-aligned cell rectangle in mxGraph user units.
type rect struct{ x, y, w, h int }

// placedNode is one resource icon.
type placedNode struct {
	id     string
	value  string
	style  string
	geom   rect
	parent string
}

// placedGroup is one container (management group, subscription, or
// resource group) with its nested content. members holds direct
// resource icons (only ever populated for a resource-group leaf);
// children holds nested containers (only ever populated for a
// management-group/subscription level, or a resource-group area's own
// flattened list of resource-group boxes).
type placedGroup struct {
	id        string
	value     string
	groupType string
	geom      rect
	members   []placedNode
	children  []placedGroup
}

// hierarchyIndex is the read-only index built once from the model,
// used throughout placement.
type hierarchyIndex struct {
	byID       map[string]*topology.Group
	cellID     map[string]string // group ID or node ID -> mxCell id
	childrenOf map[string][]string
	rgMembers  map[string][]topology.Node // resource-group ID -> its visible members, in sorted node order
	nodeByID   map[string]topology.Node   // every visible node, by ID looked up while building composite boxes (VNet/subnet/VM-wrapper/NSG badge)
}

// hierarchyGroupTypes is the set of OSIRIS JSON group types this page
// draws as a container at all. Anything else (in particular
// "container.region") is skipped entirely for layout purposes; its
// resources fall back to their resource-group membership instead.
var hierarchyGroupTypes = map[string]bool{
	"logical.managementgroup": true,
	"logical.subscription":    true,
	"logical.resourcegroup":   true,
}

// buildHierarchyIndex indexes m for placement: which groups nest under
// which, and which resources are the visible members of which resource
// group.
func buildHierarchyIndex(m *topology.Topology) *hierarchyIndex {
	idx := &hierarchyIndex{
		byID:       make(map[string]*topology.Group, len(m.Groups)),
		cellID:     make(map[string]string, len(m.Groups)+len(m.Nodes)),
		childrenOf: make(map[string][]string),
		rgMembers:  make(map[string][]topology.Node),
		nodeByID:   make(map[string]topology.Node, len(m.Nodes)),
	}
	for i := range m.Groups {
		g := &m.Groups[i]
		idx.byID[g.ID] = g
		idx.cellID[g.ID] = "g" + strconv.Itoa(i)
	}
	// childrenOf: only record a parent/child edge when both ends are a
	// recognized hierarchy type, a region group is never a parent
	// (it has no children of its own) and never treated as a child
	// here even if some future document listed one.
	for i := range m.Groups {
		g := &m.Groups[i]
		if !hierarchyGroupTypes[g.Type] || g.ParentGroupID == "" {
			continue
		}
		if parent := idx.byID[g.ParentGroupID]; parent != nil && hierarchyGroupTypes[parent.Type] {
			idx.childrenOf[g.ParentGroupID] = append(idx.childrenOf[g.ParentGroupID], g.ID)
		}
	}
	// nodeToRG: a resource's effective container is whichever
	// logical.resourcegroup lists it as a member never whichever
	// group happens to sort first (that was the flat, alphabetically-
	// driven region container the first round rendered into).
	nodeToRG := make(map[string]string, len(m.Nodes))
	for i := range m.Groups {
		g := &m.Groups[i]
		if g.Type != "logical.resourcegroup" {
			continue
		}
		for _, nid := range g.MemberNodeIDs {
			if _, already := nodeToRG[nid]; !already {
				nodeToRG[nid] = g.ID
			}
		}
	}
	for i, n := range m.Nodes {
		idx.cellID[n.ID] = "n" + strconv.Itoa(i)
		if hiddenResourceTypes[n.Type] {
			continue
		}
		idx.nodeByID[n.ID] = n
		rgID, ok := nodeToRG[n.ID]
		if !ok {
			continue
		}
		idx.rgMembers[rgID] = append(idx.rgMembers[rgID], n)
	}
	return idx
}

// placeRoots places every hierarchy-type group with no recognized
// parent, stacked top to bottom starting at (x0, y0), and returns the
// combined content extent (w, h) measured from (x0, y0). availWidth is
// the usable width this content may wrap to fill ultimately the
// chosen page's own width minus its margins (topology.go's
// layoutTopology), threaded down through every nested container so a
// subscription's own resource-group grid wraps against the real page
// width, not a fixed guess.
func (r *TopologyRenderer) placeRoots(idx *hierarchyIndex, x0, y0, availWidth int) (roots []placedGroup, w, h int) {
	cursorY := y0
	for i := range r.model.Groups {
		g := &r.model.Groups[i]
		if !hierarchyGroupTypes[g.Type] || g.ParentGroupID != "" {
			continue
		}
		box := r.placeContainer(g.ID, idx, availWidth)
		box.geom.x = x0
		box.geom.y = cursorY
		roots = append(roots, box)
		if box.geom.w > w {
			w = box.geom.w
		}
		cursorY += box.geom.h + tenantPad
	}
	if len(roots) > 0 {
		cursorY -= tenantPad
	}
	h = cursorY - y0
	return roots, w, h
}

// placeContainer places gid: a resource-group leaf via
// placeResourceGroupLeaf, or a management-group/subscription level via
// its children (either a nested management-group/subscription chain,
// or at the level directly above one or more resource groups the
// resource-group area). availWidth is the usable width available to
// gid's own box at its actual nesting depth (see placeRoots).
func (r *TopologyRenderer) placeContainer(gid string, idx *hierarchyIndex, availWidth int) placedGroup {
	g := idx.byID[gid]
	if g.Type == "logical.resourcegroup" {
		return r.placeResourceGroupLeaf(gid, idx, availWidth)
	}

	pg := placedGroup{id: idx.cellID[gid], value: groupValue(*g), groupType: g.Type}
	children := idx.childrenOf[gid]

	var rgChildren, otherChildren []string
	for _, cid := range children {
		if idx.byID[cid].Type == "logical.resourcegroup" {
			rgChildren = append(rgChildren, cid)
		} else {
			otherChildren = append(otherChildren, cid)
		}
	}

	switch {
	case len(rgChildren) > 0:
		wrapWidth := availWidth - rgOffsetX - rgGap
		boxes, areaW, areaH := r.placeResourceGroupsArea(rgChildren, idx, wrapWidth)
		for i := range boxes {
			boxes[i].geom.x += rgOffsetX
			boxes[i].geom.y += startSize + rgOffsetY
		}
		pg.children = boxes
		pg.geom = rect{0, 0, rgOffsetX + areaW + rgGap, startSize + rgOffsetY + areaH + rgGap}
	case len(otherChildren) > 0:
		y := startSize + tenantPad
		innerW := 0
		for _, cid := range otherChildren {
			child := r.placeContainer(cid, idx, availWidth-2*tenantPad)
			child.geom.x = tenantPad
			child.geom.y = y
			if child.geom.w > innerW {
				innerW = child.geom.w
			}
			pg.children = append(pg.children, child)
			y += child.geom.h + tenantPad
		}
		pg.geom = rect{0, 0, innerW + 2*tenantPad, y}
	default:
		pg.geom = rect{0, 0, 200, startSize + 2*tenantPad}
	}
	return pg
}

// placeResourceGroupLeaf places rgID's own visible members.
func (r *TopologyRenderer) placeResourceGroupLeaf(rgID string, idx *hierarchyIndex, wrapWidth int) placedGroup {
	g := idx.byID[rgID]
	members := idx.rgMembers[rgID]
	pg := placedGroup{id: idx.cellID[rgID], value: groupValue(*g), groupType: g.Type}

	if len(members) == 0 {
		pg.geom = rect{0, 0, 200, startSize + 2*rgContentPad}
		return pg
	}

	flat, composites := r.composeResourceGroup(members, idx, wrapWidth-2*rgContentPad)

	y := startSize + rgContentPad
	innerW := 0

	if len(flat) > 0 {
		cols, rows := grid(len(flat))
		pitchW, pitchH := r.pitchFor(flat)
		for j, n := range flat {
			col, row := snakeIndex(j, cols)
			w, h := r.nodeSizeFor(n)
			pg.members = append(pg.members, placedNode{
				id:     idx.cellID[n.ID],
				value:  r.nodeValue(n),
				style:  r.nodeStyleFor(n),
				geom:   rect{rgContentPad + col*(pitchW+hGap), y + row*(pitchH+vGap), w, h},
				parent: pg.id,
			})
		}
		if w := cols*pitchW + maxInt(cols-1, 0)*hGap; w > innerW {
			innerW = w
		}
		y += rows*pitchH + maxInt(rows-1, 0)*vGap + vGap
	}

	if len(composites) > 0 {
		packed, compW, compH := packWrapped(composites, hGap, wrapWidth-2*rgContentPad)
		for i := range packed {
			packed[i].geom.x += rgContentPad
			packed[i].geom.y += y
		}
		pg.children = append(pg.children, packed...)
		if compW > innerW {
			innerW = compW
		}
		y += compH
	} else {
		y -= vGap // no composite row follows the flat icons; drop the trailing gap
	}

	pg.geom = rect{0, 0, innerW + 2*rgContentPad, y + rgContentPad}
	return pg
}

// childContainerTypes are OSIRIS JSON resource types whose own real
// "contains" edges to other visible members of the same resource group
// get nested into a titled container box, named after the parent
// resource, instead of cluttering the resource group with a flat icon
// per child.
// Built only from the document's own parent -> child "contains" edge,
// same discipline as the VNet/subnet and VM/NIC composites below: a
// parent with zero such edges among this resource group's own members
// simply stays a flat icon, never assumed empty.
var childContainerTypes = map[string]bool{
	"osiris.azure.emailservice": true,
	"osiris.azure.sqlmi":        true,
	"osiris.azure.sqlserver":    true,
	"compute.cluster":           true,
}

// composeResourceGroup partitions members into plain flat icons and
// edge-confirmed composite containers:
//
//   - a VNet nests every subnet the document's own "contains" edge
//     (subnet -> vnet) assigns it. A subnet stays visually empty inside
//     (the subnet can remain visually empty, with attachment edges
//     terminating on its boundary" a resource that attaches to it is
//     drawn where it actually lives, at the resource-group level, with
//     an edge into the subnet's own boundary, not nested inside it).
//   - a compute.vm or osiris.azure.sqlvm nests every network.interface
//     a real document edge directly connects it to, in a small
//     "Virtual Machine <name>" wrapper built only from that edge,
//     never from naming or co-location: an OSIRIS JSON consumer must
//     never infer or invent what doesn't exist in the JSON document.
//   - a network.security.group with a real edge to a subnet or to a NIC
//     that ends up drawn (flat or inside a VM wrapper) becomes a small
//     boundary badge at that specific endpoint's own bottom-left corner
//     instead of its own icon one badge per real association, so an
//     NSG associated with several subnets/NICs gets one badge at each,
//     never merged into a single ambiguous placement. An NSG with zero
//     real associations stays a flat icon (never silently dropped).
//   - a childContainerTypes resource (an email service, a SQL Managed
//     Instance) nests every member this resource group's own "contains"
//     edges assign it as a child, in a titled container named after the
//     parent resource.
//
// Every composed edge is recorded in r.consumedEdgeIDs so Render's
// generic edge-line loop does not also draw a redundant line for the
// same relationship the composite already shows structurally.
func (r *TopologyRenderer) composeResourceGroup(members []topology.Node, idx *hierarchyIndex, wrapWidth int) (flat []topology.Node, composites []placedGroup) {
	var vnets, subnets, vmLikes, nics, nsgs, rest []topology.Node
	for _, n := range members {
		switch {
		case n.Type == "network.vpc":
			vnets = append(vnets, n)
		case n.Type == "network.subnet":
			subnets = append(subnets, n)
		case n.Type == "compute.vm" || n.Type == "osiris.azure.sqlvm":
			vmLikes = append(vmLikes, n)
		case n.Type == "network.interface":
			nics = append(nics, n)
		case n.Type == "network.security.group":
			nsgs = append(nsgs, n)
		default:
			rest = append(rest, n)
		}
	}

	restByID := make(map[string]topology.Node, len(rest))
	for _, n := range rest {
		restByID[n.ID] = n
	}
	consumedByChildContainer := make(map[string]bool)
	var childContainers []placedGroup
	for _, parent := range rest {
		if !childContainerTypes[parent.Type] {
			continue
		}
		var children []topology.Node
		for _, e := range r.model.Edges {
			if e.Type != "contains" || e.Source != parent.ID {
				continue
			}
			child, ok := restByID[e.Target]
			if !ok || consumedByChildContainer[child.ID] {
				continue
			}
			children = append(children, child)
			consumedByChildContainer[child.ID] = true
			r.markEdgeConsumed(parent.ID, child.ID)
		}
		if len(children) == 0 {
			continue
		}
		consumedByChildContainer[parent.ID] = true
		childContainers = append(childContainers, r.placeChildContainer(parent, children, idx))
	}
	if len(childContainers) > 0 {
		filtered := rest[:0:0]
		for _, n := range rest {
			if !consumedByChildContainer[n.ID] {
				filtered = append(filtered, n)
			}
		}
		rest = filtered
	}

	consumedNIC := make(map[string]bool)
	consumedVM := make(map[string]bool)
	nsgBadgeSeq := make(map[string]int) // NSG ID -> next badge sequence number; a present key means "has at least one badge"

	var vmWrappers []placedGroup
	for _, vm := range vmLikes {
		var attached []topology.Node
		for _, nbID := range r.neighborsOfType(vm.ID, "network.interface") {
			nic, ok := idx.nodeByID[nbID]
			if !ok || consumedNIC[nic.ID] {
				continue
			}
			attached = append(attached, nic)
		}
		if len(attached) == 0 {
			continue
		}
		for _, nic := range attached {
			consumedNIC[nic.ID] = true
		}
		consumedVM[vm.ID] = true
		vmWrappers = append(vmWrappers, r.placeVMWrapper(vm, attached, idx, nsgBadgeSeq))
	}

	vnetBoxes, consumedSubnet := r.composeVNets(vnets, subnets, idx, nsgBadgeSeq, wrapWidth)

	composites = append(composites, vnetBoxes...)
	composites = append(composites, vmWrappers...)
	composites = append(composites, childContainers...)

	flat = append(flat, rest...)
	for _, vm := range vmLikes {
		if !consumedVM[vm.ID] {
			flat = append(flat, vm)
		}
	}
	for _, nic := range nics {
		if !consumedNIC[nic.ID] {
			flat = append(flat, nic)
		}
	}
	for _, nsg := range nsgs {
		if _, ok := nsgBadgeSeq[nsg.ID]; !ok {
			flat = append(flat, nsg)
		}
	}
	for _, s := range subnets {
		if !consumedSubnet[s.ID] {
			// No VNet in this resource group claims it via a real
			// "contains" edge still shown, as a flat icon, rather
			// than silently dropped.
			flat = append(flat, s)
		}
	}

	return r.clusterByAdjacency(flat), composites
}

// clusterByAdjacency reorders flat so two members with a real document
// edge between them land as close together as possible in the eventual
// row-major icon grid.
func (r *TopologyRenderer) clusterByAdjacency(flat []topology.Node) []topology.Node {
	if len(flat) < 2 {
		return flat
	}
	inFlat := make(map[string]bool, len(flat))
	byID := make(map[string]topology.Node, len(flat))
	for _, n := range flat {
		inFlat[n.ID] = true
		byID[n.ID] = n
	}
	visited := make(map[string]bool, len(flat))
	ordered := make([]topology.Node, 0, len(flat))
	var visit func(id string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		ordered = append(ordered, byID[id])
		for _, nb := range r.neighbors[id] {
			if inFlat[nb] && !visited[nb] {
				visit(nb)
			}
		}
	}
	for _, n := range flat {
		visit(n.ID)
	}
	return ordered
}

// composeVNets builds one VNet container per entry in vnets, nesting
// every subnet a real "contains" edge assigns it.
// Shared by composeResourceGroup and placeUngrouped: a VNet with no
// resource-group membership of its own, typically a cross-subscription
// peered network this document only knows about via a network.peering
// edge to it, not a resource this subscription actually owns still
// renders as a (possibly empty) container, exactly like one that does,
// rather than a flat icon.
func (r *TopologyRenderer) composeVNets(vnets, subnets []topology.Node, idx *hierarchyIndex, nsgBadgeSeq map[string]int, wrapWidth int) (boxes []placedGroup, consumedSubnet map[string]bool) {
	consumedSubnet = make(map[string]bool)
	for _, vnet := range vnets {
		var ownSubnets []topology.Node
		for _, s := range subnets {
			if consumedSubnet[s.ID] {
				continue
			}
			if r.hasContainsEdge(s.ID, vnet.ID) {
				ownSubnets = append(ownSubnets, s)
				consumedSubnet[s.ID] = true
				r.markEdgeConsumed(s.ID, vnet.ID)
			}
		}
		boxes = append(boxes, r.placeVNetContainer(vnet, ownSubnets, idx, nsgBadgeSeq, wrapWidth))
	}
	return boxes, consumedSubnet
}

// crossSubscriptionBucket is one foreign subscription's worth of
// same-tenant cross-subscription network.vpc/network.subnet resources
// grouped under one subscription-style container instead of each
// rendering as its own independent box in the ungrouped area.
// subscriptionID is the raw GUID: the OSIRI JSON producer for Microsoft
// Azure does not currently emit a friendly subscription name for one it
// only knows about via properties.subscription_id.
type crossSubscriptionBucket struct {
	subscriptionID string
	vnets, subnets []topology.Node
}

// placeCrossSubscriptionBucket renders b as a subscription-style
// container (same visual family as a real logical.subscription group),
// nesting its VNets and any subnet a real "contains" edge assigns one
// of them via composeVNets, same as a resource group's own VNets.
// Any subnet with no matching VNet inside this bucket is returned
// separately rather than dropped, for the caller to place as a flat
// icon alongside this box.
func (r *TopologyRenderer) placeCrossSubscriptionBucket(b crossSubscriptionBucket, idx *hierarchyIndex, wrapWidth int) (box placedGroup, leftoverSubnets []topology.Node) {
	innerWrap := wrapWidth - 2*tenantPad
	vnetBoxes, consumedSubnet := r.composeVNets(b.vnets, b.subnets, idx, make(map[string]int), innerWrap)
	for _, s := range b.subnets {
		if !consumedSubnet[s.ID] {
			leftoverSubnets = append(leftoverSubnets, s)
		}
	}
	packed, innerW, innerH := packWrapped(vnetBoxes, hGap, innerWrap)
	for i := range packed {
		packed[i].geom.x += tenantPad
		packed[i].geom.y += startSize + tenantPad
	}
	box = placedGroup{
		id:        "xsub-" + b.subscriptionID,
		groupType: "azure.crosssubscription",
		geom:      rect{0, 0, innerW + 2*tenantPad, startSize + tenantPad + innerH + tenantPad},
		children:  packed,
	}
	return box, leftoverSubnets
}

// hasContainsEdge reports whether the document asserts a literal
// "contains" connection from subnetID to vnetID the only signal this
// package uses to nest a subnet inside a VNet, never inferred from
// naming or a shared resource group.
func (r *TopologyRenderer) hasContainsEdge(subnetID, vnetID string) bool {
	for _, e := range r.model.Edges {
		if e.Type == "contains" && e.Source == subnetID && e.Target == vnetID {
			return true
		}
	}
	return false
}

// markEdgeConsumed finds the edge directly connecting idA and idB
// (either direction) and records its connection ID in
// r.consumedEdgeIDs, so Render's generic edge-line loop skips it the
// relationship is already shown structurally (a badge, or a wrapper).
// A no-op when idB is empty (defensive: callers that have already
// resolved a real neighbor never pass one).
func (r *TopologyRenderer) markEdgeConsumed(idA, idB string) {
	if idB == "" {
		return
	}
	for _, e := range r.model.Edges {
		if (e.Source == idA && e.Target == idB) || (e.Source == idB && e.Target == idA) {
			r.consumedEdgeIDs[e.ID] = true
		}
	}
}

// placeChildContainer nests children in the same icon grid a flat
// resource-group section would use (grid/pitchFor/nodeSizeFor), inside
// a titled container box named after parent (childContainerTypes).
// The grid starts below the container's own header bar, same inset
// (rgContentPad) a resource group uses around its own member grid.
func (r *TopologyRenderer) placeChildContainer(parent topology.Node, children []topology.Node, idx *hierarchyIndex) placedGroup {
	pg := placedGroup{id: idx.cellID[parent.ID], value: r.nodeValue(parent), groupType: parent.Type}
	cols, rows := grid(len(children))
	pitchW, pitchH := r.pitchFor(children)
	for j, n := range children {
		col, row := snakeIndex(j, cols)
		w, h := r.nodeSizeFor(n)
		pg.members = append(pg.members, placedNode{
			id:     idx.cellID[n.ID],
			value:  r.nodeValue(n),
			style:  r.nodeStyleFor(n),
			geom:   rect{rgContentPad + col*(pitchW+hGap), startSize + rgContentPad + row*(pitchH+vGap), w, h},
			parent: pg.id,
		})
	}
	innerW := cols*pitchW + maxInt(cols-1, 0)*hGap
	innerH := rows*pitchH + maxInt(rows-1, 0)*vGap
	pg.geom = rect{0, 0, innerW + 2*rgContentPad, startSize + rgContentPad + innerH + rgContentPad}
	return pg
}

// placeVNetContainer nests subnets inside vnet, each separated by hGap
// and inset from the VNet's own edges by hGap too, wrapped at wrapWidth
// via packWrapped rather than one unbounded row. A VNet with no
// confirmed subnet still renders as an empty container at a reasonable
// default size, never dropped.
func (r *TopologyRenderer) placeVNetContainer(vnet topology.Node, subnets []topology.Node, idx *hierarchyIndex, nsgBadgeSeq map[string]int, wrapWidth int) placedGroup {
	pg := placedGroup{id: idx.cellID[vnet.ID], value: r.nodeValue(vnet), groupType: "network.vpc"}
	if len(subnets) == 0 {
		pg.geom = rect{0, 0, 200, startSize + 2*rgContentPad}
		return pg
	}
	var subnetBoxes []placedGroup
	for _, s := range subnets {
		subnetBoxes = append(subnetBoxes, r.placeSubnetContainer(s, idx, nsgBadgeSeq))
	}
	packed, packedW, packedH := packWrapped(subnetBoxes, hGap, maxInt(wrapWidth-2*hGap, subnetMinW))
	for i := range packed {
		packed[i].geom.x += hGap
		packed[i].geom.y += startSize + rgContentPad
	}
	pg.children = append(pg.children, packed...)
	pg.geom = rect{0, 0, packedW + 2*hGap, startSize + rgContentPad + packedH + rgContentPad}
	return pg
}

// placeSubnetContainer returns subnet's own container: a fixed-size,
// visually empty box except for any NSG boundary badge a real document
// edge assigns it.
func (r *TopologyRenderer) placeSubnetContainer(subnet topology.Node, idx *hierarchyIndex, nsgBadgeSeq map[string]int) placedGroup {
	pg := placedGroup{id: idx.cellID[subnet.ID], value: r.nodeValue(subnet), groupType: "network.subnet"}
	pg.geom = rect{0, 0, subnetMinW, subnetMinH}
	pg.members = r.subnetNSGBadges(idx, subnet.ID, pg.id, nsgBadgeSeq)
	return pg
}

// subnetNSGBadges returns one badge, nested inside subnetCellID itself
// at its own bottom-left corner, for every network.security.group a
// real document edge connects to subnetID.
func (r *TopologyRenderer) subnetNSGBadges(idx *hierarchyIndex, subnetID, subnetCellID string, nsgBadgeSeq map[string]int) []placedNode {
	var badges []placedNode
	for _, nbID := range r.neighborsOfType(subnetID, "network.security.group") {
		nsg, ok := idx.nodeByID[nbID]
		if !ok {
			continue
		}
		seq := nsgBadgeSeq[nsg.ID]
		badges = append(badges, placedNode{
			id:     badgeCellID(idx.cellID[nsg.ID], seq),
			value:  r.nodeValue(nsg),
			style:  nsgBadgeStyle,
			geom:   rect{nsgBadgeX, subnetMinH - nsgBadgeH, nsgBadgeW, nsgBadgeH},
			parent: subnetCellID,
		})
		r.markEdgeConsumed(subnetID, nsg.ID)
		nsgBadgeSeq[nsg.ID] = seq + 1
	}
	return badges
}

// siblingNSGBadges returns one badge, a sibling of anchorID positioned
// at anchorGeom's own bottom-left corner, for every
// network.security.group a real document edge connects to anchorID
// used for a NIC drawn inside a VM wrapper (there is no container to
// nest a badge inside; the NIC is a plain icon, so the badge sits next
// to it instead, in the same parent).
func (r *TopologyRenderer) siblingNSGBadges(idx *hierarchyIndex, anchorID string, anchorGeom rect, parentCellID string, nsgBadgeSeq map[string]int) []placedNode {
	var badges []placedNode
	for _, nbID := range r.neighborsOfType(anchorID, "network.security.group") {
		nsg, ok := idx.nodeByID[nbID]
		if !ok {
			continue
		}
		seq := nsgBadgeSeq[nsg.ID]
		badges = append(badges, placedNode{
			id:     badgeCellID(idx.cellID[nsg.ID], seq),
			value:  r.nodeValue(nsg),
			style:  nsgBadgeStyle,
			geom:   rect{anchorGeom.x + nsgBadgeX, anchorGeom.y + anchorGeom.h - nsgBadgeH, nsgBadgeW, nsgBadgeH},
			parent: parentCellID,
		})
		r.markEdgeConsumed(anchorID, nsg.ID)
		nsgBadgeSeq[nsg.ID] = seq + 1
	}
	return badges
}

// badgeCellID returns base for an NSG's first badge (so any other real
// edge involving it, should one ever exist, can still resolve to a
// real emitted cell) and a suffixed, unique ID for every further badge
// the same NSG needs (one real resource, several real associations).
func badgeCellID(base string, seq int) string {
	if seq == 0 {
		return base
	}
	return base + "-nsg" + strconv.Itoa(seq)
}

// placeVMWrapper nests vm and every NIC a real document edge connected
// it with, left to right, inside a small "Virtual Machine <name>" box.
func (r *TopologyRenderer) placeVMWrapper(vm topology.Node, nics []topology.Node, idx *hierarchyIndex, nsgBadgeSeq map[string]int) placedGroup {
	pg := placedGroup{
		id:        "vmw-" + idx.cellID[vm.ID],
		value:     vmWrapperLabel(vm.Label),
		groupType: "azure.vmwrapper",
	}
	items := append([]topology.Node{vm}, nics...)
	x := vmWrapperPadX
	maxH := 0
	for _, n := range items {
		w, h := r.nodeSizeFor(n)
		nodeGeom := rect{x, vmWrapperPadY, w, h}
		pg.members = append(pg.members, placedNode{
			id:     idx.cellID[n.ID],
			value:  r.nodeValue(n),
			style:  r.nodeStyleFor(n),
			geom:   nodeGeom,
			parent: pg.id,
		})
		if n.ID != vm.ID {
			// The VM<->NIC edge itself is now shown structurally (both
			// drawn together in this same wrapper) never also drawn
			// as a redundant line.
			r.markEdgeConsumed(vm.ID, n.ID)
		}
		pg.members = append(pg.members, r.siblingNSGBadges(idx, n.ID, nodeGeom, pg.id, nsgBadgeSeq)...)
		if h > maxH {
			maxH = h
		}
		x += w + vmWrapperGapX
	}
	pg.geom = rect{0, 0, x - vmWrapperGapX + vmWrapperPadX, vmWrapperPadY + maxH + vmWrapperPadY}
	return pg
}

// placeResourceGroupsArea packs rgIDs some populated, some empty of
// any visible member into one flow-wrapped area: populated resource
// groups first (packWrapped at wrapWidth the usable width this area's
// own container was actually given, see placeContainer), then every
// empty resource group collapsed to a title-only box, packed the same
// way, in its own row or rows below them.
// Returned boxes are positioned relative to the area's own origin
// (0, 0); the caller offsets them by (rgOffsetX, startSize+rgOffsetY).
func (r *TopologyRenderer) placeResourceGroupsArea(rgIDs []string, idx *hierarchyIndex, wrapWidth int) (boxes []placedGroup, w, h int) {
	var populated, empty []string
	for _, id := range rgIDs {
		if len(idx.rgMembers[id]) > 0 {
			populated = append(populated, id)
		} else {
			empty = append(empty, id)
		}
	}

	var populatedBoxes []placedGroup
	for _, id := range populated {
		populatedBoxes = append(populatedBoxes, r.placeResourceGroupLeaf(id, idx, wrapWidth))
	}
	packedPopulated, wPop, hPop := packWrapped(populatedBoxes, rgGap, wrapWidth)
	boxes = append(boxes, packedPopulated...)

	yOffset := hPop
	if len(populated) > 0 && len(empty) > 0 {
		yOffset += rgGap
	}
	var emptyBoxes []placedGroup
	for _, id := range empty {
		emptyBoxes = append(emptyBoxes, r.placeEmptyResourceGroup(id, idx))
	}
	packedEmpty, wEmpty, hEmpty := packWrapped(emptyBoxes, rgGap, wrapWidth)
	for i := range packedEmpty {
		packedEmpty[i].geom.y += yOffset
	}
	boxes = append(boxes, packedEmpty...)

	return boxes, maxInt(wPop, wEmpty), yOffset + hEmpty
}

// packWrapped packs boxes into rows ("shelves") wrapped at wrapWidth,
// gap apart on both axes, using First-Fit Decreasing Height: boxes are
// placed tallest first, each one on the first existing shelf with
// enough remaining width whose own height (set by the first, tallest
// box placed on it) is at least as tall, or a new shelf of its own
// height when none fits. Ties on height keep the input slice's own
// order (sort.SliceStable), so this stays deterministic for a given,
// already-deterministically-ordered input.
func packWrapped(boxes []placedGroup, gap, wrapWidth int) (packed []placedGroup, w, h int) {
	if len(boxes) == 0 {
		return nil, 0, 0
	}
	type shelf struct{ y, height, cursorX int }

	order := make([]int, len(boxes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return boxes[order[a]].geom.h > boxes[order[b]].geom.h
	})

	var shelves []shelf
	for _, i := range order {
		box := boxes[i]
		placed := false
		for s := range shelves {
			sh := &shelves[s]
			nextX := sh.cursorX
			if nextX > 0 {
				nextX += gap
			}
			if nextX+box.geom.w <= wrapWidth && box.geom.h <= sh.height {
				box.geom.x = nextX
				box.geom.y = sh.y
				sh.cursorX = nextX + box.geom.w
				packed = append(packed, box)
				if sh.cursorX > w {
					w = sh.cursorX
				}
				placed = true
				break
			}
		}
		if !placed {
			y := 0
			if n := len(shelves); n > 0 {
				y = shelves[n-1].y + shelves[n-1].height + gap
			}
			box.geom.x = 0
			box.geom.y = y
			shelves = append(shelves, shelf{y: y, height: box.geom.h, cursorX: box.geom.w})
			packed = append(packed, box)
			if box.geom.w > w {
				w = box.geom.w
			}
		}
	}
	if n := len(shelves); n > 0 {
		h = shelves[n-1].y + shelves[n-1].height
	}
	return packed, w, h
}

// placeEmptyResourceGroup returns the title-only box for a resource
// group with no visible member. Its width is an approximation of the
// title's own rendered width (real font-metric measurement is not
// implemented an explicitly flagged open item, not a considered
// design choice).
func (r *TopologyRenderer) placeEmptyResourceGroup(rgID string, idx *hierarchyIndex) placedGroup {
	g := idx.byID[rgID]
	w := clampInt(emptyRGWidthBase+emptyRGWidthPerChar*len(g.Label), emptyRGMinW, emptyRGMaxW)
	return placedGroup{
		id:        idx.cellID[rgID],
		value:     groupValue(*g),
		groupType: g.Type,
		geom:      rect{0, 0, w, emptyRGHeight},
	}
}

// grid returns the column and row count for laying out n cells in as
// square a grid as possible, deterministically.
func grid(n int) (cols, rows int) {
	if n <= 0 {
		return 1, 1
	}
	cols = 1
	for cols*cols < n {
		cols++
	}
	rows = (n + cols - 1) / cols
	return cols, rows
}

// snakeIndex returns the (col, row) position for sequence index j in a
// cols-wide grid, alternating fill direction every row (boustrophedon/
// "snake" order) instead of always left to right: the item at the end
// of one row sits directly above/below the item at the start of the
// next row, rather than diagonally opposite across the grid. Used
// wherever a grid is filled from a sequence clusterByAdjacency already
// ordered for locality.
func snakeIndex(j, cols int) (col, row int) {
	row = j / cols
	col = j % cols
	if row%2 == 1 {
		col = cols - 1 - col
	}
	return col, row
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
