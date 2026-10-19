// Tests for the Markdown report renderer: determinism, the injected
// artifact time, structural framing, and injection safety over
// adversarial document strings.

package markdown

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/report"
	"go.osirisjson.org/consumers/internal/validation"
)

func model(t *testing.T, rel string, opts report.Options) *report.Report {
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

func renderMD(t *testing.T, m *report.Report, at time.Time) string {
	t.Helper()
	var buf bytes.Buffer
	if err := NewReportRenderer(m, at).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

var fixedTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func TestRenderIsByteIdentical(t *testing.T) {
	m := model(t, "valid/mixed-provider.json", report.Options{})
	a := renderMD(t, m, fixedTime)
	b := renderMD(t, m, fixedTime)
	if a != b {
		t.Fatalf("two renders of the same model differ")
	}
	if !strings.HasPrefix(a, "# OSIRIS JSON inventory brief\n") {
		t.Errorf("missing title heading")
	}
	if !strings.Contains(a, "Generated 2026-03-04T05:06:07Z") {
		t.Errorf("injected artifact time not stamped")
	}
}

func TestRenderZeroTimeOmitsGeneratedLine(t *testing.T) {
	m := model(t, "valid/mixed-provider.json", report.Options{})
	out := renderMD(t, m, time.Time{})
	if strings.Contains(out, "Generated ") {
		t.Errorf("zero time must not stamp a generated line:\n%s", firstLines(out, 4))
	}
	if !strings.Contains(out, "Input SHA-256 `") {
		t.Errorf("hash line missing")
	}
}

func TestRenderExactCountsInInventory(t *testing.T) {
	m := model(t, "valid/mixed-provider.json", report.Options{})
	out := renderMD(t, m, fixedTime)
	for _, want := range []string{
		"| Resources | 4 |",
		"| Connections | 3 |",
		"| Groups | 3 |",
		"| Edges | 3 |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("inventory row %q not found", want)
		}
	}
}

func TestRenderInjectionSafety(t *testing.T) {
	m := model(t, "rendering/injection-strings.json", report.Options{})
	out := renderMD(t, m, fixedTime)

	// No raw HTML tag survives as an active tag.
	if strings.Contains(out, "<script>") || strings.Contains(out, "<img src=x") {
		t.Errorf("raw HTML tag left active in the Markdown")
	}
	// No injected line can open a new table row or block: every line
	// that is part of a table starts with "|" only where we put it, and
	// no stray unescaped pipe appears mid-cell.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "| `inj-c-1`") {
			// The connection name held "| table | row |"; after escaping
			// the row must still have the fixed column count.
			if got := strings.Count(line, " | "); got > 10 {
				t.Errorf("connection row column count inflated by injection: %q", line)
			}
		}
	}
	// Control characters are gone.
	if strings.ContainsRune(out, 0x00) || strings.ContainsRune(out, 0x1b) {
		t.Errorf("control character survived into the Markdown")
	}
	// Multilingual text and quotes are preserved (escaped, not dropped).
	if !strings.Contains(out, "duo lingua") {
		t.Errorf("multilingual text dropped")
	}
}

func TestRenderAnomaliesTableAndConnectionNotes(t *testing.T) {
	m := model(t, "adversarial/dangling-connection.json", report.Options{})
	out := renderMD(t, m, fixedTime)
	if !strings.Contains(out, "## Structural anomalies") ||
		!strings.Contains(out, "| Kind | Subject | Reference | Detail |") {
		t.Errorf("anomalies table not rendered when anomalies exist")
	}
	if !strings.Contains(out, "`dangling-connection-target`") {
		t.Errorf("anomaly kind not in the table")
	}
	if !strings.Contains(out, "unresolved") {
		t.Errorf("a connection with a missing endpoint should be annotated 'unresolved'")
	}
}

func TestRenderTruncationNotice(t *testing.T) {
	m := model(t, "valid/mixed-provider.json", report.Options{MaxRows: 1})
	out := renderMD(t, m, fixedTime)
	if !strings.Contains(out, "## Notes") {
		t.Fatalf("bounded render has no Notes section")
	}
	if !strings.Contains(out, "Section resources bounded: showing the first 1 rows of 4") {
		t.Errorf("truncation notice missing or wrong:\n%s", out[strings.Index(out, "## Notes"):])
	}
}

func TestRenderAbsentOptionalValuesUseSentinel(t *testing.T) {
	m := model(t, "valid/no-provider.json", report.Options{})
	out := renderMD(t, m, fixedTime)
	if !strings.Contains(out, "## Resources") || !strings.Contains(out, "## Providers") {
		t.Errorf("core sections missing")
	}
	// A resource that carries no region/status renders the explicit
	// "(none)" sentinel, never a blank cell that could be misread.
	if !strings.Contains(out, "(none)") {
		t.Errorf("absent optional values not rendered with the (none) sentinel")
	}
}

func TestRenderCancellation(t *testing.T) {
	m := model(t, "valid/mixed-provider.json", report.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := NewReportRenderer(m, fixedTime).Render(ctx, &buf); err == nil {
		t.Fatal("Render returned nil on a cancelled context")
	}
}

func TestRenderPropagatesWriteError(t *testing.T) {
	m := model(t, "valid/mixed-provider.json", report.Options{})
	err := NewReportRenderer(m, fixedTime).Render(context.Background(), failWriter{})
	if err == nil {
		t.Fatal("Render swallowed a write error")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errBoom }

var errBoom = &boomError{}

type boomError struct{}

func (*boomError) Error() string { return "boom" }

func firstLines(s string, n int) string {
	parts := strings.SplitN(s, "\n", n+1)
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, "\n")
}

// tocHeadings extracts every "## " (H2) heading from out, in order.
func tocHeadings(out string) []string {
	var headings []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimPrefix(line, "## "))
		}
	}
	return headings
}

// TestReportTOCMatchesActualHeadings guards against reportTOC (and its
// conditional "Notes" entry) drifting from the "## " headings Render
// actually writes: every H2 heading in the output, in order, must
// equal the expected list exactly, a title added, removed, renamed,
// or reordered in either place without the other would otherwise be a
// silent dead (or missing) table-of-contents link. Checked once
// without truncations (no "Notes" heading) and once with (drawn from
// the adversarial fixture with a bounded section, so "Notes" actually
// renders).
func TestReportTOCMatchesActualHeadings(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		opts    report.Options
	}{
		{"without truncation", "valid/mixed-provider.json", report.Options{}},
		{"with truncation", "valid/mixed-provider.json", report.Options{MaxRows: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := model(t, tc.fixture, tc.opts)
			out := renderMD(t, m, fixedTime)
			headings := tocHeadings(out)
			base := reportTOCWithConditional(m)
			want := append([]string{base[0], TOCHeading}, base[1:]...)
			if len(headings) != len(want) {
				t.Fatalf("found %d \"## \" headings, want %d\nheadings: %v\nwant: %v",
					len(headings), len(want), headings, want)
			}
			for i, w := range want {
				if headings[i] != w {
					t.Errorf("heading[%d] = %q, want %q", i, headings[i], w)
				}
			}
		})
	}
}
