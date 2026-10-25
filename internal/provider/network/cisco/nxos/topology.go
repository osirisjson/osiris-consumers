// This file renders the Cisco NX-OS device connectivity topology page:
// one Cisco19-styled icon for the switch itself on the left, one
// generic device icon for each CDP/LLDP-discovered neighbour stacked
// on the right, and one edge per physical.ethernet connection between
// them, labeled with the switch's own local port. Unlike Cisco APIC's
// multi-pod fabric swimlane page, a single NX-OS document describes
// exactly one switch, so this page draws a view of what
// that switch connects to rather than a fabric layout, ports,
// interfaces, port-channels, VLANs, and VRFs stay fully represented in
// the generic topology model and this report's own tables, and are
// deliberately absent from this curated device-level page.
//
// This is a self-contained alternate renderer for the whole document,
// selected by Applies (auto-detect, no provider flag, no first-
// resource inference) not a per-node style hook layered onto the
// generic renderer.
package nxos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/drawio"
)

// Layout geometry, in mxGraph user units.
const (
	nxosPageMargin = 40
	nxosHeaderH    = 80  // space reserved above the nodes for the logo row.
	nxosColGap     = 420 // horizontal spacing between the switch column and the neighbour column.
	nxosRowGap     = 60  // vertical spacing between stacked neighbour nodes.
	nxosFooterGap  = 40  // gap between the last content row and the footer band.
	nxosFooterRefW = 1169

	nxosLogoX, nxosLogoY = 20, 20

	nxosFooterH      = 30
	nxosFooterMargin = 20 // clearance kept between the footer band and the page's own bottom edge, the same 20 units nxosLogoY keeps at the top, so header and footer sit symmetrically firm to their own corners.
)

// topoNeighbour is one CDP/LLDP-discovered neighbour, as this page's
// layout needs it: just enough to place an icon and
// label an edge, not the full report row (see report_model.go's
// NeighbourInfo for the report's own, fuller reading of the same
// resources).
type topoNeighbour struct {
	ID, Label, LocalPort string
}

// buildNeighbours returns every CDP/LLDP-discovered neighbour resource
// (nxos.isExternallyDiscovered), sorted by ID for deterministic
// placement.
func buildNeighbours(g *graph.Graph) []topoNeighbour {
	var out []topoNeighbour
	var ids []string
	for _, id := range g.ResourceIDs {
		r := g.Resources[id]
		if r != nil && isExternallyDiscovered(r.Provider.Name) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := g.Resources[id]
		var n neighbourProperties
		_ = json.Unmarshal(r.Properties, &n)
		label := n.RemoteSystem
		if label == "" {
			label = labelOr(r.Name, id)
		}
		out = append(out, topoNeighbour{ID: id, Label: label, LocalPort: localPortFor(g, id)})
	}
	return out
}

// TopologyRenderer renders the Cisco NX-OS device connectivity
// topology page.
// The zero value is not usable; construct one with NewTopologyRenderer.
type TopologyRenderer struct {
	switchInfo switchTopoInfo
	neighbours []topoNeighbour
	metadata   document.Metadata
	scope      nxosScopeFields
	artifact   time.Time

	// panelPorts and panelNeighbours are the confirmed-panel pages
	// own richer readings (panel_ports.go, panel_neighbours.go)
	// unused by the generic page below, but always built
	// alongside neighbours (cheap: both are linear passes already
	// implied by g), so ChooseTopologyPage and Render agree on exactly
	// the same content the confirmed-panel pages will draw without a
	// second, possibly-diverging graph pass.
	panelPorts      []panelPort
	panelNeighbours []panelNeighbour
	panelVLANBadges []panelVLANBadge
}

// switchTopoInfo is just enough of the switch resource for this page's
// own icon/label and the confirmed-panel pages identity block
// report_model.go's SwitchInfo carries the fuller reading the report
// table needs.
type switchTopoInfo struct {
	ID, Name, Model, ManagementIP, ChassisProductID string
	Serial, SoftwareVersion                         string
	present                                         bool
}

func buildSwitchTopoInfo(g *graph.Graph) switchTopoInfo {
	s := buildSwitch(g)
	if s.ID == "" {
		return switchTopoInfo{}
	}
	return switchTopoInfo{
		ID: s.ID, Name: s.Name, Model: s.ModelDescription, ManagementIP: s.ManagementIP,
		ChassisProductID: s.ChassisProductID, Serial: s.Serial, SoftwareVersion: s.SoftwareVersion,
		present: true,
	}
}

// NewTopologyRenderer returns a renderer for the document g indexes.
// md supplies the footer's snapshot metadata. artifactTime is stamped
// into the title the same way the generic renderer treats it: the zero
// time omits the generated line.
func NewTopologyRenderer(g *graph.Graph, md document.Metadata, artifactTime time.Time) *TopologyRenderer {
	ports := buildPanelPorts(g)
	return &TopologyRenderer{
		switchInfo:      buildSwitchTopoInfo(g),
		neighbours:      buildNeighbours(g),
		metadata:        md,
		scope:           decodeNXOSScope(md.Scope),
		artifact:        artifactTime.UTC(),
		panelPorts:      ports,
		panelNeighbours: buildPanelNeighbours(g, panelPortsByName(ports)),
		panelVLANBadges: buildPanelVLANBadges(g, buildLAGMemberPortNames(g)),
	}
}

// contentExtent returns the bounding box r's nodes need, before the
// footer band is added below it.
func (r *TopologyRenderer) contentExtent() (w, h int) {
	rows := len(r.neighbours)
	w = nxosPageMargin + nxosColGap + iconIntrinsic + nxosPageMargin
	h = nxosHeaderH + iconIntrinsic
	if rows > 1 {
		h = nxosHeaderH + rows*iconIntrinsic + (rows-1)*nxosRowGap
	}
	return w, h
}

// ChooseTopologyPage returns the page size a TopologyRenderer would
// choose for the document g indexes, the same content-fit computation
// Render uses, exposed as a pure function of g so a caller (the
// consumer facade) can report the choice without re-rendering. A
// confirmed model (ConfirmedPanelForModel) renders three diagram tabs
// (panel_render.go), each sized independently for its own content;
// this reports the FrontPhysical tab's own size, the busiest of the
// three and the one a reader opens first, not a size shared by every
// tab. Anything else falls back to the page's own content-fit ISO climb.
func ChooseTopologyPage(g *graph.Graph) PageInfo {
	ports := buildPanelPorts(g)
	r := &TopologyRenderer{
		switchInfo: buildSwitchTopoInfo(g), neighbours: buildNeighbours(g),
		panelPorts: ports, panelNeighbours: buildPanelNeighbours(g, panelPortsByName(ports)),
	}
	if _, ok := ConfirmedPanelForModel(r.switchInfo.ChassisProductID); ok {
		return chooseConfirmedPanelPage(r.panelNeighbours, panelPortAnchor)
	}
	w, h := r.contentExtent()
	return chooseNXOSPage(w, h+nxosFooterGap+nxosFooterH+nxosFooterMargin)
}

// Render writes the draw.io XML to w. A confirmed model
// (ConfirmedPanelForModel) renders the front-panel page
// (panel_render.go); anything else falls back to the page below.
func (r *TopologyRenderer) Render(ctx context.Context, w io.Writer) error {
	if panel, ok := ConfirmedPanelForModel(r.switchInfo.ChassisProductID); ok {
		return r.renderConfirmedPanelPage(ctx, w, panel)
	}

	cw := render.NewCheckedWriter(w)
	p := &nxosPrinter{w: cw}

	contentW, contentH := r.contentExtent()
	page := chooseNXOSPage(contentW, contentH+nxosFooterGap+nxosFooterH+nxosFooterMargin)

	p.raw(`<mxfile host="osirisjson-consumer docs.osirisjson.org/osiris-consumers/getting-started">` + "\n")
	p.raw(`  <diagram id="osirisjson-cisco-nxos-topology" name="Topology">` + "\n")
	p.raw(fmt.Sprintf(
		`    <mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="0" arrows="1" fold="1" page="1" pageScale="1" pageWidth="%d" pageHeight="%d" math="0" shadow="0">`+"\n",
		page.Width, page.Height))
	p.raw(`      <root>` + "\n")
	p.raw(`        <mxCell id="0" />` + "\n")
	p.raw(`        <mxCell id="1" parent="0" />` + "\n")

	p.cell("logo", "1", drawio.LogoStyle, "", nxosLogoX, nxosLogoY, drawio.LogoIntrinsicW, drawio.LogoIntrinsicH)
	r.emitFooter(p, page.Width, page.Height)

	if err := ctx.Err(); err != nil {
		return err
	}

	_, contentH2 := r.contentExtent()
	switchY := nxosHeaderH
	if contentH2 > iconIntrinsic {
		switchY = nxosHeaderH + (contentH2-nxosHeaderH-iconIntrinsic)/2
	}
	switchID := "switch"
	if r.switchInfo.present {
		p.cell(switchID, "1", r.switchStyle(), r.switchValue(), nxosPageMargin, switchY, iconIntrinsic, iconIntrinsic)
	}

	neighX := nxosPageMargin + nxosColGap
	ids := make([]string, len(r.neighbours))
	for i, n := range r.neighbours {
		id := "n" + strconv.Itoa(i)
		ids[i] = id
		y := nxosHeaderH + i*(iconIntrinsic+nxosRowGap)
		p.cell(id, "1", neighbourStyle, drawio.Attr("<b>"+n.Label+"</b>"), neighX, y, iconIntrinsic, iconIntrinsic)
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if r.switchInfo.present {
		for i, n := range r.neighbours {
			label := ""
			if n.LocalPort != "(absent)" {
				label = n.LocalPort
			}
			p.edge("e"+strconv.Itoa(i), switchID, ids[i], edgeStyle, drawio.Attr(label))
			if i%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
		}
	}

	p.raw(`      </root>` + "\n")
	p.raw(`    </mxGraphModel>` + "\n")
	p.raw(`  </diagram>` + "\n")
	p.raw(`</mxfile>` + "\n")

	return cw.Err()
}

// switchStyle picks the draw.io Cisco19 shape library for the switch: a
// bare rect, plus a model-derived prIcon when the chassis product ID
// matches a confirmed Nexus prefix (icons.go).
func (r *TopologyRenderer) switchStyle() string {
	base := "sketch=0;shape=mxgraph.cisco19.rect;verticalLabelPosition=bottom;html=1;verticalAlign=top;" +
		"aspect=fixed;align=center;pointerEvents=1;fillColor=#FAFAFA;strokeColor=#005073;fontSize=10;" +
		"labelBackgroundColor=default;"
	if icon := prIconForModel(r.switchInfo.ChassisProductID); icon != "" {
		base += "prIcon=" + icon + ";"
	}
	return base
}

// switchValue is the switch icon's label: its bold name, then its
// management IP.
func (r *TopologyRenderer) switchValue() string {
	name := "<b>" + r.switchInfo.Name + "</b>"
	addr := "<div>" + r.switchInfo.ManagementIP + "</div>"
	return drawio.Attr(name + addr)
}

// neighbourStyle is the generic device icon every CDP/LLDP-discovered
// neighbour gets: these are devices this switch merely observed, never
// fully inventoried, so no vendor-specific shape library is asserted
// for them.
const neighbourStyle = "rounded=0;whiteSpace=wrap;html=1;verticalAlign=middle;align=center;fillColor=#FAFAFA;strokeColor=#666666;fontSize=10;"

// edgeStyle is every physical connectivity edge on this page: a plain
// line with no arrowhead (a physical link, not a directional flow).
const edgeStyle = "edgeStyle=orthogonalEdgeStyle;rounded=0;html=1;endArrow=none;startArrow=none;fontSize=9;"

// nxosScopeFields is the subset of "metadata.scope" the footer
// discloses.
type nxosScopeFields struct {
	Purpose string `json:"purpose"`
}

func decodeNXOSScope(raw json.RawMessage) nxosScopeFields {
	var s nxosScopeFields
	if len(raw) == 0 {
		return s
	}
	_ = json.Unmarshal(raw, &s)
	return s
}

// emitFooter writes the bottom band: the OSIRIS JSON credit on the
// left, snapshot timestamp, scope purpose, and generator version
// stacked right to left from the right margin, mirroring Cisco APIC
// and HPE Aruba Central's own topology footers, scaled to the actual
// chosen page width pageW so the band keeps its proportions on every
// ISO page size. Every literal x+width below stays at or under
// nxosFooterRefW (footer-generator, the rightmost, ends at 1159 of
// 1169): scale() only ever grows a page from that reference, never
// shrinks it (every caller's pageW is >= nxosFooterRefW, the smallest
// ISO page in play), so the footer can never scale past the page's own
// right edge. A design that left any element's un-scaled extent past
// nxosFooterRefW would overflow at the reference size already, and
// grow proportionally worse on every larger page.
func (r *TopologyRenderer) emitFooter(p *nxosPrinter, pageW, pageH int) {
	ts := stringOrAbsent(r.metadata.Timestamp)
	purpose := stringOrAbsent(r.scope.Purpose)
	genVersion := "(absent)"
	if r.metadata.Generator != nil {
		genVersion = stringOrAbsent(r.metadata.Generator.Version)
	}
	footerY := pageH - nxosFooterMargin - nxosFooterH
	scale := func(ref int) int {
		return ref * pageW / nxosFooterRefW
	}
	p.textCell("footer-osirisjson", footerTextStyle, drawio.Attr("<b>OSIRIS JSON</b><br>This topology was created with OSIRIS JSON. Read more on https://osirisjson.org"), scale(10), footerY, scale(450), nxosFooterH)
	p.textCell("footer-timestamp", footerTextStyle, drawio.Attr("<div><b>Snapshot created on</b></div><div>"+ts+"</div>"), scale(679), footerY, scale(140), nxosFooterH)
	p.textCell("footer-scope", footerTextStyle, drawio.Attr("<div><b>Snapshot scope</b></div><div>"+purpose+"</div>"), scale(839), footerY, scale(130), nxosFooterH)
	p.textCell("footer-generator", footerTextStyle, drawio.Attr("<div><b>OSIRIS JSON for Cisco NX-OS</b></div><div>"+genVersion+"</div>"), scale(989), footerY, scale(170), nxosFooterH)
}

const footerTextStyle = "text;html=1;whiteSpace=wrap;strokeColor=none;fillColor=none;verticalAlign=middle;rounded=0;align=left;"

// --- xml printer -----------------------------------------------------

type nxosPrinter struct {
	w *render.CheckedWriter
}

func (p *nxosPrinter) raw(s string) { _, _ = p.w.WriteString(s) }

func (p *nxosPrinter) cell(id, parent, style, value string, x, y, w, h int) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" vertex="1" parent="%s">`+"\n",
		drawio.Attr(id), value, style, drawio.Attr(parent)))
	p.raw(fmt.Sprintf(`          <mxGeometry x="%d" y="%d" width="%d" height="%d" as="geometry" />`+"\n", x, y, w, h))
	p.raw("        </mxCell>\n")
}

func (p *nxosPrinter) textCell(id, style, value string, x, y, w, h int) {
	p.cell(id, "1", style, value, x, y, w, h)
}

func (p *nxosPrinter) edge(id, source, target, style, label string) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" edge="1" parent="1" source="%s" target="%s">`+"\n",
		drawio.Attr(id), label, style, drawio.Attr(source), drawio.Attr(target)))
	p.raw(`          <mxGeometry relative="1" as="geometry" />` + "\n")
	p.raw("        </mxCell>\n")
}
