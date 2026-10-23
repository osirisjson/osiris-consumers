// This file renders the Aruba Central topology overview page: a
// provenance badge, a snapshot-metadata footer band, one icon node for
// each network.gateway, network.switch, and
// osiris.hpe.arubacentral.accesspoint resource, and a curved edge for
// each connection whose both endpoints are one of those three visible
// types. It deliberately does not reuse internal/render/drawio swimlane
// layout: that engine nests the group forest as containers, which this
// design replaces with a flat icon grid plus a metadata footer.
//
// Sharing only the lower-level escaping (drawio.Attr) and
// the checked writer keeps the generic renderer's tested layout
// untouched.
//
// This is a self-contained alternate renderer for the whole document,
// selected by Applies (auto-detect, no provider flag, no first-
// resource inference) not a per-node style hook layered onto the
// generic renderer.
//
// Layout: nodes are placed in three fixed columns, left to right:
// network.gateway, network.switch, osiris.hpe.arubacentral.accesspoint,
// each stacked vertically in the model's own sorted order this
// replicate the official HPE Arubacentral web topology composition.
//
// Page size climbs the ISO 216 landscape series from A4 through A0
// (see page.go), then falls back to a custom landscape page above A0;
// content is never scaled or dropped to fit.
package arubacentral

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/drawio"
	"go.osirisjson.org/consumers/internal/topology"
)

// vsxConnectionType is the connection type the document uses to assert
// a VSX pairing between two switches.
const vsxConnectionType = "osiris.hpe.arubacentral.vsx"

// visibleTopologyTypes is every resource type this page draws a node
// for. Everything else (ports, radios, WLANs, tunnel endpoints,
// clients, and any type this package does not recognize) is left off
// this page by design: it stays fully represented in the
// generic topology model, this is a curated device-level view of it.
var visibleTopologyTypes = map[string]bool{
	"network.gateway":                     true,
	"network.switch":                      true,
	"osiris.hpe.arubacentral.accesspoint": true,
}

// acColumnOrder is the fixed left-to-right column order: gateway, then
// switch, then access point.
var acColumnOrder = []string{
	"network.gateway",
	"network.switch",
	"osiris.hpe.arubacentral.accesspoint",
}

// StackedSwitches returns the resource identifiers that participate in
// at least one vsxConnectionType connection: the document's own
// assertion of switch stacking, never an inferred structural guess.
func StackedSwitches(g *graph.Graph) map[string]bool {
	stacked := make(map[string]bool)
	for _, e := range g.Edges {
		if e.Type != vsxConnectionType {
			continue
		}
		if e.Source != "" {
			stacked[e.Source] = true
		}
		if e.Target != "" {
			stacked[e.Target] = true
		}
	}
	return stacked
}

// TopologyRenderer renders the Aruba Central topology overview page.
// The zero value is not usable; construct one with NewTopologyRenderer.
type TopologyRenderer struct {
	model    *topology.Topology
	stacked  map[string]bool
	scope    scopeFields
	metadata document.Metadata
	artifact time.Time
}

// NewTopologyRenderer returns a renderer for model. g supplies the VSX
// stacking assertion (StackedSwitches); md supplies the footer's
// snapshot metadata. artifactTime is stamped into the title the same
// way the generic renderer treats it: the zero time omits the
// generated line.
func NewTopologyRenderer(model *topology.Topology, g *graph.Graph, md document.Metadata, artifactTime time.Time) *TopologyRenderer {
	return &TopologyRenderer{
		model:    model,
		stacked:  StackedSwitches(g),
		scope:    decodeScope(md.Scope),
		metadata: md,
		artifact: artifactTime.UTC(),
	}
}

// Layout geometry, in mxGraph user units.
const (
	acPageMargin = 40
	acHeaderH    = 80   // space reserved above the device columns for the logo row
	acColGap     = 320  // horizontal spacing between the 3 device columns
	acRowGap     = 100  // vertical spacing between stacked nodes in a column
	acFooterGap  = 40   // gap between the last content row and the footer band
	acFooterRefW = 1654 // width the footer offsets below were tuned against; scaled to whatever page is actually chosen

	acLogoX, acLogoY = 20, 20

	acNodeSize = 40
	acFooterH  = 30
)

// acPlacedNode is one visible-type node at its deterministic column
// position.
type acPlacedNode struct {
	node topology.Node
	x, y int
}

// placeByColumn lays out visible in the fixed column order
// (acColumnOrder), each column stacked vertically in the model own
// sorted order (topology.Build sorts Nodes by ID), so placement stays
// deterministic. contentW/contentH is the bounding box these nodes
// need, before the footer band is added below it.
func placeByColumn(visible []topology.Node) (placed []acPlacedNode, contentW, contentH int) {
	colOf := make(map[string]int, len(acColumnOrder))
	for i, t := range acColumnOrder {
		colOf[t] = i
	}
	rows := make([]int, len(acColumnOrder))
	for _, n := range visible {
		c := colOf[n.Type]
		row := rows[c]
		rows[c]++
		placed = append(placed, acPlacedNode{
			node: n,
			x:    acPageMargin + c*acColGap,
			y:    acHeaderH + row*(acNodeSize+acRowGap),
		})
	}
	maxRows := 0
	for _, rc := range rows {
		if rc > maxRows {
			maxRows = rc
		}
	}
	contentW = acPageMargin + (len(acColumnOrder)-1)*acColGap + acNodeSize + acPageMargin
	contentH = acHeaderH
	if maxRows > 0 {
		contentH += maxRows*acNodeSize + (maxRows-1)*acRowGap
	}
	return placed, contentW, contentH
}

// ChooseTopologyPage returns the page size a TopologyRenderer would
// choose for model the same content-fit computation Render uses,
// exposed as a pure function of the model so a caller (the consumer
// facade) can report the choice without re-rendering. Mirrors
// internal/render/drawio.ChoosePage's role for the generic renderer.
func ChooseTopologyPage(model *topology.Topology) PageInfo {
	var visible []topology.Node
	for _, n := range model.Nodes {
		if visibleTopologyTypes[n.Type] {
			visible = append(visible, n)
		}
	}
	_, contentW, contentH := placeByColumn(visible)
	return chooseACPage(contentW, contentH+acFooterGap+acFooterH+acPageMargin)
}

// Render writes the draw.io XML to w.
func (r *TopologyRenderer) Render(ctx context.Context, w io.Writer) error {
	cw := render.NewCheckedWriter(w)
	p := &acPrinter{w: cw}
	m := r.model

	var visible []topology.Node
	for _, n := range m.Nodes {
		if visibleTopologyTypes[n.Type] {
			visible = append(visible, n)
		}
	}
	placed, contentW, contentH := placeByColumn(visible)
	page := chooseACPage(contentW, contentH+acFooterGap+acFooterH+acPageMargin)

	p.raw(`<mxfile host="osirisjson-consumer docs.osirisjson.org/osiris-consumers/getting-started">` + "\n")
	p.raw(`  <diagram id="osirisjson-hpe-arubacentral-topology" name="Topology">` + "\n")
	p.raw(fmt.Sprintf(
		`    <mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="0" arrows="1" fold="1" page="1" pageScale="1" pageWidth="%d" pageHeight="%d" math="0" shadow="0">`+"\n",
		page.Width, page.Height))
	p.raw(`      <root>` + "\n")
	p.raw(`        <mxCell id="0" />` + "\n")
	p.raw(`        <mxCell id="1" parent="0" />` + "\n")

	p.cell("logo", "1", drawio.LogoStyle, "", acLogoX, acLogoY, drawio.LogoIntrinsicW, drawio.LogoIntrinsicH)
	r.emitFooter(p, page.Width, page.Height)

	if err := ctx.Err(); err != nil {
		return err
	}

	ids := make(map[string]string, len(placed))
	for i, pl := range placed {
		id := "n" + strconv.Itoa(i)
		ids[pl.node.ID] = id
		p.cell(id, "1", r.nodeStyle(pl.node), r.nodeValue(pl.node), pl.x, pl.y, acNodeSize, acNodeSize)
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// A connection is drawn only when both endpoints are one of the
	// visible types above not because it is unresolved, but because
	// this page curates to devices only. A port-to-device "contains"
	// edge, for instance, never appears here.
	edgeSeq := 0
	for _, e := range m.Edges {
		src, srcOK := ids[e.Source]
		dst, dstOK := ids[e.Target]
		if !srcOK || !dstOK {
			continue
		}
		p.edge("e"+strconv.Itoa(edgeSeq), src, dst, edgeStyle(e))
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

// nodeStyle picks the vendor icon for one of the three visible types
// (nodeStyle is only ever called with one of them see
// visibleTopologyTypes). A switch that StackedSwitches names gets the
// stack variant.
func (r *TopologyRenderer) nodeStyle(n topology.Node) string {
	switch n.Type {
	case "network.gateway":
		return iconStyle(gatewayIconDataURI)
	case "network.switch":
		if r.stacked[n.ID] {
			return iconStyle(switchStackIconDataURI)
		}
		return iconStyle(switchIconDataURI)
	case "osiris.hpe.arubacentral.accesspoint":
		return iconStyle(accesspointIconDataURI)
	default:
		// Unreachable given visibleTopologyTypes, but a style string
		// is still required if this ever changes.
		return genericNodeStyle
	}
}

// nodeValue is the icon label: the resource's own name (or its opaque
// ID when unnamed Node.Label already carries that fallback), shown
// in bold via the style's fontStyle=1, not the value itself.
func (r *TopologyRenderer) nodeValue(n topology.Node) string {
	return drawio.Attr(n.Label)
}

// emitFooter writes the snapshot-metadata band: SITE, the OSIRIS JSON
// credit line, CLUSTER, snapshot timestamp, generator version, and
// scope purpose, each shown as the document asserts it, "(absent)"
// when the document carried no such field (evidence-state, never
// fabricated). Every offset and width was tuned against acFooterRefW
// and is scaled to the actual chosen page width pageW so the band
// keeps its proportions on every ISO page size, and pageH anchors it
// near the bottom of whichever page was chosen.
func (r *TopologyRenderer) emitFooter(p *acPrinter, pageW, pageH int) {
	site := joinOrAbsent(r.scope.Sites)
	cluster := joinOrAbsent(r.scope.Clusters)
	ts := stringOrAbsent(r.metadata.Timestamp)
	purpose := stringOrAbsent(r.scope.Purpose)
	genVersion := "(absent)"
	if r.metadata.Generator != nil {
		genVersion = stringOrAbsent(r.metadata.Generator.Version)
	}
	footerY := pageH - acPageMargin - acFooterH
	scale := func(ref int) int {
		return int(math.Round(float64(ref) * float64(pageW) / float64(acFooterRefW)))
	}

	// Each label is composed as raw HTML (including its own literal
	// markup) and escaped exactly once, at the end, by drawio.Attr
	// the same single-escape-point convention nodeValue and the
	// generic renderer's titleValue use. Escaping only the dynamic
	// pieces and leaving the surrounding "<b>"/"<div>" tags raw would
	// produce an unescaped "<" inside the XML attribute.
	p.textCell("footer-site", footerTextStyle+"align=right;", drawio.Attr("<b>SITE</b><br>"+site), scale(1160), acLogoY, scale(480), acFooterH)
	p.textCell("footer-osirisjson", footerTextStyle, drawio.Attr("<b>OSIRIS JSON</b><br>This topology was created with OSIRIS JSON. Read more on https://osirisjson.org"), scale(10), footerY, scale(450), acFooterH)
	p.textCell("footer-cluster", footerTextStyle, drawio.Attr("<b>CLUSTER</b><div>"+cluster+"</div>"), scale(720), footerY, scale(360), acFooterH)
	p.textCell("footer-timestamp", footerTextStyle, drawio.Attr("<div><b>Snapshot created on</b></div><div>"+ts+"</div>"), scale(1100), footerY, scale(140), acFooterH)
	p.textCell("footer-scope", footerTextStyle, drawio.Attr("<div><b>Snapshot scope</b></div><div>"+purpose+"</div>"), scale(1260), footerY, scale(140), acFooterH)
	p.textCell("footer-generator", footerTextStyle, drawio.Attr("<div><b>OSIRIS JSON for Aruba Central</b></div><div>"+genVersion+"</div>"), scale(1440), footerY, scale(200), acFooterH)
}

// scopeFields is the subset of "metadata.scope" the footer discloses.
// Decoded best-effort: an absent or malformed field stays zero rather
// than failing the render (the receipt already gates malformed input;
// this is presentation over an already-validated document).
type scopeFields struct {
	Purpose   string   `json:"purpose"`
	Providers []string `json:"providers"`
	Sites     []string `json:"sites"`
	Clusters  []string `json:"clusters"`
}

func decodeScope(raw json.RawMessage) scopeFields {
	var s scopeFields
	if len(raw) == 0 {
		return s
	}
	_ = json.Unmarshal(raw, &s)
	return s
}

func joinOrAbsent(vals []string) string {
	if len(vals) == 0 {
		return "(absent)"
	}
	out := vals[0]
	for _, v := range vals[1:] {
		out += ", " + v
	}
	return out
}

func stringOrAbsent(s string) string {
	if s == "" {
		return "(absent)"
	}
	return s
}

// --- draw.io styles --------------------------------------------------

const (
	footerTextStyle  = "text;html=1;whiteSpace=wrap;strokeColor=none;fillColor=none;verticalAlign=middle;rounded=0;align=left;"
	genericNodeStyle = "rounded=0;whiteSpace=wrap;html=1;verticalAlign=middle;align=center;"
)

// iconStyle carries fontStyle=1 so the device name below the icon
// renders bold.
func iconStyle(dataURI string) string {
	return "shape=image;verticalLabelPosition=bottom;labelBackgroundColor=default;verticalAlign=top;aspect=fixed;imageAspect=0;editableCssRules=.*;image=" + dataURI + ";fontStyle=1;"
}

// edgeStyle maps the literal connection direction onto arrow heads,
// mirroring internal/render/drawio's generic edgeStyle (that function
// is unexported to this package's own renderer, drawn separately).
// Every edge on this page is curved: with the flat,
// swimlane-free icon grid, curved routing reads better than orthogonal
// right angles. This replicate the same visual topology composition
// available on HPE Arubacentral web interface.
func edgeStyle(e topology.Edge) string {
	base := "edgeStyle=orthogonalEdgeStyle;rounded=0;curved=1;html=1;"
	switch e.Direction {
	case "reverse":
		base += "endArrow=none;startArrow=block;"
	case "bidirectional":
		base += "endArrow=block;startArrow=block;"
	default:
		base += "endArrow=block;startArrow=none;"
	}
	return base
}

// --- xml printer -----------------------------------------------------

type acPrinter struct {
	w *render.CheckedWriter
}

func (p *acPrinter) raw(s string) { _, _ = p.w.WriteString(s) }

func (p *acPrinter) cell(id, parent, style, value string, x, y, w, h int) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" vertex="1" parent="%s">`+"\n",
		drawio.Attr(id), value, style, drawio.Attr(parent)))
	p.raw(fmt.Sprintf(`          <mxGeometry x="%d" y="%d" width="%d" height="%d" as="geometry" />`+"\n", x, y, w, h))
	p.raw("        </mxCell>\n")
}

func (p *acPrinter) textCell(id, style, value string, x, y, w, h int) {
	p.cell(id, "1", style, value, x, y, w, h)
}

func (p *acPrinter) edge(id, source, target, style string) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="" style="%s" edge="1" parent="1" source="%s" target="%s">`+"\n",
		drawio.Attr(id), style, drawio.Attr(source), drawio.Attr(target)))
	p.raw(`          <mxGeometry relative="1" as="geometry" />` + "\n")
	p.raw("        </mxCell>\n")
}
