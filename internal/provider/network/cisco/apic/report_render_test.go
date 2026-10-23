// Tests for the Cisco APIC report renderer: every expected section is
// present, the title picks up the fabric domain, and rendering is
// deterministic and cancellation-aware like every other renderer in
// this repository.

package apic

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/render/markdown"
)

func renderReport(t *testing.T, m ReportModel) string {
	t.Helper()
	r := NewReportRenderer(m, time.Unix(0, 0), "")
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestReportRenderEmitsEveryExpectedSection(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	out := renderReport(t, m)

	for _, want := range []string{
		"## Faults",
		"## Fabric summary",
		"## Controllers",
		"## Fabric architecture",
		"### Pod 1",
		"### Pod 2",
		"## Tenants, VRFs, and EPGs",
		"## L3outs",
		"## Bridge domains and subnets",
		"## Ports",
		"## Externally discovered neighbours",
		"## Out of scope for this report",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain section %q", want)
		}
	}
}

func TestReportTitleUsesFabricDomainWhenPresent(t *testing.T) {
	got := reportTitle(FabricSummary{Domain: "MXP_DC"})
	want := "OSIRIS JSON Cisco APIC report for MXP\\_DC"
	if got != want {
		t.Errorf("reportTitle = %q, want %q", got, want)
	}
}

func TestReportTitleFallsBackWhenDomainAbsent(t *testing.T) {
	for _, domain := range []string{"", "(absent)"} {
		got := reportTitle(FabricSummary{Domain: domain})
		want := "OSIRIS JSON Cisco APIC report"
		if got != want {
			t.Errorf("reportTitle(Domain=%q) = %q, want %q", domain, got, want)
		}
	}
}

func TestReportRenderIsDeterministic(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	first := renderReport(t, m)
	second := renderReport(t, m)
	if first != second {
		t.Error("Render produced different output across two runs over the same input")
	}
}

func TestReportRenderRespectsCancellation(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	r := NewReportRenderer(m, time.Unix(0, 0), "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := r.Render(ctx, &buf); err == nil {
		t.Fatal("Render returned nil error on a cancelled context")
	}
}

// The fault table sits at the very top of the report, right below the
// table of contents actionable and should not be buried under the
// inventory tables.
func TestReportFaultsSectionIsImmediatelyAfterTheTableOfContents(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	out := renderReport(t, m)

	tocIdx := strings.Index(out, "## "+markdown.TOCHeading)
	faultsIdx := strings.Index(out, "## Faults")
	summaryIdx := strings.Index(out, "## Fabric summary")
	if tocIdx < 0 || faultsIdx < 0 || summaryIdx < 0 {
		t.Fatalf("missing expected heading: toc=%d faults=%d summary=%d", tocIdx, faultsIdx, summaryIdx)
	}
	if !(tocIdx < faultsIdx && faultsIdx < summaryIdx) {
		t.Errorf("expected order Table of Content < Faults < Fabric summary, got indices %d, %d, %d", tocIdx, faultsIdx, summaryIdx)
	}
}

func TestReportInvitesToTheXLSXCompanionWhenNeeded(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	big := BuildReportModel(g, doc.Metadata, doc.SHA256())
	if !big.NeedsXLSXCompanion() {
		t.Fatal("reference fixture does not trigger NeedsXLSXCompanion - test fixture assumption is stale")
	}
	out := renderReport(t, big)
	for _, want := range []string{"fault list", "per-VRF and per-EPG detail", "bridge domain and subnet detail", "port list", "neighbour list"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not invite the reader to the xlsx companion for %q", want)
		}
	}

	small := ReportModel{Tenants: []TenantInfo{{Name: "common"}}}
	if small.NeedsXLSXCompanion() {
		t.Fatal("small model unexpectedly triggers NeedsXLSXCompanion")
	}
	out = renderReport(t, small)
	if strings.Contains(out, "accompanying spreadsheet") {
		t.Error("output invites the reader to an xlsx companion that was not needed")
	}
}

// TestSectionFaultsCompactsAboveThreshold confirms the per-fault table
// (Node/Severity/Cause/Description/...) does not appear in the
// Markdown once the fault count exceeds the row threshold, it moves
// entirely to the .xlsx companion, leaving only a severity-count
// summary inline.
func TestSectionFaultsCompactsAboveThreshold(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	if len(m.Faults) <= apicXLSXRowThreshold {
		t.Fatalf("reference fixture has only %d faults, at or under the threshold - test assumption is stale", len(m.Faults))
	}
	out := renderReport(t, m)

	if !strings.Contains(out, "| Severity | Count |") {
		t.Error("output does not contain the compact severity-count summary")
	}
	if strings.Contains(out, "| Node | Severity | Cause | Description | Domain | Lifecycle | Created | Last transition |") {
		t.Error("output still contains the full per-fault table header above the threshold")
	}
}

// TestSectionFaultsShowsFullTableUnderThreshold confirms a small fault
// list still renders in full inline, unchanged from before the .xlsx
// companion existed.
func TestSectionFaultsShowsFullTableUnderThreshold(t *testing.T) {
	m := ReportModel{Faults: []FaultRecord{
		{Node: "leaf-10", Severity: "critical", Cause: "interface-physical-down"},
	}}
	out := renderReport(t, m)
	if !strings.Contains(out, "| Node | Severity | Cause | Description | Domain | Lifecycle | Created | Last transition |") {
		t.Error("output does not contain the full per-fault table for a small fault list")
	}
	if strings.Contains(out, "| Severity | Count |") {
		t.Error("output shows the compact summary for a fault list under the threshold")
	}
}

// TestReportInviteNoteUsesTheRealFileName confirms the invite note
// names the actual .xlsx file when the renderer is given one, instead
// of the generic "(same file name, .xlsx extension)" placeholder.
func TestReportInviteNoteUsesTheRealFileName(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	r := NewReportRenderer(m, time.Unix(0, 0), "cisco-apic-fabric-report.xlsx")
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "`cisco-apic-fabric-report.xlsx`") {
		t.Error("output does not name the actual xlsx file")
	}
	if strings.Contains(out, "(same file name, `.xlsx` extension)") {
		t.Error("output still uses the generic placeholder phrase alongside a real file name")
	}
}

// TestReportTOCMatchesActualHeadings guards against apicReportTOC
// drifting from the "## " headings Render actually writes: a title
// added, removed, renamed, or reordered in either place without the
// other would otherwise be a silent dead (or missing) table-of-
// contents link.
func TestReportTOCMatchesActualHeadings(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	out := renderReport(t, m)

	var headings []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimPrefix(line, "## "))
		}
	}
	want := append([]string{markdown.TOCHeading}, apicReportTOC...)
	if len(headings) != len(want) {
		t.Fatalf("found %d \"## \" headings, want %d\nheadings: %v\nwant: %v",
			len(headings), len(want), headings, want)
	}
	for i, w := range want {
		if headings[i] != w {
			t.Errorf("heading[%d] = %q, want %q", i, headings[i], w)
		}
	}
}

// TestSectionOutOfScopeLinksToProducerDocs confirms the scope note
// always links to the Cisco APIC producer's own docs (hardcoded, since
// the producer's own metadata.generator.url is absent in every fixture
// seen so far), and mentions the xlsx companion only when one is
// actually needed.
func TestSectionOutOfScopeLinksToProducerDocs(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	if !m.NeedsXLSXCompanion() {
		t.Fatal("reference fixture does not trigger NeedsXLSXCompanion - test assumption is stale")
	}
	out := renderReport(t, m)
	if !strings.Contains(out, ciscoAPICDocsURL) {
		t.Error("output does not link to the Cisco APIC producer docs")
	}
	if !strings.Contains(out, "accompanying .xlsx spreadsheet instead") {
		t.Error("output does not mention the xlsx companion when one was needed")
	}

	small := ReportModel{Tenants: []TenantInfo{{Name: "common"}}}
	out = renderReport(t, small)
	if strings.Contains(out, "accompanying .xlsx spreadsheet instead") {
		t.Error("output mentions the xlsx companion when none was needed")
	}
	if !strings.Contains(out, "no .xlsx companion was needed") {
		t.Error("output does not explain that no xlsx companion was needed")
	}
}
