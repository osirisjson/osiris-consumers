// This file renders the Azure topology as draw.io (mxGraph) XML: a
// provenance badge, a top-right title carrying the document's own
// scope name and generic counts, the Azure hierarchy (management
// group -> subscription -> resource group) as nested containers with
// each resource drawn as its azure2 icon, and a metadata footer band
// replacing the first round's generic-swimlane-plus-icons approach
// with a purpose-built layout.
//
// This is a self-contained renderer, not a per-node style hook layered
// onto the generic one. It consumes the same topology.Topology model
// and graph.Graph the generic renderer would have used (render.go
// builds the model unconditionally before dispatch), and the generic
// model's own node/edge/group counts, including every hidden-type
// resource and every container.region grouping this page does not draw
// are what the title reports: hiding a type from this curated page
// never changes what the generic model counts.
//
// Cell identifiers and geometry come from the model's own sorted
// traversal order, never a clock or a random source, so identical model
// plus identical artifact time produce byte-identical output.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/drawio"
	"go.osirisjson.org/consumers/internal/topology"
)

// Page geometry, in mxGraph user units.
const (
	pageMargin      = 20
	pageMarginRight = 16
	headerH         = 60 // vertical space reserved for the logo/title row before content starts
	titleW          = 480
	titleH          = 40

	footerBandH    = 54 // reserved between the content's own bottom edge and the page's bottom edge
	footerTopGap   = 10 // gap between the content's bottom edge and the footer text row's own top
	footerH        = 30 // the footer text row's own height
	footerBlockGap = 40 // horizontal gap between adjacent footer text blocks

	footerOsirisW    = 649 // "OSIRIS JSON" credit line fixed text, never data-dependent
	footerTimestampW = 156
	footerScopeW     = 116
	footerGeneratorW = 239

	iconSlotW = 160 // packing pitch for a member-icon grid: room for the label below the icon
	iconSlotH = 90

	nodeW = 200 // fallback plain-rectangle size for an unrecognized ARM type
	nodeH = 60
)

// TopologyRenderer renders an Azure-enriched draw.io topology page.
// The zero value is not usable; construct one with NewTopologyRenderer.
type TopologyRenderer struct {
	model             *topology.Topology
	armType           map[string]string
	nodeType          map[string]string
	extraLabel        map[string]string
	neighbors         map[string][]string
	tenant            map[string]string
	subscription      map[string]string
	crossSubscription map[string]bool
	consumedEdgeIDs   map[string]bool
	scope             azureScope
	metadata          document.Metadata
	artifact          time.Time
}

// azureScope is "metadata.scope" as this package discloses it, the
// topology page's title/footer only need Name/Purpose, but the report's
// Signature section (report_render.go) shows every field here, so this
// one decode covers both renderers.
type azureScope struct {
	Name          string   `json:"name"`
	Purpose       string   `json:"purpose"`
	Providers     []string `json:"providers"`
	Accounts      []string `json:"accounts"`
	Regions       []string `json:"regions"`
	Subscriptions []string `json:"subscriptions"`
}

func decodeAzureScope(raw json.RawMessage) azureScope {
	var s azureScope
	if len(raw) == 0 {
		return s
	}
	_ = json.Unmarshal(raw, &s)
	return s
}

// NewTopologyRenderer returns a renderer for model. g supplies each
// node's ARM native type (resource.provider.type) for the icon lookup;
// md supplies the title's scope name and the footer's snapshot
// metadata; artifactTime is currently accepted for API consistency with
// every other capability renderer (never read the wall clock inside a
// renderer) but this page's design has no "generated at" line of its
// own the footer's own "Snapshot created on" is the document's own
// metadata.timestamp instead.
func NewTopologyRenderer(model *topology.Topology, g *graph.Graph, md document.Metadata, artifactTime time.Time) *TopologyRenderer {
	armType := make(map[string]string, len(model.Nodes))
	nodeType := make(map[string]string, len(model.Nodes))
	extraLabel := make(map[string]string, len(model.Nodes))
	tenant := make(map[string]string, len(model.Nodes))
	subscription := make(map[string]string, len(model.Nodes))
	crossSubscription := make(map[string]bool, len(model.Nodes))
	for _, n := range model.Nodes {
		res, ok := g.Resources[n.ID]
		if ok {
			armType[n.ID] = res.Provider.Type
			tenant[n.ID] = res.Provider.Tenant
			subscription[n.ID] = res.Provider.Subscription
			if v, vok := azurePropertyString(decodeAzureProperties(res.Properties), "cross_subscription"); vok && v == "true" {
				crossSubscription[n.ID] = true
			}
		}
		nodeType[n.ID] = n.Type
		if ok {
			if lines := propertyLabelLines(n.Type, res.Properties); lines != "" {
				extraLabel[n.ID] = lines
			}
		}
	}
	neighbors := make(map[string][]string, len(model.Nodes))
	for _, e := range model.Edges {
		if !e.SourceResolved || !e.TargetResolved {
			continue
		}
		neighbors[e.Source] = append(neighbors[e.Source], e.Target)
		neighbors[e.Target] = append(neighbors[e.Target], e.Source)
	}
	return &TopologyRenderer{
		model:             model,
		armType:           armType,
		nodeType:          nodeType,
		extraLabel:        extraLabel,
		neighbors:         neighbors,
		tenant:            tenant,
		subscription:      subscription,
		crossSubscription: crossSubscription,
		consumedEdgeIDs:   make(map[string]bool),
		scope:             decodeAzureScope(md.Scope),
		metadata:          md,
		artifact:          artifactTime.UTC(),
	}
}

// neighborsOfType returns every node directly edge-connected to id
// (either direction) whose OSIRIS JSON type is exactly wantType.
func (r *TopologyRenderer) neighborsOfType(id, wantType string) []string {
	var out []string
	for _, nb := range r.neighbors[id] {
		if r.nodeType[nb] == wantType {
			out = append(out, nb)
		}
	}
	return out
}

// propertyLabelLines returns the "<br>"-joined extra label lines a
// node's own "properties" object contributes under its name.
// A property the document does not carry for this resource is
// omitted entirely (not shown as "(absent)") unlike a report table,
// a diagram label has no fixed column to disclose absence in, and
// omitting an optional line keeps the diagram legible. Any other
// OSIRIS JSON type contributes no extra line.
func propertyLabelLines(osirisType string, rawProperties json.RawMessage) string {
	var keys []string
	switch osirisType {
	case "network.interface":
		keys = []string{"private_ip"}
	case "osiris.azure.publicip":
		keys = []string{"primary", "ip_address", "sku"}
	case "osiris.azure.aks.nodepool":
		keys = []string{"cluster_name", "orchestrator_version", "os_sku", "os_type", "vm_size"}
	default:
		return ""
	}
	props := decodeAzureProperties(rawProperties)
	var lines []string
	for _, k := range keys {
		if v, ok := azurePropertyString(props, k); ok {
			lines = append(lines, escapeHTML(v))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "<br>" + strings.Join(lines, "<br>")
}

// decodeAzureProperties decodes a resource's raw "properties" object
// best-effort; a malformed or absent value yields nil, never an error
// (the receipt already gates malformed input at the document level).
func decodeAzureProperties(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(raw, &obj)
	return obj
}

// azurePropertyString returns key's display text and true when props
// carries it, or false when it does not the caller decides how to
// treat absence (a diagram label omits the line; a report cell would
// show "(absent)"). Both a quoted string and a bare JSON scalar
// (number, bool) are handled.
func azurePropertyString(props map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := props[key]
	if !ok || len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	return string(raw), true
}

// ChooseTopologyPage returns the page size a TopologyRenderer built
// from model will choose, so the facade can report it without
// rendering twice.
func ChooseTopologyPage(model *topology.Topology, g *graph.Graph) PageInfo {
	r := NewTopologyRenderer(model, g, document.Metadata{}, time.Time{})
	idx := buildHierarchyIndex(model)
	_, _, _, page := r.layoutTopology(idx)
	return page
}

// layoutTopology runs the full placement, choosing the smallest ISO
// landscape page (A3..A0) whose own usable width the resource-group
// grid is wrapped to fill, instead of laying out once at a fixed guess
// width and accepting whatever page that guess happens to need: a
// document that only needs A2 should actually use A2's width, not wrap
// at a narrower guess and leave the rest of the page blank.
// Falls back to a custom landscape page, wrapped at the largest ISO
// candidate's own usable width, when even that candidate is not
// tall enough.
func (r *TopologyRenderer) layoutTopology(idx *hierarchyIndex) (roots, ungroupedBoxes []placedGroup, ungrouped []placedNode, page PageInfo) {
	layoutAt := func(pageW int) (roots, ungroupedBoxes []placedGroup, ungrouped []placedNode, totalW, totalH int) {
		availWidth := pageW - pageMargin - pageMarginRight
		roots, contentW, contentH := r.placeRoots(idx, pageMargin, pageMargin+headerH, availWidth)
		ungroupedBoxes, ungrouped, ungroupedW, ungroupedH := r.placeUngrouped(idx, pageMargin, pageMargin+headerH+contentH, availWidth)
		totalW = maxInt(contentW, ungroupedW) + pageMargin + pageMarginRight
		totalH = headerH + contentH + ungroupedH + footerBandH
		return roots, ungroupedBoxes, ungrouped, totalW, totalH
	}
	for _, cand := range isoLandscape {
		roots, ungroupedBoxes, ungrouped, totalW, totalH := layoutAt(cand.Width)
		if totalW <= cand.Width && totalH <= cand.Height {
			return roots, ungroupedBoxes, ungrouped, cand
		}
	}
	last := isoLandscape[len(isoLandscape)-1]
	roots, ungroupedBoxes, ungrouped, totalW, totalH := layoutAt(last.Width)
	return roots, ungroupedBoxes, ungrouped, choosePage(totalW, totalH)
}

// Render writes the draw.io XML to w. It checks ctx at collection
// boundaries and returns the first write error without wrapping it
// away.
func (r *TopologyRenderer) Render(ctx context.Context, w io.Writer) error {
	cw := render.NewCheckedWriter(w)
	p := &xmlPrinter{w: cw}
	m := r.model

	idx := buildHierarchyIndex(m)
	roots, ungroupedBoxes, ungrouped, page := r.layoutTopology(idx)

	p.raw(`<mxfile host="osirisjson-consumer docs.osirisjson.org/osiris-consumers/getting-started">` + "\n")
	p.raw(`  <diagram id="osirisjson-azure-topology" name="OSIRIS JSON Microsoft Azure topology">` + "\n")
	p.raw(fmt.Sprintf(
		`    <mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="0" arrows="1" fold="1" page="1" pageScale="1" pageWidth="%d" pageHeight="%d" math="0" shadow="0">`+"\n",
		page.Width, page.Height))
	p.raw(`      <root>` + "\n")
	p.raw(`        <mxCell id="0" />` + "\n")
	p.raw(`        <mxCell id="1" parent="0" />` + "\n")

	p.cell("logo", "1", drawio.LogoStyle, "", geomRect(rect{pageMargin, pageMargin, drawio.LogoIntrinsicW, drawio.LogoIntrinsicH}))
	p.cell("title", "1", titleStyle, r.titleValue(), geomRect(rect{page.Width - pageMargin - titleW, pageMargin, titleW, titleH}))
	r.emitFooter(p, page.Width, page.Height)

	if err := ctx.Err(); err != nil {
		return err
	}

	drawn := make(map[string]bool, len(idx.cellID))

	var emitContainer func(box placedGroup, parentID string) error
	emitContainer = func(box placedGroup, parentID string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		style, iconPath := containerStyle(box.groupType)
		p.cell(box.id, parentID, style, box.value, geomRect(box.geom))
		if iconPath != "" {
			iw, ih := containerIconSize(iconPath)
			p.cell(box.id+"-icon", box.id, groupIconStyle(iconPath), "", geomRect(rect{7, 6, iw, ih}))
		}
		drawn[box.id] = true
		for _, n := range box.members {
			p.cell(n.id, n.parent, n.style, n.value, geomRect(n.geom))
			drawn[n.id] = true
		}
		for _, c := range box.children {
			if err := emitContainer(c, box.id); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range roots {
		if err := emitContainer(root, "1"); err != nil {
			return err
		}
	}
	for _, box := range ungroupedBoxes {
		if err := emitContainer(box, "1"); err != nil {
			return err
		}
	}
	for _, n := range ungrouped {
		p.cell(n.id, "1", n.style, n.value, geomRect(n.geom))
		drawn[n.id] = true
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// One edge per connection whose both endpoints this page actually
	// drew. A hidden-type endpoint (icons.go's hiddenResourceTypes) or
	// an endpoint this page never placed for any other reason curates
	// the edge off this page too it stays a literal edge in the
	// generic topology.Topology model underneath, just not
	// drawn here.
	edgeSeq := 0
	for _, e := range m.Edges {
		if r.consumedEdgeIDs[e.ID] {
			// Already shown structurally a boundary badge (NSG) or a
			// wrapper nesting both endpoints together (VM<->NIC) a
			// redundant line here would show the same relationship
			// twice.
			continue
		}
		src, srcOK := idx.cellID[e.Source]
		dst, dstOK := idx.cellID[e.Target]
		if !srcOK || !dstOK || !drawn[src] || !drawn[dst] {
			continue
		}
		style, label := r.edgeStyleAndLabel(e)
		p.edge("e"+strconv.Itoa(edgeSeq), src, dst, style, drawio.Attr(label))
		edgeSeq++
		if edgeSeq%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	p.raw(`      </root>` + "\n")
	p.raw(`    </mxGraphModel>` + "\n")
	p.raw(`  </diagram>` + "\n")
	p.raw(`</mxfile>` + "\n")

	return cw.Err()
}

// titleValue is the top-right title: the document's own scope name in
// bold, then the generic model's exact counts the full generic
// count, unaffected by this page's hidden types or skipped region
// containers.
func (r *TopologyRenderer) titleValue() string {
	c := r.model.Counts()
	label := "<b>" + escapeHTML(r.scope.Name) + "</b><div>" +
		fmt.Sprintf("%d nodes, %d edges, %d groups", c.Nodes, c.Edges, c.Groups) + "</div>"
	return drawio.Attr(label)
}

// containerStyle returns the swimlane style and its icon path.
func containerStyle(groupType string) (style, iconPath string) {
	switch {
	case groupType == "logical.managementgroup":
		return managementGroupStyle, mustIconForGroupType(groupType)
	case groupType == "logical.subscription":
		return subscriptionStyle, mustIconForGroupType(groupType)
	case groupType == "logical.resourcegroup":
		return resourceGroupStyle, ""
	case groupType == "network.vpc":
		return vnetSubnetContainerStyle, "networking/Virtual_Networks.svg"
	case groupType == "network.subnet":
		return vnetSubnetContainerStyle, "networking/Subnet.svg"
	case groupType == "azure.vmwrapper":
		return vmWrapperStyle, ""
	case childContainerTypes[groupType]:
		path, _ := iconForOSIRISType(groupType)
		return childContainerStyle, path
	case groupType == "azure.crosssubscription":
		return subscriptionStyle, ""
	default:
		return managementGroupStyle, ""
	}
}

func mustIconForGroupType(groupType string) string {
	path, _ := iconForGroupType(groupType)
	return path
}

// emitFooter writes the snapshot-metadata band: the OSIRIS JSON credit
// line, the document's own snapshot timestamp, scope purpose, and the
// Azure enrichment's own version each shown as the document asserts
// it, "(absent)" when the document carried no such field.
// The three right-hand blocks are right-aligned to the page's own
// pageMarginRight, working backward with a uniform footerBlockGap
// between them, so they land at the confirmed position regardless of
// the page size actually chosen unlike the previous approach
// (proportionally scaling fixed A3 reference offsets), block width no
// longer depends on page size, matching how draw.io text actually
// behaves.
func (r *TopologyRenderer) emitFooter(p *xmlPrinter, pageW, pageH int) {
	ts := stringOrAbsent(r.metadata.Timestamp)
	purpose := stringOrAbsent(r.scope.Purpose)
	genVersion := "(absent)"
	if r.metadata.Generator != nil {
		genVersion = stringOrAbsent(r.metadata.Generator.Version)
	}
	footerY := pageH - footerBandH + footerTopGap
	generatorX := pageW - pageMarginRight - footerGeneratorW
	scopeX := generatorX - footerBlockGap - footerScopeW
	timestampX := scopeX - footerBlockGap - footerTimestampW

	p.cell("footer-osirisjson", "1", footerTextStyle, drawio.Attr("<b>OSIRIS JSON</b><br>This topology was created with OSIRIS JSON. Read more on https://osirisjson.org"),
		geomRect(rect{pageMargin, footerY, footerOsirisW, footerH}))
	p.cell("footer-timestamp", "1", footerTextStyle, drawio.Attr("<div><b>Snapshot created on</b></div><div>"+ts+"</div>"),
		geomRect(rect{timestampX, footerY, footerTimestampW, footerH}))
	p.cell("footer-scope", "1", footerTextStyle, drawio.Attr("<div><b>Snapshot scope</b></div><div>"+purpose+"</div>"),
		geomRect(rect{scopeX, footerY, footerScopeW, footerH}))
	p.cell("footer-generator", "1", footerTextStyle, drawio.Attr("<div><b>OSIRIS JSON for Microsoft Azure</b></div><div>"+genVersion+"</div>"),
		geomRect(rect{generatorX, footerY, footerGeneratorW, footerH}))
}

func stringOrAbsent(s string) string {
	if s == "" {
		return "(absent)"
	}
	return s
}

// escapeHTML escapes the literal HTML metacharacters drawio.Attr's own
// XML-attribute escaping does not: titleValue composes "<b>"/"</b>"
// itself around dynamic text, so the dynamic text must not be allowed
// to inject its own tags before that final escaping pass.
func escapeHTML(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			out = append(out, "&amp;"...)
		case '<':
			out = append(out, "&lt;"...)
		case '>':
			out = append(out, "&gt;"...)
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

// iconPathFor returns n's azure2 icon path and whether one was found,
// looking up its ARM native type recorded at construction time.
// iconPathFor checks an ARM-type-specific entry first (icons.go's
// armTypeIcon e.g. a VM Scale Set's own ARM type overrides the
// generic "compute.cluster" default below), then falls back to n's
// OSIRIS JSON type (osirisTypeIcon e.g. every other compute.cluster
// resource, or one with no recorded ARM type at all).
func (r *TopologyRenderer) iconPathFor(n topology.Node) (string, bool) {
	if armType, ok := r.armType[n.ID]; ok {
		if path, ok := iconForARMType(armType); ok {
			return path, true
		}
	}
	return iconForOSIRISType(n.Type)
}

// nodeStyleFor returns the icon style for n when its ARM native type is
// recognized, or the generic plain rectangle style otherwise.
func (r *TopologyRenderer) nodeStyleFor(n topology.Node) string {
	if style, ok := stencilStyleForOSIRISType(n.Type); ok {
		return style
	}
	if path, ok := r.iconPathFor(n); ok {
		return iconStyle(path)
	}
	return nodeStyle
}

// nodeSizeFor returns the mxCell geometry width/height for n: its own
// icon's real, non-distorting size (icons.go) for a recognized ARM
// type, or the plain nodeW x nodeH rectangle otherwise.
func (r *TopologyRenderer) nodeSizeFor(n topology.Node) (w, h int) {
	if _, ok := stencilStyleForOSIRISType(n.Type); ok {
		return defaultResourceIconSize, defaultResourceIconSize
	}
	if path, ok := r.iconPathFor(n); ok {
		return resourceIconSize(path)
	}
	return nodeW, nodeH
}

// hasCompactVisual reports whether n renders as a small icon-sized
// shape (an azure2 image icon or a vector stencil like the Kubernetes
// node-pool shape) rather than the wide generic fallback rectangle
// the two cases pitchFor packs at the compact iconSlotW/iconSlotH
// pitch.
func (r *TopologyRenderer) hasCompactVisual(n topology.Node) bool {
	if _, ok := stencilStyleForOSIRISType(n.Type); ok {
		return true
	}
	_, ok := r.iconPathFor(n)
	return ok
}

// pitchFor returns the grid packing pitch (column/row stride, before
// hGap/vGap) for a set of nodes laid out together: the icon slot size
// when every one of them drew as an icon (the common case the icon
// itself keeps its own true size, but the slot leaves room for the
// label below it), or the wider generic rectangle pitch when the set
// mixes in an unrecognized type.
func (r *TopologyRenderer) pitchFor(nodes []topology.Node) (w, h int) {
	allIcons := true
	for _, n := range nodes {
		if !r.hasCompactVisual(n) {
			allIcons = false
			break
		}
	}
	if allIcons {
		return iconSlotW, iconSlotH
	}
	return maxInt(iconSlotW, nodeW), maxInt(iconSlotH, nodeH)
}

// placeUngrouped places every non-hidden node with no resource-group
// membership: a network.vpc (and any network.subnet a real "contains"
// edge assigns it) as its own container via composeVNets, exactly like
// one that does belong to a resource group.
// A VNet whose own properties.cross_subscription is true and whose
// provider.tenant matches this document's own tenant
// (metadata.scope.accounts) groups with its siblings under one
// subscription-style container instead, titled by the raw subscription
// ID, the producer has no friendly name for it. Every other ungrouped
// node packs into a plain icon grid below the containers, at
// (x0, y0 + the containers' own height).
func (r *TopologyRenderer) placeUngrouped(idx *hierarchyIndex, x0, y0, wrapWidth int) (boxes []placedGroup, nodes []placedNode, w, h int) {
	placedInRG := make(map[string]bool)
	for _, members := range idx.rgMembers {
		for _, n := range members {
			placedInRG[n.ID] = true
		}
	}
	sameTenant := func(id string) bool {
		t := r.tenant[id]
		if t == "" {
			return false
		}
		for _, acc := range r.scope.Accounts {
			if acc == t {
				return true
			}
		}
		return false
	}

	var vnets, subnets, rest []topology.Node
	bucketOf := make(map[string]*crossSubscriptionBucket)
	var bucketOrder []string
	for _, n := range r.model.Nodes {
		if hiddenResourceTypes[n.Type] || placedInRG[n.ID] {
			continue
		}
		if (n.Type == "network.vpc" || n.Type == "network.subnet") && r.crossSubscription[n.ID] && sameTenant(n.ID) {
			sid := r.subscription[n.ID]
			b, ok := bucketOf[sid]
			if !ok {
				b = &crossSubscriptionBucket{subscriptionID: sid}
				bucketOf[sid] = b
				bucketOrder = append(bucketOrder, sid)
			}
			if n.Type == "network.vpc" {
				b.vnets = append(b.vnets, n)
			} else {
				b.subnets = append(b.subnets, n)
			}
			continue
		}
		switch n.Type {
		case "network.vpc":
			vnets = append(vnets, n)
		case "network.subnet":
			subnets = append(subnets, n)
		default:
			rest = append(rest, n)
		}
	}
	sort.Strings(bucketOrder)

	vnetBoxes, consumedSubnet := r.composeVNets(vnets, subnets, idx, make(map[string]int), wrapWidth)
	for _, s := range subnets {
		if !consumedSubnet[s.ID] {
			rest = append(rest, s)
		}
	}

	var subBoxes []placedGroup
	for _, sid := range bucketOrder {
		box, leftover := r.placeCrossSubscriptionBucket(*bucketOf[sid], idx, wrapWidth)
		box.value = drawio.Attr(sid)
		subBoxes = append(subBoxes, box)
		rest = append(rest, leftover...)
	}

	allBoxes := append(subBoxes, vnetBoxes...)
	if len(allBoxes) == 0 && len(rest) == 0 {
		return nil, nil, 0, 0
	}

	packedBoxes, boxesW, boxesH := packWrapped(allBoxes, hGap, wrapWidth)
	for i := range packedBoxes {
		packedBoxes[i].geom.x += x0
		packedBoxes[i].geom.y += y0
	}

	restY := y0
	if len(packedBoxes) > 0 {
		restY += boxesH + vGap
	}
	w, h = boxesW, boxesH
	if len(rest) > 0 {
		cols, rows := grid(len(rest))
		pitchW, pitchH := r.pitchFor(rest)
		for j, n := range rest {
			col, row := snakeIndex(j, cols)
			nw, nh := r.nodeSizeFor(n)
			nodes = append(nodes, placedNode{
				id:    idx.cellID[n.ID],
				value: r.nodeValue(n),
				style: r.nodeStyleFor(n),
				geom:  rect{x0 + col*(pitchW+hGap), restY + row*(pitchH+vGap), nw, nh},
			})
		}
		restW := cols*pitchW + maxInt(cols-1, 0)*hGap
		restH := rows*pitchH + maxInt(rows-1, 0)*vGap
		w = maxInt(w, restW)
		h = (restY - y0) + restH
	}
	return packedBoxes, nodes, w, h
}

// groupValue is the container label: the group's own name.
func groupValue(g topology.Group) string { return drawio.Attr(g.Label) }

// nodeValue is n's label plus any extra property-derived lines
// propertyLabelLines computed for it, the whole composed string passes
// through drawio.Attr once, matching titleValue's own
// single-escape-point convention, so the literal "<br>" separators
// survive XML entity-encoding and are then interpreted as real line
// breaks by draw.io's html=1 label rendering.
func (r *TopologyRenderer) nodeValue(n topology.Node) string {
	return drawio.Attr(escapeHTML(n.Label) + r.extraLabel[n.ID])
}

// vmWrapperLabel is a "Virtual Machine <name>" wrapper box's own title
// a synthetic label, not the VM resource's own name verbatim (see
// layout.go's placeVMWrapper).
func vmWrapperLabel(vmName string) string {
	return drawio.Attr("Virtual Machine " + escapeHTML(vmName))
}

// --- draw.io styles --------------------------------------------------

const (
	titleStyle = "text;html=1;align=right;verticalAlign=top;whiteSpace=wrap;strokeColor=none;fillColor=none;"
	nodeStyle  = "rounded=0;whiteSpace=wrap;html=1;verticalAlign=middle;align=center;"

	managementGroupStyle = "swimlane;html=1;whiteSpace=wrap;rounded=1;startSize=30;collapsible=0;" +
		"fillColor=#f5f5f5;verticalAlign=middle;align=center;fontSize=10;fontColor=#333333;strokeColor=#666666;"

	subscriptionStyle = "swimlane;html=1;whiteSpace=wrap;rounded=1;startSize=30;collapsible=0;" +
		"fillColor=#fff2cc;verticalAlign=middle;align=center;fontSize=10;strokeColor=#d6b656;"

	resourceGroupStyle = "swimlane;html=1;whiteSpace=wrap;rounded=1;startSize=30;collapsible=0;" +
		"fillColor=#dae8fc;verticalAlign=middle;align=right;fontSize=10;strokeColor=#6c8ebf;spacingRight=5;"

	vnetSubnetContainerStyle = "swimlane;whiteSpace=wrap;html=1;rounded=1;collapsible=0;" +
		"fillColor=#f5f5f5;fontColor=#333333;strokeColor=#666666;align=right;startSize=30;"

	vmWrapperStyle = "swimlane;whiteSpace=wrap;html=1;fillColor=#f5f5f5;fontColor=#333333;strokeColor=#666666;rounded=1;collapsible=0;"

	childContainerStyle = "swimlane;whiteSpace=wrap;html=1;rounded=1;collapsible=0;" +
		"fillColor=#f5f5f5;fontColor=#333333;strokeColor=#666666;align=right;startSize=30;"

	nsgBadgeStyle = "shape=image;verticalLabelPosition=middle;labelBackgroundColor=default;verticalAlign=middle;" +
		"align=left;html=1;imageAspect=0;fontSize=9;image=img/lib/azure2/networking/Network_Security_Groups.svg;" +
		"labelPosition=right;spacingRight=0;spacingLeft=5;whiteSpace=wrap;labelWidth=65;"

	footerTextStyle = "text;html=1;whiteSpace=wrap;strokeColor=none;fillColor=none;verticalAlign=middle;rounded=0;align=left;"
)

// edgeClass is a confirmed relationship style: the label a reviewer
// sees, and the line's color/weight/dash pattern.
type edgeClass struct {
	label       string
	strokeColor string
	fontColor   string
	strokeWidth string
	dashed      bool
	dashPattern string // only used when dashed is true; "" draws.io's own default dash
}

// edgeClassify maps "connection.Type|source.Type|target.Type" (every
// field literal, opaque-ID discipline, never inferred or reordered)
// to its confirmed relationship style. This is deliberately a closed,
// confirmed set, not a general per-connection-type rule: two
// "network" connections between the same two resource types can still
// mean different things (a Bastion's relationship to a subnet is not a
// NIC's), so the classification keys on the full (type, source type,
// target type) triple.
var edgeClassify = map[string]edgeClass{
	"contains|network.subnet|network.vpc":                {label: "contains", strokeColor: "#6c8ebf", fontColor: "#526b8e", strokeWidth: "1.4", dashed: true, dashPattern: "2 4"},
	"network|network.interface|network.subnet":           {label: "subnet attachment", strokeColor: "#6c8ebf", fontColor: "#526b8e", strokeWidth: "1.7", dashed: true, dashPattern: "3 4"},
	"network|network.subnet|network.security.group":      {label: "NSG association", strokeColor: "#9673a6", fontColor: "#76558a", strokeWidth: "1.8", dashed: true, dashPattern: "6 4"},
	"network|osiris.azure.bastion|network.subnet":        {label: "subnet attachment", strokeColor: "#666666", fontColor: "#444444", strokeWidth: "1.6"},
	"network|osiris.azure.bastion|osiris.azure.publicip": {label: "public IP association", strokeColor: "#666666", fontColor: "#444444", strokeWidth: "1.6"},
	"network|network.loadbalancer|osiris.azure.publicip": {label: "frontend IP", strokeColor: "#FF0000", fontColor: "#FF0000", strokeWidth: "1.7"},
}

// edgeTypeClassify maps a bare connection type (no source/target type
// qualifier) to its confirmed style: unlike edgeClassify's triple-keyed
// entries above, network.bgp and network.peering are specific enough
// connection types on their own that no source/target-type pair changes
// what they mean. Checked after edgeClassify's own triple-keyed lookup,
// before the generic fallback.
var edgeTypeClassify = map[string]edgeClass{
	"network.bgp":     {label: "network.bgp", strokeColor: "#3333FF", fontColor: "#666666", strokeWidth: "2", dashed: true},
	"network.peering": {label: "network.peering", strokeColor: "#FF3399", fontColor: "#666666", strokeWidth: "2", dashed: true},
}

// arrowsForDirection returns the draw.io startArrow/endArrow value for
// a connection's own "direction" field. "forward" draws.io's edge
// already runs source -> target, so the arrowhead sits at the target
// end (endArrow); "reverse" is the same line with the asserted
// direction running target -> source, so the arrowhead sits at the
// source end (startArrow) instead of reversing the line itself;
// "bidirectional" gets both; empty or any other value (the document
// omitted it, or a future value this package has not seen) gets
// neither never invented.
func arrowsForDirection(direction string) (startArrow, endArrow string) {
	switch direction {
	case "forward":
		return "none", "classic"
	case "reverse":
		return "classic", "none"
	case "bidirectional":
		return "classic", "classic"
	default:
		return "none", "none"
	}
}

// edgeStyleAndLabel returns e's line style and label.
//
// An (connType, srcType, tgtType) triple with no confirmed entry in
// edgeClassify, nor its own bare-type entry in edgeTypeClassify, falls
// back to a neutral line labeled with the connection's own literal
// type never a guessed relationship name for a combination nobody
// has reviewed.
func (r *TopologyRenderer) edgeStyleAndLabel(e topology.Edge) (style, label string) {
	key := e.Type + "|" + r.nodeType[e.Source] + "|" + r.nodeType[e.Target]
	c, ok := edgeClassify[key]
	if !ok {
		c, ok = edgeTypeClassify[e.Type]
	}
	if !ok {
		c = edgeClass{label: e.Type, strokeColor: "#999999", fontColor: "#666666", strokeWidth: "1"}
	}
	startArrow, endArrow := arrowsForDirection(e.Direction)
	style = fmt.Sprintf(
		"rounded=0;html=1;endArrow=%s;startArrow=%s;strokeColor=%s;strokeWidth=%s;fontSize=8;fontColor=%s;labelBackgroundColor=#ffffff;",
		endArrow, startArrow, c.strokeColor, c.strokeWidth, c.fontColor)
	if c.dashed {
		style += "dashed=1;"
		if c.dashPattern != "" {
			style += "dashPattern=" + c.dashPattern + ";"
		}
	}
	return style, c.label
}

// --- xml printer -----------------------------------------------------

type xmlPrinter struct {
	w *render.CheckedWriter
}

func (p *xmlPrinter) raw(s string) { _, _ = p.w.WriteString(s) }

func (p *xmlPrinter) cell(id, parent, style, value, geometry string) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" vertex="1" parent="%s">`+"\n",
		drawio.Attr(id), value, style, drawio.Attr(parent),
	))
	p.raw("          " + geometry + "\n")
	p.raw("        </mxCell>\n")
}

func (p *xmlPrinter) edge(id, source, target, style, label string) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" edge="1" parent="1" source="%s" target="%s">`+"\n",
		drawio.Attr(id), label, style, drawio.Attr(source), drawio.Attr(target),
	))
	p.raw(`          <mxGeometry relative="1" as="geometry" />` + "\n")
	p.raw("        </mxCell>\n")
}

func geomRect(r rect) string {
	return fmt.Sprintf(`<mxGeometry x="%d" y="%d" width="%d" height="%d" as="geometry" />`, r.x, r.y, r.w, r.h)
}
