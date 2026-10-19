// This file is the generic, vendor-neutral PDF renderer for the same
// format-neutral report.Report model internal/render/markdown already
// renders: a document header, an evidence-state disclosure of which
// optional regions the input carried, exact counts, per-provider and
// per-type breakdowns, and full resource, connection, and group
// tables. It mirrors markdown/report.go's section list, order, and
// wording exactly, translated into pdf.Block values instead of
// Markdown text, so a reader who opens both artifacts for the same
// document sees the same content either way.
//
// This is the no-provider-recognized baseline only a vendor's own
// enriched PDF (when one exists) gets its own bespoke renderer, the
// same precedent already established for Markdown.
// This file never imports anything under internal/provider.
package pdf

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"go.osirisjson.org/consumers/internal/report"
)

// placeholder text for an absent optional value in a table cell
// identical wording to markdown/report.go's own, so the two formats
// never disagree about what "nothing here" means.
const (
	none       = "(none)"
	noProvider = "(no provider)"
)

// ReportRenderer renders a report.Report as PDF.
//
// The zero value is not usable; construct one with NewReportRenderer.
type ReportRenderer struct {
	model    *report.Report
	artifact time.Time
}

// NewReportRenderer returns a renderer for model. artifactTime is
// stamped into the cover page, the running footer, and the PDF Info
// dictionary; the zero time omits it everywhere, keeping output
// deterministic without a clock.
func NewReportRenderer(model *report.Report, artifactTime time.Time) *ReportRenderer {
	return &ReportRenderer{model: model, artifact: artifactTime.UTC()}
}

// Render writes the PDF to w. It checks ctx at section boundaries, the
// same discipline markdown.ReportRenderer.Render already follows, and
// returns the first error Write reports without wrapping it away.
func (r *ReportRenderer) Render(ctx context.Context, w io.Writer) error {
	m := r.model
	var blocks []Block
	for _, section := range []func(*report.Report) []Block{
		sectionDocumentPDF,
		sectionDisclosurePDF,
		sectionInventoryPDF,
		sectionProvidersPDF,
		sectionResourceTypesPDF,
		sectionConnectionTypesPDF,
		sectionResourcesPDF,
		sectionConnectionsPDF,
		sectionGroupsPDF,
		sectionAnomaliesPDF,
		sectionNotesPDF,
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		blocks = append(blocks, section(m)...)
	}

	return Write(w, Document{
		Title:        reportTitlePDF(m.Document.ScopeName),
		Subject:      "OSIRIS JSON vendor-neutral inventory brief",
		FontFamily:   FontMontserrat,
		CreationDate: r.artifact,
		FooterLeft:   footerLeftPDF(r.artifact),
		FooterRight:  footerGeneratorLinePDF(m.Document),
		Blocks:       blocks,
	})
}

// reportTitlePDF mirrors the per-vendor pdfReportTitle convention
// (e.g. internal/provider/hyperscalers/azure/manual_test.go): a fixed
// first row, and when the document discloses a scope name a
// second row carrying it, split by the embedded newline
// pdf.Document.Title's own two-row running-header/cover convention
// reads. Never built through markdown's own Text()/TableCell()
// escaping, since this package does its own destination-aware
// escaping (the same lesson already logged once for a vendor PDF
// title bleeding Markdown's backslash-escaping into the PDF).
func reportTitlePDF(scopeName string) string {
	const base = "OSIRIS JSON inventory brief"
	if scopeName == "" {
		return base
	}
	return base + " for\n" + scopeName
}

// footerLeftPDF is the generic OSIRIS JSON brand/copyright line not
// vendor-specific, so it is identical across every PDF this engine
// produces, generic or enriched.
func footerLeftPDF(artifact time.Time) string {
	return "(c) " + strconv.Itoa(artifact.Year()) + " OSIRIS JSON - https://osirisjson.org\n" +
		"Open Specification for Infrastructure Resource Interchange Schema."
}

// footerGeneratorLinePDF joins the document's own generator name and
// version, the only generator-identifying fields report.DocumentInfo
// carries (a vendor's own Signature additionally has a scope purpose
// field this generic model has no equivalent of).
func footerGeneratorLinePDF(d report.DocumentInfo) string {
	return strings.TrimSpace(d.GeneratorName + " " + d.GeneratorVersion)
}

func sectionDocumentPDF(m *report.Report) []Block {
	d := m.Document
	rows := [][]string{
		{"OSIRIS JSON version", cellOrNonePDF(d.OSIRISVersion)},
		{"Snapshot timestamp", cellOrNonePDF(d.Timestamp)},
	}
	if d.GeneratorName != "" || d.GeneratorVersion != "" {
		rows = append(rows, []string{"Generator", strings.TrimSpace(d.GeneratorName + " " + d.GeneratorVersion)})
	} else {
		rows = append(rows, []string{"Generator", none})
	}
	if d.ScopeName != "" {
		rows = append(rows, []string{"Scope", d.ScopeName})
	}
	if d.ScopeDescription != "" {
		rows = append(rows, []string{"Scope description", d.ScopeDescription})
	}
	rows = append(rows, []string{"Validation", fmt.Sprintf(
		"%s %s, profile %s, spec %s, %d errors, %d warnings",
		cellOrNonePDF(d.EngineName), cellOrNonePDF(d.EngineVersion),
		cellOrNonePDF(d.Profile), cellOrNonePDF(d.SpecificationVersion),
		d.ErrorCount, d.WarningCount,
	)})
	return []Block{
		Heading{Level: 2, Text: "Document", PageBreakBefore: true},
		Table{Rows: rows},
	}
}

func sectionDisclosurePDF(m *report.Report) []Block {
	d := m.Disclosure
	return []Block{
		Heading{Level: 2, Text: "Disclosure", PageBreakBefore: true},
		Paragraph{Text: "Whether the input carried each optional region. \"absent\" means the " +
			"document did not include it, not that it is empty or non-compliant."},
		Table{Rows: [][]string{
			{"Generator metadata", observedAbsentPDF(d.Generator)},
			{"Scope metadata", observedAbsentPDF(d.Scope)},
			{"Connections array", observedAbsentPDF(d.Connections)},
			{"Groups array", observedAbsentPDF(d.Groups)},
			{"Unknown top-level fields", observedAbsentPDF(d.UnknownTopLevelFields)},
		}},
	}
}

func sectionInventoryPDF(m *report.Report) []Block {
	c := m.Counts
	rows := [][]string{
		{"Resources", strconv.Itoa(c.Resources)},
		{"Connections", strconv.Itoa(c.Connections)},
		{"Edges", strconv.Itoa(c.Edges)},
		{"Groups", strconv.Itoa(c.Groups)},
		{"Providers", strconv.Itoa(c.Providers)},
		{"Resource types", strconv.Itoa(c.ResourceTypes)},
		{"Connection types", strconv.Itoa(c.ConnectionTypes)},
		{"Resources with no or unknown provider", strconv.Itoa(c.UnknownProviders)},
		{"Structural anomalies", strconv.Itoa(c.Anomalies)},
	}
	return []Block{
		Heading{Level: 2, Text: "Inventory", PageBreakBefore: true},
		Table{Columns: []string{"Metric", "Count"}, Align: []Align{AlignLeft, AlignRight}, Rows: rows},
	}
}

func sectionProvidersPDF(m *report.Report) []Block {
	blocks := []Block{Heading{Level: 2, Text: "Providers", PageBreakBefore: true}}
	if len(m.Providers) == 0 {
		return append(blocks, Paragraph{Text: "No resources."})
	}
	var rows [][]string
	for _, pv := range m.Providers {
		name := noProvider
		if pv.Name != "" {
			name = pv.Name
		}
		rows = append(rows, []string{name, strconv.Itoa(pv.ResourceCount), joinCellPDF(pv.ResourceTypes)})
	}
	return append(blocks, Table{
		Columns: []string{"Provider", "Resources", "Resource types"},
		Align:   []Align{AlignLeft, AlignRight, AlignLeft},
		Rows:    rows,
	})
}

func sectionResourceTypesPDF(m *report.Report) []Block {
	return append([]Block{Heading{Level: 2, Text: "Resource types", PageBreakBefore: true}},
		typeTableBlocksPDF(m.ResourceTypes, "No resources.")...)
}

func sectionConnectionTypesPDF(m *report.Report) []Block {
	if !m.Disclosure.Connections {
		return nil
	}
	return append([]Block{Heading{Level: 2, Text: "Connection types", PageBreakBefore: true}},
		typeTableBlocksPDF(m.ConnectionTypes, "No connections.")...)
}

func typeTableBlocksPDF(rows []report.TypeBreakdown, empty string) []Block {
	if len(rows) == 0 {
		return []Block{Paragraph{Text: empty}}
	}
	var out [][]string
	for _, row := range rows {
		t := none
		if row.Type != "" {
			t = row.Type
		}
		out = append(out, []string{t, strconv.Itoa(row.Count)})
	}
	return []Block{Table{Columns: []string{"Type", "Count"}, Align: []Align{AlignLeft, AlignRight}, Rows: out}}
}

func sectionResourcesPDF(m *report.Report) []Block {
	blocks := []Block{Heading{Level: 2, Text: "Resources", PageBreakBefore: true}}
	if len(m.Resources) == 0 {
		return append(blocks, Paragraph{Text: "No resources."})
	}
	var rows [][]string
	for _, e := range m.Resources {
		rows = append(rows, []string{
			e.ID,
			cellOrNonePDF(e.Type),
			cellOrNonePDF(e.Name),
			providerCellPDF(e.Provider),
			cellOrNonePDF(e.Region),
			cellOrNonePDF(e.Status),
			cellOrNonePDF(e.State),
			strconv.Itoa(e.OutDegree),
			strconv.Itoa(e.InDegree),
			joinCellPDF(e.GroupIDs),
			carriesPDF(e.HasProperties, e.HasExtensions, e.ExtraKeys),
		})
	}
	blocks = append(blocks, Table{
		Columns: []string{"ID", "Type", "Name", "Provider", "Region", "Status", "State", "Out", "In", "Groups", "Carries"},
		Align:   []Align{AlignLeft, AlignLeft, AlignLeft, AlignLeft, AlignLeft, AlignLeft, AlignLeft, AlignRight, AlignRight, AlignLeft, AlignLeft},
		Rows:    rows,
	})
	return append(blocks, nativeIDBlocksPDF(m.Resources)...)
}

// nativeIDBlocksPDF lists every resource that carries a provider
// native_id, verbatim, mirroring markdown/report.go's writeNativeIDs.
func nativeIDBlocksPDF(rows []report.ResourceEntry) []Block {
	var have [][]string
	for _, e := range rows {
		if e.NativeID != "" {
			have = append(have, []string{e.ID, e.NativeID})
		}
	}
	if len(have) == 0 {
		return nil
	}
	return []Block{
		Heading{Level: 2, Text: "Resource native identifiers"},
		Paragraph{Text: "Provider native_id values, carried verbatim and never parsed."},
		Table{Columns: []string{"ID", "native_id"}, Rows: have},
	}
}

func sectionConnectionsPDF(m *report.Report) []Block {
	if !m.Disclosure.Connections {
		return nil
	}
	blocks := []Block{Heading{Level: 2, Text: "Connections", PageBreakBefore: true}}
	if len(m.Connections) == 0 {
		return append(blocks, Paragraph{Text: "No connections."})
	}
	var rows [][]string
	for _, e := range m.Connections {
		rows = append(rows, []string{
			e.ID,
			cellOrNonePDF(e.Type),
			endpointCellPDF(e.Source, e.SourceMissing),
			endpointCellPDF(e.Target, e.TargetMissing),
			cellOrNonePDF(e.Direction),
			cellOrNonePDF(e.Status),
			cellOrNonePDF(e.Name),
			connectionNotesPDF(e),
		})
	}
	return append(blocks, Table{
		Columns: []string{"ID", "Type", "Source", "Target", "Direction", "Status", "Name", "Notes"},
		Rows:    rows,
	})
}

func sectionGroupsPDF(m *report.Report) []Block {
	if !m.Disclosure.Groups {
		return nil
	}
	blocks := []Block{Heading{Level: 2, Text: "Groups", PageBreakBefore: true}}
	if len(m.Groups) == 0 {
		return append(blocks, Paragraph{Text: "No groups."})
	}
	var rows [][]string
	for _, e := range m.Groups {
		rows = append(rows, []string{
			e.ID,
			cellOrNonePDF(e.Type),
			cellOrNonePDF(e.Name),
			strconv.Itoa(e.MemberCount),
			strconv.Itoa(e.ChildCount),
			carriesPDF(e.HasProperties, e.HasExtensions, e.ExtraKeys),
		})
	}
	blocks = append(blocks, Table{
		Columns: []string{"ID", "Type", "Name", "Members", "Children", "Carries"},
		Align:   []Align{AlignLeft, AlignLeft, AlignLeft, AlignRight, AlignRight, AlignLeft},
		Rows:    rows,
	})
	for _, e := range m.Groups {
		if e.MemberCount == 0 && e.ChildCount == 0 {
			continue
		}
		blocks = append(blocks, Heading{Level: 2, Text: groupHeadingPDF(e)})
		if e.MemberCount > 0 {
			blocks = append(blocks, Paragraph{Text: "Members: " + strings.Join(e.Members, ", ")})
		}
		if e.ChildCount > 0 {
			blocks = append(blocks, Paragraph{Text: "Child groups: " + strings.Join(e.Children, ", ")})
		}
	}
	return blocks
}

func sectionAnomaliesPDF(m *report.Report) []Block {
	blocks := []Block{Heading{Level: 2, Text: "Structural anomalies", PageBreakBefore: true}}
	if len(m.Anomalies) == 0 {
		return append(blocks, Paragraph{Text: "None observed. The document's internal references are all consistent."})
	}
	blocks = append(blocks, Paragraph{Text: "Defensive notes about the document's own references. Not canonical " +
		"validation diagnostics and not graded."})
	var rows [][]string
	for _, a := range m.Anomalies {
		rows = append(rows, []string{a.Kind, a.Subject, a.Reference, cellOrNonePDF(a.Detail)})
	}
	return append(blocks, Table{Columns: []string{"Kind", "Subject", "Reference", "Detail"}, Rows: rows})
}

func sectionNotesPDF(m *report.Report) []Block {
	if len(m.Truncations) == 0 {
		return nil
	}
	blocks := []Block{Heading{Level: 2, Text: "Notes", PageBreakBefore: true}}
	for _, t := range m.Truncations {
		blocks = append(blocks, Paragraph{Text: fmt.Sprintf(
			"Section %s bounded: showing the first %d rows of %d in sort order.",
			t.Section, t.Shown, t.Total,
		)})
	}
	return blocks
}

// small rendering helpers, mirroring markdown/report.go's own without
// any Markdown-specific escaping (this package's Write does its own
// destination-aware escaping once, at the writer boundary).

func cellOrNonePDF(s string) string {
	if s == "" {
		return none
	}
	return s
}

func providerCellPDF(name string) string {
	if name == "" {
		return noProvider
	}
	return name
}

func endpointCellPDF(id string, missing bool) string {
	if missing {
		return id + " (unresolved)"
	}
	return id
}

func observedAbsentPDF(present bool) string {
	if present {
		return "observed"
	}
	return "absent"
}

func carriesPDF(hasProps, hasExt bool, extra []string) string {
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

func connectionNotesPDF(e report.ConnectionEntry) string {
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
	return strings.Join(parts, ", ")
}

func groupHeadingPDF(e report.GroupEntry) string {
	if e.Name != "" {
		return e.Name + " (" + e.ID + ")"
	}
	return e.ID
}

func joinCellPDF(items []string) string {
	if len(items) == 0 {
		return none
	}
	return strings.Join(items, ", ")
}
