// This file renders the Cisco vManage draw.io topology in one diagram
// tab per network.router this document describes, each showing
// that router's own hand-drawn front-panel illustration (panel.go)
// with a status-colored marker at every one of its own ports the
// illustration actually draws. Each port gets its own titled container
// (a swimlane, colored the same as its marker) holding a small badge
// for every IP address that port reports, connected back to its own
// marker by an edge carrying that port's own speed/MTU/VRF/MAC.
//
// Unlike Cisco NX-OS's confirmed panel, there is no CDP/LLDP-style
// neighbour resource still in the vmanage producer's own data model
// so there is nothing to draw as a connected device box every container
// describes the port itself, not a remote neighbour.
//
// ConfirmedTopologyApplies is a stricter, whole-document check than
// Applies alone: every router this document describes must resolve to
// a confirmed model (panel.go), not just at least one. A document
// mixing a confirmed and an unconfirmed router falls back to the fully
// generic topology entirely, rather than silently drawing a page for
// some of its routers and dropping the rest.
package vmanage

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

// Front-illustration geometry, in the illustration's own SVG user
// units (viewBox "0 0 2964 274" the same canvas convention
// internal/provider/network/cisco/nxos's own front illustration uses,
// coincidentally or by the same author's own habit, not because the
// two share any geometry). Every constant below was read by composing
// the SVG's own group transform matrices in a throwaway script, not
// eyeballed see each constant's own comment for exactly which
// elements it came from.
const (
	panelViewW, panelViewH = 2964, 274

	// wan-rj45-0 and wan-rj45-1 (GigabitEthernet0/0/0 and 0/0/1) are
	// plain rects with no group transform of their own, just the
	// "WAN-ports" layer's shared translate(-18,-23): rect x=670 y=99
	// w=104 h=75 and x=670 y=190 w=104 h=75, centers translated.
	ge0PortLocalX  = 704.0
	ge0UpperLocalY = 113.5
	ge0LowerLocalY = 204.5

	// The two SFP cages (GigabitEthernet0/0/2 and 0/0/3) each carry
	// their own group transform in addition to the layer's shared
	// translate(-18,-23): 0/0/2's own
	// matrix(1.402242,0,0,1.0652249,841.32865,99.161968) and 0/0/3's
	// own matrix(-1.4050855,0,0,-1.0684786,944.9516,264.37985) (a
	// negative scale, the cage is drawn rotated 180 degrees), each
	// applied to their own 74x70 local rect's center (37,35). The two
	// composed x results (875.21 and 874.96) differ only by hand-
	// placement sub-pixel noise; rounded to one shared constant.
	ge23PortLocalX  = 875.0
	ge23UpperLocalY = 113.44
	ge23LowerLocalY = 203.98

	// console-rj45 (rect id="rect4"): x=230 y=217 w=104 h=75, no own
	// group transform beyond the "Console and USB" layer's shared
	// translate(-18,-23). This is this router's own out-of-band
	// management access point, properties.management_ip has no
	// dedicated network.router.port resource of its own to anchor to,
	// so it anchors here instead of being dropped.
	consoleLocalX = 264.0
	consoleLocalY = 231.5
)

// panelPortAnchor returns the fractional (0..1) position within the
// confirmed panel illustration's own coordinate space where localPort's
// connector sits. Only the four built-in Gigabit Ethernet ports this
// illustration actually draws resolve; anything else, a port a NIM
// module in this unit's own NIM bay would add (this base illustration
// cannot know what, if anything, is installed there), a subinterface,
// a loopback, a tunnel, or the system/vmanage-mgmt interfaces, returns
// ok=false rather than a guessed position.
func panelPortAnchor(localPort string) (fx, fy float64, ok bool) {
	var x, y float64
	switch localPort {
	case "GigabitEthernet0/0/0":
		x, y = ge0PortLocalX, ge0UpperLocalY
	case "GigabitEthernet0/0/1":
		x, y = ge0PortLocalX, ge0LowerLocalY
	case "GigabitEthernet0/0/2":
		x, y = ge23PortLocalX, ge23UpperLocalY
	case "GigabitEthernet0/0/3":
		x, y = ge23PortLocalX, ge23LowerLocalY
	default:
		return 0, 0, false
	}
	return x / panelViewW, y / panelViewH, true
}

// consoleAnchor returns the fractional (0..1) position of the console
// connector this router's own management_ip is anchored to.
func consoleAnchor() (fx, fy float64) {
	return consoleLocalX / panelViewW, consoleLocalY / panelViewH
}

// routerModel returns router resource r's own "properties.model"
// string, or "" when absent.
func routerModel(r *document.Resource) string {
	var p routerProperties
	_ = json.Unmarshal(r.Properties, &p)
	return p.Model
}

// ConfirmedTopologyApplies reports whether g qualifies for the
// confirmed-panel topology: Applies(g) is true, at least one
// network.router resource exists, and every one of them resolves to a
// confirmed model (panel.go) see this file's own doc comment for why
// a mixed document falls back to fully generic instead of a partial
// confirmed page.
func ConfirmedTopologyApplies(g *graph.Graph) bool {
	if !Applies(g) {
		return false
	}
	var any bool
	for _, id := range g.ByType["network.router"] {
		r := g.Resources[id]
		if r == nil {
			continue
		}
		any = true
		if _, ok := ConfirmedPanelForModel(routerModel(r)); !ok {
			return false
		}
	}
	return any
}

// panelPort is one of a router's own network.router.port resources,
// filtered to those panelPortAnchor resolves.
type panelPort struct {
	Name, Status          string
	SpeedMbps, MTU, VRFID string
	MACAddress            string
	PublicIP              string
	PrivateIP             []string
	Color, NATType        string
}

// panelRouter is one network.router this document describes, with its
// own confirmed panel, its own identity fields, and the subset of its
// own ports the illustration draws a connector for.
type panelRouter struct {
	ID, Name         string
	Panel            ConfirmedPanel
	Ports            []panelPort
	ManagementIP     string
	Status           string // resource-level, literal colors the management marker.
	ChassisNumber    string
	Serial           string
	SoftwareVersion  string
	ConnectedManager []string
	Site             string
}

// buildPanelRouters reads every network.router resource under
// ProviderName once, sorted by ID, each with its own confirmed panel,
// its own identity fields, and its own ports (resolved from the
// document's own "contains" connections, source the router, target the
// port) never assuming exactly one router the way a single-diagram
// design would.
func buildPanelRouters(g *graph.Graph) []panelRouter {
	var out []panelRouter
	for _, id := range sortedByType(g, "network.router") {
		r := g.Resources[id]
		if r.Provider.Name != ProviderName {
			continue
		}
		panel, ok := ConfirmedPanelForModel(routerModel(r))
		if !ok {
			continue
		}
		var props routerProperties
		_ = json.Unmarshal(r.Properties, &props)
		ext := decodeRouterExtensions(r.Extensions)
		pr := panelRouter{
			ID: id, Name: labelOr(r.Name, id), Panel: panel,
			ManagementIP: props.ManagementIP, Status: r.Status,
			ChassisNumber: ext.ChassisNumber, Serial: props.SerialNumber, SoftwareVersion: props.Version,
			ConnectedManager: ext.ConnectedManager, Site: siteNameFor(g, id),
		}
		for _, idx := range g.OutEdges[id] {
			e := g.Edges[idx]
			if e.Type != "contains" {
				continue
			}
			pt := g.Resources[e.Target]
			if pt == nil || pt.Type != "network.router.port" {
				continue
			}
			var pp portProperties
			_ = json.Unmarshal(pt.Properties, &pp)
			ext := decodePortExtensions(pt.Extensions)
			name := pp.InterfaceName
			if name == "" {
				name = labelOr(pt.Name, e.Target)
			}
			pr.Ports = append(pr.Ports, panelPort{
				Name: name, Status: pt.Status,
				SpeedMbps: int64OrAbsent(pp.SpeedMbps), MTU: int64OrAbsent(pp.MTU), VRFID: stringOrAbsent(pp.VRFID),
				MACAddress: stringOrAbsent(pp.MACAddress), PublicIP: stringOrAbsent(pp.IPAddresses.PublicIP),
				PrivateIP: pp.IPAddresses.PrivateIP,
				Color:     stringOrAbsent(ext.Color), NATType: stringOrAbsent(ext.NATType),
			})
		}
		sort.Slice(pr.Ports, func(i, j int) bool { return pr.Ports[i].Name < pr.Ports[j].Name })
		out = append(out, pr)
	}
	return out
}

// TopologyRenderer renders the Cisco vManage confirmed-panel topology:
// one diagram tab per confirmed router.
//
// The zero value is not usable; construct one with NewTopologyRenderer.
type TopologyRenderer struct {
	routers  []panelRouter
	metadata document.Metadata
	scope    vmanageScopeFields
	artifact time.Time
}

// vmanageScopeFields is the subset of "metadata.scope" the footer
// discloses.
type vmanageScopeFields struct {
	Purpose string `json:"purpose"`
}

func decodeVManageScope(raw json.RawMessage) vmanageScopeFields {
	var s vmanageScopeFields
	if len(raw) == 0 {
		return s
	}
	_ = json.Unmarshal(raw, &s)
	return s
}

// NewTopologyRenderer returns a renderer for the document g indexes.
// Callers check ConfirmedTopologyApplies(g) first; this constructor
// does not re-check it.
func NewTopologyRenderer(g *graph.Graph, md document.Metadata, artifactTime time.Time) *TopologyRenderer {
	return &TopologyRenderer{
		routers: buildPanelRouters(g), metadata: md,
		scope: decodeVManageScope(md.Scope), artifact: artifactTime.UTC(),
	}
}

// Page and panel geometry, in mxGraph user units. The page is always
// fixed A3 landscape, not content-fit like Cisco NX-OS's own climb
// because this illustration's own four ports, plus the management
// marker, never come close to needing more room; revisit if a future
// confirmed model's own illustration carries enough ports to need it.
// The panel image itself keeps an intentionally asymmetric margin
// (vmanagePanelLeftMargin, vmanagePanelRightMargin), not centered
// margins on both sides.
const (
	vmanageA3Width  = 1654
	vmanageA3Height = 1169

	vmanagePanelLeftMargin  = 40
	vmanagePanelRightMargin = 50

	vmanageFooterH      = 30
	vmanageFooterMargin = 20

	vmanageLogoX, vmanageLogoY = 20, 20

	vmanagePortMarkerSize = 12

	vmanagePortContainerW    = 160
	vmanagePortContainerH    = 80
	vmanagePortContainerGap  = 160 // minimum horizontal clearance enforced between two adjacent containers on the same side.
	vmanagePortContainerVGap = 150 // vertical clearance between the panel image and a container placed above or below it.

	vmanagePortBadgeW = 80
	vmanagePortBadgeH = 20
)

// ChooseTopologyPage returns the fixed A3 landscape page every
// confirmed-panel diagram uses, exposed as a pure function of g so a
// caller (the consumer facade) can report the choice without
// re-rendering. ok is false when g does not qualify
// (ConfirmedTopologyApplies).
func ChooseTopologyPage(g *graph.Graph) (PageInfo, bool) {
	if !ConfirmedTopologyApplies(g) {
		return PageInfo{}, false
	}
	return PageInfo{Format: "A3", Width: vmanageA3Width, Height: vmanageA3Height}, true
}

// Render writes the draw.io XML to w: one <diagram> tab per confirmed
// router.
func (r *TopologyRenderer) Render(ctx context.Context, w io.Writer) error {
	cw := render.NewCheckedWriter(w)
	p := &vmanagePrinter{w: cw}

	p.raw(`<mxfile host="osirisjson-consumer docs.osirisjson.org/osiris-consumers/getting-started">` + "\n")
	for i, router := range r.routers {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.renderRouterDiagram(ctx, p, "router-"+strconv.Itoa(i), router); err != nil {
			return err
		}
	}
	p.raw(`</mxfile>` + "\n")
	return cw.Err()
}

func (r *TopologyRenderer) renderRouterDiagram(ctx context.Context, p *vmanagePrinter, diagramID string, router panelRouter) error {
	const pageW, pageH = vmanageA3Width, vmanageA3Height

	p.raw(`  <diagram id="` + drawio.Attr(diagramID) + `" name="` + drawio.Attr(router.Name) + `">` + "\n")
	p.raw(fmt.Sprintf(
		`    <mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="0" arrows="1" fold="1" page="1" pageScale="1" pageWidth="%d" pageHeight="%d" math="0" shadow="0">`+"\n",
		pageW, pageH))
	p.raw(`      <root>` + "\n")
	p.raw(`        <mxCell id="0" />` + "\n")
	p.raw(`        <mxCell id="1" parent="0" />` + "\n")

	p.cell("logo", "1", drawio.LogoStyle, "", vmanageLogoX, vmanageLogoY, drawio.LogoIntrinsicW, drawio.LogoIntrinsicH)
	r.emitFooter(p, pageW, pageH)
	p.textCell("router-identity", routerIdentityStyle, drawio.Attr(routerIdentityValue(router)),
		routerIdentityX, routerIdentityY, routerIdentityW, routerIdentityH)

	if err := ctx.Err(); err != nil {
		return err
	}

	panelW := pageW - vmanagePanelLeftMargin - vmanagePanelRightMargin
	panelH := int(float64(panelW) * router.Panel.Front.Height / router.Panel.Front.Width)
	panelX := vmanagePanelLeftMargin
	panelY := (pageH - panelH) / 2

	const panelID = "panel-image"
	p.cell(panelID, "1", panelImageStyle(router.Panel.Front.DataURI), "", panelX, panelY, panelW, panelH)

	if err := ctx.Err(); err != nil {
		return err
	}

	// The management marker: this router's own console connector,
	// doubling as its out-of-band management access point, colored by
	// the router's own resource-level status (there is no dedicated
	// port resource, and so no port-level status, for this field).
	if router.ManagementIP != "" && router.ManagementIP != "(absent)" {
		fx, fy := consoleAnchor()
		mgmtID := "mgmt-marker"
		mx := panelX + int(fx*float64(panelW)) - vmanagePortMarkerSize/2
		my := panelY + int(fy*float64(panelH)) - vmanagePortMarkerSize/2
		p.cell(mgmtID, "1", portMarkerStyleForStatus(router.Status), "", mx, my, vmanagePortMarkerSize, vmanagePortMarkerSize)

		badgeID := "mgmt-badge"
		badgeX := mx + vmanagePortMarkerSize/2 - vmanagePortBadgeW/2
		badgeY := my - vmanagePortContainerVGap
		if fy >= 0.5 {
			badgeY = my + vmanagePortContainerVGap
		}
		p.cell(badgeID, "1", portBadgeStyle, drawio.Attr("<b>"+router.ManagementIP+"</b>"),
			badgeX, badgeY, vmanagePortBadgeW, vmanagePortBadgeH)
		p.edge("mgmt-edge", badgeID, mgmtID, portEdgeStyle+"entryX=0.5;entryY=0.5;entryDx=0;entryDy=0;entryPerimeter=0;", "")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	placements := placePortContainers(router.Ports, panelX, panelY, panelW, panelH)

	for i, port := range router.Ports {
		pl, ok := placements[i]
		if !ok {
			continue
		}
		markerID := "m" + strconv.Itoa(i)
		fx, fy, _ := panelPortAnchor(port.Name) // already filtered to ok=true by placePortContainers.
		mx := panelX + int(fx*float64(panelW)) - vmanagePortMarkerSize/2
		my := panelY + int(fy*float64(panelH)) - vmanagePortMarkerSize/2
		p.cell(markerID, "1", portMarkerStyleForStatus(port.Status), "", mx, my, vmanagePortMarkerSize, vmanagePortMarkerSize)

		containerID := "c" + strconv.Itoa(i)
		p.cell(containerID, "1", portContainerStyleForStatus(port.Status), drawio.Attr("<b>"+port.Name+"</b>"),
			pl.X, pl.Y, vmanagePortContainerW, vmanagePortContainerH)

		badges := portIPBadges(port)
		badgeX := (vmanagePortContainerW - vmanagePortBadgeW) / 2
		badgeStartY := (vmanagePortContainerH-20)/2 - (len(badges)-1)*11
		for bi, ip := range badges {
			p.cell(containerID+"-b"+strconv.Itoa(bi), containerID, portBadgeStyle, drawio.Attr("<b>"+ip+"</b>"),
				badgeX, badgeStartY+bi*22, vmanagePortBadgeW, vmanagePortBadgeH)
		}

		p.edge("e"+strconv.Itoa(i), containerID, markerID, portEdgeStyle+"entryX=0.5;entryY=0.5;entryDx=0;entryDy=0;entryPerimeter=0;",
			drawio.Attr(portEdgeLabel(port)))

		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	p.raw(`      </root>` + "\n")
	p.raw(`    </mxGraphModel>` + "\n")
	p.raw(`  </diagram>` + "\n")
	return nil
}

// portPlacement is one port's own container position (top-left, in
// mxGraph user units).
type portPlacement struct {
	X, Y int
}

// placePortContainers positions each port's own container centered on
// its own physical anchor, above the panel when that port sits in the
// illustration's own upper half, below when it sits in the lower half
// (mirrors Cisco NX-OS's own placePanelNeighbours rule), spacing
// adjacent containers on the same side at least
// vmanagePortContainerW+vmanagePortContainerGap apart, two of this
// illustration's own ports (e.g. 0/0/0 and 0/0/2) sit only ~90 units
// apart natively, far closer than one 160-wide container needs. Ports
// panelPortAnchor does not resolve are omitted from the result map.
func placePortContainers(ports []panelPort, panelX, panelY, panelW, panelH int) map[int]portPlacement {
	type candidate struct {
		index  int
		idealX float64
	}
	var above, below []candidate
	for i, port := range ports {
		fx, fy, ok := panelPortAnchor(port.Name)
		if !ok {
			continue
		}
		idealX := float64(panelX) + fx*float64(panelW) - vmanagePortContainerW/2
		c := candidate{index: i, idealX: idealX}
		if fy < 0.5 {
			above = append(above, c)
		} else {
			below = append(below, c)
		}
	}

	out := make(map[int]portPlacement, len(above)+len(below))
	const pitch = vmanagePortContainerW + vmanagePortContainerGap
	place := func(cands []candidate, y int) {
		sort.Slice(cands, func(i, j int) bool { return cands[i].idealX < cands[j].idealX })
		lastX := -1 << 62
		for _, c := range cands {
			x := int(c.idealX)
			if lastX != -1<<62 && x-lastX < pitch {
				x = lastX + pitch
			}
			lastX = x
			out[c.index] = portPlacement{X: x, Y: y}
		}
	}
	place(above, panelY-vmanagePortContainerVGap-vmanagePortContainerH)
	place(below, panelY+panelH+vmanagePortContainerVGap)
	return out
}

// portIPBadges returns the IP addresses to badge inside a port's own
// container: its own public IP first (if present), then every private
// IP it reports, never fabricated, never more than the document
// itself carries.
func portIPBadges(port panelPort) []string {
	var out []string
	if port.PublicIP != "" && port.PublicIP != "(absent)" {
		out = append(out, port.PublicIP)
	}
	out = append(out, port.PrivateIP...)
	return out
}

// portEdgeLabel is one port's own connecting-edge label: its speed,
// MTU, VRF, MAC address, and SD-WAN transport color/NAT type, every
// field literally as the document carries it, omitting only what that
// port did not report.
func portEdgeLabel(port panelPort) string {
	var b strings.Builder
	if v := port.SpeedMbps; v != "" && v != "(absent)" {
		b.WriteString("<div>" + v + " mbps</div>")
	}
	if v := port.MTU; v != "" && v != "(absent)" {
		b.WriteString("<div>" + v + " mtu</div>")
	}
	if v := port.VRFID; v != "" && v != "(absent)" {
		b.WriteString("<div>vrf " + v + "</div>")
	}
	if v := port.MACAddress; v != "" && v != "(absent)" {
		b.WriteString("<div>" + v + "</div>")
	}
	if v := port.Color; v != "" && v != "(absent)" {
		b.WriteString("<div>" + v + "</div>")
	}
	if v := port.NATType; v != "" && v != "(absent)" {
		b.WriteString("<div>nat " + v + "</div>")
	}
	return b.String()
}

// routerIdentityValue is this router's own identity block: name,
// chassis number, serial, the vManage controller(s) it reports being
// connected to, its own site, and software version, every field
// literally as the document carries it, omitting only what the router
// did not report.
func routerIdentityValue(r panelRouter) string {
	var b strings.Builder
	b.WriteString("<div><b>" + r.Name + "</b></div>")
	if r.ChassisNumber != "" && r.ChassisNumber != "(absent)" {
		b.WriteString("<div>" + r.ChassisNumber + "</div>")
	}
	if r.Serial != "" && r.Serial != "(absent)" {
		b.WriteString("<div>Serial&nbsp;" + r.Serial + "</div>")
	}
	if len(r.ConnectedManager) > 0 {
		b.WriteString("<div>Connected manager&nbsp;" + strings.Join(r.ConnectedManager, ", ") + "</div>")
	}
	if r.Site != "" && r.Site != "(absent)" {
		b.WriteString("<div>Site&nbsp;" + r.Site + "</div>")
	}
	if r.SoftwareVersion != "" && r.SoftwareVersion != "(absent)" {
		b.WriteString("<div>Version&nbsp;" + r.SoftwareVersion + "</div>")
	}
	return b.String()
}

// panelImageStyle mirrors drawio.LogoStyle's own shape=image
// convention for an embedded panel illustration.
func panelImageStyle(dataURI string) string {
	return "shape=image;verticalLabelPosition=bottom;labelBackgroundColor=default;verticalAlign=top;aspect=fixed;imageAspect=0;editableCssRules=.*;image=" + dataURI + ";"
}

// Router identity block geometry: a right-aligned text block in the top
// right corner, clear of the logo (top left) and the panel image below
// it.
const (
	routerIdentityX, routerIdentityY = 840, 20
	routerIdentityW, routerIdentityH = 800, 90
)

const routerIdentityStyle = "text;html=1;whiteSpace=wrap;strokeColor=none;fillColor=none;align=right;verticalAlign=middle;rounded=0;"

// portBadgeStyle is a plain generic rounded rectangle, not a vendor
// icon the same gray Cisco NX-OS's own VLAN badges use.
const portBadgeStyle = "rounded=1;whiteSpace=wrap;html=1;fillColor=#f5f5f5;fontColor=#333333;strokeColor=#666666;fontSize=8;"

// portEdgeStyle is a plain solid line from a port's own container (or
// the management badge) to its own marker on the panel image
// deliberately not dashed: unlike Cisco NX-OS's own VLAN membership
// edges (a configured logical relationship layered over a physical
// port), this edge just points at where its own container's data
// physically lives.
const portEdgeStyle = "rounded=0;orthogonalLoop=1;jettySize=auto;html=1;startArrow=classic;startFill=1;fontSize=9;"

// emitFooter writes the bottom band: the OSIRIS JSON credit on the
// left, snapshot timestamp, scope purpose, and generator version
// stacked right to left from the right margin, mirrors Cisco NX-OS's
// own footer, including its own firmly-to-the-bottom fix
// (vmanageFooterMargin).
func (r *TopologyRenderer) emitFooter(p *vmanagePrinter, pageW, pageH int) {
	ts := stringOrAbsent(r.metadata.Timestamp)
	purpose := stringOrAbsent(r.scope.Purpose)
	genVersion := "(absent)"
	if r.metadata.Generator != nil {
		genVersion = stringOrAbsent(r.metadata.Generator.Version)
	}
	footerY := pageH - vmanageFooterMargin - vmanageFooterH
	p.textCell("footer-osirisjson", footerTextStyle, drawio.Attr("<b>OSIRIS JSON</b><br>This topology was created with OSIRIS JSON. Read more on https://osirisjson.org"), 12, footerY, 636, vmanageFooterH)
	p.textCell("footer-timestamp", footerTextStyle, drawio.Attr("<div><b>Snapshot created on</b></div><div>"+ts+"</div>"), 990, footerY, 226, vmanageFooterH)
	p.textCell("footer-scope", footerTextStyle, drawio.Attr("<div><b>Snapshot scope</b></div><div>"+purpose+"</div>"), 1245, footerY, 198, vmanageFooterH)
	p.textCell("footer-generator", footerTextStyle, drawio.Attr("<div><b>OSIRIS JSON for Cisco vManage</b></div><div>"+genVersion+"</div>"), 1457, footerY, 183, vmanageFooterH)
}

const footerTextStyle = "text;html=1;whiteSpace=wrap;strokeColor=none;fillColor=none;verticalAlign=middle;rounded=0;align=left;"

// --- xml printer -----------------------------------------------------

type vmanagePrinter struct {
	w *render.CheckedWriter
}

func (p *vmanagePrinter) raw(s string) { _, _ = p.w.WriteString(s) }

func (p *vmanagePrinter) cell(id, parent, style, value string, x, y, w, h int) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" vertex="1" parent="%s">`+"\n",
		drawio.Attr(id), value, style, drawio.Attr(parent)))
	p.raw(fmt.Sprintf(`          <mxGeometry x="%d" y="%d" width="%d" height="%d" as="geometry" />`+"\n", x, y, w, h))
	p.raw("        </mxCell>\n")
}

func (p *vmanagePrinter) textCell(id, style, value string, x, y, w, h int) {
	p.cell(id, "1", style, value, x, y, w, h)
}

func (p *vmanagePrinter) edge(id, source, target, style, value string) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" edge="1" parent="1" source="%s" target="%s">`+"\n",
		drawio.Attr(id), value, style, drawio.Attr(source), drawio.Attr(target)))
	p.raw(`          <mxGeometry relative="1" as="geometry" />` + "\n")
	p.raw("        </mxCell>\n")
}
