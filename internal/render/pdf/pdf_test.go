// Tests for the standard-library PDF writer: file structure (header,
// cross-reference offsets, trailer), determinism, text encoding and
// escaping, column fitting, page breaking with a repeated table
// header, and the bookmark outline.
//
// Several tests inflate the page content streams and assert on the
// operators inside, which is the only way to prove that a break
// actually repeated a header row rather than silently dropping it.
package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// render is a test helper that writes doc and fails on error.
func render(t *testing.T, doc Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, doc); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}

// objHeaderRe finds every "N 0 obj" object header, anchored to the
// start of a line so a two-digit object number (say 17) can never be
// mistaken for a one-digit one (7) by a bare substring search.
var objHeaderRe = regexp.MustCompile(`(?:^|\n)(\d+) 0 obj\n`)

// contentsRefRe finds every page's own "/Contents N 0 R" entry, in
// the order pages were written this, not a bare byte offset, is
// what identifies which objects are page content streams.
var contentsRefRe = regexp.MustCompile(`/Contents (\d+) 0 R`)

var lengthRe = regexp.MustCompile(`/Length (\d+)`)

// contentStreams inflates every page content stream in out, in page
// order.
//
// It locates each stream by its own object's declared /Length rather
// than by searching for a literal "endstream" delimiter in the raw
// bytes: Flate-compressed binary data can coincidentally contain that
// exact byte sequence, which silently truncated a match here once a
// real PDF's compressed content happened to contain it the /Length
// a writer declares is the only way to know a stream's true extent.
func contentStreams(t *testing.T, out []byte) []string {
	t.Helper()
	objOffset := make(map[string]int)
	for _, m := range objHeaderRe.FindAllSubmatchIndex(out, -1) {
		objOffset[string(out[m[2]:m[3]])] = m[1]
	}

	var got []string
	for _, m := range contentsRefRe.FindAllSubmatch(out, -1) {
		objNum := string(m[1])
		off, ok := objOffset[objNum]
		if !ok {
			t.Fatalf("content object %s not found", objNum)
		}
		got = append(got, inflateStreamAt(t, out, off, objNum))
	}
	return got
}

// inflateStreamAt inflates the stream belonging to the object whose
// "N 0 obj\n" header ends at byte offset objStart.
func inflateStreamAt(t *testing.T, out []byte, objStart int, objNum string) string {
	t.Helper()
	dictEnd := bytes.Index(out[objStart:], []byte("stream\n"))
	if dictEnd < 0 {
		t.Fatalf("object %s has no stream keyword", objNum)
	}
	lm := lengthRe.FindSubmatch(out[objStart : objStart+dictEnd])
	if lm == nil {
		t.Fatalf("object %s has no /Length", objNum)
	}
	n, err := strconv.Atoi(string(lm[1]))
	if err != nil {
		t.Fatalf("unparsable /Length for object %s: %v", objNum, err)
	}
	start := objStart + dictEnd + len("stream\n")
	if start+n > len(out) {
		t.Fatalf("object %s /Length %d runs past the end of the file", objNum, n)
	}
	zr, err := zlib.NewReader(bytes.NewReader(out[start : start+n]))
	if err != nil {
		t.Fatalf("opening object %s's content stream: %v", objNum, err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("inflating object %s's content stream: %v", objNum, err)
	}
	zr.Close()
	return string(raw)
}

func simpleDoc() Document {
	return Document{
		Title: "OSIRIS JSON test report",
		Blocks: []Block{
			Heading{Level: 1, Text: "OSIRIS JSON test report"},
			Paragraph{Text: "A short paragraph of body text."},
			Heading{Level: 2, Text: "Inventory"},
			Table{
				Columns: []string{"Name", "Region", "Count"},
				Align:   []Align{AlignLeft, AlignLeft, AlignRight},
				Rows: [][]string{
					{"LAB-RTR01", "westeurope", "3"},
					{"LAB-SW01", "northeurope", "11"},
				},
			},
		},
	}
}

// TestDefaultPageSizeIsA4Portrait pins the zero-value Document's page
// geometry: A4, portrait, never landscape unless a caller explicitly
// asks for it. Document.Landscape is a bool, so this already holds
// simply by Go's own zero-value rule, but the behavior is important
// enough (every renderer built on this engine inherits it) to assert
// directly rather than rely on that being obvious from the field's
// type alone.
func TestDefaultPageSizeIsA4Portrait(t *testing.T) {
	out := render(t, Document{Blocks: []Block{Heading{Level: 1, Text: "Portrait"}}})
	want := fmt.Sprintf("/MediaBox [0 0 %s %s]", num(a4Width), num(a4Height))
	if !bytes.Contains(out, []byte(want)) {
		t.Errorf("default MediaBox is not A4 portrait (%s x %s); want %q in output", num(a4Width), num(a4Height), want)
	}
	if bytes.Contains(out, []byte(fmt.Sprintf("/MediaBox [0 0 %s %s]", num(a4Height), num(a4Width)))) {
		t.Error("output also contains an A4-landscape MediaBox unexpected for a single-page document")
	}
}

func TestLandscapeTrueSwapsPageDimensions(t *testing.T) {
	out := render(t, Document{Landscape: true, Blocks: []Block{Heading{Level: 1, Text: "Landscape"}}})
	want := fmt.Sprintf("/MediaBox [0 0 %s %s]", num(a4Height), num(a4Width))
	if !bytes.Contains(out, []byte(want)) {
		t.Errorf("Landscape: true did not swap the MediaBox; want %q in output", want)
	}
}

func TestWriteEmitsWellFormedFileStructure(t *testing.T) {
	out := render(t, simpleDoc())

	if !bytes.HasPrefix(out, []byte("%PDF-1.7\n")) {
		t.Errorf("missing PDF 1.7 header, got %q", out[:min(16, len(out))])
	}
	if !bytes.HasSuffix(out, []byte("%%EOF\n")) {
		t.Error("file does not end with the EOF marker")
	}
	for _, want := range []string{"/Type /Catalog", "/Type /Pages", "/BaseFont /Helvetica", "/BaseFont /Helvetica-Bold", "trailer", "startxref"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("output is missing %q", want)
		}
	}
}

// TestXrefOffsetsPointAtTheirObjects is the structural test that
// matters most: a reader locates every object through this table, so
// an off-by-one here makes the whole file unreadable even though every
// byte of content is correct.
func TestXrefOffsetsPointAtTheirObjects(t *testing.T) {
	out := render(t, simpleDoc())

	idx := bytes.LastIndex(out, []byte("startxref\n"))
	if idx < 0 {
		t.Fatal("no startxref")
	}
	var startxref int
	fmt.Sscanf(string(out[idx+len("startxref\n"):]), "%d", &startxref)
	if startxref <= 0 || startxref >= len(out) {
		t.Fatalf("startxref %d is outside the file (%d bytes)", startxref, len(out))
	}
	if !bytes.HasPrefix(out[startxref:], []byte("xref\n")) {
		t.Fatalf("startxref does not point at the xref table, found %q", out[startxref:min(startxref+16, len(out))])
	}

	var size int
	fmt.Sscanf(string(out[startxref+len("xref\n"):]), "0 %d", &size)
	if size < 2 {
		t.Fatalf("implausible xref size %d", size)
	}

	// Entries are fixed-width: a 20-byte row per object after the
	// "xref\n0 N\n" preamble.
	preamble := startxref + len("xref\n") + len(fmt.Sprintf("0 %d\n", size))
	for n := 1; n < size; n++ {
		row := out[preamble+20*n : preamble+20*n+20]
		off, err := strconv.Atoi(strings.TrimSpace(string(row[:10])))
		if err != nil {
			t.Fatalf("object %d has an unparsable xref row %q", n, row)
		}
		if off <= 0 || off >= len(out) {
			t.Fatalf("object %d has offset %d, outside the file", n, off)
		}
		want := []byte(fmt.Sprintf("%d 0 obj", n))
		if !bytes.HasPrefix(out[off:], want) {
			t.Errorf("xref offset for object %d points at %q, want %q", n, out[off:min(off+16, len(out))], want)
		}
	}
}

func TestWriteIsDeterministic(t *testing.T) {
	doc := simpleDoc()
	doc.CreationDate = time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	if a, b := render(t, doc), render(t, doc); !bytes.Equal(a, b) {
		t.Error("two renders of the same document differ")
	}
}

func TestCreationDateIsInjectedNeverRead(t *testing.T) {
	zero := render(t, simpleDoc())
	if bytes.Contains(zero, []byte("/CreationDate")) {
		t.Error("a zero CreationDate still wrote a date entry")
	}

	doc := simpleDoc()
	doc.CreationDate = time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	dated := render(t, doc)
	if !bytes.Contains(dated, []byte("/CreationDate (D:20261014093000Z)")) {
		t.Error("the injected creation date is missing or misformatted")
	}
}

func TestRowWiderThanHeaderIsRejected(t *testing.T) {
	err := Write(io.Discard, Document{Blocks: []Block{
		Table{Columns: []string{"A", "B"}, Rows: [][]string{{"1", "2", "3"}}},
	}})
	if err == nil {
		t.Fatal("a row with more cells than the header was accepted")
	}
	if !strings.Contains(err.Error(), "more than its 2 header columns") {
		t.Errorf("unhelpful error: %v", err)
	}
}

func TestShortRowIsAccepted(t *testing.T) {
	if err := Write(io.Discard, Document{Blocks: []Block{
		Table{Columns: []string{"A", "B", "C"}, Rows: [][]string{{"1"}}},
	}}); err != nil {
		t.Fatalf("a short row should render blank cells, got %v", err)
	}
}

// TestTableRepeatsHeaderOnEveryPage proves a continued table is never
// a headerless block of cells.
func TestTableRepeatsHeaderOnEveryPage(t *testing.T) {
	rows := make([][]string, 200)
	for i := range rows {
		rows[i] = []string{fmt.Sprintf("LAB-RTR%03d", i), "westeurope", strconv.Itoa(i)}
	}
	// No Title here deliberately: this test is about table-pagination
	// mechanics, not the cover/header/footer feature (TestCover* and
	// TestFooterWith* cover that); the in-flow Heading below is
	// independent of Title either way.
	out := render(t, Document{
		Blocks: []Block{
			Heading{Level: 1, Text: "Wide inventory"},
			Table{Columns: []string{"Name", "Region", "Count"}, Rows: rows},
		},
	})

	streams := contentStreams(t, out)
	if len(streams) < 3 {
		t.Fatalf("200 rows produced only %d page(s)", len(streams))
	}
	for i, s := range streams {
		if !strings.Contains(s, "(Region) Tj") {
			t.Errorf("page %d does not repeat the table header", i+1)
		}
		if !strings.Contains(s, fmt.Sprintf("(Page %d of %d) Tj", i+1, len(streams))) {
			t.Errorf("page %d has a wrong or missing footer", i+1)
		}
	}
}

func TestOutlineRecordsLevel2Headings(t *testing.T) {
	out := render(t, Document{
		Title: "Outlined",
		Blocks: []Block{
			Heading{Level: 1, Text: "Outlined"},
			Heading{Level: 2, Text: "First section"},
			Paragraph{Text: "body"},
			Heading{Level: 2, Text: "Second section"},
		},
	})
	if !bytes.Contains(out, []byte("/Type /Outlines")) {
		t.Fatal("no outline was written")
	}
	for _, want := range []string{"/Title (First section)", "/Title (Second section)"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("outline is missing %q", want)
		}
	}
	if bytes.Contains(out, []byte("/Title (Outlined)\n")) {
		t.Error("the level-1 title should not become a bookmark")
	}
}

func TestPageBreakStartsANewPage(t *testing.T) {
	out := render(t, Document{Blocks: []Block{
		Paragraph{Text: "first"},
		PageBreak{},
		Paragraph{Text: "second"},
	}})
	if n := len(contentStreams(t, out)); n != 2 {
		t.Errorf("got %d pages, want 2", n)
	}
}

func TestHeadingPageBreakBeforeStartsANewPage(t *testing.T) {
	out := render(t, Document{Blocks: []Block{
		Paragraph{Text: "first"},
		Heading{Level: 2, Text: "Section", PageBreakBefore: true},
		Paragraph{Text: "second"},
	}})
	if n := len(contentStreams(t, out)); n != 2 {
		t.Errorf("got %d pages, want 2", n)
	}
}

func TestHeadingPageBreakBeforeIsANoOpOnAFreshPage(t *testing.T) {
	// A heading that already starts a page (nothing drawn there yet)
	// must not consume a second, blank page just because the flag is
	// set the same no-op rule the standalone PageBreak block follows.
	out := render(t, Document{Blocks: []Block{
		Heading{Level: 2, Text: "Section", PageBreakBefore: true},
		Paragraph{Text: "body"},
	}})
	if n := len(contentStreams(t, out)); n != 1 {
		t.Errorf("got %d pages, want 1", n)
	}
}

func TestHeadingWithoutPageBreakBeforeStaysOnTheSamePageWhenItFits(t *testing.T) {
	out := render(t, Document{Blocks: []Block{
		Paragraph{Text: "first"},
		Heading{Level: 2, Text: "Section"},
		Paragraph{Text: "second"},
	}})
	if n := len(contentStreams(t, out)); n != 1 {
		t.Errorf("got %d pages, want 1 (PageBreakBefore defaults to false)", n)
	}
}

func TestEscapeString(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain", "(plain)"},
		{"a(b)c", `(a\(b\)c)`},
		{`back\slash`, `(back\\slash)`},
		{"tab\there", "(tab here)"},
		{"café", `(caf\351)`},
		{"中", "(?)"},
	} {
		if got := escapeString(tc.in); got != tc.want {
			t.Errorf("escapeString(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestWrapHardSplitsAnUnbreakableToken(t *testing.T) {
	long := strings.Repeat("a", 400)
	lines := wrap(long, 8, face{}, 100)
	if len(lines) < 2 {
		t.Fatalf("a 400-character token produced %d line(s)", len(lines))
	}
	joined := strings.Join(lines, "")
	if joined != long {
		t.Error("hard splitting lost or reordered characters")
	}
	for i, l := range lines {
		if w := textWidth(l, 8, face{}); w > 100.01 {
			t.Errorf("line %d is %.2fpt wide, over the 100pt limit", i, w)
		}
	}
}

func TestWrapKeepsWordsWhole(t *testing.T) {
	lines := wrap("alpha beta gamma delta epsilon", 9, face{}, 60)
	if len(lines) < 2 {
		t.Fatal("expected the text to wrap")
	}
	for _, l := range lines {
		if strings.HasPrefix(l, " ") || strings.HasSuffix(l, " ") {
			t.Errorf("line %q has stray padding", l)
		}
	}
	if got := strings.Join(lines, " "); got != "alpha beta gamma delta epsilon" {
		t.Errorf("wrapping changed the text: %q", got)
	}
}

func TestWrapAlwaysReturnsALine(t *testing.T) {
	if got := wrap("", 9, face{}, 50); len(got) != 1 || got[0] != "" {
		t.Errorf("wrap(%q) = %#v, want one empty line", "", got)
	}
}

// TestColumnWidthsNeverOverflowThePrintableWidth covers the case the
// Azure report actually hits in this initial release: a table with many
// columns, far wider than the page, which must be compressed to fit
// rather than run off the edge.
func TestColumnWidthsNeverOverflowThePrintableWidth(t *testing.T) {
	cols := make([]string, 24)
	row := make([]string, 24)
	for i := range cols {
		cols[i] = fmt.Sprintf("column-header-%d", i)
		row[i] = strings.Repeat("x", 30)
	}
	l := &layouter{pw: a4Height, ph: a4Width, avail: a4Height - marginLeft - marginRight}
	widths := l.columnWidths(Table{Columns: cols, Rows: [][]string{row}}, len(cols))

	total := 0.0
	for _, w := range widths {
		if w <= 0 {
			t.Fatalf("non-positive column width %v", w)
		}
		total += w
	}
	if total > l.avail+0.5 {
		t.Errorf("columns total %.2fpt, over the %.2fpt printable width", total, l.avail)
	}
	if total < l.avail-1 {
		t.Errorf("columns total %.2fpt, leaving %.2fpt of the printable width unused", total, l.avail-total)
	}
}

func TestColumnWidthsKeepNaturalWidthsWhenTheyFit(t *testing.T) {
	l := &layouter{pw: a4Width, ph: a4Height, avail: a4Width - marginLeft - marginRight}
	tbl := Table{Columns: []string{"Name", "Region"}, Rows: [][]string{{"LAB-RTR01", "westeurope"}}}
	widths := l.columnWidths(tbl, 2)
	for c, w := range widths {
		natural := textWidth(tbl.Columns[c], tableSize, face{bold: true})
		if cw := textWidth(tbl.Rows[0][c], tableSize, face{}); cw > natural {
			natural = cw
		}
		if diff := w - (natural + 2*cellPadX); diff > 0.01 || diff < -0.01 {
			t.Errorf("column %d is %.2fpt, want its natural %.2fpt", c, w, natural+2*cellPadX)
		}
	}
}

func TestTextWidthUsesRealMetrics(t *testing.T) {
	// 'i' is far narrower than 'W' in Helvetica; a monospace
	// approximation would make these equal and ruin every column fit.
	if textWidth("i", 10, face{}) >= textWidth("W", 10, face{}) {
		t.Error("Helvetica metrics look wrong: 'i' is not narrower than 'W'")
	}
	if textWidth("Name", 10, face{bold: true}) <= textWidth("Name", 10, face{}) {
		t.Error("bold Helvetica should be wider than regular for this string")
	}
}

func TestEncodeRuneFallsBackToQuestionMark(t *testing.T) {
	if b, ok := encodeRune('中'); ok || b != '?' {
		t.Errorf("encodeRune(CJK) = %q, %v; want '?', false", b, ok)
	}
	if b, ok := encodeRune('€'); !ok || b != 0x80 {
		t.Errorf("encodeRune(euro) = %#x, %v; want 0x80, true", b, ok)
	}
}

func TestNumIsCompactAndHasNoNegativeZero(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{0, "0"}, {-0.001, "0"}, {12, "12"}, {12.5, "12.5"}, {12.345, "12.35"},
	} {
		if got := num(tc.in); got != tc.want {
			t.Errorf("num(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
