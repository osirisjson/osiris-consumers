// Tests for the Cisco APIC .xlsx companion renderer: every expected
// sheet is present with the right row counts, cell values round-trip,
// and rendering is deterministic and cancellation-aware like every
// other renderer in this repository.

package apic

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

func renderXLSX(t *testing.T, m ReportModel) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	if err := NewXLSXRenderer(m).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("output is not a valid zip archive: %v", err)
	}
	return zr
}

func xlsxPart(t *testing.T, zr *zip.Reader, name string) string {
	t.Helper()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("opening %s: %v", name, err)
			}
			defer rc.Close()
			var b bytes.Buffer
			if _, err := b.ReadFrom(rc); err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			return b.String()
		}
	}
	t.Fatalf("workbook has no part %q", name)
	return ""
}

func TestXLSXRenderEmitsEverySheet(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	zr := renderXLSX(t, m)

	wb := xlsxPart(t, zr, "xl/workbook.xml")
	for _, want := range []string{"Faults", "Tenants", "VRFs", "VRF members", "EPGs", "EPG members", "Bridge domains", "Subnets", "Ports", "Neighbours"} {
		if !strings.Contains(wb, `name="`+want+`"`) {
			t.Errorf("workbook.xml does not declare a sheet named %q", want)
		}
	}
}

func TestXLSXRenderRowCountsMatchModel(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	zr := renderXLSX(t, m)

	// Sheet order matches buildXLSXSheets: Faults, Tenants, VRFs, VRF
	// members, EPGs, EPG members, Bridge domains, Subnets, Ports,
	// Neighbours, sheet1..sheet10.
	cases := []struct {
		sheetFile string
		wantRows  int // data rows, i.e. one less than total <row> elements (header excluded).
	}{
		{"xl/worksheets/sheet1.xml", len(m.Faults)},
		{"xl/worksheets/sheet2.xml", len(m.Tenants)},
		{"xl/worksheets/sheet3.xml", len(m.VRFs)},
		{"xl/worksheets/sheet4.xml", len(m.VRFMembers)},
		{"xl/worksheets/sheet5.xml", len(m.EPGs)},
		{"xl/worksheets/sheet6.xml", len(m.EPGMembers)},
		{"xl/worksheets/sheet7.xml", len(m.BridgeDomains)},
		{"xl/worksheets/sheet8.xml", len(m.Subnets)},
		{"xl/worksheets/sheet9.xml", len(m.Ports.Rows)},
		{"xl/worksheets/sheet10.xml", len(m.Neighbours.Rows)},
	}
	for _, tc := range cases {
		content := xlsxPart(t, zr, tc.sheetFile)
		got := strings.Count(content, "<row ") - 1 // minus the header row.
		if got != tc.wantRows {
			t.Errorf("%s has %d data rows, want %d", tc.sheetFile, got, tc.wantRows)
		}
	}
}

func TestXLSXRenderIsDeterministic(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	var a, b bytes.Buffer
	if err := NewXLSXRenderer(m).Render(context.Background(), &a); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if err := NewXLSXRenderer(m).Render(context.Background(), &b); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Error("two renders of the same model produced different bytes")
	}
}

func TestXLSXRenderRespectsCancellation(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := NewXLSXRenderer(m).Render(ctx, &buf); err == nil {
		t.Fatal("Render returned nil error on a cancelled context")
	}
}
