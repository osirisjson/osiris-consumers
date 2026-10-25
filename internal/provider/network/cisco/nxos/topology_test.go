package nxos

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// TestTopologyRendererOnRealFixture proves the confirmed-panel page,
// the fixture's own chassis product ID, N9K-C93180YC-FX, resolves via
// ConfirmedPanelForModel: the front-panel illustration, an A3 page, one
// Cisco19 l2_switch icon per neighbour, and one edge per neighbour
// anchored to its own local port on the illustration.
func TestTopologyRendererOnRealFixture(t *testing.T) {
	g := fixture(t)
	var buf bytes.Buffer
	r := NewTopologyRenderer(g, document.Metadata{Timestamp: "2026-09-11T08:26:00Z"}, time.Time{})
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "<mxfile") {
		t.Error("output does not start with <mxfile")
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), "</mxfile>") {
		t.Error("output does not end with </mxfile>")
	}
	if !strings.Contains(out, `id="`+panelDiagramID+`"`) {
		t.Error("missing the confirmed-panel diagram id this page is identified by")
	}
	if !strings.Contains(out, `pageWidth="1654" pageHeight="1169"`) {
		t.Error("confirmed-panel page is not the fixed A3 landscape size")
	}
	panel, ok := ConfirmedPanelForModel("N9K-C93180YC-FX")
	if !ok {
		t.Fatal("fixture sanity: N9K-C93180YC-FX should resolve to a confirmed panel")
	}
	if !strings.Contains(out, "shape=image") || !strings.Contains(out, panel.Front.DataURI[:40]) {
		t.Error("panel image cell not found in the output")
	}
	if !strings.Contains(out, "prIcon=l2_switch") {
		t.Error("neighbour nodes did not get the expected Cisco19 l2_switch shape")
	}
	// 15 neighbours in this fixture (14 front, 1 rear): one physical
	// edge per neighbour, plus the FrontVLANs page's own dashed
	// membership edges (240 in this fixture, not counted here; see
	// TestPanelVLANBadgeCountOnRealFixture). exitPerimeter=0 only ever
	// appears on a physical edge's own exit point (panel_vlans.go's own
	// edges never set it on their exit side), so it stays an exact
	// proxy for the physical-edge count regardless of the VLANs page.
	//
	// exitPerimeter=0 is required for a fixed exit point that sits
	// inside the shape's bounding box (never exactly on its edge, since
	// every port is interior to the panel image) to be honored
	// literally rather than re-projected to the shape's outer boundary
	// without it every edge collapses onto whichever side of the
	// (very wide, short) panel image is nearest, not the actual port.
	if got := strings.Count(out, "exitPerimeter=0"); got != 15 {
		t.Errorf("exitPerimeter=0 count = %d, want 15 (one per physical edge)", got)
	}
	// One status marker per network.switch.port resource this
	// illustration draws a connector for, on both FrontPhysical and
	// FrontVLANs (54 Ethernet1/N ports each) plus Rear's own mgmt0: 8
	// active + 46 inactive Ethernet1/N ports, mgmt0 itself active, so
	// (8+8+1)=17 active and (46+46+0)=92 inactive across the whole
	// file; this fixture's own ports are all resolved "active" or
	// "inactive", so the neutral fallback color never appears.
	if got := strings.Count(out, portMarkerActiveStyle); got != 17 {
		t.Errorf("active port marker count = %d, want 17", got)
	}
	if got := strings.Count(out, portMarkerInactiveStyle); got != 92 {
		t.Errorf("inactive port marker count = %d, want 92", got)
	}
	if got := strings.Count(out, portMarkerStyle); got != 0 {
		t.Errorf("neutral-status port marker count = %d, want 0 (every port in this fixture is active or inactive)", got)
	}
}

// TestConfirmedPanelPagesFitOnTheirOwnDeclaredPage is a general
// regression guard: every cell's x+width on every diagram tab must fit
// within that diagram's own declared pageWidth (mxGraph's page-break
// view otherwise tiles a second full page as soon as content crosses
// the boundary, which is what "a single A3 page" looked like "double
// width" the actual bug was footer-generator overflowing by ~85
// units; see emitFooter's own doc comment for the fix).
func TestConfirmedPanelPagesFitOnTheirOwnDeclaredPage(t *testing.T) {
	g := fixture(t)
	var buf bytes.Buffer
	r := NewTopologyRenderer(g, document.Metadata{Timestamp: "2026-09-11T08:26:00Z"}, time.Time{})
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	diagramRe := regexp.MustCompile(`(?s)<diagram id="([^"]+)"[^>]*>.*?pageWidth="(\d+)".*?</diagram>`)
	geomRe := regexp.MustCompile(`<mxGeometry[^>]*\bx="(-?\d+)"[^>]*\bwidth="(\d+)"`)
	diagrams := diagramRe.FindAllStringSubmatch(out, -1)
	if len(diagrams) != 3 {
		t.Fatalf("found %d <diagram> blocks, want 3 (front physical, front VLANs, and rear)", len(diagrams))
	}
	for _, d := range diagrams {
		id, pageWidth := d[1], atoi(t, d[2])
		block := d[0]
		for _, geom := range geomRe.FindAllStringSubmatch(block, -1) {
			x, w := atoi(t, geom[1]), atoi(t, geom[2])
			if x+w > pageWidth {
				t.Errorf("%s: a cell at x=%d width=%d ends at %d, past pageWidth=%d", id, x, w, x+w, pageWidth)
			}
		}
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("parsing %q as int: %v", s, err)
	}
	return n
}

// TestTopologyRendererFallsBackToHubAndSpokeForUnconfirmedModel proves
// the other half of the dispatch: a switch whose chassis product ID has
// no confirmed illustration stays on the generic page,
// never the confirmed-panel one.
func TestTopologyRendererFallsBackToHubAndSpokeForUnconfirmedModel(t *testing.T) {
	doc := &document.Document{
		Topology: document.Topology{
			Resources: []document.Resource{
				{
					ID: "s1", Type: "network.switch", Name: "LAB-SW421", Provider: document.Provider{Name: ProviderName},
					Extensions: []byte(`{"osiris.cisco":{"inventory":[{"name":"Chassis","product_id":"N9K-C9336C-FX2"}]}}`),
				},
			},
		},
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	var buf bytes.Buffer
	r := NewTopologyRenderer(g, document.Metadata{}, time.Time{})
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, panelDiagramID) {
		t.Error("an unconfirmed model rendered the confirmed-panel page")
	}
	if !strings.Contains(out, `id="osirisjson-cisco-nxos-topology"`) {
		t.Error("missing the diagram id this page is identified by")
	}
}

func TestChooseTopologyPageMatchesRenderedPageSize(t *testing.T) {
	g := fixture(t)
	page := ChooseTopologyPage(g)

	var buf bytes.Buffer
	r := NewTopologyRenderer(g, document.Metadata{}, time.Time{})
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "pageWidth=\"" + strconv.Itoa(page.Width) + "\" pageHeight=\"" + strconv.Itoa(page.Height) + "\""
	if !strings.Contains(buf.String(), want) {
		t.Errorf("ChooseTopologyPage = %+v did not match the page size Render actually emitted", page)
	}
}

func TestTopologyRendererNoNeighbours(t *testing.T) {
	doc := &document.Document{
		Topology: document.Topology{
			Resources: []document.Resource{
				{ID: "s1", Type: "network.switch", Name: "LAB-SW421", Provider: document.Provider{Name: ProviderName}},
			},
		},
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	var buf bytes.Buffer
	r := NewTopologyRenderer(g, document.Metadata{}, time.Time{})
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Count(buf.String(), `edge="1"`) != 0 {
		t.Error("expected no edges for a switch with no discovered neighbours")
	}
}
