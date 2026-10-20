// This file is the deterministic Markdown renderer for the Aruba
// Central report model: a Signature table (input hash plus every
// document metadata field), a Site table, a fleet health table per
// device type, client connectivity, a per-device properties table,
// and an explicit note on what a snapshot document cannot show. Every
// dynamic string is escaped through internal/render/markdown's
// destination-aware helpers; the only variable input is the injected
// artifact time.

package arubacentral

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/markdown"
)

// acFleetSectionTitle names the fleet-health and device-properties
// subsection for each visible type, in acColumnOrder's order.
var acFleetSectionTitle = map[string]string{
	"network.gateway":                     "Gateways",
	"network.switch":                      "Switches",
	"osiris.hpe.arubacentral.accesspoint": "Access Points",
}

// acReportTOC is every H2 heading this renderer writes, in emission
// order, kept in sync with Render's own section list by hand (see
// markdown.TableOfContents' doc comment): Signature has no heading of
// its own (it goes straight under the title) and so is not listed
// here, there would be nothing to link to.
var acReportTOC = []string{
	"Site",
	"Fleet health",
	"Client connectivity",
	"Device properties",
	"Out of scope for this report",
}

// reportTitle appends the site name to the report title when the
// document carries a site resource; falls back to the plain title
// otherwise, rather than fabricating a name.
func reportTitle(s SiteInfo) string {
	const base = "OSIRIS JSON HPE Aruba Central report"
	if !s.Present || s.Name == "" {
		return base
	}
	return base + " for " + markdown.Text(s.Name)
}

// ReportRenderer renders a ReportModel as Markdown.
// The zero value is not usable; construct one with NewReportRenderer.
type ReportRenderer struct {
	model        ReportModel
	artifact     time.Time
	xlsxFileName string
}

// NewReportRenderer returns a renderer for model. artifactTime is
// this render's own generation time, distinct from the document's own
// metadata.timestamp (which appears in the Signature section
// regardless); the zero time omits the "Report generated" row and
// keeps the output free of any clock reading. xlsxFileName is the
// .xlsx companion's own file name (not a path, this renderer never
// handles paths), named literally in the invite note wherever
// NeedsXLSXCompanion is true; pass "" when the caller has no companion
// file name yet (e.g. a test), and the note falls back to a generic
// phrase instead of a blank name.
func NewReportRenderer(model ReportModel, artifactTime time.Time, xlsxFileName string) *ReportRenderer {
	return &ReportRenderer{model: model, artifact: artifactTime.UTC(), xlsxFileName: xlsxFileName}
}

// Render writes the Markdown report to w.
func (r *ReportRenderer) Render(ctx context.Context, w io.Writer) error {
	cw := render.NewCheckedWriter(w)
	p := &acMDPrinter{w: cw, xlsxFileName: r.xlsxFileName}

	p.linef("# %s", reportTitle(r.model.Site))
	p.blank()

	if err := ctx.Err(); err != nil {
		return err
	}
	sectionSignature(p, r.model, r.artifact)

	p.raw(markdown.TableOfContentsSection(acReportTOC))

	for _, section := range []func(*acMDPrinter, ReportModel, time.Time){
		sectionSite,
		sectionFleetHealth,
		sectionClientConnectivity,
		sectionDeviceProperties,
		sectionScopeNote,
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		section(p, r.model, r.artifact)
	}

	return cw.Err()
}

// sectionSignature is the document's own provenance, as a table: the
// exact input hash and every "metadata" field the document carries.
// No heading: it goes straight under the title, not under its own
// "## Signature" section.
func sectionSignature(p *acMDPrinter, m ReportModel, artifact time.Time) {
	s := m.Signature
	p.line("| Field | Value |")
	p.line("| --- | --- |")
	p.linef("| Document fingerprint (SHA-256) | `%s` |", hexOnlySHA(s.InputSHA256))
	if !artifact.IsZero() {
		p.linef("| Report generated | %s |", artifact.Format(time.RFC3339))
	}
	p.linef("| Snapshot timestamp | %s |", markdown.TableCell(s.Timestamp))
	p.linef("| Generator name | %s |", markdown.TableCell(s.GeneratorName))
	p.linef("| Generator version | %s |", markdown.TableCell(s.GeneratorVersion))
	p.linef("| Generator URL | %s |", generatorURLCell(s.GeneratorURL))
	p.linef("| Scope purpose | %s |", markdown.TableCell(s.ScopePurpose))
	p.linef("| Scope providers | %s |", joinOrAbsentCell(s.ScopeProviders))
	p.linef("| Scope sites | %s |", joinOrAbsentCell(s.ScopeSites))
	p.linef("| Scope clusters | %s |", joinOrAbsentCell(s.ScopeClusters))
	extra := "(none)"
	if len(s.ExtraMetadataKeys) > 0 {
		extra = joinOrAbsentCell(s.ExtraMetadataKeys)
	}
	p.linef("| Additional metadata fields | %s |", extra)
	p.blank()
}

// googleMapsLinkCell renders a lat/lon pair as one Markdown link to
// Google Maps, replacing the separate Latitude/Longitude rows when
// both are present. The visible text is the coordinate pair itself, so
// the value is readable even before clicking; markdown.Link's own
// destination sanitization applies here exactly as it does for the
// Generator URL, the coordinates are document-derived, so
// still untrusted.
func googleMapsLinkCell(lat, lon string) string {
	return markdown.Link(lat+", "+lon, "https://www.google.com/maps?q="+lat+","+lon)
}

// generatorURLCell renders the generator URL as a real link when
// present (markdown.Link only makes http/https/mailto active, so any
// other scheme still renders as inert text), or "(absent)".
func generatorURLCell(url string) string {
	if url == "" {
		return "(absent)"
	}
	return markdown.Link(url, url)
}

// keysCell renders a list of disclosed-but-uninterpreted field names,
// or "(none)" when there are none, distinct from "(absent)", which
// this package uses when a whole field is missing rather than merely
// unenumerated.
func keysCell(keys []string) string {
	if len(keys) == 0 {
		return "(none)"
	}
	out := keys[0]
	for _, k := range keys[1:] {
		out += ", " + k
	}
	return markdown.TableCell(out)
}

func joinOrAbsentCell(vals []string) string {
	if len(vals) == 0 {
		return "(absent)"
	}
	out := vals[0]
	for _, v := range vals[1:] {
		out += ", " + v
	}
	return markdown.TableCell(out)
}

// hexOnlySHA mirrors internal/render/markdown's hexOnly: a malformed
// hash can never inject Markdown through the header.
func hexOnlySHA(s string) string {
	if s == "" {
		return "(unknown)"
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "(unknown)"
		}
	}
	return s
}

// xlsxInviteNote returns the line inviting the reader to the .xlsx
// companion for what, or "" when m does not need one
// (NeedsXLSXCompanion): once the fleet or the client list crosses the
// row threshold, both (and the per-device properties dump) get a full
// sheet in the same workbook (report_model.go), so every section
// backed by one of those categories gets this same note. Names
// p.xlsxFileName literally when the caller supplied one; falls back to
// a generic phrase when it did not (e.g. a test with no real output
// file in play).
func xlsxInviteNote(p *acMDPrinter, m ReportModel, what string) string {
	if !m.NeedsXLSXCompanion() {
		return ""
	}
	file := "the accompanying spreadsheet (same file name, `.xlsx` extension)"
	if p.xlsxFileName != "" {
		safe := strings.ReplaceAll(p.xlsxFileName, "`", "")
		file = "`" + safe + "`"
	}
	return "The complete " + what + " is also available in " + file + " for easier sorting and filtering."
}

func sectionSite(p *acMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Site")
	p.blank()
	s := m.Site
	if !s.Present {
		p.line("No `osiris.hpe.arubacentral.site` resource in this document.")
		p.blank()
		return
	}
	p.line("| Field | Value |")
	p.line("| --- | --- |")
	p.linef("| Name | %s |", markdown.TableCell(s.Name))
	p.linef("| Address | %s |", markdown.TableCell(s.Address))
	p.linef("| City | %s |", markdown.TableCell(s.City))
	p.linef("| State | %s |", markdown.TableCell(s.State))
	p.linef("| Zipcode | %s |", markdown.TableCell(s.Zipcode))
	p.linef("| Country | %s |", markdown.TableCell(s.Country))
	if s.Latitude != "(absent)" && s.Longitude != "(absent)" {
		p.linef("| Location | %s |", googleMapsLinkCell(s.Latitude, s.Longitude))
	} else {
		p.linef("| Latitude | %s |", markdown.TableCell(s.Latitude))
		p.linef("| Longitude | %s |", markdown.TableCell(s.Longitude))
	}
	p.linef("| Timezone | %s (%s) |", markdown.TableCell(s.TimezoneName), markdown.TableCell(s.TimezoneID))
	p.linef("| Device count (per site resource) | %s |", markdown.TableCell(s.DeviceCount))
	p.blank()
}

func sectionFleetHealth(p *acMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Fleet health")
	p.blank()
	p.line("OSIRIS JSON is a point-in-time infrastructure portable snapshot, it describes topology and inventory state, not time-series metrics, logs or observability data. The following data is not to be considered as live telemetry data.")
	p.blank()
	if len(m.Fleet) == 0 {
		p.line("No gateway, switch, or access point resources in this document.")
		p.blank()
		return
	}

	// Above acXLSXRowThreshold, the per-device tables move to the
	// .xlsx companion entirely, only a per-type count stays
	// inline here.
	if len(m.Fleet) > acXLSXRowThreshold {
		if note := xlsxInviteNote(p, m, "fleet"); note != "" {
			p.line(note)
			p.blank()
		}
		p.line("| Type | Count |")
		p.line("| --- | --: |")
		for _, typ := range acColumnOrder {
			count := 0
			for _, d := range m.Fleet {
				if d.Type == typ {
					count++
				}
			}
			p.linef("| %s | %d |", acFleetSectionTitle[typ], count)
		}
		p.blank()
		return
	}

	for _, typ := range acColumnOrder {
		var rows []DeviceHealth
		for _, d := range m.Fleet {
			if d.Type == typ {
				rows = append(rows, d)
			}
		}
		p.linef("### %s", acFleetSectionTitle[typ])
		p.blank()
		if len(rows) == 0 {
			p.line("None for this site.")
			p.blank()
			continue
		}
		p.line("| Name | Model | Firmware | Status | State | CPU % | Memory % | Power (W) | Temp (C) | Uptime |")
		p.line("| --- | --- | --- | --- | --- | --: | --: | --: | --: | --: |")
		for _, d := range rows {
			p.linef("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |",
				markdown.TableCell(d.Name), markdown.TableCell(d.Model), markdown.TableCell(d.FirmwareVersion),
				markdown.TableCell(d.Status), markdown.TableCell(d.State),
				markdown.TableCell(d.CPUPercent), markdown.TableCell(d.MemPercent),
				markdown.TableCell(d.PowerWatts), markdown.TableCell(d.TempC), markdown.TableCell(d.UptimeSeconds))
		}
		p.blank()
	}
}

func sectionClientConnectivity(p *acMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Client connectivity")
	p.blank()
	if len(m.Clients) == 0 {
		p.line("No `osiris.hpe.arubacentral.client` resources in this document: 0 clients observed in this snapshot.")
		p.blank()
		return
	}
	p.linef("%d client(s) observed in this snapshot.", len(m.Clients))
	p.blank()
	if note := xlsxInviteNote(p, m, "client list"); note != "" {
		p.line(note)
		p.blank()
	}
	if len(m.Clients) > acXLSXRowThreshold {
		return
	}
	p.line("| Name / MAC | Manufacturer | OS | Connected to | Port | VLAN | Connected at |")
	p.line("| --- | --- | --- | --- | --- | --- | --- |")
	for _, c := range m.Clients {
		vlan := c.VLANID
		if c.VLANName != "(absent)" {
			vlan = c.VLANID + " (" + c.VLANName + ")"
		}
		connectedTo := c.ConnectedToName
		if connectedTo == "" {
			connectedTo = "(absent)"
		}
		port := c.Port
		if port == "" {
			port = "(absent)"
		}
		p.linef("| %s | %s | %s | %s | %s | %s | %s |",
			markdown.TableCell(c.Name), markdown.TableCell(c.Manufacturer), markdown.TableCell(c.OS),
			markdown.TableCell(connectedTo), markdown.TableCell(port), markdown.TableCell(vlan),
			markdown.TableCell(c.ConnectedAt))
	}
	p.blank()
}

// sectionDeviceProperties writes one subsection per device, heading
// only its name, with the resource ID as the first table row rather
// than in the heading, two devices sharing a Stack's name still stay
// distinguishable in the table itself. Unlike Fleet health and Client
// connectivity, this section has no .xlsx companion and no compacting
// behavior above the row threshold: it always shows every device's full
// property breakdown, regardless of fleet size.
func sectionDeviceProperties(p *acMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Device properties")
	p.blank()
	if len(m.Fleet) == 0 {
		p.line("No gateway, switch, or access point resources in this document.")
		p.blank()
		return
	}
	for _, d := range m.Fleet {
		p.linef("### %s", markdown.Text(d.Name))
		p.blank()
		p.line("| Field | Value |")
		p.line("| --- | --- |")
		p.linef("| Resource ID | `%s` |", d.ResourceID)
		p.linef("| Native ID | %s |", markdown.TableCell(d.NativeID))
		p.linef("| Provider site | %s |", markdown.TableCell(d.ProviderSite))
		p.linef("| Provider source | %s |", markdown.TableCell(d.ProviderSource))
		p.linef("| Description | %s |", markdown.TableCell(d.Description))
		p.linef("| Tags | %s |", markdown.TableCell(d.Tags))
		for _, row := range d.Properties {
			p.linef("| %s | %s |", markdown.TableCell(row.Label), markdown.TableCell(row.Value))
		}
		p.linef("| Extensions (unrecognized, keys only) | %s |", keysCell(d.ExtensionKeys))
		p.linef("| Additional unrecognized fields (keys only) | %s |", keysCell(d.ExtraKeys))
		p.blank()
	}
}

// sectionScopeNote explains what a single OSIRIS JSON snapshot cannot
// show and points at the producer's own documentation page for future
// coverage, the URL comes from the document's own
// metadata.generator.url (Signature.GeneratorURL), never hardcoded, so
// this stays correct if the producer's docs move.
func sectionScopeNote(p *acMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Out of scope for this report")
	p.blank()
	p.line("Insights, historical events, and connectivity performance trends are not shown here: " +
		"they come from Aruba Central's own time-series backend, and there is no resource type or " +
		"property in an OSIRIS JSON snapshot document that carries them. The fleet health values " +
		"above are the point-in-time reading this document's own snapshot carries, not a live or " +
		"historical view.")
	if url := m.Signature.GeneratorURL; url != "" {
		p.blank()
		p.line("Coverage may grow in a future release of the HPE Aruba Central producer, see " +
			markdown.Link(url, url) + " for what it currently emits.")
	}
	p.blank()
}

// printer
type acMDPrinter struct {
	w            *render.CheckedWriter
	xlsxFileName string
}

func (p *acMDPrinter) line(s string) { _, _ = p.w.WriteString(s + "\n") }
func (p *acMDPrinter) blank()        { _, _ = p.w.WriteString("\n") }
func (p *acMDPrinter) raw(s string)  { _, _ = p.w.WriteString(s) }
func (p *acMDPrinter) linef(format string, a ...any) {
	_, _ = p.w.WriteString(fmt.Sprintf(format, a...) + "\n")
}
