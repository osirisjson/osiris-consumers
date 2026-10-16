// Tests for the minimal .xlsx writer: the produced file is a valid
// zip archive, every XML part is well-formed, every cell value shows
// up somewhere in its sheet's part, and malformed input is rejected
// rather than silently truncated or corrupted.

package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, sheets []Sheet) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, sheets); err != nil {
		t.Fatalf("Write: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("resulting bytes are not a valid zip archive: %v", err)
	}
	return zr
}

func part(t *testing.T, zr *zip.Reader, name string) string {
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
	t.Fatalf("zip archive has no part %q", name)
	return ""
}

func TestWriteProducesEveryExpectedPart(t *testing.T) {
	zr := mustWrite(t, []Sheet{
		{Name: "Faults", Header: []string{"Node", "Severity"}, Rows: [][]string{{"leaf-1", "critical"}}},
		{Name: "Tenants", Header: []string{"Name"}, Rows: [][]string{{"common"}}},
	})
	want := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"xl/workbook.xml",
		"xl/_rels/workbook.xml.rels",
		"xl/styles.xml",
		"xl/worksheets/sheet1.xml",
		"xl/worksheets/sheet2.xml",
	}
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing zip part %q", w)
		}
	}
}

func TestEveryXMLPartIsWellFormed(t *testing.T) {
	zr := mustWrite(t, []Sheet{
		{Name: "Faults", Header: []string{"Node", "Severity", "Description"}, Rows: [][]string{
			{"leaf-1", "critical", "Port is down, reason <linkNotConnected> & unstable"},
			{"leaf-2", "warning", ""},
		}},
	})
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", f.Name, err)
		}
		dec := xml.NewDecoder(rc)
		var tokenErr error
		for {
			if _, tokenErr = dec.Token(); tokenErr != nil {
				break
			}
		}
		rc.Close()
		// A well-formed document's token stream ends with io.EOF; any
		// other terminal error means malformed XML.
		if tokenErr != io.EOF {
			t.Errorf("%s is not well-formed XML: %v", f.Name, tokenErr)
		}
	}
}

func TestCellValuesAppearInTheirSheet(t *testing.T) {
	zr := mustWrite(t, []Sheet{
		{Name: "Ports", Header: []string{"Node", "Port"}, Rows: [][]string{
			{"leaf-10", "eth1/1"},
			{"leaf-11", "eth1/2"},
		}},
	})
	sheet1 := part(t, zr, "xl/worksheets/sheet1.xml")
	for _, want := range []string{"Node", "Port", "leaf-10", "eth1/1", "leaf-11", "eth1/2"} {
		if !strings.Contains(sheet1, want) {
			t.Errorf("sheet1.xml does not contain %q", want)
		}
	}
}

func TestWriteEscapesXMLMetacharacters(t *testing.T) {
	zr := mustWrite(t, []Sheet{
		{Name: "Faults", Header: []string{"Description"}, Rows: [][]string{
			{`<script>alert("&")</script>`},
		}},
	})
	sheet1 := part(t, zr, "xl/worksheets/sheet1.xml")
	if strings.Contains(sheet1, "<script>") {
		t.Error("raw <script> tag leaked into the worksheet XML unescaped")
	}
	if !strings.Contains(sheet1, "&lt;script&gt;") {
		t.Error("expected the description to be entity-escaped")
	}
}

func TestWriteRejectsRowLongerThanHeader(t *testing.T) {
	err := Write(&bytes.Buffer{}, []Sheet{
		{Name: "Bad", Header: []string{"A"}, Rows: [][]string{{"1", "2"}}},
	})
	if err == nil {
		t.Fatal("Write returned nil error for a row longer than its header")
	}
}

func TestColumnLetter(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "A"}, {2, "B"}, {26, "Z"}, {27, "AA"}, {28, "AB"}, {52, "AZ"}, {53, "BA"},
	}
	for _, tc := range cases {
		if got := columnLetter(tc.n); got != tc.want {
			t.Errorf("columnLetter(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestSheetNameSanitizesReservedCharactersAndLength(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Tenants", "Tenants"},
		{"Externally discovered neighbours - full list here", "Externally discovered neighbour"},
		{"a:b\\c/d?e*f[g]h", "a b c d e f g h"},
		{"", "Sheet1"},
	}
	for _, tc := range cases {
		got := sheetName(tc.name, 0)
		if got != tc.want {
			t.Errorf("sheetName(%q) = %q, want %q", tc.name, got, tc.want)
		}
		if len(got) > 31 {
			t.Errorf("sheetName(%q) = %q, longer than 31 characters", tc.name, got)
		}
	}
}

func TestWriteHandlesShortRowsAndEmptyCells(t *testing.T) {
	zr := mustWrite(t, []Sheet{
		{Name: "Sparse", Header: []string{"A", "B", "C"}, Rows: [][]string{
			{"x"},
			{"", "y", ""},
		}},
	})
	// Just must not error and must produce a readable sheet part.
	_ = part(t, zr, "xl/worksheets/sheet1.xml")
}
