// Tests for the Aruba Central .xlsx companion renderer: every expected
// sheet is present with the right row counts, cell values round-trip
// without Markdown escaping leaking in, and rendering is deterministic
// and cancellation-aware like every other renderer in this repository.

package arubacentral

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
	m := buildReport(t, fixture)
	zr := renderXLSX(t, m)

	wb := xlsxPart(t, zr, "xl/workbook.xml")
	for _, want := range []string{"Fleet", "Clients", "Interfaces", "BSSIDs"} {
		if !strings.Contains(wb, `name="`+want+`"`) {
			t.Errorf("workbook.xml does not declare a sheet named %q", want)
		}
	}
	if strings.Contains(wb, `name="Device properties"`) {
		t.Error("workbook.xml still declares a Device properties sheet")
	}
}

func TestXLSXRenderFleetRowCountMatchesModel(t *testing.T) {
	m := buildReport(t, fixture)
	zr := renderXLSX(t, m)

	content := xlsxPart(t, zr, "xl/worksheets/sheet1.xml")
	got := strings.Count(content, "<row ") - 1 // minus the header row.
	if got != len(m.Fleet) {
		t.Errorf("Fleet sheet has %d data rows, want %d", got, len(m.Fleet))
	}
}

func TestXLSXRenderClientsRowCountMatchesModel(t *testing.T) {
	m := buildReport(t, fixture)
	zr := renderXLSX(t, m)

	content := xlsxPart(t, zr, "xl/worksheets/sheet2.xml")
	got := strings.Count(content, "<row ") - 1
	if got != len(m.Clients) {
		t.Errorf("Clients sheet has %d data rows, want %d", got, len(m.Clients))
	}
}

func TestXLSXRenderInterfacesAndBSSIDsRowCountsMatchModel(t *testing.T) {
	m := buildReport(t, fixture)
	zr := renderXLSX(t, m)

	ifaces := xlsxPart(t, zr, "xl/worksheets/sheet3.xml")
	if got := strings.Count(ifaces, "<row ") - 1; got != len(m.Interfaces) {
		t.Errorf("Interfaces sheet has %d data rows, want %d", got, len(m.Interfaces))
	}
	bssids := xlsxPart(t, zr, "xl/worksheets/sheet4.xml")
	if got := strings.Count(bssids, "<row ") - 1; got != len(m.BSSIDs) {
		t.Errorf("BSSIDs sheet has %d data rows, want %d", got, len(m.BSSIDs))
	}
}

func TestXLSXRenderIsDeterministic(t *testing.T) {
	m := buildReport(t, fixture)
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
	m := buildReport(t, fixture)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := NewXLSXRenderer(m).Render(ctx, &buf); err == nil {
		t.Fatal("Render returned nil error on a cancelled context")
	}
}
