// This file renders the Cisco NX-OS confirmed-panel "FrontVLANs" page:
// the same front illustration and full port-status marker set
// emitPanelBase already draws for FrontPhysical, plus one swimlane
// container per port that carries any network.l2 membership, each
// holding that port's own small VLAN badges in a grid, every port's
// own VLAN membership exactly as the document records it, never
// filtered or summarized away. A port-channel's own network.l2
// membership has no physical position of its own to anchor to, so it
// fans out to each of that port-channel's real member ports (the
// document's own "contains" connections), each badge's label naming
// the port-channel it arrived through, the membership is never
// dropped, and never misplaced on a fictional bundle location either.
package nxos

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/provider/network/assets"
	"go.osirisjson.org/consumers/internal/render/drawio"
)

// vlanMarker is the substring a network.vlan resource's own ID always
// carries right before its own native VLAN identifier (e.g.
// "cisco.nxos::ABC00001DE2/vlan/1934").
const vlanMarker = "/vlan/"

// vlanNativeLabel returns id's own "vlan/<n>" tail verbatim, the same
// native-ID-tail reading neighbourNativeLabel uses for a neighbour's
// own ID, kept adapter-local.
// Falls back to id itself when the marker is absent.
func vlanNativeLabel(id string) string {
	if i := strings.LastIndex(id, vlanMarker); i >= 0 {
		if rest := id[i+1:]; rest != "" {
			return rest
		}
	}
	return id
}

// panelVLANBadge is one network.l2 connection placed on the FrontVLANs
// page: LocalPort is the real physical port it anchors to (its own
// port, or for a port-channel's own membership one of that port-
// channel's own member ports), Label is the badge's own display text.
type panelVLANBadge struct {
	LocalPort string
	Label     string
}

// buildLAGMemberPortNames maps each network.interface.lag resource's
// own ID to its member ports' own names, read from the document's own
// "contains" connections (source the LAG, target the member port)
// mirrors report_model.go's buildLAGs, kept separate since this page
// needs a name-keyed map rather than a report row.
func buildLAGMemberPortNames(g *graph.Graph) map[string][]string {
	out := map[string][]string{}
	for _, id := range sortedByType(g, "network.interface.lag") {
		r := g.Resources[id]
		if r.Provider.Name != ProviderName {
			continue
		}
		var members []string
		for _, idx := range g.OutEdges[id] {
			e := g.Edges[idx]
			if e.Type != "contains" {
				continue
			}
			if mr, ok := g.Resources[e.Target]; ok {
				members = append(members, labelOr(mr.Name, e.Target))
			}
		}
		sort.Strings(members)
		out[id] = members
	}
	return out
}

// buildPanelVLANBadges reads every network.l2 connection once, in
// declaration order then stably sorted by the port it lands on: a
// direct network.switch.port source becomes one badge on its own port;
// a network.interface.lag source fans out to every one of that port-
// channel's own member ports (lagMembers), one badge per member, each
// naming the port-channel it arrived through. Any other source type is
// left unplaced (adapter-local best effort report_model.go's own
// tables still carry the connection in full).
func buildPanelVLANBadges(g *graph.Graph, lagMembers map[string][]string) []panelVLANBadge {
	var out []panelVLANBadge
	for _, id := range g.ConnectionIDs {
		c := g.Connections[id]
		if c == nil || c.Type != "network.l2" {
			continue
		}
		src, ok := g.Resources[c.Source]
		if !ok {
			continue
		}
		vlan, ok := g.Resources[c.Target]
		if !ok {
			continue
		}
		label := "<b>" + vlanNativeLabel(vlan.ID) + "</b>"
		switch src.Type {
		case "network.switch.port":
			out = append(out, panelVLANBadge{LocalPort: labelOr(src.Name, src.ID), Label: label})
		case "network.interface.lag":
			lagName := labelOr(src.Name, src.ID)
			for _, member := range lagMembers[src.ID] {
				out = append(out, panelVLANBadge{LocalPort: member, Label: label + "<div>via " + lagName + "</div>"})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LocalPort < out[j].LocalPort })
	return out
}

// panelVLANBadgeW, panelVLANBadgeH, and panelVLANBadgeGap size and
// space each small VLAN badge inside its own port's container.
// panelVLANContainerHeaderH is the swimlane title bar height
// (panelVLANContainerStyle's own startSize); panelVLANContainerPad is
// the clearance kept between the badge grid and the container's own
// border; panelVLANMaxRows caps a container's own height by wrapping
// into a new column once a port's own badge count would otherwise make
// one endless column (a busy trunk port can carry dozens of VLANs);
// panelVLANContainerGap is the minimum horizontal clearance kept
// between two adjacent ports' own containers.
const (
	panelVLANBadgeW           = 70
	panelVLANBadgeH           = 20
	panelVLANBadgeGap         = 6
	panelVLANContainerHeaderH = 20
	panelVLANContainerPad     = 10
	panelVLANMaxRows          = 8
	panelVLANContainerGap     = 40
)

// panelVLANContainer is one port's own badges, grouped into a single
// swimlane box sized to hold them in a Columns x Rows grid (column-
// major: a column fills top to bottom before the next one starts)
// the container-per-port design a busy trunk port's own dozens of
// VLANs need to stay readable, rather than one badge per port spilling
// outward indefinitely.
type panelVLANContainer struct {
	Port          string
	Badges        []panelVLANBadge
	Columns, Rows int
	Width, Height int
}

// buildVLANContainers groups badges by LocalPort (skipping any whose
// anchor does not resolve on this illustration the caller has
// already filtered, but a container's own size must only ever count
// badges it will actually place) and sizes each one's own grid: Rows is
// the badge count capped at panelVLANMaxRows, Columns is however many
// of those rows-tall columns the remaining badges need. A port with a
// single VLAN gets a one-badge, one-row container; a port with 28 gets
// four columns of 7 (or similar), never one 28-badge-tall column.
func buildVLANContainers(badges []panelVLANBadge, anchor panelAnchorFunc) []*panelVLANContainer {
	byPort := map[string]*panelVLANContainer{}
	var order []string
	for _, b := range badges {
		if _, _, ok := anchor(b.LocalPort); !ok {
			continue
		}
		c, seen := byPort[b.LocalPort]
		if !seen {
			c = &panelVLANContainer{Port: b.LocalPort}
			byPort[b.LocalPort] = c
			order = append(order, b.LocalPort)
		}
		c.Badges = append(c.Badges, b)
	}

	out := make([]*panelVLANContainer, 0, len(order))
	for _, port := range order {
		c := byPort[port]
		n := len(c.Badges)
		rows := n
		if rows > panelVLANMaxRows {
			rows = panelVLANMaxRows
		}
		if rows < 1 {
			rows = 1
		}
		cols := (n + rows - 1) / rows
		if cols < 1 {
			cols = 1
		}
		c.Rows, c.Columns = rows, cols
		c.Width = panelVLANContainerPad*2 + cols*panelVLANBadgeW + (cols-1)*panelVLANBadgeGap
		c.Height = panelVLANContainerHeaderH + panelVLANContainerPad*2 + rows*panelVLANBadgeH + (rows-1)*panelVLANBadgeGap
		out = append(out, c)
	}
	return out
}

// panelVLANContainerPlacement is one container's computed top-left
// position, in mxGraph user units, keyed by its own Port.
type panelVLANContainerPlacement map[string]struct{ X, Y int }

// panelVLANContainerMinGap is the floor placePanelVLANContainers
// compresses the gap between adjacent containers to, once the
// preferred panelVLANContainerGap does not fit the panel's own width
// mirrors placePanelNeighbours's own compress-before-grow policy
// (panel_layout.go), a container's own width just never shrinks (it is
// content-driven, not decorative), only the gap between containers
// does.
const panelVLANContainerMinGap = 10

// placePanelVLANContainers positions each container centered on its own
// port's physical anchor whenever that fits the panel's own width at
// the preferred panelVLANContainerGap, above the panel when that port
// sits in the illustration's own upper half, below when it sits in the
// lower half (placePanelNeighbours's own rule), falling back to an
// even left-to-right spread across the panel's own width compressing
// the gap down to panelVLANContainerMinGap, never the containers
// own widths only when the preferred gap would not fit.
func placePanelVLANContainers(containers []*panelVLANContainer, anchor panelAnchorFunc, panelX, panelY, panelW, panelH int) panelVLANContainerPlacement {
	type item struct {
		c         *panelVLANContainer
		idealLeft int
		above     bool
	}
	var items []item
	for _, c := range containers {
		fx, fy, ok := anchor(c.Port)
		if !ok {
			continue
		}
		idealCenter := panelX + int(fx*float64(panelW))
		items = append(items, item{c: c, idealLeft: idealCenter - c.Width/2, above: fy < 0.5})
	}

	out := make(panelVLANContainerPlacement, len(items))
	place := func(its []item, yFor func(height int) int) {
		sort.Slice(its, func(i, j int) bool { return its[i].idealLeft < its[j].idealLeft })
		n := len(its)
		if n == 0 {
			return
		}
		widths := make([]int, n)
		idealLefts := make([]int, n)
		for i, it := range its {
			widths[i] = it.c.Width
			idealLefts[i] = it.idealLeft
		}
		xs := gapEnforcedPositions(idealLefts, widths, panelVLANContainerGap)
		if xs[0] < panelX || xs[n-1]+widths[n-1] > panelX+panelW {
			xs = distributeVariableEvenly(widths, panelX, panelW, panelVLANContainerMinGap)
		}
		for i, it := range its {
			out[it.c.Port] = struct{ X, Y int }{X: xs[i], Y: yFor(it.c.Height)}
		}
	}
	var above, below []item
	for _, it := range items {
		if it.above {
			above = append(above, it)
		} else {
			below = append(below, it)
		}
	}
	place(above, func(height int) int { return panelY - panelNeighbourGap - height })
	place(below, func(height int) int { return panelY + panelH + panelNeighbourGap })
	return out
}

// gapEnforcedPositions pushes each sorted idealLeft position right just
// enough to keep at least gap clear of the previous item's own right
// edge (idealLeft[i-1] + widths[i-1]).
func gapEnforcedPositions(idealLeft, widths []int, gap int) []int {
	out := make([]int, len(idealLeft))
	last, lastWidth := -1<<62, 0
	for i, x := range idealLeft {
		if last != -1<<62 && x < last+lastWidth+gap {
			x = last + lastWidth + gap
		}
		last, lastWidth = x, widths[i]
		out[i] = x
	}
	return out
}

// distributeVariableEvenly places items of the given widths side by
// side starting at start, using the largest gap that keeps the whole
// row within width (falling back to minGap, and overflowing width only
// if even that does not fit the caller is then past what this panel
// width can hold and must grow the page instead).
func distributeVariableEvenly(widths []int, start, width, minGap int) []int {
	n := len(widths)
	out := make([]int, n)
	if n == 0 {
		return out
	}
	sum := 0
	for _, w := range widths {
		sum += w
	}
	gap := minGap
	if n > 1 {
		if g := (width - sum) / (n - 1); g > minGap {
			gap = g
		}
	}
	x := start
	for i, w := range widths {
		out[i] = x
		x += w + gap
	}
	return out
}

// choosePanelVLANPage sizes the FrontVLANs page: the fixed A3 landscape
// baseline, unless the busiest side's own containers need more than
// the panel's own A3 width even at panelVLANContainerMinGap, or a
// side's own tallest container needs more vertical room than A3 gives
// above or below the panel a closed-form calculation on each side's
// own container widths and heights, the same compress-before-grow
// policy chooseConfirmedPanelPage uses. Growing the page also grows
// panelW (and, through aspect, panelH), which can in turn make the
// height requirement (computed from that same panelH) grow too, so
// this recomputes once against the actual page width chooseNXOSPage
// returned two passes converge for every case that matters here, a
// bounded refinement rather than an open-ended search.
func choosePanelVLANPage(containers []*panelVLANContainer, anchor panelAnchorFunc, aspect float64) PageInfo {
	var aboveWidths, belowWidths []int
	var maxAboveH, maxBelowH int
	for _, c := range containers {
		_, fy, ok := anchor(c.Port)
		if !ok {
			continue
		}
		if fy < 0.5 {
			aboveWidths = append(aboveWidths, c.Width)
			if c.Height > maxAboveH {
				maxAboveH = c.Height
			}
		} else {
			belowWidths = append(belowWidths, c.Width)
			if c.Height > maxBelowH {
				maxBelowH = c.Height
			}
		}
	}

	neededPanelW := 0
	if w := minRowSpan(aboveWidths, panelVLANContainerMinGap); w > neededPanelW {
		neededPanelW = w
	}
	if w := minRowSpan(belowWidths, panelVLANContainerMinGap); w > neededPanelW {
		neededPanelW = w
	}
	neededPageW := neededPanelW + 2*nxosPanelMarginX
	if neededPageW < nxosA3Width {
		neededPageW = nxosA3Width
	}

	var page PageInfo
	for i := 0; i < 2; i++ {
		panelW := neededPageW - 2*nxosPanelMarginX
		panelH := int(float64(panelW) * aspect)
		neededPageH := nxosHeaderH + panelNeighbourGap + maxAboveH + panelH + panelNeighbourGap + maxBelowH + nxosFooterGap + nxosFooterH + nxosFooterMargin
		if neededPageH < nxosA3Height {
			neededPageH = nxosA3Height
		}
		page = chooseNXOSPage(neededPageW, neededPageH)
		if page.Width == neededPageW {
			break
		}
		neededPageW = page.Width
	}
	return page
}

// minRowSpan returns the width a row of items with the given widths
// needs, packed side by side with at least gap between each.
func minRowSpan(widths []int, gap int) int {
	if len(widths) == 0 {
		return 0
	}
	sum := 0
	for _, w := range widths {
		sum += w
	}
	return sum + (len(widths)-1)*gap
}

// renderVLANDiagram writes the FrontVLANs page: emitPanelBase's shared
// shell (logo, footer, switch identity, the front illustration, and
// every port's own status marker), then one swimlane container per
// port that carries any network.l2 membership, holding that port's own
// badges in a grid (buildVLANContainers), connected to that port's
// exact position by a single dashed line per container dashed
// because this is a configured logical membership, never a
// physical.ethernet link, and one edge per container (not per badge)
// since every badge inside already names its own VLAN.
func (r *TopologyRenderer) renderVLANDiagram(ctx context.Context, p *nxosPrinter, diagramID, name string, icon assets.Icon, anchor panelAnchorFunc) error {
	var badges []panelVLANBadge
	for _, b := range r.panelVLANBadges {
		if _, _, ok := anchor(b.LocalPort); ok {
			badges = append(badges, b)
		}
	}
	containers := buildVLANContainers(badges, anchor)
	page := choosePanelVLANPage(containers, anchor, icon.Height/icon.Width)

	panelDiagramOpen(p, diagramID, name, page)

	panelID, panelX, panelY, panelW, panelH, err := r.emitPanelBase(ctx, p, page, icon, anchor)
	if err != nil {
		return err
	}

	placements := placePanelVLANContainers(containers, anchor, panelX, panelY, panelW, panelH)

	for ci, c := range containers {
		pl, ok := placements[c.Port]
		if !ok {
			continue
		}
		containerID := "vc" + strconv.Itoa(ci)
		p.cell(containerID, "1", panelVLANContainerStyle, drawio.Attr(c.Port), pl.X, pl.Y, c.Width, c.Height)
		for bi, b := range c.Badges {
			col, row := bi/c.Rows, bi%c.Rows
			bx := panelVLANContainerPad + col*(panelVLANBadgeW+panelVLANBadgeGap)
			by := panelVLANContainerHeaderH + panelVLANContainerPad + row*(panelVLANBadgeH+panelVLANBadgeGap)
			p.cell(containerID+"-b"+strconv.Itoa(bi), containerID, panelVLANBadgeStyle, drawio.Attr(b.Label),
				bx, by, panelVLANBadgeW, panelVLANBadgeH)
		}
		if ci%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	for ci, c := range containers {
		if _, ok := placements[c.Port]; !ok {
			continue
		}
		fx, fy, _ := anchor(c.Port) // already filtered to ok=true above.
		containerID := "vc" + strconv.Itoa(ci)

		// exitY follows which side placePanelVLANContainers put this
		// container on: a container above the panel exits from its own
		// bottom edge (toward the panel below it), a container below it
		// exits from its own top edge (toward the panel above it).
		exitY := 1.0
		if fy >= 0.5 {
			exitY = 0.0
		}
		style := panelVLANEdgeStyle + "exitX=0.5;exitY=" + fnum(exitY) + ";exitDx=0;exitDy=0;" +
			"entryX=" + fnum(fx) + ";entryY=" + fnum(fy) + ";entryDx=0;entryDy=0;entryPerimeter=0;"
		p.edge("vce"+strconv.Itoa(ci), containerID, panelID, style, "")
		if ci%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	panelDiagramClose(p)
	return nil
}

// panelVLANContainerStyle is a collapsible-off swimlane: a titled box
// (the port's own name) holding that port's own badges as children, so
// a busy trunk port's dozens of VLANs stay a labeled group instead of a
// wall of identical dots.
const panelVLANContainerStyle = "swimlane;whiteSpace=wrap;html=1;fillColor=#f5f5f5;fontColor=#333333;strokeColor=#666666;rounded=1;collapsible=0;fontSize=9;startSize=20;"

// panelVLANBadgeStyle is a plain generic rounded rectangle, not a
// vendor icon (the generic renderer is programmatic, not
// composed from a vetted shape library) the same gray as its own
// container, distinct from the green/red port status markers and the
// blue-outlined neighbour boxes, so a reader reads it as
// "configuration", not "connectivity" or "state".
const panelVLANBadgeStyle = "rounded=1;whiteSpace=wrap;html=1;fillColor=#f5f5f5;fontColor=#333333;strokeColor=#666666;fontSize=6;"

// panelVLANEdgeStyle is a dashed line a network.l2 membership is a
// configured logical relationship, never a physical.ethernet link, so
// it never shares panelEdgeStyle's own solid, filled-arrow styling.
const panelVLANEdgeStyle = "rounded=0;orthogonalLoop=1;jettySize=auto;html=1;dashed=1;fillColor=#f5f5f5;strokeColor=#666666;startArrow=classic;startFill=1;endArrow=none;"
