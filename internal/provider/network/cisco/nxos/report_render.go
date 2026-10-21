// This file is the deterministic Markdown renderer for the Cisco
// NX-OS report model: a Signature table, a switch chassis summary, a
// port census, the SVI/loopback/management interface inventory, the
// port-channel (LAG) inventory, the VLAN inventory, the VRF inventory,
// a CDP/LLDP-discovered neighbour census, and an explicit note on what
// this curated report does not enumerate individually. Every dynamic
// string is escaped through internal/render/markdown's destination-
// aware helpers; the only variable input is the injected artifact
// time.
package nxos

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/markdown"
)

// nxosReportTOC is every H2 heading this renderer writes, in emission
// order (see markdown.TableOfContents' doc comment). Signature has no
// heading of its own (it goes straight under the title) and so is not
// listed here.
var nxosReportTOC = []string{
	"Switch summary",
	"Ports",
	"Interfaces",
	"Port-channels",
	"VLANs",
	"VRFs",
	"Discovered neighbours",
	"Out of scope for this report",
}

// reportTitle appends the switch's own name to the report title when
// the document discloses one; falls back to the plain title otherwise,
// rather than fabricating a name.
func reportTitle(s SwitchInfo) string {
	const base = "OSIRIS JSON Cisco NX-OS switch report"
	if s.Name == "" || s.Name == "(absent)" {
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
	p := &nxosMDPrinter{w: cw, xlsxFileName: r.xlsxFileName}

	p.linef("# %s", reportTitle(r.model.Switch))
	p.blank()

	if err := ctx.Err(); err != nil {
		return err
	}
	sectionSignature(p, r.model, r.artifact)

	p.raw(markdown.TableOfContentsSection(nxosReportTOC))

	for _, section := range []func(*nxosMDPrinter, ReportModel, time.Time){
		sectionSwitchSummary,
		sectionPorts,
		sectionInterfaces,
		sectionLAGs,
		sectionVLANs,
		sectionVRFs,
		sectionNeighbours,
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
// No heading: it goes straight under the title.
func sectionSignature(p *nxosMDPrinter, m ReportModel, artifact time.Time) {
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
func xlsxInviteNote(p *nxosMDPrinter, m ReportModel, what string) string {
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

func sectionSwitchSummary(p *nxosMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Switch summary")
	p.blank()
	s := m.Switch
	p.line("| Field | Value |")
	p.line("| --- | --- |")
	p.linef("| Name | %s |", markdown.TableCell(s.Name))
	p.linef("| Model | %s |", markdown.TableCell(s.ModelDescription))
	p.linef("| Chassis product ID | %s |", markdown.TableCell(s.ChassisProductID))
	p.linef("| Manufacturer | %s |", markdown.TableCell(s.Manufacturer))
	p.linef("| Serial number | %s |", markdown.TableCell(s.Serial))
	p.linef("| Software version | %s |", markdown.TableCell(s.SoftwareVersion))
	p.linef("| BIOS version | %s |", markdown.TableCell(s.BiosVersion))
	p.linef("| Management IP | %s |", markdown.TableCell(s.ManagementIP))
	p.linef("| Memory (MB) | %s |", markdown.TableCell(s.MemoryMB))
	p.linef("| Port count | %s |", markdown.TableCell(s.PortCount))
	p.blank()
}

// sectionPorts stays aggregate-only in Markdown regardless of the
// switch's own port count (a chassis with many dozens of ports would
// otherwise dominate the report): the full per-port table lives only
// in the .xlsx companion once NeedsXLSXCompanion is true.
func sectionPorts(p *nxosMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Ports")
	p.blank()
	pt := m.Ports
	if pt.Total == 0 {
		p.line("No `network.switch.port` resources in this document.")
		p.blank()
		return
	}
	p.linef("%d port(s) observed in this snapshot.", pt.Total)
	p.blank()
	if note := xlsxInviteNote(p, m, "port list"); note != "" {
		p.line(note)
		p.blank()
	}
	p.line("| Field | Count |")
	p.line("| --- | --- |")
	p.linef("| Admin up | %d |", pt.AdminUp)
	p.linef("| Admin down | %d |", pt.AdminDown)
	p.blank()
	if len(pt.ByPortMode) > 0 {
		p.line("By port mode:")
		p.blank()
		p.line("| Port mode | Count |")
		p.line("| --- | --: |")
		for _, kv := range pt.ByPortMode {
			p.linef("| %s | %d |", markdown.TableCell(kv.Key), kv.Count)
		}
		p.blank()
	}
}

// sectionInterfaces lists every SVI, loopback, and management
// interface individually when the switch carries a manageable number
// of them; above nxosXLSXRowThreshold it collapses to the same
// aggregate-only shape as Ports, with the full table moved to the
// .xlsx companion.
func sectionInterfaces(p *nxosMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Interfaces")
	p.blank()
	p.line("SVIs, loopbacks, and the management interface - this switch's own `network.interface` resources. " +
		"CDP/LLDP-discovered neighbour interfaces are not included here; see \"Discovered neighbours\" below.")
	p.blank()
	it := m.Interfaces
	if it.Total == 0 {
		p.line("No `network.interface` resources under this switch's own provider in this document.")
		p.blank()
		return
	}
	if it.Total > nxosXLSXRowThreshold {
		if note := xlsxInviteNote(p, m, "interface list"); note != "" {
			p.line(note)
			p.blank()
		}
		p.linef("%d interface(s) observed in this snapshot.", it.Total)
		p.blank()
		if len(it.ByAdminStatus) > 0 {
			p.line("| Admin status | Count |")
			p.line("| --- | --: |")
			for _, kv := range it.ByAdminStatus {
				p.linef("| %s | %d |", markdown.TableCell(kv.Key), kv.Count)
			}
			p.blank()
		}
		return
	}
	p.line("| Name | Admin status | IP address |")
	p.line("| --- | --- | --- |")
	for _, i := range it.Rows {
		p.linef("| %s | %s | %s |", markdown.TableCell(i.Name), markdown.TableCell(i.AdminStatus), markdown.TableCell(i.IPAddress))
	}
	p.blank()
}

func sectionLAGs(p *nxosMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Port-channels")
	p.blank()
	if len(m.LAGs) == 0 {
		p.line("No `network.interface.lag` resources in this document.")
		p.blank()
		return
	}
	if note := xlsxInviteNote(p, m, "port-channel member list"); note != "" {
		p.line(note)
		p.blank()
	}
	p.line("| Name | Admin status | Protocol | Port mode | Native VLAN | Access VLAN | Member count | Members |")
	p.line("| --- | --- | --- | --- | --- | --- | --: | --- |")
	for _, l := range m.LAGs {
		p.linef("| %s | %s | %s | %s | %s | %s | %s | %s |",
			markdown.TableCell(l.Name), markdown.TableCell(l.AdminStatus), markdown.TableCell(l.Protocol),
			markdown.TableCell(l.PortMode), markdown.TableCell(l.NativeVLAN), markdown.TableCell(l.AccessVLAN),
			markdown.TableCell(l.MemberCount), joinOrAbsentCell(l.Members))
	}
	p.blank()
}

// sectionVLANs lists every VLAN individually when the switch carries a
// manageable number of them; above nxosXLSXRowThreshold it collapses
// to an aggregate-only shape, with the full table moved to the .xlsx
// companion.
func sectionVLANs(p *nxosMDPrinter, m ReportModel, _ time.Time) {
	p.line("## VLANs")
	p.blank()
	vt := m.VLANs
	if vt.Total == 0 {
		p.line("No `network.vlan` resources in this document.")
		p.blank()
		return
	}
	if vt.Total > nxosXLSXRowThreshold {
		if note := xlsxInviteNote(p, m, "VLAN list"); note != "" {
			p.line(note)
			p.blank()
		}
		p.linef("%d VLAN(s) observed in this snapshot.", vt.Total)
		p.blank()
		if len(vt.ByAdminState) > 0 {
			p.line("| Admin state | Count |")
			p.line("| --- | --: |")
			for _, kv := range vt.ByAdminState {
				p.linef("| %s | %d |", markdown.TableCell(kv.Key), kv.Count)
			}
			p.blank()
		}
		return
	}
	p.line("| VLAN ID | Name | Admin state |")
	p.line("| --: | --- | --- |")
	for _, v := range vt.Rows {
		p.linef("| %s | %s | %s |", markdown.TableCell(v.ID), markdown.TableCell(v.Name), markdown.TableCell(v.AdminState))
	}
	p.blank()
}

func sectionVRFs(p *nxosMDPrinter, m ReportModel, _ time.Time) {
	p.line("## VRFs")
	p.blank()
	if len(m.VRFs) == 0 {
		p.line("No `logical.vrf` groups in this document.")
		p.blank()
		return
	}
	p.line("| Name | VRF ID | Route distinguisher | State | Members |")
	p.line("| --- | --- | --- | --- | --: |")
	for _, v := range m.VRFs {
		p.linef("| %s | %s | %s | %s | %d |",
			markdown.TableCell(v.Name), markdown.TableCell(v.VRFID), markdown.TableCell(v.RouteDistinguisher),
			markdown.TableCell(v.State), v.Members)
	}
	p.blank()
}

// sectionNeighbours stays aggregate-only in Markdown, mirroring
// sectionPorts: the full per-neighbour table lives only in the .xlsx
// companion once NeedsXLSXCompanion is true.
func sectionNeighbours(p *nxosMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Discovered neighbours")
	p.blank()
	n := m.Neighbours
	if n.Total == 0 {
		p.line("No CDP/LLDP-discovered neighbours in this document.")
		p.blank()
		return
	}
	p.linef("%d neighbour(s) observed in this snapshot: devices CDP/LLDP discovered beyond this switch itself, "+
		"carrying only what the discovery protocol asserts, never a full inventory.", n.Total)
	p.blank()
	if note := xlsxInviteNote(p, m, "neighbour list"); note != "" {
		p.line(note)
		p.blank()
	}
	if len(n.ByDiscoveredVia) > 0 {
		p.line("| Discovered via | Count |")
		p.line("| --- | --: |")
		for _, kv := range n.ByDiscoveredVia {
			p.linef("| %s | %d |", markdown.TableCell(kv.Key), kv.Count)
		}
		p.blank()
	}
}

// ciscoNXOSDocsURL is this producer's own OSIRIS JSON documentation
// page.
const ciscoNXOSDocsURL = "https://docs.osirisjson.org/osiris-producers/network/cisco"

// sectionOutOfScope explains what this curated report does not show,
// and why.
func sectionOutOfScope(p *nxosMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Out of scope for this report")
	p.blank()
	p.line("This report curates the switch's own chassis inventory, port and interface census, port-channel and " +
		"VLAN inventory, VRF inventory, and CDP/LLDP-discovered neighbour census. Above a certain size, ports, " +
		"interfaces, VLANs, and neighbours move to aggregate counts only in this Markdown file.")
	p.blank()
	if m.NeedsXLSXCompanion() {
		p.line("This document's inventory is large enough that those complete per-item lists live in the " +
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
		"spreadsheet covers.")
	p.blank()
	p.line("This is a point-in-time snapshot, not live or historical telemetry: OSIRIS JSON describes topology " +
		"and inventory state, not a monitoring feed.")
	p.blank()
	p.line("Coverage may grow in a future release of the Cisco NX-OS producer - see " +
		markdown.Link(ciscoNXOSDocsURL, ciscoNXOSDocsURL) + " for what it currently emits.")
	p.blank()
}

// printer

type nxosMDPrinter struct {
	w            *render.CheckedWriter
	xlsxFileName string
}

func (p *nxosMDPrinter) line(s string) { _, _ = p.w.WriteString(s + "\n") }
func (p *nxosMDPrinter) blank()        { _, _ = p.w.WriteString("\n") }
func (p *nxosMDPrinter) raw(s string)  { _, _ = p.w.WriteString(s) }
func (p *nxosMDPrinter) linef(format string, a ...any) {
	_, _ = p.w.WriteString(fmt.Sprintf(format, a...) + "\n")
}
