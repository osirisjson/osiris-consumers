package nxos

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
	m := BuildReportModel(g, document.Metadata{Timestamp: "2026-09-11T08:26:00Z"}, "deadbeef")

	var buf bytes.Buffer
	r := NewReportRenderer(m, time.Time{}, "cisco-nxos-report.xlsx")
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	if !strings.HasPrefix(out, "# OSIRIS JSON Cisco NX-OS report for TEST-MXP-") {
		t.Errorf("unexpected title:\n%s", firstLine(out))
	}
	// Ports crosses the .xlsx threshold in this fixture, so the
	// Markdown report must stay aggregate-only for it (no per-port
	// table) and must name the companion file.
	if strings.Contains(out, "| Ethernet1/1 |") {
		t.Error("Ports section listed individual ports in Markdown; it should have stayed aggregate-only above the threshold")
	}
	if !strings.Contains(out, "cisco-nxos-report.xlsx") {
		t.Error("report does not name the .xlsx companion file")
	}
	// VRFs (4 rows) stays well under the threshold, so it must list
	// every VRF individually.
	if !strings.Contains(out, "| management |") {
		t.Error("VRFs section did not list the management VRF individually")
	}
	if !strings.Contains(out, "## Out of scope for this report") {
		t.Error("missing the out-of-scope section")
	}
}

func TestReportRendererEscapesInjectionAttempts(t *testing.T) {
	doc := &document.Document{
		Topology: document.Topology{
			Resources: []document.Resource{
				{ID: "s1", Type: "network.switch", Name: "bad | name", Provider: document.Provider{Name: ProviderName}},
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
		t.Error("a pipe character in a switch name broke out of its table cell")
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
