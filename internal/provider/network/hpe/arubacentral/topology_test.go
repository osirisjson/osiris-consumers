// Tests for the Aruba Central topology overview renderer: well-formed
// XML, one icon node per resource with the right icon per type, VSX
// stacking detected from the document's own connection, and the
// snapshot-metadata footer.

package arubacentral

import (
	"bytes"
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/topology"
)

func buildModel(t *testing.T, rel string) (*topology.Topology, *document.Document) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "testdata", rel))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", rel, err)
	}
	doc, err := document.Parse(b)
	if err != nil {
		t.Fatalf("parsing %s: %v", rel, err)
	}
	g := build(t, rel)
	top, err := topology.Build(context.Background(), doc, g)
	if err != nil {
		t.Fatalf("building topology for %s: %v", rel, err)
	}
	return top, doc
}

func renderTopology(t *testing.T, rel string) string {
	t.Helper()
	top, doc := buildModel(t, rel)
	g := build(t, rel)
	r := NewTopologyRenderer(top, g, doc.Metadata, time.Time{})
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestTopologyRenderIsWellFormedXML(t *testing.T) {
	out := renderTopology(t, fixture)
	var v any
	if err := xml.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not well-formed XML: %v", err)
	}
}

func TestTopologyRenderOnlyDrawsTheThreeVisibleTypes(t *testing.T) {
	top, _ := buildModel(t, fixture)
	var visible int
	for _, n := range top.Nodes {
		if visibleTopologyTypes[n.Type] {
			visible++
		}
	}
	if visible == 0 || visible == len(top.Nodes) {
		t.Fatalf("fixture sanity: %d of %d nodes are a visible type; want some but not all, or this test proves nothing", visible, len(top.Nodes))
	}

	out := renderTopology(t, fixture)
	got := strings.Count(out, `vertex="1"`)
	// One cell per visible-type node (not every resource, ports,
	// radios, etc. are deliberately left off this page), plus the logo
	// badge and the 6 footer text cells.
	want := visible + 1 /* logo */ + 6 /* footer text cells */
	if got != want {
		t.Errorf("emitted %d vertex cells, want %d (%d visible-type nodes + logo + footer)", got, want, visible)
	}
}

func TestTopologyRenderGatewayGetsGatewayIcon(t *testing.T) {
	out := renderTopology(t, fixture)
	if !strings.Contains(out, gatewayIconDataURI) {
		t.Error("gateway icon not found in output")
	}
}

func TestTopologyRenderAccesspointGetsAccesspointIcon(t *testing.T) {
	out := renderTopology(t, fixture)
	if !strings.Contains(out, accesspointIconDataURI) {
		t.Error("accesspoint icon not found in output")
	}
}

func TestTopologyRenderNodeValueIsJustTheName(t *testing.T) {
	out := renderTopology(t, fixture)
	if !strings.Contains(out, `value="TEST-GW01"`) {
		t.Error(`gateway node value should be exactly its name "TEST-GW01"`)
	}
	if strings.Contains(out, "resources.type.") {
		t.Error(`"resources.type." leaked into rendered output`)
	}
	if strings.Contains(out, "&#10;stack&#10;") || strings.Contains(out, "\nstack\n") {
		t.Error("a separate stack text line should no longer be rendered")
	}
	if !strings.Contains(out, "fontStyle=1") {
		t.Error("icon style should carry fontStyle=1 for a bold name")
	}
}

func TestTopologyRenderStackedSwitchGetsStackIcon(t *testing.T) {
	// Both switches in the fixture are VSX-paired with each other; the
	// icon variant alone conveys that now (no separate text label).
	out := renderTopology(t, fixture)
	if !strings.Contains(out, switchStackIconDataURI) {
		t.Error("stack icon not found; expected both switches to be VSX-paired")
	}
	if strings.Contains(out, switchIconDataURI) {
		t.Error("plain switch icon used where a stacked one was expected for every switch in this fixture")
	}
}

func TestTopologyRenderOnlyDrawsDeviceToDeviceEdges(t *testing.T) {
	// The fixture asserts exactly one connection between two visible-
	// type resources: the VSX pairing between the two switches. Every
	// other connection touches a port, radio, WLAN, tunnel endpoint, or
	// client none of those are drawn on this page.
	out := renderTopology(t, fixture)
	got := strings.Count(out, `edge="1"`)
	if got != 1 {
		t.Errorf("emitted %d edges, want exactly 1 (the VSX pairing)", got)
	}
	if !strings.Contains(out, "curved=1") {
		t.Error("edge style should always be curved")
	}
}

func TestStackedSwitchesFindsTheVSXPair(t *testing.T) {
	g := build(t, fixture)
	stacked := StackedSwitches(g)
	for _, id := range []string{"hpe.arubacentral::TST0000002", "hpe.arubacentral::AB12CDE3F4"} {
		if !stacked[id] {
			t.Errorf("StackedSwitches missing %s", id)
		}
	}
	if len(stacked) != 2 {
		t.Errorf("got %d stacked resources, want exactly the 2 VSX-paired switches", len(stacked))
	}
}

func TestTopologyRenderFooterUsesDocumentMetadata(t *testing.T) {
	out := renderTopology(t, fixture)
	// The OSIRIS JSON document metadata.scope: purpose "audit", sites
	// ["BRANCH-01"], clusters ["eucentral3 (...)"].
	for _, want := range []string{"BRANCH-01", "audit", "eucentral3"} {
		if !strings.Contains(out, want) {
			t.Errorf("footer missing %q; full output was:\n%s", want, out)
		}
	}
}

func TestTopologyRenderNonHomogeneousFixtureStillHasAbsentFooterFields(t *testing.T) {
	// mixed-provider.json carries no metadata.scope at all the
	// footer must disclose "(absent)", not fabricate a value.
	top, doc := buildModel(t, "valid/mixed-provider.json")
	g := build(t, "valid/mixed-provider.json")
	r := NewTopologyRenderer(top, g, doc.Metadata, time.Time{})
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), "(absent)") {
		t.Error("expected at least one (absent) footer field for a document with no metadata.scope")
	}
}

func TestPlaceByColumnOrdersLeftToRight(t *testing.T) {
	nodes := []topology.Node{
		{ID: "ap-01", Type: "osiris.hpe.arubacentral.accesspoint", Label: "ap-01"},
		{ID: "sw-01", Type: "network.switch", Label: "sw-01"},
		{ID: "gw-01", Type: "network.gateway", Label: "gw-01"},
		{ID: "gw-02", Type: "network.gateway", Label: "gw-02"},
	}
	placed, _, _ := placeByColumn(nodes)

	byID := map[string]acPlacedNode{}
	for _, p := range placed {
		byID[p.node.ID] = p
	}
	gwX := byID["gw-01"].x
	swX := byID["sw-01"].x
	apX := byID["ap-01"].x
	if !(gwX < swX && swX < apX) {
		t.Errorf("column x positions gateway=%d switch=%d accesspoint=%d, want strictly increasing left to right", gwX, swX, apX)
	}
	if byID["gw-02"].x != gwX {
		t.Errorf("gw-02.x = %d, want the same column as gw-01 (%d)", byID["gw-02"].x, gwX)
	}
	if byID["gw-02"].y == byID["gw-01"].y {
		t.Error("two gateways in the same column should be stacked at different y, not overlapping")
	}
}

func TestPlaceByColumnContentExtentCoversEveryNode(t *testing.T) {
	nodes := []topology.Node{
		{ID: "gw-01", Type: "network.gateway"},
		{ID: "sw-01", Type: "network.switch"},
		{ID: "sw-02", Type: "network.switch"},
		{ID: "sw-03", Type: "network.switch"},
		{ID: "ap-01", Type: "osiris.hpe.arubacentral.accesspoint"},
	}
	placed, contentW, contentH := placeByColumn(nodes)
	for _, p := range placed {
		if p.x+acNodeSize > contentW {
			t.Errorf("node %s right edge %d exceeds contentW %d", p.node.ID, p.x+acNodeSize, contentW)
		}
		if p.y+acNodeSize > contentH {
			t.Errorf("node %s bottom edge %d exceeds contentH %d", p.node.ID, p.y+acNodeSize, contentH)
		}
	}
}
