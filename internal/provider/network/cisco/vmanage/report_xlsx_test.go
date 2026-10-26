package vmanage

import (
	"bytes"
	"context"
	"testing"

	"go.osirisjson.org/consumers/internal/document"
)

func TestXLSXRendererOnRealFixture(t *testing.T) {
	g := fixture(t)
	m := BuildReportModel(g, document.Metadata{}, "deadbeef")

	var buf bytes.Buffer
	r := NewXLSXRenderer(m)
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("Render produced no bytes")
	}
	// A minimal signature check: a .xlsx file is a zip archive.
	if !bytes.HasPrefix(buf.Bytes(), []byte("PK")) {
		t.Error("output does not look like a zip/.xlsx archive")
	}
}

func TestBuildXLSXSheetsCoversEverySheet(t *testing.T) {
	g := fixture(t)
	m := BuildReportModel(g, document.Metadata{}, "deadbeef")
	sheets := buildXLSXSheets(m)
	want := []string{"Routers", "Ports"}
	if len(sheets) != len(want) {
		t.Fatalf("len(sheets) = %d, want %d", len(sheets), len(want))
	}
	for i, name := range want {
		if sheets[i].Name != name {
			t.Errorf("sheets[%d].Name = %q, want %q", i, sheets[i].Name, name)
		}
	}
}
