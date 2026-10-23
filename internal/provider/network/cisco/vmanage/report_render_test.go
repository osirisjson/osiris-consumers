package vmanage

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

func TestReportRendererOnRealFixture(t *testing.T) {
	g := fixture(t)
	m := BuildReportModel(g, document.Metadata{Timestamp: "2026-08-06T16:53:27Z"}, "deadbeef")

	var buf bytes.Buffer
	r := NewReportRenderer(m, time.Time{}, "cisco-vmanage-report.xlsx")
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	if !strings.HasPrefix(out, "# "+vmanageReportTitle) {
		t.Errorf("unexpected title:\n%s", firstLine(out))
	}
	if !strings.Contains(out, "| TEST\\ROUTER01 |") {
		t.Error("Routers section did not list the fixture's own router individually")
	}
	// This fixture stays under the .xlsx threshold, so no companion
	// file should be named anywhere in the report.
	if strings.Contains(out, "cisco-vmanage-report.xlsx") {
		t.Error("report named the .xlsx companion file even though NeedsXLSXCompanion is false")
	}
	// Below the threshold, Ports must list every port individually in
	// Markdown (unlike Cisco NX-OS's own always-aggregate Ports
	// section) otherwise, with no .xlsx companion either, per-port
	// detail would appear nowhere at all.
	if !strings.Contains(out, "| TEST\\ROUTER01 | GigabitEthernet0\\/0\\/0 |") {
		t.Error("Ports section did not list an individual port row below the .xlsx threshold")
	}
	if !strings.Contains(out, "## Out of scope for this report") {
		t.Error("missing the out-of-scope section")
	}
}

func TestReportRendererEscapesInjectionAttempts(t *testing.T) {
	doc := &document.Document{
		Topology: document.Topology{
			Resources: []document.Resource{
				{ID: "r1", Type: "network.router", Name: "bad | name", Provider: document.Provider{Name: ProviderName}},
			},
		},
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	m := BuildReportModel(g, document.Metadata{}, "deadbeef")
	var buf bytes.Buffer
	r := NewReportRenderer(m, time.Time{}, "")
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(buf.String(), "| bad | name |") {
		t.Error("a pipe character in a router name broke out of its table cell")
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
