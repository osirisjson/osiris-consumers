// This file is the deterministic Markdown renderer for the Cisco
// vManage report model: a Signature table, one router summary per
// edge router this snapshot describes, a router.port census, and the
// site/room groups the fleet belongs to. Every dynamic string is
// escaped through internal/render/markdown's destination-aware
// helpers; the only variable input is the injected artifact time.
package vmanage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/markdown"
)

// vmanageReportTOC is every H2 heading this renderer writes, in
// emission order (see markdown.TableOfContents' doc comment).
// Signature has no heading of its own and so is not listed here.
var vmanageReportTOC = []string{
	"Routers",
	"Ports",
	"Sites",
	"Out of scope for this report",
}

const vmanageReportTitle = "OSIRIS JSON Cisco vManage report"

// ReportRenderer renders a ReportModel as Markdown.
// The zero value is not usable; construct one with NewReportRenderer.
type ReportRenderer struct {
	model        ReportModel
	artifact     time.Time
	xlsxFileName string
}

// NewReportRenderer returns a renderer for model. artifactTime is this
// render's own generation time, distinct from the document's own
// metadata.timestamp (which appears in the Signature section
// regardless); the zero time omits the "Report generated" row.
// xlsxFileName is the .xlsx companion's own file name (not a path),
// named literally in the invite note wherever NeedsXLSXCompanion is
// true; pass "" when the caller has no companion file name yet (e.g. a
// test), and the note falls back to a generic phrase instead.
func NewReportRenderer(model ReportModel, artifactTime time.Time, xlsxFileName string) *ReportRenderer {
	return &ReportRenderer{model: model, artifact: artifactTime.UTC(), xlsxFileName: xlsxFileName}
}

// Render writes the Markdown report to w.
func (r *ReportRenderer) Render(ctx context.Context, w io.Writer) error {
	cw := render.NewCheckedWriter(w)
	p := &vmanageMDPrinter{w: cw, xlsxFileName: r.xlsxFileName}

	p.linef("# %s", vmanageReportTitle)
	p.blank()

	if err := ctx.Err(); err != nil {
		return err
	}
	sectionSignature(p, r.model, r.artifact)

	p.raw(markdown.TableOfContentsSection(vmanageReportTOC))

	for _, section := range []func(*vmanageMDPrinter, ReportModel, time.Time){
		sectionRouters,
		sectionPorts,
		sectionSites,
		sectionOutOfScope,
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		section(p, r.model, r.artifact)
	}

	return cw.Err()
}

// sectionSignature is the document's own provenance, as a table.
func sectionSignature(p *vmanageMDPrinter, m ReportModel, artifact time.Time) {
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
	p.linef("| Scope name | %s |", markdown.TableCell(s.ScopeName))
	p.linef("| Scope description | %s |", markdown.TableCell(s.ScopeDescription))
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

func generatorURLCell(url string) string {
	if url == "" {
		return "(absent)"
	}
	return markdown.Link(url, url)
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

// hexOnlySHA mirrors internal/render/markdown's own hexOnly: a
// malformed hash can never inject Markdown through the header.
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
// (NeedsXLSXCompanion). Names p.xlsxFileName literally when the caller
// supplied one; falls back to a generic phrase when it did not.
func xlsxInviteNote(p *vmanageMDPrinter, m ReportModel, what string) string {
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

// sectionRouters lists every network.router this document describes
// individually a vManage document's own router count stays bounded
// by real site fleet size.
func sectionRouters(p *vmanageMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Routers")
	p.blank()
	if len(m.Routers) == 0 {
		p.line("No `network.router` resources in this document.")
		p.blank()
		return
	}
	if note := xlsxInviteNote(p, m, "router list"); note != "" {
		p.line(note)
		p.blank()
	}
	p.line("| Name | Site | Model | Chassis number | Serial | Management IP | Software version | Status | State | Health | Connected manager |")
	p.line("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, r := range m.Routers {
		p.linef("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |",
			markdown.TableCell(r.Name), markdown.TableCell(r.Site), markdown.TableCell(r.Model),
			markdown.TableCell(r.ChassisNumber), markdown.TableCell(r.Serial),
			markdown.TableCell(r.ManagementIP), markdown.TableCell(r.SoftwareVersion),
			markdown.TableCell(r.Status), markdown.TableCell(r.State), markdown.TableCell(r.Health),
			joinOrAbsentCell(r.ConnectedManager))
	}
	p.blank()
}

// sectionPorts lists every router.port individually (each row naming
// its own owning router) when the document carries a manageable number
// of them; above vmanageXLSXRowThreshold it collapses to an aggregate-
// only shape, with the full per-port table moved to the .xlsx
// companion.
func sectionPorts(p *vmanageMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Ports")
	p.blank()
	pt := m.Ports
	if pt.Total == 0 {
		p.line("No `network.router.port` resources in this document.")
		p.blank()
		return
	}
	p.linef("%d port(s) observed in this snapshot, across %d router(s).", pt.Total, len(m.Routers))
	p.blank()
	if pt.Total > vmanageXLSXRowThreshold {
		if note := xlsxInviteNote(p, m, "port list"); note != "" {
			p.line(note)
			p.blank()
		}
		if len(pt.ByStatus) > 0 {
			p.line("By status:")
			p.blank()
			p.line("| Status | Count |")
			p.line("| --- | --: |")
			for _, kv := range pt.ByStatus {
				p.linef("| %s | %d |", markdown.TableCell(kv.Key), kv.Count)
			}
			p.blank()
		}
		if len(pt.ByAdminStatus) > 0 {
			p.line("By admin status:")
			p.blank()
			p.line("| Admin status | Count |")
			p.line("| --- | --: |")
			for _, kv := range pt.ByAdminStatus {
				p.linef("| %s | %d |", markdown.TableCell(kv.Key), kv.Count)
			}
			p.blank()
		}
		return
	}
	p.line("| Router | Name | Status | Admin status | Oper status | Speed (Mbps) | MTU | MAC address | VRF ID | Public IP | Private IP | Color | NAT type |")
	p.line("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, port := range pt.Rows {
		p.linef("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |",
			markdown.TableCell(port.RouterName), markdown.TableCell(port.Name), markdown.TableCell(port.Status),
			markdown.TableCell(port.AdminStatus), markdown.TableCell(port.OperStatus), markdown.TableCell(port.SpeedMbps),
			markdown.TableCell(port.MTU), markdown.TableCell(port.MACAddress), markdown.TableCell(port.VRFID),
			markdown.TableCell(port.PublicIP), joinOrAbsentCell(port.PrivateIP),
			markdown.TableCell(port.Color), markdown.TableCell(port.NATType))
	}
	p.blank()
}

func sectionSites(p *vmanageMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Sites")
	p.blank()
	if len(m.Sites) == 0 {
		p.line("No groups in this document.")
		p.blank()
		return
	}
	p.line("| Name | Type | Latitude | Longitude | Members |")
	p.line("| --- | --- | --- | --- | --: |")
	for _, s := range m.Sites {
		p.linef("| %s | %s | %s | %s | %d |",
			markdown.TableCell(s.Name), markdown.TableCell(s.Type), markdown.TableCell(s.Latitude),
			markdown.TableCell(s.Longitude), s.Members)
	}
	p.blank()
}

// ciscoVManageDocsURL is this producer's own OSIRIS JSON documentation
// page.
const ciscoVManageDocsURL = "https://docs.osirisjson.org/osiris-producers/network/cisco"

// sectionOutOfScope explains what this curated report does not show,
// and why.
func sectionOutOfScope(p *vmanageMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Out of scope for this report")
	p.blank()
	p.line("This report curates every router this document describes and their own router.port census, plus the " +
		"site/room groups they belong to. Above a certain size, the port census moves to aggregate counts only in " +
		"this Markdown file.")
	p.blank()
	if m.NeedsXLSXCompanion() {
		p.line("This document's inventory is large enough that the complete per-item lists live in the " +
			"accompanying .xlsx spreadsheet instead (see the invite notes in the sections above), not in this " +
			"Markdown file.")
	} else {
		p.line("This document's inventory stayed small enough that no .xlsx companion was needed for this " +
			"snapshot; a larger inventory would carry one, with the complete per-item lists moved there instead " +
			"of this Markdown file.")
	}
	p.blank()
	p.line("Either way, the generic OSIRIS JSON inventory (this consumer's format-neutral path) represents " +
		"every resource, connection, and group with no semantic loss, for anything neither this report nor its " +
		"spreadsheet covers. In particular, this report does not yet curate SD-WAN overlay state (tunnel health, " +
		"OMP routes, control connections, or policy) beyond what a router.port resource's own properties and " +
		"extensions carry the generic inventory still represents any such resource this producer emits, this " +
		"curated report simply has not been extended to summarize it yet.")
	p.blank()
	p.line("This is a point-in-time snapshot, not live or historical telemetry: OSIRIS JSON describes topology " +
		"and inventory state, not a monitoring feed.")
	p.blank()
	p.line("Coverage may grow in a future release of the Cisco vManage producer, see " +
		markdown.Link(ciscoVManageDocsURL, ciscoVManageDocsURL) + " for what it currently emits.")
	p.blank()
}

// printer
type vmanageMDPrinter struct {
	w            *render.CheckedWriter
	xlsxFileName string
}

func (p *vmanageMDPrinter) line(s string) { _, _ = p.w.WriteString(s + "\n") }
func (p *vmanageMDPrinter) blank()        { _, _ = p.w.WriteString("\n") }
func (p *vmanageMDPrinter) raw(s string)  { _, _ = p.w.WriteString(s) }
func (p *vmanageMDPrinter) linef(format string, a ...any) {
	_, _ = p.w.WriteString(fmt.Sprintf(format, a...) + "\n")
}
