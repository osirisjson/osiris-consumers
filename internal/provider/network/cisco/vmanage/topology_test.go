package vmanage

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/document"
)

func TestTopologyRendererOnRealFixture(t *testing.T) {
	g := fixture(t)
	if !ConfirmedTopologyApplies(g) {
		t.Fatal("fixture sanity: expected ConfirmedTopologyApplies")
	}

	var buf bytes.Buffer
	r := NewTopologyRenderer(g, document.Metadata{Timestamp: "2026-08-06T16:53:27Z"}, time.Time{})
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
	if got := strings.Count(out, "<diagram "); got != 1 {
		t.Errorf("diagram tab count = %d, want 1 (this fixture has one confirmed router)", got)
	}
	if !strings.Contains(out, `name="TEST-ROUTER01"`) {
		t.Error("missing the router's own name as the diagram tab name")
	}
	panel, ok := ConfirmedPanelForModel("vedge-C8200L-1N-4T")
	if !ok {
		t.Fatal("fixture sanity: vedge-C8200L-1N-4T should resolve to a confirmed panel")
	}
	if !strings.Contains(out, "shape=image") || !strings.Contains(out, panel.Front.DataURI[:40]) {
		t.Error("panel image cell not found in the output")
	}
	// Vertex cells: logo, 4 footer cells, router-identity, panel-image,
	// a management marker+badge (this fixture's own router carries a
	// management_ip), 4 port markers, 4 port containers, and one IP
	// badge child (only GigabitEthernet0/0/0 carries a public IP in
	// this fixture) = 18.
	if got := strings.Count(out, `vertex="1"`); got != 18 {
		t.Errorf("vertex cell count = %d, want 18", got)
	}
	// Edges: one management edge, one per port (4) = 5.
	if got := strings.Count(out, `edge="1"`); got != 5 {
		t.Errorf("edge count = %d, want 5", got)
	}
	if !strings.Contains(out, portContainerDegradedStyle) {
		t.Error("expected the fixture's own degraded port container to use the degraded color")
	}
	if !strings.Contains(out, "192\\.0\\.2\\.10") && !strings.Contains(out, "192.0.2.10") {
		t.Error("expected the router's own management IP to appear as the management badge")
	}
	if !strings.Contains(out, "203.0.113.10") && !strings.Contains(out, "203\\.0\\.113\\.10") {
		t.Error("expected GigabitEthernet0/0/0's own public IP to appear as a container badge")
	}
	if !strings.Contains(out, "mbps") || !strings.Contains(out, "mtu") || !strings.Contains(out, "vrf") {
		t.Error("expected a port edge label to carry speed/MTU/VRF")
	}
}

func TestChooseTopologyPageMatchesRenderedPageSize(t *testing.T) {
	g := fixture(t)
	page, ok := ChooseTopologyPage(g)
	if !ok {
		t.Fatal("ChooseTopologyPage ok = false, want true")
	}

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

func TestChooseTopologyPageRejectsUnconfirmedModel(t *testing.T) {
	g := fixtureWithModel(t, "vedge-C8000L")
	if _, ok := ChooseTopologyPage(g); ok {
		t.Error("ChooseTopologyPage ok = true, want false (no confirmed router in this document)")
	}
}
