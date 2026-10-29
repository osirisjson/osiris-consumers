package azure

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"
)

func TestXLSXRendersOneSheetPerCategoryPlusResourceGroups(t *testing.T) {
	doc, g := loadGraph(t, azureFixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())

	var buf bytes.Buffer
	if err := NewXLSXRenderer(m).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("output is not a valid zip archive: %v", err)
	}

	sheetFiles := 0
	for _, f := range zr.File {
		if len(f.Name) > len("xl/worksheets/sheet") && f.Name[:len("xl/worksheets/sheet")] == "xl/worksheets/sheet" {
			sheetFiles++
		}
	}
	// 1 resource-groups sheet + 11 confirmed categories + 1 "Other
	// resources" sheet.
	want := 1 + len(categoryOrder) + 1
	if sheetFiles != want {
		t.Errorf("workbook has %d worksheet parts, want %d", sheetFiles, want)
	}
}

func TestXLSXSheetBuildersMapRowsCorrectly(t *testing.T) {
	rg := resourceGroupsSheet([]ResourceGroupRow{{Name: "rg1", Region: "westeurope", ResourceCount: 5}})
	if rg.Name != "Resource groups" || len(rg.Rows) != 1 || rg.Rows[0][0] != "rg1" || rg.Rows[0][4] != "5" {
		t.Errorf("resourceGroupsSheet = %+v", rg)
	}

	res := resourceSheet("Virtual Machines", []ResourceRow{{Name: "vm1", ResourceGroup: "rg1", Region: "westeurope"}})
	if res.Name != "Virtual Machines" || len(res.Rows) != 1 || res.Rows[0][0] != "vm1" || res.Rows[0][1] != "rg1" {
		t.Errorf("resourceSheet = %+v", res)
	}
}

func TestXLSXCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := NewXLSXRenderer(ReportModel{}).Render(ctx, &buf); err == nil {
		t.Fatal("expected cancellation to propagate")
	}
}
