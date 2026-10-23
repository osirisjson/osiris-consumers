// This file is the deterministic Markdown renderer for the Cisco APIC
// report model: a Signature table, a fabric-wide summary, a
// controller inventory, one spine and one leaf inventory table per
// pod (each row carrying its own point-in-time fault severity
// counts), a tenant/VRF/EPG rollup, an L3out listing, bridge-domain
// and subnet counts, a port census, an externally discovered
// neighbour census, and an explicit note on what this curated report
// does not enumerate individually. Every dynamic string is escaped
// through internal/render/markdown's destination-aware helpers; the
// only variable input is the injected artifact time.
package apic

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/markdown"
)

// apicReportTOC is every H2 heading this renderer writes, in emission
// order, kept in sync with Render's own section list by hand (see
// markdown.TableOfContents' doc comment): Signature has no heading of
// its own (it goes straight under the title) and so is not listed
// here, there would be nothing to link to. The per-pod "### Pod N"
// and "#### Spine"/"#### Leaf" subheadings inside "Fabric architecture"
// are deliberately left out too: a fabric can have many pods, and
// this table of contents stays a fixed, short jump list regardless of
// fabric size.
var apicReportTOC = []string{
	"Faults",
	"Fabric summary",
	"Controllers",
	"Fabric architecture",
	"Tenants, VRFs, and EPGs",
	"L3outs",
	"Bridge domains and subnets",
	"Ports",
	"Externally discovered neighbours",
	"Out of scope for this report",
}

// reportTitle appends the fabric domain to the report title when the
// document discloses one; falls back to the plain title otherwise,
// rather than fabricating a name.
func reportTitle(f FabricSummary) string {
	const base = "OSIRIS JSON Cisco APIC report"
	if f.Domain == "" || f.Domain == "(absent)" {
		return base
	}
	return base + " for " + markdown.Text(f.Domain)
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
	p := &apicMDPrinter{w: cw, xlsxFileName: r.xlsxFileName}

	p.linef("# %s", reportTitle(r.model.Fabric))
	p.blank()

	if err := ctx.Err(); err != nil {
		return err
	}
	sectionSignature(p, r.model, r.artifact)

	p.raw(markdown.TableOfContentsSection(apicReportTOC))

	for _, section := range []func(*apicMDPrinter, ReportModel, time.Time){
		sectionFaults,
		sectionFabricSummary,
		sectionControllers,
		sectionFabricSwitches,
		sectionTenants,
		sectionL3Outs,
		sectionBridgeDomainsAndSubnets,
		sectionPorts,
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
func sectionSignature(p *apicMDPrinter, m ReportModel, artifact time.Time) {
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
// (NeedsXLSXCompanion): once any of the eight enumerable categories
// crosses the row threshold, every one of them gets a full sheet in
// the same workbook (report_model.go), so every section that lists one
// of the eight gets this same note. Names p.xlsxFileName literally
// when the caller supplied one; falls back to a generic phrase when it
// did not (e.g. a test rendering with no real output file in play).
func xlsxInviteNote(p *apicMDPrinter, m ReportModel, what string) string {
	if !m.NeedsXLSXCompanion() {
		return ""
	}
	file := "the accompanying spreadsheet (same file name, `.xlsx` extension)"
	if p.xlsxFileName != "" {
		// A code span's own delimiter is a backtick, so one embedded in
		// the name would break out of it; strip it defensively (a real
		// file name will never legitimately contain one).
		safe := strings.ReplaceAll(p.xlsxFileName, "`", "")
		file = "`" + safe + "`"
	}
	return "The complete " + what + " is also available in " + file + " for easier sorting and filtering."
}

// sectionFaults lists every fault any controller or role-bearing
// switch asserts, critical-first (see buildFaults), right below the
// table of contents: OSIRIS JSON is a point-in-time snapshot document,
// not a monitoring stream, but a fault is still immediately actionable
// information and belongs at the top rather than buried under the
// inventory tables.
func sectionFaults(p *apicMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Faults")
	p.blank()
	if len(m.Faults) == 0 {
		p.line("No faults reported by any controller or role-bearing switch in this document.")
		p.blank()
		return
	}
	p.linef("%d fault(s) observed in this snapshot, sorted by severity (critical first). "+
		"OSIRIS JSON is a point-in-time infrastructure portable snapshot, it describes topology and inventory state, not time-series metrics, logs or observability data. The following data is not to be considered as live telemetry data. see \"Out of scope for this report\" below.",
		len(m.Faults))
	p.blank()

	// Above apicXLSXRowThreshold, the per-fault table moves to the
	// .xlsx companion entirely only a severity breakdown stays
	// inline here.
	if len(m.Faults) > apicXLSXRowThreshold {
		if note := xlsxInviteNote(p, m, "fault list"); note != "" {
			p.line(note)
			p.blank()
		}
		var counts FaultCounts
		for _, f := range m.Faults {
			switch f.Severity {
			case "critical":
				counts.Critical++
			case "major":
				counts.Major++
			case "minor":
				counts.Minor++
			case "warning":
				counts.Warning++
			}
		}
		p.line("| Severity | Count |")
		p.line("| --- | --: |")
		p.linef("| Critical | %d |", counts.Critical)
		p.linef("| Major | %d |", counts.Major)
		p.linef("| Minor | %d |", counts.Minor)
		p.linef("| Warning | %d |", counts.Warning)
		p.blank()
		return
	}

	if note := xlsxInviteNote(p, m, "fault list"); note != "" {
		p.line(note)
		p.blank()
	}
	p.line("| Node | Severity | Cause | Description | Domain | Lifecycle | Created | Last transition |")
	p.line("| --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, f := range m.Faults {
		p.linef("| %s | %s | %s | %s | %s | %s | %s | %s |",
			markdown.TableCell(f.Node), markdown.TableCell(f.Severity), markdown.TableCell(f.Cause),
			markdown.TableCell(f.Description), markdown.TableCell(f.Domain), markdown.TableCell(f.Lifecycle),
			markdown.TableCell(f.Created), markdown.TableCell(f.LastTransition))
	}
	p.blank()
}

func sectionFabricSummary(p *apicMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Fabric summary")
	p.blank()
	f := m.Fabric
	p.line("| Field | Count |")
	p.line("| --- | --- |")
	p.linef("| Fabric domain | %s |", markdown.TableCell(f.Domain))
	p.linef("| Pods | %d |", f.Pods)
	p.linef("| Controllers | %d |", f.Controllers)
	p.linef("| Spine switches | %d |", f.Spines)
	p.linef("| Leaf switches | %d |", f.Leafs)
	p.linef("| Externally discovered neighbours | %d |", f.Neighbours)
	p.linef("| Tenants | %d |", f.Tenants)
	p.linef("| VRFs | %d |", f.VRFs)
	p.linef("| EPGs | %d |", f.EPGs)
	p.linef("| Bridge domains | %d |", f.BridgeDomains)
	p.linef("| Subnets | %d |", f.Subnets)
	p.linef("| L3outs | %d |", f.L3Outs)
	p.linef("| Ports | %d |", f.Ports)
	p.blank()
}

// nodeTableHeader and nodeTableRow are shared by the controller and
// spine/leaf inventory tables, so every fabric node's columns line up
// the same way throughout the report.
func nodeTableHeader(p *apicMDPrinter) {
	p.line("| Name | Model | Serial | Firmware | OOB mgmt | In-band mgmt | State | Uptime | Faults (C/Ma/Mi/W) |")
	p.line("| --- | --- | --- | --- | --- | --- | --- | --- | --: |")
}

func nodeTableRow(p *apicMDPrinter, n NodeInfo) {
	faults := fmt.Sprintf("%d/%d/%d/%d", n.Faults.Critical, n.Faults.Major, n.Faults.Minor, n.Faults.Warning)
	p.linef("| %s | %s | %s | %s | %s | %s | %s | %s | %s |",
		markdown.TableCell(n.Name), markdown.TableCell(n.Model), markdown.TableCell(n.Serial),
		markdown.TableCell(n.Firmware), markdown.TableCell(n.OobMgmtAddr), markdown.TableCell(n.InbMgmtAddr),
		markdown.TableCell(n.SystemState), markdown.TableCell(n.Uptime), markdown.TableCell(faults))
}

func sectionControllers(p *apicMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Controllers")
	p.blank()
	var rows []NodeInfo
	for _, n := range m.Nodes {
		if n.Band == bandController {
			rows = append(rows, n)
		}
	}
	if len(rows) == 0 {
		p.line("No `compute.server` resources in this document.")
		p.blank()
		return
	}
	nodeTableHeader(p)
	for _, n := range rows {
		nodeTableRow(p, n)
	}
	p.blank()
}

// sectionFabricSwitches writes one Spine and one Leaf inventory table
// per pod, in ascending pod-number order (m.Nodes is already sorted
// that way see buildNodes).
func sectionFabricSwitches(p *apicMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Fabric architecture")
	p.blank()
	byPod := map[string][2][]NodeInfo{} // [0]=spines, [1]=leafs
	var podOrder []string
	for _, n := range m.Nodes {
		if n.Band != bandSpine && n.Band != bandLeaf {
			continue
		}
		if _, ok := byPod[n.Pod]; !ok {
			podOrder = append(podOrder, n.Pod)
		}
		e := byPod[n.Pod]
		if n.Band == bandSpine {
			e[0] = append(e[0], n)
		} else {
			e[1] = append(e[1], n)
		}
		byPod[n.Pod] = e
	}
	if len(podOrder) == 0 {
		p.line("No role-bearing `network.switch` resources in this document.")
		p.blank()
		return
	}
	for _, pod := range podOrder {
		p.linef("### Pod %s", markdown.Text(podLabel(pod)))
		p.blank()
		e := byPod[pod]
		for _, section := range [2]struct {
			title string
			rows  []NodeInfo
		}{{"Spine", e[0]}, {"Leaf", e[1]}} {
			p.linef("#### %s", section.title)
			p.blank()
			if len(section.rows) == 0 {
				p.line("None for this pod.")
				p.blank()
				continue
			}
			nodeTableHeader(p)
			for _, n := range section.rows {
				nodeTableRow(p, n)
			}
			p.blank()
		}
	}
}

// podLabel returns the pod's display number (the "pod-N" identifier
// with its "pod-" prefix trimmed, since the heading already says
// "Pod"), or "(unknown)" for a switch whose native_id carried no pod
// segment.
func podLabel(pod string) string {
	if pod == "" {
		return "(unknown)"
	}
	return strings.TrimPrefix(pod, "pod-")
}

func sectionTenants(p *apicMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Tenants, VRFs, and EPGs")
	p.blank()
	p.line("Tenant rollup only: a fabric can carry hundreds of individual VRFs and EPGs.")
	p.blank()
	if note := xlsxInviteNote(p, m, "per-VRF and per-EPG detail"); note != "" {
		p.line(note)
		p.blank()
	}
	if len(m.Tenants) == 0 {
		p.line("No `logical.tenant` groups in this document.")
		p.blank()
		return
	}
	p.line("| Tenant | VRFs | EPGs | Direct members |")
	p.line("| --- | --: | --: | --: |")
	for _, t := range m.Tenants {
		p.linef("| %s | %d | %d | %d |", markdown.TableCell(t.Name), t.VRFs, t.EPGs, t.DirectMembers)
	}
	p.blank()
}

func sectionL3Outs(p *apicMDPrinter, m ReportModel, _ time.Time) {
	p.line("## L3outs")
	p.blank()
	if len(m.L3Outs) == 0 {
		p.line("No `osiris.cisco.l3out` resources in this document.")
		p.blank()
		return
	}
	p.line("| Name | Tenant | Native ID |")
	p.line("| --- | --- | --- |")
	for _, l := range m.L3Outs {
		p.linef("| %s | %s | %s |", markdown.TableCell(l.Name), markdown.TableCell(l.Tenant), markdown.TableCell(l.NativeID))
	}
	p.blank()
}

func sectionBridgeDomainsAndSubnets(p *apicMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Bridge domains and subnets")
	p.blank()
	p.line("Counts only: see \"L3outs\" and \"Tenants, VRFs, and EPGs\" above for the tenant structure these belong to.")
	p.blank()
	if note := xlsxInviteNote(p, m, "bridge domain and subnet detail"); note != "" {
		p.line(note)
		p.blank()
	}
	p.line("| Field | Count |")
	p.line("| --- | --- |")
	p.linef("| Bridge domains (`osiris.cisco.domain.bridge`) | %d |", m.Fabric.BridgeDomains)
	p.linef("| Subnets (`network.subnet`) | %d |", m.Fabric.Subnets)
	p.blank()
}

func sectionPorts(p *apicMDPrinter, m ReportModel, _ time.Time) {
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
	p.linef("| Operationally up | %d |", pt.OperUp)
	p.linef("| Operationally down | %d |", pt.OperDown)
	p.linef("| Operational status not asserted | %d |", pt.OperUnknown)
	p.blank()
	if len(pt.ByUsage) > 0 {
		p.line("By usage:")
		p.blank()
		p.line("| Usage | Count |")
		p.line("| --- | --: |")
		for _, kv := range pt.ByUsage {
			p.linef("| %s | %d |", markdown.TableCell(kv.Key), kv.Count)
		}
		p.blank()
	}
}

func sectionNeighbours(p *apicMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Externally discovered neighbours")
	p.blank()
	n := m.Neighbours
	if n.Total == 0 {
		p.line("No externally discovered (role-less) `network.switch` resources in this document.")
		p.blank()
		return
	}
	p.linef("%d neighbour(s) observed in this snapshot: devices LLDP/CDP discovered beyond the fabric's own controllers and switches, carrying only a chassis identifier and discovery method, never a role or name.", n.Total)
	p.blank()
	if note := xlsxInviteNote(p, m, "neighbour list"); note != "" {
		p.line(note)
		p.blank()
	}
	if len(n.ByDiscoveredBy) > 0 {
		p.line("| Discovered by | Count |")
		p.line("| --- | --: |")
		for _, kv := range n.ByDiscoveredBy {
			p.linef("| %s | %d |", markdown.TableCell(kv.Key), kv.Count)
		}
		p.blank()
	}
}

// ciscoAPICDocsURL is this producer's own OSIRIS JSON documentation
// page.
const ciscoAPICDocsURL = "https://docs.osirisjson.org/osiris-producers/network/cisco/"

// sectionOutOfScope explains what this curated report does not show,
// and why: what is curated vs. only counted, where the
// full per-item detail actually lives (the .xlsx companion when one
// was needed, the generic inventory otherwise), that fault and uptime
// values are a snapshot rather than a live feed, and a pointer to the
// producer's own docs for future coverage.
func sectionOutOfScope(p *apicMDPrinter, m ReportModel, _ time.Time) {
	p.line("## Out of scope for this report")
	p.blank()
	p.line("This report curates the fabric's controllers, spine and leaf switches, tenant structure, and " +
		"aggregate counts for ports, bridge domains, subnets, and externally discovered neighbours. It does not " +
		"individually list every port, bridge domain, subnet, VRF, or EPG inline.")
	p.blank()
	if m.NeedsXLSXCompanion() {
		p.line("This document's fabric is large enough that those complete per-item lists live in the " +
			"accompanying .xlsx spreadsheet instead (see the invite notes in the sections above), not in this " +
			"Markdown file.")
	} else {
		p.line("This document's fabric stayed small enough that no .xlsx companion was needed for this snapshot; " +
			"a larger fabric would carry one, with the complete per-item lists moved there instead of this " +
			"Markdown file.")
	}
	p.blank()
	p.line("Either way, the generic OSIRIS JSON inventory (this consumer's format-neutral path) represents " +
		"every resource, connection, and group with no semantic loss, for anything neither this report nor its " +
		"spreadsheet covers.")
	p.blank()
	p.line("Fault counts and uptime above are this document's own point-in-time snapshot values, not live or " +
		"historical telemetry: OSIRIS JSON describes topology and inventory state, not a monitoring feed.")
	p.blank()
	p.line("Coverage may grow in a future release of the Cisco APIC producer, see " +
		markdown.Link(ciscoAPICDocsURL, ciscoAPICDocsURL) + " for what it currently emits.")
	p.blank()
}

// printer

type apicMDPrinter struct {
	w            *render.CheckedWriter
	xlsxFileName string
}

func (p *apicMDPrinter) line(s string) { _, _ = p.w.WriteString(s + "\n") }
func (p *apicMDPrinter) blank()        { _, _ = p.w.WriteString("\n") }
func (p *apicMDPrinter) raw(s string)  { _, _ = p.w.WriteString(s) }
func (p *apicMDPrinter) linef(format string, a ...any) {
	_, _ = p.w.WriteString(fmt.Sprintf(format, a...) + "\n")
}
