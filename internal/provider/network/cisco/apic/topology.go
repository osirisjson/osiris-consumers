// This file renders the Cisco APIC fabric topology page: one FABRIC
// CONTROLLERS swimlane spanning every pod, followed by one
// SPINE POD-N / LEAF POD-N pair of swimlanes per pod, rendered
// left to right.
// Only compute.server and role-bearing (fabricNode)
// network.switch resources are drawn, ports, bridge domains, subnets,
// L3outs, and role-less externally discovered neighbour switches stay
// fully represented in the generic model and are deliberately absent
// from this curated high-level page.
//
// Layout: this page tiles one page unit (see page.go) per pod column
// horizontally rather than fitting everything on a single page. Every
// row (the controller row, the always-single-row spine row, and each
// leaf row) is centered within its swimlane's usable width: the 80pt
// minimum gap between icons is a floor, not a target, and any leftover
// width is distributed evenly across the gaps so a row with few items
// still reads clearly and symmetrically. Icons are 40x40; a switch's
// label stacks its model, bold name, and out-of-band management
// address; a controller's label omits the model line (it has none) and
// so needs less bottom clearance below its row.
//
// Edges are drawn literally from topology.connections for any
// connection whose both endpoints are one of the visible fabric nodes
// above (controller<->leaf, spine<->leaf, and spine<->spine inter-pod
// links all occur in real APIC documents; controller<->spine does not,
// and this page draws whatever the document actually asserts, not a
// fixed pattern). Every edge is emitted before every container cell so
// containers and their child icons always render on top and the
// lines stay readable underneath.
package apic

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
)

// fabricProperties is the subset of a fabric node's "properties" this
// page reads. Decoded best-effort: a missing or malformed field stays
// zero rather than failing the render (the receipt already gates
// malformed input; this is presentation over an already-validated
// document).
type fabricProperties struct {
	Model       string `json:"model"`
	Role        string `json:"role"`
	OobMgmtAddr string `json:"oob_mgmt_addr"`
}

func decodeFabricProperties(raw json.RawMessage) fabricProperties {
	var p fabricProperties
	if len(raw) == 0 {
		return p
	}
	_ = json.Unmarshal(raw, &p)
	return p
}

// fabricBand is a node's position in the fixed vertical stacking order:
// controllers on top, then spines, then leafs.
type fabricBand int

const (
	bandController fabricBand = iota
	bandSpine
	bandLeaf
)

// fabricNode is one controller or role-bearing switch this page draws.
type fabricNode struct {
	ID          string
	Label       string
	Model       string // empty for controllers: their label has no model line.
	OobMgmtAddr string
	Band        fabricBand
}

// podFabric is one pod's spine and leaf role-bearing switches, each
// sorted by resource ID for deterministic placement.
type podFabric struct {
	Pod    string // e.g. "pod-1"; "" when a switch's native_id carried no pod segment.
	Spines []fabricNode
	Leafs  []fabricNode
}

// podFromNativeID extracts the "pod-N" segment from a fabric resource's
// provider.native_id (e.g. "topology/pod-1/node-1" -> "pod-1"). It
// returns "" when no such segment is present.
func podFromNativeID(nativeID string) string {
	for _, seg := range strings.Split(nativeID, "/") {
		if strings.HasPrefix(seg, "pod-") {
			return seg
		}
	}
	return ""
}

// podNumber returns the numeric suffix of a "pod-N" identifier, for
// sorting pods in ascending numeric order. A pod with no parseable
// number (including "") sorts before every numbered pod.
func podNumber(pod string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(pod, "pod-"))
	if err != nil {
		return -1
	}
	return n
}

func labelOr(name, id string) string {
	if name != "" {
		return name
	}
	return id
}

// buildFabric collects every compute.server and role-bearing (spine or
// leaf) network.switch resource this document declares under
// ProviderName, grouped by pod in ascending pod-number order.
func buildFabric(g *graph.Graph) (controllers []fabricNode, pods []podFabric) {
	for _, id := range g.ByType["compute.server"] {
		r := g.Resources[id]
		if r == nil || r.Provider.Name != ProviderName {
			continue
		}
		props := decodeFabricProperties(r.Properties)
		controllers = append(controllers, fabricNode{
			ID:          r.ID,
			Label:       labelOr(r.Name, r.ID),
			OobMgmtAddr: props.OobMgmtAddr,
			Band:        bandController,
		})
	}
	sort.Slice(controllers, func(i, j int) bool { return controllers[i].ID < controllers[j].ID })

	podMap := map[string]*podFabric{}
	var podOrder []string
	for _, id := range g.ByType["network.switch"] {
		r := g.Resources[id]
		if r == nil || r.Provider.Name != ProviderName {
			continue
		}
		props := decodeFabricProperties(r.Properties)
		var band fabricBand
		switch props.Role {
		case "spine":
			band = bandSpine
		case "leaf":
			band = bandLeaf
		default:
			continue // role-less: an externally discovered neighbour, out of scope for this page.
		}
		pod := podFromNativeID(r.Provider.NativeID)
		pf, ok := podMap[pod]
		if !ok {
			pf = &podFabric{Pod: pod}
			podMap[pod] = pf
			podOrder = append(podOrder, pod)
		}
		n := fabricNode{
			ID:          r.ID,
			Label:       labelOr(r.Name, r.ID),
			Model:       props.Model,
			OobMgmtAddr: props.OobMgmtAddr,
			Band:        band,
		}
		if band == bandSpine {
			pf.Spines = append(pf.Spines, n)
		} else {
			pf.Leafs = append(pf.Leafs, n)
		}
	}

	sort.Slice(podOrder, func(i, j int) bool { return podNumber(podOrder[i]) < podNumber(podOrder[j]) })
	for _, pod := range podOrder {
		pf := podMap[pod]
		sort.Slice(pf.Spines, func(i, j int) bool { return pf.Spines[i].ID < pf.Spines[j].ID })
		sort.Slice(pf.Leafs, func(i, j int) bool { return pf.Leafs[i].ID < pf.Leafs[j].ID })
		pods = append(pods, *pf)
	}
	return controllers, pods
}

// podCounts returns the per-pod spine/leaf census page.go's page
// chooser needs.
func podCounts(pods []podFabric) []podFabricCounts {
	out := make([]podFabricCounts, len(pods))
	for i, p := range pods {
		out[i] = podFabricCounts{Spines: len(p.Spines), Leafs: len(p.Leafs)}
	}
	return out
}

// --- row rendering composition ---------------------------------------

// placedNode is one fabricNode at its deterministic position, relative
// to its own swimlane cell's coordinate space. CellID is filled in
// later, once Render assigns every visible node a deterministic mxCell
// id (see Render's indexNodes).
type placedNode struct {
	fabricNode
	X, Y   int
	CellID string
}

// iconWidthFor returns a node's icon width: apicControllerIconWidth for
// a controller (mxgraph.cisco19.server2 is not natively square at 40pt
// height), apicIconSize for a spine or leaf switch
// (mxgraph.cisco19.rect is square). Icon height is always apicIconSize
// for every band.
func iconWidthFor(band fabricBand) int {
	if band == bandController {
		return apicControllerIconWidth
	}
	return apicIconSize
}

// placeRowX returns the left edge x of each of n iconWidth-wide icons,
// centered within a container width wide: the 80pt minimum gap is a
// floor, and any leftover width beyond the minimum-packed row is
// distributed evenly across the gaps (remainder points going to the
// leftmost gaps first, so the result is exact and deterministic), so
// the row spans the full usable width symmetrically instead of
// clustering at the left margin.
func placeRowX(containerWidth, n, iconWidth int) []int {
	if n <= 0 {
		return nil
	}
	inner := containerWidth - 2*apicMargin
	if n == 1 {
		return []int{apicMargin + (inner-iconWidth)/2}
	}
	slack := inner - n*iconWidth - (n-1)*apicMinGap
	if slack < 0 {
		slack = 0 // defensive: callers only pass n <= rowCapacity(containerWidth, iconWidth).
	}
	gap := apicMinGap + slack/(n-1)
	remainder := slack % (n - 1)
	xs := make([]int, n)
	x := apicMargin
	for i := 0; i < n; i++ {
		xs[i] = x
		extra := gap
		if i < remainder {
			extra++
		}
		x += iconWidth + extra
	}
	return xs
}

// placeRows lays items out in as many rows as
// rowCapacity(containerWidth, iconWidth) requires, each row centered
// per placeRowX. Used for the leaf swimlane (which may wrap) and, with
// a width chosen so it never needs to, the controller and spine
// swimlanes. Every item in items shares the same band and therefore
// the same iconWidth (see iconWidthFor).
func placeRows(containerWidth int, items []fabricNode, iconWidth int) []placedNode {
	cap := rowCapacity(containerWidth, iconWidth)
	if cap <= 0 {
		return nil
	}
	var out []placedNode
	for start := 0; start < len(items); start += cap {
		end := start + cap
		if end > len(items) {
			end = len(items)
		}
		row := items[start:end]
		xs := placeRowX(containerWidth, len(row), iconWidth)
		y := apicHeaderH + apicMargin + (start/cap)*apicPitch
		for i, n := range row {
			out = append(out, placedNode{fabricNode: n, X: xs[i], Y: y})
		}
	}
	return out
}

// --- layout ----------------------------------------------------------

// placedContainer is one swimlane at its canvas position, with its
// already-placed child icons.
type placedContainer struct {
	ID    string
	Title string
	X, Y  int
	W, H  int
	Nodes []placedNode
}

// podColumn is one pod's spine and leaf swimlanes, side by side with
// every other pod's.
type podColumn struct {
	Pod   string
	Spine placedContainer // zero value (Nodes == nil) when the pod has no spines.
	Leaf  placedContainer // zero value when the pod has no leafs.
}

// fabricLayout is the complete, positioned page.
type fabricLayout struct {
	Page        PageInfo
	CanvasWidth int
	Controllers placedContainer // zero value when the document has no controllers.
	Columns     []podColumn
}

// buildLayout positions every swimlane and icon for controllers and
// pods, choosing the page (tile) size first (page.go) so every
// container's width is already known. The whole canvas, the logo, the
// controller swimlane, and every column, starts flush at
// apicCanvasLeft aligned to the logo's own left edge, and numPods
// columns span exactly numPods page tiles (totalCanvasWidth), each
// column's own rendered width one apicColumnGap narrower than its tile
// so a visible gap separates it from the next column without adding
// width on top of that N-tile span.
func buildLayout(controllers []fabricNode, pods []podFabric) fabricLayout {
	page := chooseAPICPage(len(pods), len(controllers), podCounts(pods))
	tierWidth := page.Width
	colW := colRenderWidth(tierWidth, len(pods))
	totalW := totalCanvasWidth(len(pods), tierWidth)

	y := apicCanvasTopMargin
	var lay fabricLayout
	lay.Page = page
	lay.CanvasWidth = totalW

	if len(controllers) > 0 {
		rows := rowsForCount(len(controllers), rowCapacity(totalW, apicControllerIconWidth))
		h := swimlaneHeight(rows, apicBottomPadNoModel)
		lay.Controllers = placedContainer{
			ID: "controllers", Title: "FABRIC CONTROLLERS",
			X: apicCanvasLeft, Y: y, W: totalW, H: h,
			Nodes: placeRows(totalW, controllers, apicControllerIconWidth),
		}
		y += h + apicRowStackGap
	}

	anySpines := false
	for _, p := range pods {
		if len(p.Spines) > 0 {
			anySpines = true
			break
		}
	}
	spineY := y
	spineH := swimlaneHeight(1, apicBottomPadModel) // spines never wrap: always exactly one row.
	if anySpines {
		y = spineY + spineH + apicRowStackGap
	}
	leafY := y

	for i, p := range pods {
		colX := apicCanvasLeft + i*tierWidth
		col := podColumn{Pod: p.Pod}
		title := strings.ToUpper(strings.TrimPrefix(p.Pod, "pod-"))
		if title != "" {
			title = " POD-" + title
		}
		if len(p.Spines) > 0 {
			col.Spine = placedContainer{
				ID: "spine-" + p.Pod, Title: "SPINE" + title,
				X: colX, Y: spineY, W: colW, H: spineH,
				Nodes: placeRows(colW, p.Spines, apicIconSize),
			}
		}
		if len(p.Leafs) > 0 {
			rows := rowsForCount(len(p.Leafs), rowCapacity(colW, apicIconSize))
			h := swimlaneHeight(rows, apicBottomPadModel)
			col.Leaf = placedContainer{
				ID: "leaf-" + p.Pod, Title: "LEAF" + title,
				X: colX, Y: leafY, W: colW, H: h,
				Nodes: placeRows(colW, p.Leafs, apicIconSize),
			}
		}
		lay.Columns = append(lay.Columns, col)
	}
	return lay
}

// apicCanvasTopMargin is the gap between the page top and the first
// (FABRIC CONTROLLERS) swimlane. apicCanvasLeft is the left margin
// shared by the logo and every swimlane, so the first column always
// starts flush with the logo's own left edge.
const (
	apicCanvasTopMargin = 80
	apicCanvasLeft      = 20
)

// nodeInfo is what an edge needs to know about one endpoint: its mxCell
// id, its band, and its pod-column index (-1 for a controller, which
// spans every column) so edgeAnchors can orient the connector.
type nodeInfo struct {
	cellID string
	band   fabricBand
	col    int
}

// preparedEdge is one connection resolved to its two endpoint cell ids
// and its four connection-point anchors, ready to print.
type preparedEdge struct {
	id               string
	srcCell, dstCell string
	exitX, exitY     float64
	entryX, entryY   float64
}

// --- renderer --------------------------------------------------------

// TopologyRenderer renders the Cisco APIC fabric topology page.
// The zero value is not usable; construct one with NewTopologyRenderer.
type TopologyRenderer struct {
	g        *graph.Graph
	scope    apicScopeFields
	metadata document.Metadata
	artifact time.Time
}

// NewTopologyRenderer returns a renderer for the fabric nodes and
// connections in g. md supplies the header and footer's snapshot
// metadata. artifactTime is accepted for parity with the other
// renderers' construction signature; this page carries no
// artifact-time-derived content beyond what md.Timestamp already
// discloses.
func NewTopologyRenderer(g *graph.Graph, md document.Metadata, artifactTime time.Time) *TopologyRenderer {
	return &TopologyRenderer{
		g:        g,
		scope:    decodeAPICScope(md.Scope),
		metadata: md,
		artifact: artifactTime.UTC(),
	}
}

// ChooseTopologyPage returns the page (tile) size a TopologyRenderer
// would choose for g, the same content-fit computation Render uses,
// exposed as a pure function so a caller can report the choice without
// re-rendering.
func ChooseTopologyPage(g *graph.Graph) PageInfo {
	controllers, pods := buildFabric(g)
	return chooseAPICPage(len(pods), len(controllers), podCounts(pods))
}

// Render writes the draw.io XML to w.
func (r *TopologyRenderer) Render(ctx context.Context, w io.Writer) error {
	cw := render.NewCheckedWriter(w)
	p := &apicPrinter{w: cw}

	controllers, pods := buildFabric(r.g)
	lay := buildLayout(controllers, pods)

	p.raw(`<mxfile host="osirisjson-consumer docs.osirisjson.org/osiris-consumers/getting-started">` + "\n")
	p.raw(`  <diagram id="osirisjson-cisco-apic-topology" name="Topology">` + "\n")
	p.raw(fmt.Sprintf(
		`    <mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="0" arrows="1" fold="1" page="1" pageScale="1" pageWidth="%d" pageHeight="%d" math="0" shadow="0">`+"\n",
		lay.Page.Width, lay.Page.Height))
	p.raw(`      <root>` + "\n")
	p.raw(`        <mxCell id="0" />` + "\n")
	p.raw(`        <mxCell id="1" parent="0" />` + "\n")

	if err := ctx.Err(); err != nil {
		return err
	}

	// Index every visible node's cell id, band, and pod-column index up
	// front, since edges must be emitted before any container cell
	// this is the entire z-order mechanic, edges declared first render
	// underneath the containers and icons declared after them.
	// Cell ids are written back onto the layout's own placedNode slices
	// (indexNodes mutates through the shared backing array), so the id
	// assigned here is the same one emitContainer prints later.
	idIndex := make(map[string]nodeInfo)
	seq := 0
	indexNodes := func(nodes []placedNode, band fabricBand, col int) {
		for i := range nodes {
			seq++
			id := "n" + strconv.Itoa(seq)
			nodes[i].CellID = id
			idIndex[nodes[i].ID] = nodeInfo{cellID: id, band: band, col: col}
		}
	}
	if lay.Controllers.Nodes != nil {
		indexNodes(lay.Controllers.Nodes, bandController, -1)
	}
	for i := range lay.Columns {
		if lay.Columns[i].Spine.Nodes != nil {
			indexNodes(lay.Columns[i].Spine.Nodes, bandSpine, i)
		}
		if lay.Columns[i].Leaf.Nodes != nil {
			indexNodes(lay.Columns[i].Leaf.Nodes, bandLeaf, i)
		}
	}

	// Prepare edges: only a connection whose both endpoints are one of
	// the indexed fabric nodes above is drawn.
	var edges []preparedEdge
	for _, e := range r.g.Edges {
		src, ok1 := idIndex[e.Source]
		dst, ok2 := idIndex[e.Target]
		if !ok1 || !ok2 {
			continue
		}
		exitX, exitY, entryX, entryY := edgeAnchors(src, dst)
		edges = append(edges, preparedEdge{
			id: e.ConnectionID, srcCell: src.cellID, dstCell: dst.cellID,
			exitX: exitX, exitY: exitY, entryX: entryX, entryY: entryY,
		})
	}
	fanOutEnd(edges, func(i int) string { return edges[i].dstCell },
		func(i int) float64 { return edges[i].entryX }, func(i int) float64 { return edges[i].entryY },
		func(i int, v float64) { edges[i].entryX = v }, func(i int, v float64) { edges[i].entryY = v })
	fanOutEnd(edges, func(i int) string { return edges[i].srcCell },
		func(i int) float64 { return edges[i].exitX }, func(i int) float64 { return edges[i].exitY },
		func(i int, v float64) { edges[i].exitX = v }, func(i int, v float64) { edges[i].exitY = v })

	for i, e := range edges {
		style := fmt.Sprintf(
			"edgeStyle=orthogonalEdgeStyle;rounded=0;orthogonalLoop=1;jettySize=auto;html=1;"+
				"exitX=%s;exitY=%s;exitDx=0;exitDy=0;exitPerimeter=0;"+
				"entryX=%s;entryY=%s;entryDx=0;entryDy=0;entryPerimeter=0;curved=1;",
			fnum(e.exitX), fnum(e.exitY), fnum(e.entryX), fnum(e.entryY))
		p.edge("e"+strconv.Itoa(i), e.srcCell, e.dstCell, style)
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	p.cell("logo", "1", drawio.LogoStyle, "", apicCanvasLeft, apicLogoY, drawio.LogoIntrinsicW, drawio.LogoIntrinsicH)
	r.emitHeaderAndFooter(p, lay.CanvasWidth, lay.Page.Height)

	emitContainer := func(c placedContainer) {
		if c.Nodes == nil && c.W == 0 {
			return
		}
		p.raw(fmt.Sprintf(
			`        <mxCell id="%s" value="%s" style="%s" vertex="1" parent="1">`+"\n",
			drawio.Attr(c.ID), drawio.Attr(c.Title), apicSwimlaneStyle))
		p.raw(fmt.Sprintf(`          <mxGeometry x="%d" y="%d" width="%d" height="%d" as="geometry" />`+"\n", c.X, c.Y, c.W, c.H))
		p.raw("        </mxCell>\n")
		for _, n := range c.Nodes {
			p.cell(n.CellID, c.ID, r.nodeStyle(n.fabricNode), r.nodeValue(n.fabricNode), n.X, n.Y, iconWidthFor(n.Band), apicIconSize)
		}
	}

	if lay.Controllers.Nodes != nil {
		emitContainer(lay.Controllers)
	}
	for _, col := range lay.Columns {
		if col.Spine.Nodes != nil {
			emitContainer(col.Spine)
		}
	}
	for _, col := range lay.Columns {
		if col.Leaf.Nodes != nil {
			emitContainer(col.Leaf)
		}
	}

	p.raw(`      </root>` + "\n")
	p.raw(`    </mxGraphModel>` + "\n")
	p.raw(`  </diagram>` + "\n")
	p.raw(`</mxfile>` + "\n")

	return cw.Err()
}

// edgeAnchors picks the exit (source-side) and entry (target-side)
// connection points from each node's band and pod column: a link
// between two bands connects bottom-to-top or top-to-bottom depending
// on which band is which (controller above spine above leaf); a link
// within the same band (a spine-to-spine inter-pod link) connects
// side-to-side, oriented by column order.
func edgeAnchors(src, dst nodeInfo) (exitX, exitY, entryX, entryY float64) {
	switch {
	case src.band < dst.band:
		return 0.5, 1, 0.5, 0
	case src.band > dst.band:
		return 0.5, 0, 0.5, 1
	case src.col <= dst.col:
		return 1, 0.5, 0, 0.5
	default:
		return 0, 0.5, 1, 0.5
	}
}

// fanOutEnd spreads one end (exit or entry) of every edge that shares
// the same grouping key (its target for the entry end, its source for
// the exit end) across {0.25, 0.5, 0.75} in sorted order, so multiple
// lines meeting the same node do not converge on one point.
// edgeAnchors always leaves exactly one of that end's two coordinates
// at the default 0.5 (the other is pinned to 0 or 1: the edge of the
// node the connector actually touches, top/bottom for a
// controller/spine/leaf link, left/right for a same-band spine-to-spine
// link) fanOutEnd perturbs whichever coordinate that is, so a vertical
// link fans out along the touched node's top or bottom edge and a side
// link fans out along its left or right edge.
func fanOutEnd(edges []preparedEdge, keyOf func(i int) string, getX, getY func(i int) float64, setX, setY func(i int, v float64)) {
	groups := map[string][]int{}
	for i := range edges {
		groups[keyOf(i)] = append(groups[keyOf(i)], i)
	}
	fanoutSeq := []float64{0.25, 0.5, 0.75}
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		sort.Slice(idxs, func(a, b int) bool { return edges[idxs[a]].id < edges[idxs[b]].id })
		for j, i := range idxs {
			v := fanoutSeq[j%len(fanoutSeq)]
			if getX(i) == 0.5 {
				setX(i, v)
			} else {
				setY(i, v)
			}
		}
	}
}

func fnum(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// nodeStyle picks the Cisco19 shape for n: server2 for a controller,
// rect with a model-derived prIcon for a spine or leaf switch.
func (r *TopologyRenderer) nodeStyle(n fabricNode) string {
	base := "sketch=0;points=[[0.015,0.015,0],[0.985,0.015,0],[0.985,0.985,0],[0.015,0.985,0],[0.25,0,0],[0.5,0,0],[0.75,0,0],[1,0.25,0],[1,0.5,0],[1,0.75,0],[0.75,1,0],[0.5,1,0],[0.25,1,0],[0,0.75,0],[0,0.5,0],[0,0.25,0]];" +
		"verticalLabelPosition=bottom;html=1;verticalAlign=top;aspect=fixed;align=center;pointerEvents=1;" +
		"fillColor=#FAFAFA;strokeColor=#005073;fontSize=10;labelBackgroundColor=default;"
	if n.Band == bandController {
		return base + "shape=mxgraph.cisco19.server2;"
	}
	shape := base + "shape=mxgraph.cisco19.rect;"
	if icon := prIconForModel(n.Model); icon != "" {
		shape += "prIcon=" + icon + ";"
	}
	return shape
}

// nodeValue is the icon label: a switch stacks its model (plain), its
// bold name, and its out-of-band management address; a controller
// omits the model line, since it has none.
func (r *TopologyRenderer) nodeValue(n fabricNode) string {
	name := "<b>" + n.Label + "</b>"
	addr := "<div>" + n.OobMgmtAddr + "</div>"
	if n.Band == bandController {
		return drawio.Attr(name + addr)
	}
	return drawio.Attr(n.Model + "<div>" + name + addr + "</div>")
}

// --- draw.io header and footer ---------------------------------------

// apicScopeFields is the subset of "metadata.scope" the header and
// footer disclose.
type apicScopeFields struct {
	Description string `json:"description"`
	Purpose     string `json:"purpose"`
}

func decodeAPICScope(raw json.RawMessage) apicScopeFields {
	var s apicScopeFields
	if len(raw) == 0 {
		return s
	}
	_ = json.Unmarshal(raw, &s)
	return s
}

func stringOrAbsent(s string) string {
	if s == "" {
		return "(absent)"
	}
	return s
}

// emitHeaderAndFooter writes the top-right fabric badge (CISCO ACI
// FABRIC plus the document's own scope description) and the bottom
// band (the OSIRIS JSON credit on the left; snapshot timestamp, scope
// purpose, and generator version stacked right to left from the right
// margin), scaled to the actual canvas width canvasW so the band keeps
// its proportions regardless of how many pod columns the fabric
// needed.
// The bottom band sits close to the page's bottom edge
// (apicFooterBottomMargin, not the wider apicMargin swimlanes use)
// rather than leaving a large unused strip below it.
func (r *TopologyRenderer) emitHeaderAndFooter(p *apicPrinter, canvasW, pageH int) {
	ts := stringOrAbsent(r.metadata.Timestamp)
	purpose := stringOrAbsent(r.scope.Purpose)
	genVersion := "(absent)"
	if r.metadata.Generator != nil {
		genVersion = stringOrAbsent(r.metadata.Generator.Version)
	}
	contentRight := apicCanvasLeft + canvasW
	footerY := pageH - apicFooterBottomMargin - apicFooterH

	const (
		genW   = 200
		scopeW = 140
		tsW    = 140
		gap    = 20
	)
	genX := contentRight - genW
	scopeX := genX - gap - scopeW
	tsX := scopeX - gap - tsW
	const headerW = 1000
	headerX := contentRight - headerW

	p.textCell("header-fabric", apicTextStyle+"align=right;", drawio.Attr("<b>CISCO ACI FABRIC</b><br>"+stringOrAbsent(r.scope.Description)), headerX, apicLogoY, headerW, apicFooterH)
	p.textCell("footer-osirisjson", apicTextStyle+"align=left;", drawio.Attr("<b>OSIRIS JSON</b><br>This topology was created with OSIRIS JSON. Read more on https://osirisjson.org"), apicCanvasLeft, footerY, 450, apicFooterH)
	p.textCell("footer-timestamp", apicTextStyle+"align=left;", drawio.Attr("<div><b>Snapshot created on</b></div><div>"+ts+"</div>"), tsX, footerY, tsW, apicFooterH)
	p.textCell("footer-scope", apicTextStyle+"align=left;", drawio.Attr("<div><b>Snapshot scope</b></div><div>"+purpose+"</div>"), scopeX, footerY, scopeW, apicFooterH)
	p.textCell("footer-generator", apicTextStyle+"align=left;", drawio.Attr("<div><b>OSIRIS JSON for Cisco APIC</b></div><div>"+genVersion+"</div>"), genX, footerY, genW, apicFooterH)
}

// --- draw.io styles --------------------------------------------------

const (
	// apicTextStyle carries no alignment of its own: every call site
	// appends align=left; or align=right; explicitly, so two alignments
	// are never concatenated onto the same style string.
	apicTextStyle     = "text;html=1;whiteSpace=wrap;strokeColor=none;fillColor=none;verticalAlign=middle;rounded=0;"
	apicSwimlaneStyle = "swimlane;whiteSpace=wrap;html=1;collapsible=0;startSize=40;rounded=1;fillColor=#f5f5f5;strokeColor=#666666;fontColor=#333333;labelBackgroundColor=none;"

	apicLogoY   = 20
	apicFooterH = 30
	// apicFooterBottomMargin is the bottom band own clearance from the
	// page's bottom edge, deliberately smaller than apicMargin (the
	// swimlanes' margin) so the band sits close to the edge instead of
	// leaving a wide unused strip below it.
	apicFooterBottomMargin = 10
)

// --- xml printer -----------------------------------------------------

type apicPrinter struct {
	w *render.CheckedWriter
}

func (p *apicPrinter) raw(s string) { _, _ = p.w.WriteString(s) }

func (p *apicPrinter) cell(id, parent, style, value string, x, y, w, h int) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" vertex="1" parent="%s">`+"\n",
		drawio.Attr(id), value, style, drawio.Attr(parent)))
	p.raw(fmt.Sprintf(`          <mxGeometry x="%d" y="%d" width="%d" height="%d" as="geometry" />`+"\n", x, y, w, h))
	p.raw("        </mxCell>\n")
}

func (p *apicPrinter) textCell(id, style, value string, x, y, w, h int) {
	p.cell(id, "1", style, value, x, y, w, h)
}

func (p *apicPrinter) edge(id, source, target, style string) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="" style="%s" edge="1" parent="1" source="%s" target="%s">`+"\n",
		drawio.Attr(id), style, drawio.Attr(source), drawio.Attr(target)))
	p.raw(`          <mxGeometry relative="1" as="geometry" />` + "\n")
	p.raw("        </mxCell>\n")
}
