// This file renders the Cisco NX-OS confirmed-model panel pages: three
// diagram page in one .drawio file (FrontPhysical, FrontVLANs, Rear
// panel_vlans.go), over the switch's front and rear illustrations
// (using real hand-drawn chassis if available under
// confirmedPanels variable in panel.go). FrontPhysical and Rear each
// place one status marker per network.switch.port
// resource this illustration draws a connector for (panel_ports.go),
// one draw.io library Cisco19 "l2_switch" icon per CDP/LLDP-discovered
// neighbour, and one edge per physical.ethernet connection, anchored to
// the exact pixel position of that port on whichever illustration
// actually draws it, never a separate synthetic switch icon.
// TopologyRenderer.Render dispatches here when confirmedPanels
// finds an entry for the document's own chassis product ID; a document
// whose model has no confirmed illustration keeps using topology.go's
// generic page.
//
// Page size is content-fit (panel_layout.go, page.go's own ISO-climb-
// then-custom policy), starting from A3 landscape (1654x1169 mxGraph
// user units) and climbing wider only when enough active ports need
// more room than that baseline gives their own neighbour boxes without
// crowding: both illustrations share a 2964x274 aspect ratio that is
// wide and short regardless of page size, so a bigger page only adds
// breathing room around it, never changes whether it fits.
//
// A neighbour's local port is only ever drawn on the one illustration
// that actually places a connector for it: the 48 SFP and 6 QSFP data
// ports live on the front panel (frontPortAnchor); mgmt0, the only
// rear connector any current fixture's connections name, lives on the
// rear panel's management block (rearPortAnchor) on most
// devices chassis. A neighbour whose local port neither anchor places
// gets no edge on either page rather than a guessed position;
// report_render.go's own tables still list it in full.
// Every anchor constant is derived once, by hand, from the
// relevant illustration's own SVG group transform matrices, see each
// function's doc comment for exactly which numbers came from where.
// TestPanelPortAnchor and TestRearPortAnchor pin the derived values
// directly; TestConfirmedPanelTopologyCoversEveryRealFixtureNeighbour
// proves every local port the real testdata fixture's own
// physical.ethernet connections name resolves on one page or the
// other, so no edge in that fixture is silently dropped.
package nxos

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"go.osirisjson.org/consumers/internal/provider/network/assets"
	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/drawio"
)

// Front-illustration geometry, in the illustration's own SVG user
// units (viewBox "0 0 2964 274"). Every constant below is read
// directly off osiris-json-cisco-N9K-C93180YC-FX-front.svg own group
// transforms, not estimated, a redrawn illustration needs these
// re-derived from its own transforms, not guessed.
const (
	panelViewW, panelViewH = 2964, 274

	// Each of the three 16-port SFP groups (sfp-port-group-{01,02,03})
	// places its ports in a fixed 8-column x 2-row grid: column p
	// (0..7) covers the pair (2p+1, 2p+2), the odd port's glyph
	// centered at the group's own local y sfpUpperLocalY, the even
	// port's glyph directly below it at sfpLowerLocalY, pairs
	// sfpPairStepX apart starting at sfpPairFirstX. Read from a pair's
	// own two "matrix(a,0,0,d,e,f)" transforms, e.g. port-01's
	// matrix(0.94594595,0,0,0.92857141,60,474.99999) and port-02's
	// matrix(-0.94594595,0,0,-0.92857141,130,619.99999): each transforms
	// a local 74x70 SFP glyph to a 70x65 glyph whose own center sits at
	// local (95,507.5) and (95,587.5).
	sfpPairFirstX  = 95.0
	sfpPairStepX   = 87.142857
	sfpUpperLocalY = 507.49999
	sfpLowerLocalY = 587.49999

	// A QSFP cage (one per uplink pair, "QSFP-N-N+1") houses its two
	// ports stacked vertically, not side by side: the "49/50"
	// text label's up/down arrows mark which physical slot is which,
	// but each port's own bezel rect (group "port-49"/"port-51"/
	// "port-53", each identical to the other cages' own copy) sits
	// dead center horizontally in the cage and offset vertically
	// qsfpLocalX is that shared local x (composing the bezel rect's own
	// transform, e.g. rect113's matrix(0.88746182,0,0,1,2311.4523,
	// 416.48545), with its parent group's own matrix(1.0295413,0,
	// 0,0.96595083,-70.455804,16.744095)); qsfpUpperLocalY and
	// qsfpLowerLocalY are the odd port's (49/51/53) and the even port's
	// (50/52/54) own bezel-center local y. Confirmed by composing the
	// full transform chain in a throwaway script against the SVG's own
	// matrices, not eyeballed, the small round-cornered indicator
	// triangles ("port-49-indicator" etc.) sit side by side purely as a
	// label glyph and do not mark the actual connector position.
	qsfpLocalX      = 2386.9425
	qsfpUpperLocalY = 445.1292
	qsfpLowerLocalY = 526.0554
	qsfpScaleX      = 0.99497967
	qsfpScaleY      = 0.98595178
)

// sfpGroupOffset is each of the three 16-port groups' own SVG
// transform="translate(tx,ty)", read directly off
// sfp-port-group-{01,02,03}.
var sfpGroupOffset = [3][2]float64{
	{0, -410.08231},
	{740.03239, -410.12709},
	{1480.0324, -410.12709},
}

// qsfpGroupOffset is each of the three QSFP cages' own SVG
// transform="matrix(qsfpScaleX,0,0,qsfpScaleY,tx,ty)", read directly
// off QSFP-{49-50,51-52,53-54}.
var qsfpGroupOffset = [3][2]float64{
	{30.040756, -343.95827},
	{240.30988, -343.70649},
	{450.30988, -343.70649},
}

// panelPortAnchor returns the fractional (0..1) position within the
// front-panel illustration's own coordinate space where localPort's
// connector sits, for use as a fixed mxGraph connection point
// (exitX/exitY) on the embedded panel image, never a separate
// invisible anchor cell. ok is false for a port name this illustration
// does not place: an unrecognized interface family, a module/port
// number this 48+6-port chassis does not have, or mgmt0, the front
// illustration draws only the 48 SFP and 6 QSFP data ports, mgmt0 lives
// on the rear management block instead (rearPortAnchor).
func panelPortAnchor(localPort string) (fx, fy float64, ok bool) {
	n, ok := ethernetPortNumber(localPort)
	if !ok {
		return 0, 0, false
	}
	switch {
	case n >= 1 && n <= 48:
		g := (n - 1) / 16
		m := (n-1)%16 + 1
		p := (m - 1) / 2
		x := sfpPairFirstX + sfpPairStepX*float64(p) + sfpGroupOffset[g][0]
		y := sfpGroupOffset[g][1]
		if m%2 == 1 {
			y += sfpUpperLocalY
		} else {
			y += sfpLowerLocalY
		}
		return x / panelViewW, y / panelViewH, true
	case n >= 49 && n <= 54:
		q := (n - 49) / 2
		localY := qsfpUpperLocalY
		if (n-49)%2 == 1 {
			localY = qsfpLowerLocalY
		}
		x := qsfpScaleX*qsfpLocalX + qsfpGroupOffset[q][0]
		y := qsfpScaleY*localY + qsfpGroupOffset[q][1]
		return x / panelViewW, y / panelViewH, true
	default:
		return 0, 0, false
	}
}

// Rear-illustration geometry, in the same 2964x274 SVG user units the
// front illustration uses. Read directly off
// osiris-json-cisco-N9K-C93180YC-FX-rear.svg own group transforms:
// the "management-block" group's own translate(2590,55), nested inside
// the whole illustration's outer translate(-18,-23) (combined additive
// offset (2572,32)), each port glyph (a 110x61 rect, local center
// (55,30.5)) placed within it by its own
// matrix(a,0,0,1,e,f) e.g. the "management-port" (mgmt0) transform
// matrix(0.80088648,0,0,1,179.01867,42.367223).
const (
	rearMgmtLocalX, rearMgmtLocalY = 2795.067, 104.867
)

// rearPortAnchor returns the fractional (0..1) position within the
// rear-panel illustration's own coordinate space where localPort's
// connector sits. Only mgmt0 is wired: the rear illustration also
// draws L1, L2, and console-port glyphs, but no confirmed OSIRIS JSON
// interface-name convention for them exists yet to match against, so
// they are left unrecognized (ok=false) rather than guessed, add them
// here once a real fixture's own connections name them.
func rearPortAnchor(localPort string) (fx, fy float64, ok bool) {
	if localPort == "mgmt0" {
		return rearMgmtLocalX / panelViewW, rearMgmtLocalY / panelViewH, true
	}
	return 0, 0, false
}

// ethernetPortNumber parses "Ethernet1/N" -> N. Only the single-module
// "Ethernet1/N" form this chassis's single-module configuration uses is
// recognized; anything else (a different module number, an
// unrecognized interface family) returns ok=false rather than guessing.
func ethernetPortNumber(localPort string) (int, bool) {
	const prefix = "Ethernet1/"
	if !strings.HasPrefix(localPort, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(localPort, prefix))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// A3 landscape page and confirmed-panel layout geometry, in mxGraph
// user units.
const (
	nxosA3Width  = 1654
	nxosA3Height = 1169

	nxosPanelMarginX = 40 // left/right clearance around the panel image.
)

// panelDiagramID, panelFrontVLANsDiagramID, and panelRearDiagramID are
// this document's three <diagram> ids, distinct from topology.go's
// "osirisjson-cisco-nxos-topology": a confirmed-model
// document never renders the page on the same run, but
// the ids stay distinct so a reader (or a test) can tell which page of
// a given .drawio file is which.
const (
	panelDiagramID           = "osirisjson-cisco-nxos-panel-front"
	panelFrontVLANsDiagramID = "osirisjson-cisco-nxos-panel-front-vlans"
	panelRearDiagramID       = "osirisjson-cisco-nxos-panel-rear"
)

// renderConfirmedPanelPage writes all three panel pages, front
// physical, front VLANs, then rear, as <diagram> tabs in one .drawio
// file. panel is the illustration pair TopologyRenderer.Render already
// resolved for this document's own chassis product ID.
func (r *TopologyRenderer) renderConfirmedPanelPage(ctx context.Context, w io.Writer, panel ConfirmedPanel) error {
	cw := render.NewCheckedWriter(w)
	p := &nxosPrinter{w: cw}

	p.raw(`<mxfile host="osirisjson-consumer docs.osirisjson.org/osiris-consumers/getting-started">` + "\n")

	if err := r.renderPanelDiagram(ctx, p, panelDiagramID, "FrontPhysical", panel.Front, panelPortAnchor); err != nil {
		return err
	}
	if err := r.renderVLANDiagram(ctx, p, panelFrontVLANsDiagramID, "FrontVLANs", panel.Front, panelPortAnchor); err != nil {
		return err
	}
	if err := r.renderPanelDiagram(ctx, p, panelRearDiagramID, "Rear", panel.Rear, rearPortAnchor); err != nil {
		return err
	}

	p.raw(`</mxfile>` + "\n")
	return cw.Err()
}

// panelAnchorFunc resolves a local port name to its fractional position
// on one illustration, panelPortAnchor or rearPortAnchor.
type panelAnchorFunc func(localPort string) (fx, fy float64, ok bool)

// panelDiagramOpen and panelDiagramClose write the <diagram>/
// <mxGraphModel>/<root> boilerplate every confirmed-panel page shares
// (renderPanelDiagram and renderVLANDiagram), sized to page.
func panelDiagramOpen(p *nxosPrinter, diagramID, name string, page PageInfo) {
	p.raw(`  <diagram id="` + diagramID + `" name="` + name + `">` + "\n")
	p.raw(fmt.Sprintf(
		`    <mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="0" arrows="1" fold="1" page="1" pageScale="1" pageWidth="%d" pageHeight="%d" math="0" shadow="0">`+"\n",
		page.Width, page.Height))
	p.raw(`      <root>` + "\n")
	p.raw(`        <mxCell id="0" />` + "\n")
	p.raw(`        <mxCell id="1" parent="0" />` + "\n")
}

func panelDiagramClose(p *nxosPrinter) {
	p.raw(`      </root>` + "\n")
	p.raw(`    </mxGraphModel>` + "\n")
	p.raw(`  </diagram>` + "\n")
}

// emitPanelBase writes the page shell every confirmed-panel diagram
// shares: the logo, the footer band, the switch identity block, the
// panel illustration itself centered on page, and one status marker
// per network.switch.port resource this illustration draws a
// connector for, not only the CDP/LLDP-discovered subset, so a reader
// sees every port's own observed state at a glance (panel_ports.go).
// Returns the panel image's own cell id and placed geometry, for the
// caller's own page-specific content (physical neighbours, or VLAN
// badges) to anchor against.
func (r *TopologyRenderer) emitPanelBase(ctx context.Context, p *nxosPrinter, page PageInfo, icon assets.Icon, anchor panelAnchorFunc) (panelID string, panelX, panelY, panelW, panelH int, err error) {
	p.cell("logo", "1", drawio.LogoStyle, "", nxosLogoX, nxosLogoY, drawio.LogoIntrinsicW, drawio.LogoIntrinsicH)
	r.emitFooter(p, page.Width, page.Height)
	p.textCell("switch-identity", switchIdentityStyle, drawio.Attr(switchIdentityValue(r.switchInfo)),
		switchIdentityX, switchIdentityY, switchIdentityW, switchIdentityH)

	if err = ctx.Err(); err != nil {
		return "", 0, 0, 0, 0, err
	}

	panelW = page.Width - 2*nxosPanelMarginX
	panelH = int(float64(panelW) * icon.Height / icon.Width)
	panelX = (page.Width - panelW) / 2
	panelY = (page.Height - panelH) / 2

	panelID = "panel-image"
	p.cell(panelID, "1", panelImageStyle(icon.DataURI), "", panelX, panelY, panelW, panelH)

	if err = ctx.Err(); err != nil {
		return "", 0, 0, 0, 0, err
	}

	markerIndex := 0
	for _, port := range r.panelPorts {
		fx, fy, ok := anchor(port.LocalPort)
		if !ok {
			continue
		}
		x := panelX + int(fx*float64(panelW)) - portMarkerSize/2
		y := panelY + int(fy*float64(panelH)) - portMarkerSize/2
		p.cell("m"+strconv.Itoa(markerIndex), "1", portMarkerStyleForStatus(port.Status), "", x, y, portMarkerSize, portMarkerSize)
		markerIndex++
		if markerIndex%1024 == 0 {
			if err = ctx.Err(); err != nil {
				return "", 0, 0, 0, 0, err
			}
		}
	}
	return panelID, panelX, panelY, panelW, panelH, ctx.Err()
}

// renderPanelDiagram writes one physical panel page (front or rear) as
// a single <diagram> tab: emitPanelBase's shared shell, then one
// neighbour icon per CDP/LLDP-discovered neighbour whose local port
// anchor resolves on this illustration (skipping any that belong on
// the other page), placed by placePanelNeighbours, and one edge each,
// anchored to that port's exact position.
func (r *TopologyRenderer) renderPanelDiagram(ctx context.Context, p *nxosPrinter, diagramID, name string, icon assets.Icon, anchor panelAnchorFunc) error {
	var neighbours []panelNeighbour
	for _, n := range r.panelNeighbours {
		if _, _, ok := anchor(n.LocalPort); ok {
			neighbours = append(neighbours, n)
		}
	}
	page := chooseConfirmedPanelPage(neighbours, anchor)

	panelDiagramOpen(p, diagramID, name, page)

	panelID, panelX, panelY, panelW, panelH, err := r.emitPanelBase(ctx, p, page, icon, anchor)
	if err != nil {
		return err
	}

	placements := placePanelNeighbours(neighbours, anchor, panelX, panelY, panelW, panelH)

	ids := make([]string, len(neighbours))
	for i, n := range neighbours {
		id := "n" + strconv.Itoa(i)
		ids[i] = id
		// The stencil itself stays the icon's own true iconIntrinsic
		// square, shape=mxgraph.cisco19.rect fills whatever geometry
		// it is given, so widening its own box to make room for a long
		// label would visibly stretch it, not just widen its label. The
		// label lives in a separate sibling cell instead: panelNeighbourBoxW
		// wide, wrapped, below the icon (placePanelNeighbours already
		// reserves panelNeighbourGap of room for it there), sharing
		// only the icon's own left edge as its own layout slot.
		iconX := placements[i].X + (panelNeighbourBoxW-iconIntrinsic)/2
		p.cell(id, "1", neighbourSwitchStyle, "", iconX, placements[i].Y, iconIntrinsic, iconIntrinsic)
		p.cell(id+"-label", "1", neighbourLabelStyle, drawio.Attr("<b>"+n.Label+"</b>"),
			placements[i].X, placements[i].Y+iconIntrinsic, panelNeighbourBoxW, panelNeighbourLabelH)
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	for i, n := range neighbours {
		fx, fy, _ := anchor(n.LocalPort) // already filtered to ok=true above.

		// entryY follows which side placePanelNeighbours put this
		// neighbour on: a box above the panel is entered from its own
		// bottom edge (entryY=1), a box below it from its own top edge
		// (entryY=0), the edge always approaches from the panel's own
		// side of the box.
		entryY := 1.0
		if !placements[i].Above {
			entryY = 0.0
		}

		// exitPerimeter=0 is load-bearing, not decorative: without it,
		// mxGraph treats exitX/exitY as a hint and re-projects a ray
		// from the panel image's own center through that point out to
		// the image's actual bounding-box edge before connecting, and
		// since the panel is far wider than it is tall (2964x274),
		// nearly every such ray exits through the left or right edge
		// regardless of which port it started at, collapsing every
		// port's fine y-position onto "the side" of the chassis. With
		// exitPerimeter=0 the fixed point is used exactly as given.
		style := panelEdgeStyle + "exitX=" + fnum(fx) + ";exitY=" + fnum(fy) + ";exitDx=0;exitDy=0;exitPerimeter=0;" +
			"entryX=0.5;entryY=" + fnum(entryY) + ";entryDx=0;entryDy=0;"
		p.edge("e"+strconv.Itoa(i), panelID, ids[i], style, drawio.Attr(panelEdgeLabel(n)))
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	panelDiagramClose(p)
	return nil
}

// panelEdgeLabel is one physical.ethernet edge's full label: the local
// port name, then its own port mode, VLAN, IP address, and speed
// properties (network.switch.port, never the neighbour, mgmt0 in
// particular carries an IP address and a speed but no port mode or
// VLAN, since it is a management port, not a switched one), then the
// transceiver this switch's own end reports (source_transceiver), then
// the discovery method, every field literally as the document carries
// it, omitting only what that connection or port did not report (never
// a fabricated placeholder line).
func panelEdgeLabel(n panelNeighbour) string {
	var b strings.Builder
	if n.LocalPort != "(absent)" {
		b.WriteString(n.LocalPort)
	}
	if v := n.Port.PortMode; v != "" && v != "(absent)" {
		b.WriteString("<div>" + v + "</div>")
	}
	if v := n.Port.NativeVLAN; v != "" && v != "(absent)" {
		b.WriteString("<div>native vlan " + v + "</div>")
	}
	if v := n.Port.AccessVLAN; v != "" && v != "(absent)" {
		b.WriteString("<div>vlan " + v + "</div>")
	}
	if v := n.Port.IPAddress; v != "" {
		b.WriteString("<div>" + v + "</div>")
	}
	if v := n.Port.SpeedMbps; v != "" && v != "(absent)" {
		b.WriteString("<div>" + v + "Mbps</div>")
	}
	if t := n.Transceiver; t != nil {
		if t.Model != "" {
			b.WriteString("<div>" + t.Model + "</div>")
		}
		if t.PartNumber != "" {
			b.WriteString("<div>" + t.PartNumber + "</div>")
		}
		if t.Vendor != "" {
			b.WriteString("<div>" + t.Vendor)
			if v := n.DiscoveredVia; v != "" && v != "(absent)" {
				b.WriteString("<br><div>discovered_via: " + v + "</div>")
			}
			b.WriteString("</div>")
		}
	} else if v := n.DiscoveredVia; v != "" && v != "(absent)" {
		b.WriteString("<div>discovered_via: " + v + "</div>")
	}
	return b.String()
}

// panelImageStyle mirrors drawio.LogoStyle's own shape=image
// convention for an embedded panel illustration.
func panelImageStyle(dataURI string) string {
	return "shape=image;verticalLabelPosition=bottom;labelBackgroundColor=default;verticalAlign=top;aspect=fixed;imageAspect=0;editableCssRules=.*;image=" + dataURI + ";"
}

// Switch identity block geometry: a right-aligned text block in the top
// right corner of both panel pages, clear of the logo (top left) and
// never overlapping the panel image or neighbour row below it.
const (
	switchIdentityX, switchIdentityY = 840, 20
	switchIdentityW, switchIdentityH = 800, 60
)

const switchIdentityStyle = "text;html=1;whiteSpace=wrap;strokeColor=none;fillColor=none;align=right;verticalAlign=middle;rounded=0;"

// switchIdentityValue is the switch's own name, model, serial, and
// software version, each straight from the document's own fields
// (report_model.go's SwitchInfo, via switchTopoInfo), "(absent)" where
// the document didn't carry one, never fabricated.
func switchIdentityValue(s switchTopoInfo) string {
	name := stringOrAbsent(s.Name)
	model := stringOrAbsent(s.Model)
	serial := stringOrAbsent(s.Serial)
	version := stringOrAbsent(s.SoftwareVersion)
	return `<font style="font-size: 14px;"><b>` + name + `</b></font>` +
		`<div>` + model + `</div>` +
		`<div>Serial&nbsp;` + serial + `</div>` +
		`<div>Version&nbsp;` + version + `</div>`
}

// neighbourSwitchStyle is the draw.io Cisco19 stencil every
// CDP/LLDP-discovered neighbour gets on the confirmed-panel pages: the
// document's own data never says whether a neighbour is itself acting
// as an L2 or L3 device (CDP/LLDP disclose a chassis and a port, not a
// role), so every neighbour gets the same l2_switch glyph, the most
// common real-world CDP/LLDP neighbour of an access/aggregation switch
// rather than guessing per neighbour. Never fabricate an l3_switch
// distinction this fixture's data does not support.
//
// It carries no label of its own and stays exactly iconIntrinsic
// square: shape=mxgraph.cisco19.rect fills whatever geometry it is
// given rather than letterboxing within it, so widening this cell to
// make room for a long remote system name would visibly stretch the
// icon, not just widen its label. neighbourLabelStyle (below) is a
// separate sibling cell for that instead.
const neighbourSwitchStyle = "sketch=0;shape=mxgraph.cisco19.rect;prIcon=l2_switch;html=1;pointerEvents=1;fillColor=#FAFAFA;strokeColor=#005073;labelBackgroundColor=default;"

// neighbourLabelStyle is a neighbour's own remote system name, in a
// plain wrapped text cell panelNeighbourBoxW wide (see
// placePanelNeighbours) positioned just below its own icon, wide
// enough that a real CDP/LLDP remote system name (e.g.
// "MXP-SW-123.example.com/Eth1/7") wraps to a few short lines
// instead of rendering as one unbroken line. Load-bearing, not
// decorative: an unwrapped line that long, centered near the panel's
// own left edge, extends into negative page coordinates, which
// draw.io then tiles as a second page, doubling the file's own
// apparent page width.
const neighbourLabelStyle = "text;html=1;whiteSpace=wrap;strokeColor=none;fillColor=none;align=center;verticalAlign=top;fontSize=10;"

// panelNeighbourLabelH is neighbourLabelStyle's own nominal box height
// a floor, not a clip: draw.io does not clip html=1 wrapped text to a
// shorter declared height, it simply renders past it, so a label
// needing more lines than this still displays in full.
const panelNeighbourLabelH = 60

// portMarkerSize draws a small, high-contrast dot at every port's exact
// position on the panel illustration, so a port is identifiable at a
// glance rather than only by tracing a thin line into a ~30px glyph.
// portMarkerStyle is the neutral fallback color (panel_ports.go's
// portMarkerStyleForStatus): a network.switch.port resource whose own
// Status is neither "active" nor "inactive" gets this marker rather
// than a guessed green or red.
const (
	portMarkerSize  = 12
	portMarkerStyle = "ellipse;whiteSpace=wrap;html=1;fillColor=#ff8000;strokeColor=#802900;strokeWidth=2;"
)

// panelEdgeStyle is every confirmed-panel physical connectivity edge: a
// straight line (never orthogonal, the panel's own ports and
// neighbour boxes already line up vertically) with a filled classic
// arrowhead at both ends, since the connection is bidirectional and
// neither end reads as "more of a source" than the other.
const panelEdgeStyle = "rounded=0;html=1;endArrow=classic;startArrow=classic;fontSize=7;startFill=1;endFill=1;"

// fnum formats a 0..1 exitX/exitY fraction for an mxCell style string.
func fnum(v float64) string {
	return strconv.FormatFloat(v, 'f', 4, 64)
}
