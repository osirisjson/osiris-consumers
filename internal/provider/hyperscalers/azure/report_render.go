// This file is the deterministic Markdown renderer for the Azure
// report model: a Signature table, the management-group/subscription
// hierarchy, a resource-group inventory, one table per confirmed
// resource category, a generic fallback table for anything
// unrecognized, and an explicit note on what a snapshot document
// cannot show. Every dynamic string is escaped through
// internal/render/markdown's destination-aware helpers; the only
// variable input is the injected artifact time.
package azure

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/markdown"
)

// azureReportTOC is every H2 heading this renderer writes, in emission
// order kept in sync with Render's own section list by hand (see
// markdown.TableOfContents' doc comment). Signature has no heading of
// its own (it goes straight under the title, matching every other
// vendor report) and so is not listed here.
var azureReportTOC = []string{
	"Hierarchy",
	"Resource groups",
	"Virtual Machines",
	"Storage Accounts",
	"Disks",
	"Virtual Networks",
	"Subnets",
	"Network Security Groups",
	"Public IP Addresses",
	"Load Balancers",
	"Network Interfaces",
	"Bastion Hosts",
	"SQL Virtual Machines",
	"Other resources",
	"Out of scope for this report",
}

// reportTitle appends the document's own scope name to the report
// title when present; falls back to the plain title otherwise, rather
// than fabricating a name.
func reportTitle(scopeName string) string {
	const base = "OSIRIS JSON Microsoft Azure report"
	if scopeName == "" {
		return base
	}
	return base + " for " + markdown.Text(scopeName)
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
// true; pass "" when the caller has no companion file name yet.
func NewReportRenderer(model ReportModel, artifactTime time.Time, xlsxFileName string) *ReportRenderer {
	return &ReportRenderer{model: model, artifact: artifactTime.UTC(), xlsxFileName: xlsxFileName}
}

// Render writes the Markdown report to w.
func (r *ReportRenderer) Render(ctx context.Context, w io.Writer) error {
	cw := render.NewCheckedWriter(w)
	p := &azureMDPrinter{w: cw, xlsxFileName: r.xlsxFileName}

	p.linef("# %s", reportTitle(r.model.Signature.ScopeName))
	p.blank()

	if err := ctx.Err(); err != nil {
		return err
	}
	sectionSignature(p, r.model, r.artifact)

	p.raw(markdown.TableOfContentsSection(azureReportTOC))

	sections := []func(*azureMDPrinter, ReportModel){
		sectionHierarchy,
		sectionResourceGroups,
	}
	for _, c := range categoryOrder {
		label := c.label
		sections = append(sections, func(p *azureMDPrinter, m ReportModel) {
			sectionCategory(p, m, label)
		})
	}
	sections = append(sections, sectionOther, sectionScopeNote)

	for _, section := range sections {
		if err := ctx.Err(); err != nil {
			return err
		}
		section(p, r.model)
	}

	return cw.Err()
}

// sectionSignature is the document's own provenance, as a table: the
// exact input hash and every "metadata" field the document carries. No
// heading (matches every other vendor report): it goes straight under
// the title.
func sectionSignature(p *azureMDPrinter, m ReportModel, artifact time.Time) {
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
	p.linef("| Scope accounts | %s |", joinOrAbsentCell(s.ScopeAccounts))
	p.linef("| Scope regions | %s |", joinOrAbsentCell(s.ScopeRegions))
	p.linef("| Scope subscriptions | %s |", joinOrAbsentCell(s.ScopeSubscriptions))
	extra := "(none)"
	if len(s.ExtraMetadataKeys) > 0 {
		extra = joinOrAbsentCell(s.ExtraMetadataKeys)
	}
	p.linef("| Additional metadata fields | %s |", extra)
	if len(m.Regions) > 0 {
		p.linef("| Regions observed among this document's resources | %s |", joinOrAbsentCell(m.Regions))
	}
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
// companion for what, or "" when m does not need one.
func xlsxInviteNote(p *azureMDPrinter, m ReportModel, what string) string {
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

func sectionHierarchy(p *azureMDPrinter, m ReportModel) {
	p.line("## Hierarchy")
	p.blank()
	if len(m.Hierarchy) == 0 {
		p.line("No management group or subscription resource in this document.")
		p.blank()
		return
	}
	p.line("| Kind | Name | Parent |")
	p.line("| --- | --- | --- |")
	for _, h := range m.Hierarchy {
		p.linef("| %s | %s | %s |", markdown.TableCell(h.Kind), markdown.TableCell(h.Name), markdown.TableCell(h.Parent))
	}
	p.blank()
}

func sectionResourceGroups(p *azureMDPrinter, m ReportModel) {
	p.line("## Resource groups")
	p.blank()
	if len(m.ResourceGroups) == 0 {
		p.line("No resource group in this document.")
		p.blank()
		return
	}
	if note := xlsxInviteNote(p, m, "resource group inventory"); note != "" && len(m.ResourceGroups) > azureXLSXRowThreshold {
		p.line(note)
		p.blank()
	}

	tagMaps := make([]map[string]string, len(m.ResourceGroups))
	for i, rg := range m.ResourceGroups {
		tagMaps[i] = rg.Tags
	}
	tagKeys := distinctTagKeys(tagMaps)

	header := "| Name | Region | Status | State | Resources |"
	divider := "| --- | --- | --- | --- | --: |"
	for _, k := range tagKeys {
		header += " " + markdown.TableCell(k) + " |"
		divider += " --- |"
	}
	p.line(header)
	p.line(divider)
	for _, rg := range m.ResourceGroups {
		row := fmt.Sprintf("| %s | %s | %s | %s | %d |",
			markdown.TableCell(rg.Name), markdown.TableCell(rg.Region), markdown.TableCell(rg.Status),
			markdown.TableCell(rg.State), rg.ResourceCount)
		for _, k := range tagKeys {
			row += " " + markdown.TableCell(tagValueOrAbsent(rg.Tags, k)) + " |"
		}
		p.line(row)
	}
	p.blank()
}

// sectionCategory writes one confirmed resource category's table,
// looked up from m.Categories by label. Above azureXLSXRowThreshold
// rows, the table moves to the .xlsx companion entirely and only the
// count stays inline same compacting rule as every other vendor
// report.
func sectionCategory(p *azureMDPrinter, m ReportModel, label string) {
	p.linef("## %s", label)
	p.blank()
	var rows []ResourceRow
	for _, c := range m.Categories {
		if c.Label == label {
			rows = c.Rows
		}
	}
	writeResourceTable(p, m, label, rows)
}

func sectionOther(p *azureMDPrinter, m ReportModel) {
	p.line("## Other resources")
	p.blank()
	p.line("Every resource whose type is not one of the categories above present in full, per " +
		"\"generic first, provider optional\": an unrecognized type is never dropped from this report.")
	p.blank()
	writeResourceTable(p, m, otherCategoryLabel, m.Other.Rows)
}

func writeResourceTable(p *azureMDPrinter, m ReportModel, what string, rows []ResourceRow) {
	if len(rows) == 0 {
		p.line("None in this document.")
		p.blank()
		return
	}
	if len(rows) > azureXLSXRowThreshold {
		if note := xlsxInviteNote(p, m, strings.ToLower(what)); note != "" {
			p.line(note)
			p.blank()
		}
		p.linef("%d resource(s) table moved to the accompanying spreadsheet above this row count.", len(rows))
		p.blank()
		return
	}
	tagMaps := make([]map[string]string, len(rows))
	for i, r := range rows {
		tagMaps[i] = r.Tags
	}
	tagKeys := distinctTagKeys(tagMaps)

	header := "| Name | Resource Group | Region | ARM Type | Status | State |"
	divider := "| --- | --- | --- | --- | --- | --- |"
	for _, k := range tagKeys {
		header += " " + markdown.TableCell(k) + " |"
		divider += " --- |"
	}
	p.line(header)
	p.line(divider)
	for _, r := range rows {
		row := fmt.Sprintf("| %s | %s | %s | %s | %s | %s |",
			markdown.TableCell(r.Name), markdown.TableCell(r.ResourceGroup), markdown.TableCell(r.Region),
			markdown.TableCell(r.ARMType), markdown.TableCell(r.Status), markdown.TableCell(r.State))
		for _, k := range tagKeys {
			row += " " + markdown.TableCell(tagValueOrAbsent(r.Tags, k)) + " |"
		}
		p.line(row)
	}
	p.blank()
}

// sectionScopeNote explains what a single OSIRIS JSON snapshot cannot
// show and points at the producer's own documentation page for future
// coverage the URL comes from the document's own
// metadata.generator.url, never hardcoded.
func sectionScopeNote(p *azureMDPrinter, m ReportModel) {
	p.line("## Out of scope for this report")
	p.blank()
	p.line("This report reflects a point-in-time OSIRIS JSON snapshot: resource inventory and " +
		"assertions the document itself carries, not a live Azure Monitor metrics, cost data, policy " +
		"or RBAC assignments, activity logs, or a Well-Architected assessment. A resource type this " +
		"producer version does not yet emit properties for shows \"(absent)\" fields above rather than " +
		"a guess or invented value.")
	if url := m.Signature.GeneratorURL; url != "" {
		p.blank()
		p.line("Coverage and report capabilities may grow in a future release of the Microsoft Azure producer, see " +
			markdown.Link(url, url) + " for what it currently emits.")
	}
	p.blank()
}

// printer

type azureMDPrinter struct {
	w            *render.CheckedWriter
	xlsxFileName string
}

func (p *azureMDPrinter) line(s string) { _, _ = p.w.WriteString(s + "\n") }
func (p *azureMDPrinter) blank()        { _, _ = p.w.WriteString("\n") }
func (p *azureMDPrinter) raw(s string)  { _, _ = p.w.WriteString(s) }
func (p *azureMDPrinter) linef(format string, a ...any) {
	_, _ = p.w.WriteString(fmt.Sprintf(format, a...) + "\n")
}
