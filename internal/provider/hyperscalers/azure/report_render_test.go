package azure

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func buildReportMD(t *testing.T, m ReportModel, artifactTime time.Time, xlsxFileName string) string {
	t.Helper()
	var buf bytes.Buffer
	r := NewReportRenderer(m, artifactTime, xlsxFileName)
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestReportTitleAndSections(t *testing.T) {
	doc, g := loadGraph(t, azureFixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	out := buildReportMD(t, m, fixedTime, "")

	if !strings.HasPrefix(out, "# OSIRIS JSON Microsoft Azure report for") {
		t.Errorf("unexpected title line: %q", strings.SplitN(out, "\n", 2)[0])
	}
	for _, want := range []string{
		"## Hierarchy", "## Resource groups", "## Virtual Machines", "## Storage Accounts",
		"## Disks", "## Virtual Networks", "## Subnets", "## Network Security Groups",
		"## Public IP Addresses", "## Load Balancers", "## Network Interfaces",
		"## Bastion Hosts", "## SQL Virtual Machines", "## Other resources",
		"## Out of scope for this report",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing section %q", want)
		}
	}
}

func TestReportTitleFallsBackWithoutScopeName(t *testing.T) {
	out := buildReportMD(t, ReportModel{}, time.Time{}, "")
	if !strings.HasPrefix(out, "# OSIRIS JSON Microsoft Azure report\n") {
		t.Errorf("expected the plain title with no scope name, got %q", strings.SplitN(out, "\n", 2)[0])
	}
}

func TestReportArtifactTime(t *testing.T) {
	doc, g := loadGraph(t, azureFixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	if !strings.Contains(buildReportMD(t, m, fixedTime, ""), "Report generated | 2026-03-04T05:06:07Z") {
		t.Error("injected artifact time not stamped into the Signature table")
	}
	if strings.Contains(buildReportMD(t, m, time.Time{}, ""), "Report generated") {
		t.Error("zero artifact time must not stamp a Report generated row")
	}
}

func TestReportXLSXInviteNoteAppearsOnlyAboveThreshold(t *testing.T) {
	m := ReportModel{Categories: []ResourceCategory{{Label: "Virtual Machines", Rows: make([]ResourceRow, azureXLSXRowThreshold+1)}}}
	out := buildReportMD(t, m, time.Time{}, "microsoft-azure-report.xlsx")
	if !strings.Contains(out, "`microsoft-azure-report.xlsx`") {
		t.Error("expected the xlsx invite note naming the real file")
	}
	if strings.Contains(out, "| Name | Resource Group |") {
		t.Error("a category over threshold must not also render its inline table")
	}

	small := ReportModel{Categories: []ResourceCategory{{Label: "Virtual Machines", Rows: []ResourceRow{{Name: "vm1"}}}}}
	if strings.Contains(buildReportMD(t, small, time.Time{}, ""), "accompanying spreadsheet") {
		t.Error("a small document must not mention an xlsx companion at all")
	}
}

func TestReportInjectionSafety(t *testing.T) {
	m := ReportModel{
		Signature: SignatureInfo{ScopeName: "a\nb|c`d"},
		Categories: []ResourceCategory{
			{Label: "Virtual Machines", Rows: []ResourceRow{{Name: "| pipe | attempt |"}}},
		},
	}
	out := buildReportMD(t, m, time.Time{}, "")
	// markdown.TableCell backslash-escapes an embedded pipe rather than
	// removing it the raw, unescaped three-pipe name must never appear
	// verbatim (that would corrupt the table's column structure).
	if strings.Contains(out, "| | pipe | attempt | |") {
		t.Error("an unescaped pipe in a resource name reached the table row")
	}
	if !strings.Contains(out, `\| pipe \| attempt \|`) {
		t.Error("expected the resource name's pipes to be backslash-escaped")
	}
}

func TestReportCancellation(t *testing.T) {
	doc, g := loadGraph(t, azureFixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := NewReportRenderer(m, time.Time{}, "")
	var buf bytes.Buffer
	if err := r.Render(ctx, &buf); err == nil {
		t.Fatal("expected cancellation to propagate")
	}
}

func TestReportPropagatesWriteError(t *testing.T) {
	doc, g := loadGraph(t, azureFixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	r := NewReportRenderer(m, time.Time{}, "")
	if err := r.Render(context.Background(), failingWriter{}); err == nil {
		t.Fatal("expected the first write error to propagate")
	}
}
