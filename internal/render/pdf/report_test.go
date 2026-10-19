// Tests for the generic report.Report PDF renderer: determinism, the
// injected artifact time, cancellation, and write-error propagation
// the same shape as internal/render/markdown/report_test.go, adapted
// for a binary, Flate-compressed, CID-hex-encoded (FontMontserrat)
// output: a drawn glyph's own bytes in a content stream are not a
// readable ASCII substring the way a Markdown line is, so fine-grained
// prose assertions are not practical here as plain Go string checks.
// The real content/prose verification for this renderer is done with
// poppler (pdftotext/pdfinfo) and Ghostscript at review time, overall
// structural validity, and the pure-Go determinism/error-propagation
// behaviors.

package pdf

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/report"
	"go.osirisjson.org/consumers/internal/validation"
)

func reportModelFor(t *testing.T, rel string, opts report.Options) *report.Report {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	doc, err := document.Parse(b)
	if err != nil {
		t.Fatalf("parsing %s: %v", rel, err)
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing %s: %v", rel, err)
	}
	r, err := report.Build(context.Background(), doc, g, validation.Receipt{
		EngineName: validation.PinnedEngineName, EngineVersion: validation.PinnedEngineVersion,
		Profile: validation.DefaultProfile, SpecificationVersion: validation.PinnedSpecVersion,
		InputSHA256: doc.SHA256(),
	}, opts)
	if err != nil {
		t.Fatalf("report.Build %s: %v", rel, err)
	}
	return r
}

func renderReportPDF(t *testing.T, m *report.Report, at time.Time) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := NewReportRenderer(m, at).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.Bytes()
}

var reportFixedTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func TestReportRendererIsByteIdentical(t *testing.T) {
	m := reportModelFor(t, "valid/mixed-provider.json", report.Options{})
	a := renderReportPDF(t, m, reportFixedTime)
	b := renderReportPDF(t, m, reportFixedTime)
	if !bytes.Equal(a, b) {
		t.Fatalf("two renders of the same model differ")
	}
	if !bytes.HasPrefix(a, []byte("%PDF-1.7")) {
		t.Errorf("missing PDF header")
	}
	if !bytes.Contains(a, []byte("OSIRIS JSON inventory brief")) {
		t.Errorf("title not found in the Info dictionary")
	}
}

func TestReportRendererZeroTimeOmitsCreationDate(t *testing.T) {
	m := reportModelFor(t, "valid/mixed-provider.json", report.Options{})
	out := renderReportPDF(t, m, time.Time{})
	if bytes.Contains(out, []byte("/CreationDate")) {
		t.Errorf("zero time must not stamp a /CreationDate")
	}

	withTime := renderReportPDF(t, m, reportFixedTime)
	if !bytes.Contains(withTime, []byte("/CreationDate")) {
		t.Errorf("non-zero artifact time must stamp a /CreationDate")
	}
}

func TestReportRendererScopeNameInTitle(t *testing.T) {
	m := reportModelFor(t, "valid/mixed-provider.json", report.Options{})
	out := renderReportPDF(t, m, reportFixedTime)
	if m.Document.ScopeName == "" {
		t.Fatalf("fixture has no scope name; this test needs one to be meaningful")
	}
	if !bytes.Contains(out, []byte(m.Document.ScopeName)) {
		t.Errorf("document's own scope name %q not found in the rendered PDF", m.Document.ScopeName)
	}
}

func TestReportRendererStructurallyValid(t *testing.T) {
	m := reportModelFor(t, "valid/mixed-provider.json", report.Options{})
	out := renderReportPDF(t, m, reportFixedTime)
	if !bytes.HasSuffix(bytes.TrimRight(out, "\n"), []byte("%%EOF")) {
		t.Errorf("missing %%%%EOF trailer")
	}
	streams := contentStreams(t, out)
	if len(streams) == 0 {
		t.Fatalf("no page content streams found")
	}
}

func TestReportRendererCancellation(t *testing.T) {
	m := reportModelFor(t, "valid/mixed-provider.json", report.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := NewReportRenderer(m, reportFixedTime).Render(ctx, &buf); err == nil {
		t.Fatal("Render returned nil on a cancelled context")
	}
}

func TestReportRendererPropagatesWriteError(t *testing.T) {
	m := reportModelFor(t, "valid/mixed-provider.json", report.Options{})
	err := NewReportRenderer(m, reportFixedTime).Render(context.Background(), reportFailWriter{})
	if err == nil {
		t.Fatal("Render swallowed a write error")
	}
}

type reportFailWriter struct{}

func (reportFailWriter) Write([]byte) (int, error) { return 0, errReportBoom }

var errReportBoom = &reportBoomError{}

type reportBoomError struct{}

func (*reportBoomError) Error() string { return "boom" }
