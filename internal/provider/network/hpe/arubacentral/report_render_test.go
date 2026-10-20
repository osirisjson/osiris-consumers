// Tests for the Aruba Central Markdown report renderer.

package arubacentral

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/render/markdown"
)

func renderReport(t *testing.T, rel string) string {
	t.Helper()
	r := NewReportRenderer(buildReport(t, rel), time.Time{}, "")
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestReportRenderStartsWithTheOSIRISHeader(t *testing.T) {
	out := renderReport(t, fixture)
	if !strings.HasPrefix(out, "# OSIRIS JSON HPE Aruba Central report") {
		t.Error("report does not start with the expected header")
	}
}

func TestReportRenderTitleIncludesSiteName(t *testing.T) {
	out := renderReport(t, fixture)
	if !strings.HasPrefix(out, "# OSIRIS JSON HPE Aruba Central report for BRANCH\\-01\n") {
		t.Errorf("title should end with the site name; got first line of:\n%s", out[:min(len(out), 80)])
	}
}

func TestReportRenderTitleOmitsSiteNameWhenAbsent(t *testing.T) {
	r := NewReportRenderer(buildReport(t, "valid/no-provider.json"), time.Time{}, "")
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "# OSIRIS JSON HPE Aruba Central report\n") {
		t.Errorf("title should stay plain with no site resource; got first line of:\n%s", out[:min(len(out), 80)])
	}
}

func TestReportRenderSignatureIsATableRightAfterTheTitleWithNoHeading(t *testing.T) {
	out := renderReport(t, fixture)
	if strings.Contains(out, "## Signature") {
		t.Error(`Signature should have no "## Signature" heading, it goes straight under the title`)
	}
	title := "# OSIRIS JSON HPE Aruba Central report for BRANCH\\-01\n\n"
	if !strings.HasPrefix(out, title+"| Field | Value |") {
		t.Errorf("Signature table should start immediately after the title; got:\n%s", out[:min(len(out), 200)])
	}
	for _, want := range []string{
		"| Field | Value |",
		"osirisjson\\-producer\\-hpe\\-arubacentral", // generator.name
		"0\\.1\\.0",   // generator.version
		"audit",       // scope.purpose
		"BRANCH\\-01", // scope.sites
		"eucentral3",  // scope.clusters
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Signature table missing %q", want)
		}
	}
}

func TestReportRenderSignatureGeneratorURLIsALink(t *testing.T) {
	out := renderReport(t, fixture)
	if !strings.Contains(out, "](https://docs.osirisjson.org/osiris-producers/network/hpe-aruba-networking-central)") {
		t.Error("generator URL should render as a Markdown link, not plain text")
	}
}

func TestReportRenderSiteSectionIsATable(t *testing.T) {
	out := renderReport(t, fixture)
	if !strings.Contains(out, "## Site") {
		t.Fatal("missing ## Site section")
	}
	for _, want := range []string{`HQ\-LAB`, "Via del Castello 9", "Brescia", `45\.5433411`, `10\.2241237`} {
		if !strings.Contains(out, want) {
			t.Errorf("site section missing %q", want)
		}
	}
	// The site block must use the table syntax, not a bullet list.
	site := out[strings.Index(out, "## Site"):strings.Index(out, "## Fleet health")]
	if !strings.Contains(site, "| Field | Value |") {
		t.Error("Site section is not a table")
	}
	if strings.Contains(site, "- **Name:**") {
		t.Error("Site section still uses the old bullet-list format")
	}
}

func TestReportRenderSiteLatLonBecomeOneLocationLink(t *testing.T) {
	out := renderReport(t, fixture)
	if strings.Contains(out, "| Latitude |") || strings.Contains(out, "| Longitude |") {
		t.Error("Latitude/Longitude rows should be replaced by Location when both are present")
	}
	if !strings.Contains(out, "| Location | [45\\.5433411\\, 10\\.2241237](https://www.google.com/maps?q=45.5433411,10.2241237) |") {
		t.Error("missing the Location row with a Google Maps link built from lat/lon")
	}
}

func TestReportRenderSiteKeepsSeparateLatLonRowsWhenEitherAbsent(t *testing.T) {
	// Constructed directly rather than from a fixture: a site present
	// with only one coordinate must not fabricate a Location link from
	// half the data.
	model := buildReport(t, fixture)
	model.Site.Longitude = "(absent)"
	r := NewReportRenderer(model, time.Time{}, "")
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "| Location |") {
		t.Error("should not build a Location link with only one coordinate present")
	}
	if !strings.Contains(out, "| Latitude |") {
		t.Error("should fall back to the separate Latitude row")
	}
}

func TestReportRenderFleetHealthUsesHumanSectionTitles(t *testing.T) {
	out := renderReport(t, fixture)
	for _, want := range []string{"### Gateways", "### Switches", "### Access Points"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing fleet health subsection %q", want)
		}
	}
	if strings.Contains(out, "### Switchs") {
		t.Error(`section title should be "Switches", not the misspelled "Switchs"`)
	}
	if !strings.Contains(out, "TEST-SW01") {
		t.Error("gateway TEST-SW01 not found in fleet health table")
	}
}

func TestReportRenderClientConnectivityResolvesPortAndDevice(t *testing.T) {
	out := renderReport(t, fixture)
	if !strings.Contains(out, `00\:00\:5E\:00\:53\:10`) {
		t.Error("known client MAC not found")
	}
	if !strings.Contains(out, `1\/1\/5`) {
		t.Error("resolved port not found in client table")
	}
}

func TestReportRenderClientConnectivitySaysNoneWhenAbsent(t *testing.T) {
	r := NewReportRenderer(buildReport(t, "valid/no-provider.json"), time.Time{}, "")
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), "0 clients observed in this snapshot") {
		t.Error("expected an explicit zero-clients disclosure, not a silently omitted section")
	}
}

func TestReportRenderIncludesScopeNoteWithProducerLink(t *testing.T) {
	out := renderReport(t, fixture)
	if !strings.Contains(out, "Insights, historical events, and connectivity performance trends") {
		t.Error("missing the explicit out-of-scope note")
	}
	// The link comes from the document's own metadata.generator.url,
	// never a hardcoded constant.
	if !strings.Contains(out, "Coverage may grow in a future release") {
		t.Error("missing the future-coverage pointer")
	}
	if !strings.Contains(out, "](https://docs.osirisjson.org/osiris-producers/network/hpe-aruba-networking-central)") {
		t.Error("scope note should link to metadata.generator.url")
	}
}

func TestReportRenderScopeNoteOmitsLinkWhenGeneratorURLAbsent(t *testing.T) {
	// mixed-provider.json carries no metadata.generator at all.
	r := NewReportRenderer(buildReport(t, "valid/mixed-provider.json"), time.Time{}, "")
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(buf.String(), "Coverage may grow") {
		t.Error("should not claim future coverage with no generator URL to point at")
	}
}

func TestReportRenderDevicePropertiesIsATableHeadedByNameOnly(t *testing.T) {
	out := renderReport(t, fixture)
	if !strings.Contains(out, "## Device properties") {
		t.Error("missing Device properties section")
	}
	if !strings.Contains(out, "### TEST-SW01") {
		t.Error(`device heading should be exactly "### TEST-SW01"`)
	}
	if strings.Contains(out, "### TEST-SW01 (") {
		t.Error("device heading should not carry the resource ID in parentheses any more")
	}
	if strings.Contains(out, "```json") {
		t.Error("device properties should no longer be a fenced JSON block")
	}
	if !strings.Contains(out, "| Resource ID | `hpe.arubacentral::TST0000001` |") {
		t.Error("resource ID should appear as the first row of the properties table")
	}
}

func TestReportRenderDevicePropertiesDecodesTheRestOfProvider(t *testing.T) {
	out := renderReport(t, fixture)
	for _, want := range []string{
		"| Native ID | TST0000001 |",
		"| Provider site | BRANCH\\-01 |",
		"| Provider source | aruba\\-new\\-central\\-api |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("device properties missing %q", want)
		}
	}
	// This fixture's devices carry none of these, must say so
	// explicitly, not drop the rows.
	if !strings.Contains(out, "| Description | \\(absent\\) |") {
		t.Error("Description row should disclose (absent), escaped, not vanish")
	}
	if !strings.Contains(out, "Extensions (unrecognized, keys only) | (none) |") {
		t.Error("Extensions row should say (none) when the resource carries no extensions")
	}
}

func TestReportRenderMalformedHashDoesNotInjectMarkdown(t *testing.T) {
	model := buildReport(t, fixture)
	model.Signature.InputSHA256 = "not-a-hash; # injected"
	r := NewReportRenderer(model, time.Time{}, "")
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), "(unknown)") {
		t.Error("a non-hex SHA should render as (unknown)")
	}
	if strings.Contains(buf.String(), "not-a-hash") {
		t.Error("the malformed hash leaked into the output verbatim")
	}
}

// TestReportTOCMatchesActualHeadings guards against acReportTOC
// drifting from the "## " headings Render actually writes: a title
// added, removed, renamed, or reordered in either place without the
// other would otherwise be a silent dead (or missing) table-of-
// contents link.
func TestReportTOCMatchesActualHeadings(t *testing.T) {
	out := renderReport(t, fixture)
	var headings []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimPrefix(line, "## "))
		}
	}
	want := append([]string{markdown.TOCHeading}, acReportTOC...)
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

// renderModel renders m directly (not via a fixture), with xlsxFileName
// passed straight through to NewReportRenderer.
func renderModel(t *testing.T, m ReportModel, xlsxFileName string) string {
	t.Helper()
	r := NewReportRenderer(m, time.Time{}, xlsxFileName)
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

// bigFleet returns n synthetic DeviceHealth rows, enough to exceed
// acXLSXRowThreshold with a small margin.
func bigFleet(n int) []DeviceHealth {
	out := make([]DeviceHealth, n)
	for i := range out {
		out[i] = DeviceHealth{Name: "device", Type: "network.switch"}
	}
	return out
}

func bigClients(n int) []ClientInfo {
	out := make([]ClientInfo, n)
	for i := range out {
		out[i] = ClientInfo{Name: "client"}
	}
	return out
}

func TestSectionFleetHealthCompactsAboveThreshold(t *testing.T) {
	m := ReportModel{Fleet: bigFleet(acXLSXRowThreshold + 1)}
	out := renderModel(t, m, "")
	if !strings.Contains(out, "| Type | Count |") {
		t.Error("output does not contain the compact per-type count summary")
	}
	if strings.Contains(out, "| Name | Model | Firmware | Status | State | CPU % | Memory % | Power (W) | Temp (C) | Uptime |") {
		t.Error("output still contains the full per-device table above the threshold")
	}
}

func TestSectionFleetHealthShowsFullTablesUnderThreshold(t *testing.T) {
	m := ReportModel{Fleet: bigFleet(3)}
	out := renderModel(t, m, "")
	if !strings.Contains(out, "| Name | Model | Firmware | Status | State | CPU % | Memory % | Power (W) | Temp (C) | Uptime |") {
		t.Error("output does not contain the full per-device table for a small fleet")
	}
	if strings.Contains(out, "| Type | Count |") {
		t.Error("output shows the compact summary for a fleet under the threshold")
	}
}

func TestSectionClientConnectivityCompactsAboveThreshold(t *testing.T) {
	m := ReportModel{Clients: bigClients(acXLSXRowThreshold + 1)}
	out := renderModel(t, m, "")
	if strings.Contains(out, "| Name / MAC | Manufacturer | OS | Connected to | Port | VLAN | Connected at |") {
		t.Error("output still contains the full per-client table above the threshold")
	}
}

// TestSectionDevicePropertiesAlwaysShowsFullDetail confirms Device
// properties has no compacting behavior.
func TestSectionDevicePropertiesAlwaysShowsFullDetail(t *testing.T) {
	m := ReportModel{Fleet: bigFleet(acXLSXRowThreshold + 1)}
	out := renderModel(t, m, "")
	section := out[strings.Index(out, "## Device properties"):]
	if !strings.Contains(section, "### device") {
		t.Error("output does not contain per-device subsections above the threshold")
	}
	if strings.Contains(section, "also available in") {
		t.Error("Device properties should not carry an xlsx invite note")
	}
}

func TestReportInviteNoteUsesTheRealFileNameAC(t *testing.T) {
	m := ReportModel{Fleet: bigFleet(acXLSXRowThreshold + 1)}
	out := renderModel(t, m, "branch-01-report.xlsx")
	if !strings.Contains(out, "`branch-01-report.xlsx`") {
		t.Error("output does not name the actual xlsx file")
	}
	if strings.Contains(out, "(same file name, `.xlsx` extension)") {
		t.Error("output still uses the generic placeholder phrase alongside a real file name")
	}
}
