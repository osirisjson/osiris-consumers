// This file is the deterministic Markdown renderer for the generic
// report model. It writes a vendor-neutral inventory brief: a document
// header, an evidence-state disclosure of which optional regions the
// input carried, exact counts, per-provider and per-type breakdowns,
// and full resource, connection, and group tables. Every dynamic string
// is escaped for its Markdown position; the only variable input is the
// injected artifact time. Identical model plus identical time produce
// byte-identical output.

package markdown

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/report"
)

// placeholder text for an absent optional value in a table cell.
const (
	none       = "(none)"
	noProvider = "(no provider)"
)

// reportTOC is every H2 heading this renderer unconditionally writes,
// in emission order, kept in sync with Render's own section list and
// each section's own "## Title" line by hand (see TableOfContents' doc
// comment): there is one deliberately small point of duplication
// rather than a heavier indirection for a list that changes rarely.
// "Notes" is not here: sectionNotes omits its own heading entirely
// when there is nothing to report (see reportTOCWithConditional).
var reportTOC = []string{
	"Document",
	"Disclosure",
	"Inventory",
	"Providers",
	"Resource types",
	"Connection types",
	"Resources",
	"Connections",
	"Groups",
	"Structural anomalies",
}

// reportTOCWithConditional returns reportTOC plus "Notes" when m will
// actually render that heading (sectionNotes writes nothing at all,
// not even its heading, when there are no truncations), so the table
// of contents never links to a heading the document doesn't have.
func reportTOCWithConditional(m *report.Report) []string {
	if len(m.Truncations) == 0 {
		return reportTOC
	}
	return append(append([]string(nil), reportTOC...), "Notes")
}

// ReportRenderer renders a report.Report as Markdown.
// The zero value is not usable; construct one with NewReportRenderer.
type ReportRenderer struct {
	model    *report.Report
	artifact time.Time
}

// NewReportRenderer returns a renderer for model. artifactTime is
// stamped into the header; the zero time means "omit the generated
// line" and keeps the output free of any clock reading.
func NewReportRenderer(model *report.Report, artifactTime time.Time) *ReportRenderer {
	return &ReportRenderer{model: model, artifact: artifactTime.UTC()}
}

// Render writes the Markdown brief to w. It checks ctx at section
// boundaries and returns the first write error without wrapping it
// away.
func (r *ReportRenderer) Render(ctx context.Context, w io.Writer) error {
	cw := render.NewCheckedWriter(w)
	p := &printer{w: cw}
	m := r.model

	p.line("# OSIRIS JSON inventory brief")
	p.blank()
	r.header(p)

	if err := ctx.Err(); err != nil {
		return err
	}
	sectionDocument(p, m)

	p.raw(TableOfContentsSection(reportTOCWithConditional(m)))

	for _, section := range []func(*printer, *report.Report){
		sectionDisclosure,
		sectionInventory,
		sectionProviders,
		sectionResourceTypes,
		sectionConnectionTypes,
		sectionResources,
		sectionConnections,
		sectionGroups,
		sectionAnomalies,
		sectionNotes,
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		section(p, m)
	}

	return cw.Err()
}

// header writes the italic provenance line under the title. The input
// hash and the artifact time are both this tool's own ASCII values, so
// they go in verbatim; only document-sourced strings are escaped.
func (r *ReportRenderer) header(p *printer) {
	sha := hexOnly(r.model.Document.InputSHA256)
	if r.artifact.IsZero() {
		p.linef("_Input SHA-256 `%s`._", sha)
	} else {
		p.linef("_Generated %s. Input SHA-256 `%s`._", r.artifact.Format(time.RFC3339), sha)
	}
	p.blank()
}

// hexOnly returns s when it is a plain lower-case hex string and the
// literal "(unknown)" otherwise, so a malformed hash can never inject
// Markdown through the header.
func hexOnly(s string) string {
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

func sectionDocument(p *printer, m *report.Report) {
	d := m.Document
	p.line("## Document")
	p.blank()
	p.bullet("OSIRIS JSON version: " + code(d.OSIRISVersion))
	p.bullet("Snapshot timestamp: " + code(d.Timestamp))
	if d.GeneratorName != "" || d.GeneratorVersion != "" {
		p.bullet("Generator: " + Text(strings.TrimSpace(d.GeneratorName+" "+d.GeneratorVersion)))
	} else {
		p.bullet("Generator: " + none)
	}
	if d.ScopeName != "" {
		p.bullet("Scope: " + Text(d.ScopeName))
	}
	if d.ScopeDescription != "" {
		p.bullet("Scope description: " + Text(d.ScopeDescription))
	}
	p.bullet(fmt.Sprintf(
		"Validation: %s %s, profile %s, spec %s, %d errors, %d warnings",
		code(d.EngineName), code(d.EngineVersion),
		code(d.Profile), code(d.SpecificationVersion),
		d.ErrorCount, d.WarningCount,
	))
	p.blank()
}

func sectionDisclosure(p *printer, m *report.Report) {
	p.line("## Disclosure")
	p.blank()
	p.line("Whether the input carried each optional region. \"absent\" means the")
	p.line("document did not include it, not that it is empty or non-compliant.")
	p.blank()
	p.bullet("Generator metadata: " + observedAbsent(m.Disclosure.Generator))
	p.bullet("Scope metadata: " + observedAbsent(m.Disclosure.Scope))
	p.bullet("Connections array: " + observedAbsent(m.Disclosure.Connections))
	p.bullet("Groups array: " + observedAbsent(m.Disclosure.Groups))
	p.bullet("Unknown top-level fields: " + observedAbsent(m.Disclosure.UnknownTopLevelFields))
	p.blank()
}

func sectionInventory(p *printer, m *report.Report) {
	c := m.Counts
	p.line("## Inventory")
	p.blank()
	p.line("| Metric | Count |")
	p.line("| --- | --: |")
	for _, row := range []struct {
		label string
		n     int
	}{
		{"Resources", c.Resources},
		{"Connections", c.Connections},
		{"Edges", c.Edges},
		{"Groups", c.Groups},
		{"Providers", c.Providers},
		{"Resource types", c.ResourceTypes},
		{"Connection types", c.ConnectionTypes},
		{"Resources with no or unknown provider", c.UnknownProviders},
		{"Structural anomalies", c.Anomalies},
	} {
		p.linef("| %s | %d |", row.label, row.n)
	}
	p.blank()
}

func sectionProviders(p *printer, m *report.Report) {
	p.line("## Providers")
	p.blank()
	if len(m.Providers) == 0 {
		p.line("No resources.")
		p.blank()
		return
	}
	p.line("| Provider | Resources | Resource types |")
	p.line("| --- | --: | --- |")
	for _, pv := range m.Providers {
		name := noProvider
		if pv.Name != "" {
			name = TableCell(pv.Name)
		}
		p.linef("| %s | %d | %s |", name, pv.ResourceCount, joinCell(pv.ResourceTypes))
	}
	p.blank()
}

func sectionResourceTypes(p *printer, m *report.Report) {
	p.line("## Resource types")
	p.blank()
	writeTypeTable(p, m.ResourceTypes, "No resources.")
}

func sectionConnectionTypes(p *printer, m *report.Report) {
	if !m.Disclosure.Connections {
		return
	}
	p.line("## Connection types")
	p.blank()
	writeTypeTable(p, m.ConnectionTypes, "No connections.")
}

func writeTypeTable(p *printer, rows []report.TypeBreakdown, empty string) {
	if len(rows) == 0 {
		p.line(empty)
		p.blank()
		return
	}
	p.line("| Type | Count |")
	p.line("| --- | --: |")
	for _, row := range rows {
		t := none
		if row.Type != "" {
			t = TableCell(row.Type)
		}
		p.linef("| %s | %d |", t, row.Count)
	}
	p.blank()
}

func sectionResources(p *printer, m *report.Report) {
	p.line("## Resources")
	p.blank()
	if len(m.Resources) == 0 {
		p.line("No resources.")
		p.blank()
		return
	}
	p.line("| ID | Type | Name | Provider | Region | Status | State | Out | In | Groups | Carries |")
	p.line("| --- | --- | --- | --- | --- | --- | --- | --: | --: | --- | --- |")
	for _, e := range m.Resources {
		p.linef("| %s | %s | %s | %s | %s | %s | %s | %d | %d | %s | %s |",
			code(e.ID),
			cellOrNone(e.Type),
			cellOrNone(e.Name),
			providerCell(e.Provider),
			cellOrNone(e.Region),
			cellOrNone(e.Status),
			cellOrNone(e.State),
			e.OutDegree, e.InDegree,
			joinCell(e.GroupIDs),
			carries(e.HasProperties, e.HasExtensions, e.ExtraKeys),
		)
	}
	p.blank()
	writeNativeIDs(p, m.Resources)
}

// writeNativeIDs lists every resource that carries a provider native_id,
// verbatim, so the value is disclosed without crowding the main table.
func writeNativeIDs(p *printer, rows []report.ResourceEntry) {
	var have []report.ResourceEntry
	for _, e := range rows {
		if e.NativeID != "" {
			have = append(have, e)
		}
	}
	if len(have) == 0 {
		return
	}
	p.line("### Resource native identifiers")
	p.blank()
	p.line("Provider `native_id` values, carried verbatim and never parsed.")
	p.blank()
	p.line("| ID | native_id |")
	p.line("| --- | --- |")
	for _, e := range have {
		p.linef("| %s | %s |", code(e.ID), code(e.NativeID))
	}
	p.blank()
}

func sectionConnections(p *printer, m *report.Report) {
	if !m.Disclosure.Connections {
		return
	}
	p.line("## Connections")
	p.blank()
	if len(m.Connections) == 0 {
		p.line("No connections.")
		p.blank()
		return
	}
	p.line("| ID | Type | Source | Target | Direction | Status | Name | Notes |")
	p.line("| --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, e := range m.Connections {
		p.linef("| %s | %s | %s | %s | %s | %s | %s | %s |",
			code(e.ID),
			cellOrNone(e.Type),
			endpointCell(e.Source, e.SourceMissing),
			endpointCell(e.Target, e.TargetMissing),
			cellOrNone(e.Direction),
			cellOrNone(e.Status),
			cellOrNone(e.Name),
			connectionNotes(e),
		)
	}
	p.blank()
}

func sectionGroups(p *printer, m *report.Report) {
	if !m.Disclosure.Groups {
		return
	}
	p.line("## Groups")
	p.blank()
	if len(m.Groups) == 0 {
		p.line("No groups.")
		p.blank()
		return
	}
	p.line("| ID | Type | Name | Members | Children | Carries |")
	p.line("| --- | --- | --- | --: | --: | --- |")
	for _, e := range m.Groups {
		p.linef("| %s | %s | %s | %d | %d | %s |",
			code(e.ID),
			cellOrNone(e.Type),
			cellOrNone(e.Name),
			e.MemberCount, e.ChildCount,
			carries(e.HasProperties, e.HasExtensions, e.ExtraKeys),
		)
	}
	p.blank()
	for _, e := range m.Groups {
		if e.MemberCount == 0 && e.ChildCount == 0 {
			continue
		}
		p.linef("### %s", Text(groupHeading(e)))
		p.blank()
		if e.MemberCount > 0 {
			p.bullet("Members: " + joinCode(e.Members))
		}
		if e.ChildCount > 0 {
			p.bullet("Child groups: " + joinCode(e.Children))
		}
		p.blank()
	}
}

func sectionAnomalies(p *printer, m *report.Report) {
	p.line("## Structural anomalies")
	p.blank()
	if len(m.Anomalies) == 0 {
		p.line("None observed. The document's internal references are all consistent.")
		p.blank()
		return
	}
	p.line("Defensive notes about the document's own references. Not canonical")
	p.line("validation diagnostics and not graded.")
	p.blank()
	p.line("| Kind | Subject | Reference | Detail |")
	p.line("| --- | --- | --- | --- |")
	for _, a := range m.Anomalies {
		p.linef("| %s | %s | %s | %s |",
			code(a.Kind),
			code(a.Subject),
			code(a.Reference),
			cellOrNone(a.Detail),
		)
	}
	p.blank()
}

func sectionNotes(p *printer, m *report.Report) {
	if len(m.Truncations) == 0 {
		return
	}
	p.line("## Notes")
	p.blank()
	for _, t := range m.Truncations {
		p.bullet(fmt.Sprintf(
			"Section %s bounded: showing the first %d rows of %d in sort order.",
			Text(t.Section), t.Shown, t.Total,
		))
	}
	p.blank()
}

// --- small rendering helpers -----------------------------------------

// printer accumulates lines through a checked writer.
type printer struct {
	w *render.CheckedWriter
}

func (p *printer) line(s string)   { _, _ = p.w.WriteString(s + "\n") }
func (p *printer) blank()          { _, _ = p.w.WriteString("\n") }
func (p *printer) bullet(s string) { _, _ = p.w.WriteString("- " + s + "\n") }
func (p *printer) raw(s string)    { _, _ = p.w.WriteString(s) }
func (p *printer) linef(format string, a ...any) {
	_, _ = p.w.WriteString(fmt.Sprintf(format, a...) + "\n")
}

// code wraps an opaque identifier in a backtick span after neutralising
// any backtick in the value.
func code(s string) string {
	if s == "" {
		return none
	}
	return "`" + strings.ReplaceAll(sanitizeControls(s), "`", "'") + "`"
}

func cellOrNone(s string) string {
	if s == "" {
		return none
	}
	return TableCell(s)
}

func providerCell(name string) string {
	if name == "" {
		return noProvider
	}
	return TableCell(name)
}

func endpointCell(id string, missing bool) string {
	if missing {
		return code(id) + " (unresolved)"
	}
	return code(id)
}

func observedAbsent(present bool) string {
	if present {
		return "observed"
	}
	return "absent"
}

// carries turns the loss-aware flags into a short phrase so a reader
// knows a payload was preserved even though it is not shown inline.
func carries(hasProps, hasExt bool, extra []string) string {
	var parts []string
	if hasProps {
		parts = append(parts, "properties")
	}
	if hasExt {
		parts = append(parts, "extensions")
	}
	if n := len(extra); n == 1 {
		parts = append(parts, "1 unknown field")
	} else if n > 1 {
		parts = append(parts, strconv.Itoa(n)+" unknown fields")
	}
	if len(parts) == 0 {
		return none
	}
	return strings.Join(parts, ", ")
}

func connectionNotes(e report.ConnectionEntry) string {
	var parts []string
	if e.SourceMissing {
		parts = append(parts, "source unresolved")
	}
	if e.TargetMissing {
		parts = append(parts, "target unresolved")
	}
	if e.HasProperties {
		parts = append(parts, "properties")
	}
	if e.HasExtensions {
		parts = append(parts, "extensions")
	}
	if n := len(e.ExtraKeys); n > 0 {
		parts = append(parts, strconv.Itoa(n)+" unknown")
	}
	if len(parts) == 0 {
		return none
	}
	return Text(strings.Join(parts, ", "))
}

func groupHeading(e report.GroupEntry) string {
	if e.Name != "" {
		return e.Name + " (" + e.ID + ")"
	}
	return e.ID
}

// joinCell renders a string slice as a comma-separated escaped cell.
func joinCell(items []string) string {
	if len(items) == 0 {
		return none
	}
	escaped := make([]string, len(items))
	for i, it := range items {
		escaped[i] = TableCell(it)
	}
	return strings.Join(escaped, ", ")
}

// joinCode renders a string slice as comma-separated backtick spans.
func joinCode(items []string) string {
	if len(items) == 0 {
		return none
	}
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = code(it)
	}
	return strings.Join(parts, ", ")
}
