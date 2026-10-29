// This file is the .xlsx companion renderer for the Azure report
// model: one sheet for resource groups, one per confirmed resource
// category and one for the "Other resources" fallback the same
// whole-workbook-trigger pattern every other provider report uses
// (Cisco APIC, HPE Aruba Central): the moment any one tracked category
// crosses azureXLSXRowThreshold rows, every category gets a full sheet
// here regardless of its own size, so the companion file is always
// consistent rather than a patchwork of which category happened to be
// large in this particular document.
//
// No third-party dependency: internal/render/xlsx is a minimal,
// standard-library-only .xlsx writer with no styling or autofilter.
package azure

import (
	"context"
	"io"
	"strconv"

	"go.osirisjson.org/consumers/internal/render/xlsx"
)

// XLSXRenderer renders a ReportModel's resource-group and resource
// categories as a multi-sheet .xlsx workbook.
//
// The zero value is not usable; construct one with NewXLSXRenderer.
type XLSXRenderer struct {
	model ReportModel
}

// NewXLSXRenderer returns a renderer for model.
func NewXLSXRenderer(model ReportModel) *XLSXRenderer {
	return &XLSXRenderer{model: model}
}

// Render writes the .xlsx workbook to w.
func (r *XLSXRenderer) Render(ctx context.Context, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return xlsx.Write(w, buildXLSXSheets(r.model))
}

func buildXLSXSheets(m ReportModel) []xlsx.Sheet {
	sheets := []xlsx.Sheet{resourceGroupsSheet(m.ResourceGroups)}
	for _, c := range m.Categories {
		sheets = append(sheets, resourceSheet(c.Label, c.Rows))
	}
	sheets = append(sheets, resourceSheet(otherCategoryLabel, m.Other.Rows))
	return sheets
}

// resourceGroupsSheet pivots rows' own Tags maps into one column per
// distinct key seen across them (report_render.go's Markdown table
// does the same pivot; see distinctTagKeys).
func resourceGroupsSheet(rows []ResourceGroupRow) xlsx.Sheet {
	tagMaps := make([]map[string]string, len(rows))
	for i, rg := range rows {
		tagMaps[i] = rg.Tags
	}
	tagKeys := distinctTagKeys(tagMaps)

	s := xlsx.Sheet{
		Name:   "Resource groups",
		Header: append([]string{"Name", "Region", "Status", "State", "Resources"}, tagKeys...),
	}
	for _, rg := range rows {
		row := []string{rg.Name, rg.Region, rg.Status, rg.State, strconv.Itoa(rg.ResourceCount)}
		for _, k := range tagKeys {
			row = append(row, tagValueOrAbsent(rg.Tags, k))
		}
		s.Rows = append(s.Rows, row)
	}
	return s
}

// resourceSheet pivots rows' own Tags maps the same way
// resourceGroupsSheet does.
func resourceSheet(label string, rows []ResourceRow) xlsx.Sheet {
	tagMaps := make([]map[string]string, len(rows))
	for i, r := range rows {
		tagMaps[i] = r.Tags
	}
	tagKeys := distinctTagKeys(tagMaps)

	s := xlsx.Sheet{
		Name:   label,
		Header: append([]string{"Name", "Resource Group", "Region", "ARM Type", "Status", "State"}, tagKeys...),
	}
	for _, r := range rows {
		row := []string{r.Name, r.ResourceGroup, r.Region, r.ARMType, r.Status, r.State}
		for _, k := range tagKeys {
			row = append(row, tagValueOrAbsent(r.Tags, k))
		}
		s.Rows = append(s.Rows, row)
	}
	return s
}
