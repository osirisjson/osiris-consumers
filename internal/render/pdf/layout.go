// This file is the layout engine: it turns a Document's ordered blocks
// into a sequence of pages, each carrying a finished content stream.
//
// Layout is a single forward pass with no backtracking, which is what
// makes it deterministic: measure the block against the space left on
// the current page, break to a new page when it does not fit, draw,
// advance the cursor. The one lookahead is a table's header row, which
// is re-drawn at the top of every page the table continues onto so a
// continued table is never a headerless block of cells.
//
// The cursor y is held as a distance from the top of the page, which
// reads naturally for top-to-bottom flow; the drawing helpers convert
// to PDF's bottom-left origin at the moment they emit an operator.
//
// A Document whose Title is set gets a cover page (its own layout,
// built by addCoverPage, never touched by the block-layout loop) plus
// a running header and a two-line footer on every page after it see
// contentTop/contentBottomLimit for how that reserved space is
// carved out of the normal content area, and pdf.go's Document doc
// comment for the feature as a whole. A Document with no title gets
// none of it: contentTop/contentBottomLimit fall back to the plain
// marginTop/marginBottom this package always used, so that caller's
// output is unaffected.
package pdf

import (
	"fmt"
	"strings"
	"time"
)

// Page geometry and type sizes, in points.
const (
	marginLeft   = 42.0
	marginRight  = 42.0
	marginTop    = 46.0
	marginBottom = 46.0

	titleSize  = 14
	h2Size     = 12
	bodySize   = 9.0
	tableSize  = 5
	footerSize = 7.0

	cellPadX    = 4.0
	cellPadY    = 3.2
	minColWidth = 34.0

	// Space a level-2 heading needs below it before it is worth
	// starting on this page at all, so a heading never sits alone at
	// the foot of a page.
	headingOrphanGuard = 46.0

	// headerBandHeight is the running header's own height (logo, title,
	// rule), reserved above marginTop on every page after the cover.
	// headerTitleSize is its own size, deliberately smaller than
	// h2Size (the body's own level-2 heading size, which the header
	// used to share before this existed) a running header reads as
	// a compact label, not a section heading.
	headerBandHeight = 42.0
	headerLogoTop    = 35.0
	headerLogoHeight = 15.0
	headerTitleSize  = 8.0

	// footerBandHeight is the running footer's extra height (two lines
	// on the right, up to two on the left, a rule) beyond the plain
	// marginBottom this package always reserved, on every page after
	// the cover. footerRuleGap is the gap between that rule and the
	// text baseline below it.
	footerBandHeight = 34.0
	footerRuleGap    = 14.0

	coverLogoHeight = 50.0
	// coverTitleSize is the cover title's first row (the fixed
	// "<product> report for" part, when Title has two rows see
	// addCoverPage); coverTitleSize2 is its second row (the specific
	// document's own name), deliberately smaller so the two rows read
	// as a heading and its subject, not two equal titles.
	coverTitleSize   = 18.0
	coverTitleSize2  = 13.0
	coverSubjectSize = 12.0
	coverMetaSize    = 9.0
)

// leadingFor returns the baseline-to-baseline distance for size.
func leadingFor(size float64) float64 { return size * 1.32 }

// page is one laid-out page and its accumulated content stream.
// family and usage are set once at newPage time from the document's
// own FontFamily, so every existing call to text() needs no change at
// all to pick the right content-stream encoding. isCover marks the
// one page addCoverPage built directly footers() and the running
// header never touch it.
type page struct {
	buf     strings.Builder
	ph      float64
	family  FontFamily
	usage   *fontUsage
	isCover bool
}

// text draws s at x with its baseline yTop points below the page top.
func (p *page) text(x, yTop, size float64, bold bool, gray float64, s string) {
	if s == "" {
		return
	}
	font := "F1"
	if bold {
		font = "F2"
	}
	operand := escapeString(s)
	if p.family == FontMontserrat {
		operand = montserratHexString(p.usage, s, bold)
	}
	fmt.Fprintf(&p.buf, "BT /%s %s Tf %s g %s %s Td %s Tj ET\n",
		font, num(size), num(gray), num(x), num(p.ph-yTop), operand)
}

// rect fills a rectangle whose top edge is yTop below the page top.
func (p *page) rect(x, yTop, w, h, gray float64) {
	fmt.Fprintf(&p.buf, "%s g %s %s %s %s re f\n",
		num(gray), num(x), num(p.ph-yTop-h), num(w), num(h))
}

// hline strokes a horizontal rule at yTop below the page top.
func (p *page) hline(x1, x2, yTop, width, gray float64) {
	fmt.Fprintf(&p.buf, "%s G %s w %s %s m %s %s l S\n",
		num(gray), num(width), num(x1), num(p.ph-yTop), num(x2), num(p.ph-yTop))
}

// outlineEntry is one bookmark: a level-2 heading and where it landed.
type outlineEntry struct {
	title   string
	pageIdx int
	y       float64 // PDF coordinates, measured from the page bottom
}

type layouter struct {
	doc      Document
	pw, ph   float64
	avail    float64
	pages    []*page
	cur      *page
	y        float64
	outline  []outlineEntry
	usage    *fontUsage
	hasCover bool
}

// layout lays doc out on pw x ph pages and returns the finished pages,
// the bookmark outline, and (for a FontMontserrat document) the
// glyphs it actually used always a non-nil, possibly empty tracker,
// so writer.go never needs a nil check.
func layout(doc Document, pw, ph float64) ([]*page, []outlineEntry, *fontUsage) {
	l := &layouter{
		doc: doc, pw: pw, ph: ph, avail: pw - marginLeft - marginRight,
		usage: newFontUsage(), hasCover: doc.Title != "",
	}
	if l.hasCover {
		l.addCoverPage()
	}
	l.newPage()

	for _, b := range doc.Blocks {
		switch v := b.(type) {
		case Heading:
			l.heading(v)
		case Paragraph:
			l.paragraph(v)
		case Table:
			l.table(v)
		case PageBreak:
			if l.y > l.contentTop() {
				l.newPage()
			}
		}
	}

	l.footers()
	return l.pages, l.outline, l.usage
}

// face returns the measurement/drawing face for the document's own
// FontFamily at the given weight.
func (l *layouter) face(bold bool) face {
	return face{family: l.doc.FontFamily, bold: bold}
}

// contentTop and contentBottomLimit are the vertical bounds every
// page-break decision measures against. They fall back to the plain
// marginTop/marginBottom whenever there is no cover (and so no
// running header or footer band), which is what keeps a Document
// with no Title laid out exactly as this package always laid it out.
func (l *layouter) contentTop() float64 {
	if l.hasCover {
		return marginTop + headerBandHeight
	}
	return marginTop
}

func (l *layouter) contentBottomLimit() float64 {
	if l.hasCover {
		return l.ph - marginBottom - footerBandHeight
	}
	return l.ph - marginBottom
}

func (l *layouter) newPage() {
	p := &page{ph: l.ph, family: l.doc.FontFamily, usage: l.usage}
	l.pages = append(l.pages, p)
	l.cur = p
	if l.hasCover {
		l.drawHeaderBand()
	}
	l.y = l.contentTop()
}

// drawHeaderBand draws the running header onto the current page: the
// word mark at the left margin and the document title to its right,
// right-aligned against the right margin (truncated with an ellipsis
// rather than wrapped, never wrapped across more lines than it is
// explicitly given a header stays a fixed, predictable height). No
// rule underneath the header band's own whitespace is what
// separates it from the body content.
//
// An embedded newline in Title (the same convention FooterLeft uses)
// splits it into exactly two header rows instead of one, vertically
// centered as a block against the logo a caller that wants, say,
// a fixed "<product> report for" on the first row and the specific
// document's own name on the second sets Title accordingly. A Title
// with no newline renders as a single row, same as before this
// existed. The cover page's own title is unaffected either way: it
// wraps naturally by width (addCoverPage's wrap() call), and Go's
// Fields-based word splitting already treats a newline as plain
// whitespace there.
func (l *layouter) drawHeaderBand() {
	logoW := drawLogo(&l.cur.buf, l.ph, marginLeft, headerLogoTop, headerLogoHeight)

	rightEdge := l.pw - marginRight
	titleAvail := rightEdge - (marginLeft + logoW + 12)
	line1, line2, twoRows := strings.Cut(l.doc.Title, "\n")
	line1 = truncateToWidth(line1, headerTitleSize, l.face(false), titleAvail)

	if !twoRows {
		baseline := headerLogoTop + headerLogoHeight*0.72
		l.cur.text(rightEdge-textWidth(line1, headerTitleSize, l.face(false)), baseline, headerTitleSize, false, 0.1, line1)
		return
	}

	line2 = truncateToWidth(line2, headerTitleSize, l.face(false), titleAvail)
	lead := leadingFor(headerTitleSize)
	logoCenter := headerLogoTop + headerLogoHeight/2
	baseline1 := logoCenter - lead/2 + headerTitleSize*0.36
	l.cur.text(rightEdge-textWidth(line1, headerTitleSize, l.face(false)), baseline1, headerTitleSize, false, 0.1, line1)
	l.cur.text(rightEdge-textWidth(line2, headerTitleSize, l.face(false)), baseline1+lead, headerTitleSize, false, 0.1, line2)
}

// addCoverPage builds the one-off cover page: the word mark centered
// on the page itself (both axes the single fixed point everything
// else is laid out relative to), the title below it, and the subject
// and creation timestamp (each optional). It is never touched by the
// normal block-layout loop Blocks starts rendering on the page
// newPage builds right after it.
//
// Title follows the same two-row convention drawHeaderBand uses (an
// embedded newline), but at two distinct sizes here rather than one:
// row one (coverTitleSize) reads as the heading, row two
// (coverTitleSize2, smaller) as the specific document's own name a
// Title with no newline draws as a single row at coverTitleSize, same
// as before two-row support existed.
func (l *layouter) addCoverPage() {
	p := &page{ph: l.ph, family: l.doc.FontFamily, usage: l.usage, isCover: true}
	l.pages = append(l.pages, p)
	cx := l.pw / 2

	logoW := logoAspect() * coverLogoHeight
	logoTop := l.ph/2 - coverLogoHeight/2
	drawLogo(&p.buf, l.ph, cx-logoW/2, logoTop, coverLogoHeight)

	y := logoTop + coverLogoHeight + 30
	line1, line2, twoRows := strings.Cut(l.doc.Title, "\n")

	lw1 := textWidth(line1, coverTitleSize, l.face(true))
	p.text(cx-lw1/2, y+coverTitleSize*0.82, coverTitleSize, true, 0, line1)
	y += leadingFor(coverTitleSize)

	if twoRows {
		lw2 := textWidth(line2, coverTitleSize2, l.face(true))
		p.text(cx-lw2/2, y+coverTitleSize2*0.82, coverTitleSize2, true, 0.15, line2)
		y += leadingFor(coverTitleSize2)
	}
	y += 10

	if l.doc.Subject != "" {
		lw := textWidth(l.doc.Subject, coverSubjectSize, l.face(false))
		p.text(cx-lw/2, y+coverSubjectSize*0.82, coverSubjectSize, false, 0.25, l.doc.Subject)
		y += leadingFor(coverSubjectSize) + 8
	}
	if !l.doc.CreationDate.IsZero() {
		text := "Generated " + l.doc.CreationDate.UTC().Format(time.RFC3339)
		lw := textWidth(text, coverMetaSize, l.face(false))
		p.text(cx-lw/2, y+coverMetaSize*0.82, coverMetaSize, false, 0.4, text)
	}
}

// ensure breaks to a new page when h points do not fit below the
// cursor. A block taller than a whole page is drawn anyway rather than
// looping forever; it simply starts at the top of a fresh page.
func (l *layouter) ensure(h float64) {
	if l.y+h <= l.contentBottomLimit() {
		return
	}
	if l.y <= l.contentTop() {
		return
	}
	l.newPage()
}

func (l *layouter) heading(h Heading) {
	if h.PageBreakBefore && l.y > l.contentTop() {
		l.newPage()
	}

	if h.Level <= 1 {
		lines := wrap(h.Text, titleSize, l.face(true), l.avail)
		lead := leadingFor(titleSize)
		l.ensure(float64(len(lines))*lead + 14)
		for _, line := range lines {
			l.cur.text(marginLeft, l.y+titleSize*0.82, titleSize, true, 0, line)
			l.y += lead
		}
		l.y += 5
		l.cur.hline(marginLeft, l.pw-marginRight, l.y, 1.1, 0.35)
		l.y += 13
		return
	}

	lines := wrap(h.Text, h2Size, l.face(true), l.avail)
	lead := leadingFor(h2Size)
	if l.y > l.contentTop() {
		l.y += 11
	}
	l.ensure(float64(len(lines))*lead + headingOrphanGuard)
	l.outline = append(l.outline, outlineEntry{
		title:   h.Text,
		pageIdx: len(l.pages) - 1,
		y:       l.ph - l.y + h2Size,
	})
	for _, line := range lines {
		l.cur.text(marginLeft, l.y+h2Size*0.82, h2Size, true, 0, line)
		l.y += lead
	}
	l.y += 2.5
	l.cur.hline(marginLeft, l.pw-marginRight, l.y, 0.5, 0.72)
	l.y += 8
}

func (l *layouter) paragraph(p Paragraph) {
	lead := leadingFor(bodySize)
	for _, para := range strings.Split(p.Text, "\n") {
		for _, line := range wrap(para, bodySize, l.face(false), l.avail) {
			l.ensure(lead)
			l.cur.text(marginLeft, l.y+bodySize*0.82, bodySize, false, 0, line)
			l.y += lead
		}
	}
	l.y += 7
}

func (l *layouter) table(t Table) {
	cols := len(t.Columns)
	for _, r := range t.Rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	if cols == 0 {
		return
	}

	widths := l.columnWidths(t, cols)
	lead := leadingFor(tableSize)

	// wrapped holds every row's cells pre-wrapped to their column, so
	// a row's height is known before anything is drawn.
	wrapped := make([][][]string, len(t.Rows))
	heights := make([]float64, len(t.Rows))
	for i, row := range t.Rows {
		cells := make([][]string, cols)
		maxLines := 1
		for c := 0; c < cols; c++ {
			var cell string
			if c < len(row) {
				cell = row[c]
			}
			cells[c] = wrap(cell, tableSize, l.face(false), widths[c]-2*cellPadX)
			if n := len(cells[c]); n > maxLines {
				maxLines = n
			}
		}
		wrapped[i] = cells
		heights[i] = float64(maxLines)*lead + 2*cellPadY
	}

	headerH := 0.0
	var headerLines [][]string
	if len(t.Columns) > 0 {
		headerLines = make([][]string, cols)
		maxLines := 1
		for c := 0; c < cols; c++ {
			var cell string
			if c < len(t.Columns) {
				cell = t.Columns[c]
			}
			headerLines[c] = wrap(cell, tableSize, l.face(true), widths[c]-2*cellPadX)
			if n := len(headerLines[c]); n > maxLines {
				maxLines = n
			}
		}
		headerH = float64(maxLines)*lead + 2*cellPadY
	}

	firstRowH := 0.0
	if len(heights) > 0 {
		firstRowH = heights[0]
	}
	l.ensure(headerH + firstRowH)
	l.drawHeader(t, headerLines, widths, headerH, lead)

	for i := range t.Rows {
		if l.y+heights[i] > l.contentBottomLimit() && l.y > l.contentTop() {
			l.newPage()
			l.drawHeader(t, headerLines, widths, headerH, lead)
		}
		l.drawRow(t, wrapped[i], widths, heights[i], lead)
	}
	l.y += 8
}

// drawHeader draws the header band when the table has one. It is a
// no-op for a headerless table, so a key/value block gets no band.
func (l *layouter) drawHeader(t Table, headerLines [][]string, widths []float64, headerH, lead float64) {
	if headerH == 0 {
		return
	}
	total := 0.0
	for _, w := range widths {
		total += w
	}
	l.cur.rect(marginLeft, l.y, total, headerH, 0.91)
	x := marginLeft
	for c, lines := range headerLines {
		for k, line := range lines {
			l.cur.text(l.cellX(t, c, x, widths[c], line, l.face(true)), l.y+cellPadY+tableSize*0.82+float64(k)*lead,
				tableSize, true, 0, line)
		}
		x += widths[c]
	}
	l.y += headerH
	l.cur.hline(marginLeft, marginLeft+total, l.y, 0.6, 0.55)
}

func (l *layouter) drawRow(t Table, cells [][]string, widths []float64, rowH, lead float64) {
	total := 0.0
	for _, w := range widths {
		total += w
	}
	x := marginLeft
	for c, lines := range cells {
		for k, line := range lines {
			l.cur.text(l.cellX(t, c, x, widths[c], line, l.face(false)), l.y+cellPadY+tableSize*0.82+float64(k)*lead,
				tableSize, false, 0, line)
		}
		x += widths[c]
	}
	l.y += rowH
	l.cur.hline(marginLeft, marginLeft+total, l.y, 0.4, 0.86)
}

// cellX returns a cell line's x, honouring the column's alignment.
func (l *layouter) cellX(t Table, col int, x, w float64, line string, f face) float64 {
	if col < len(t.Align) && t.Align[col] == AlignRight {
		return x + w - cellPadX - textWidth(line, tableSize, f)
	}
	return x + cellPadX
}

// columnWidths fits the table's natural column widths into the
// printable width.
//
// A column's natural width is its widest cell. When the naturals fit,
// they are used as-is. When they do not, a single cap is found such
// that every column takes min(natural, max(floor, cap)) and the total
// lands on the printable width: wide columns give up space, narrow
// ones keep theirs. Any slack left over is handed back to the capped
// columns in proportion, so the table always spans the full width.
func (l *layouter) columnWidths(t Table, cols int) []float64 {
	natural := make([]float64, cols)
	for c := 0; c < cols; c++ {
		w := 0.0
		if c < len(t.Columns) {
			w = textWidth(t.Columns[c], tableSize, l.face(true))
		}
		for _, row := range t.Rows {
			if c >= len(row) {
				continue
			}
			if cw := textWidth(row[c], tableSize, l.face(false)); cw > w {
				w = cw
			}
		}
		natural[c] = w + 2*cellPadX
	}

	total := 0.0
	maxNatural := 0.0
	for _, w := range natural {
		total += w
		if w > maxNatural {
			maxNatural = w
		}
	}
	if total <= l.avail {
		return natural
	}

	floor := minColWidth
	if float64(cols)*floor > l.avail {
		floor = l.avail / float64(cols)
	}

	widthsAt := func(cap float64) ([]float64, float64) {
		out := make([]float64, cols)
		sum := 0.0
		for c, n := range natural {
			w := cap
			if w < floor {
				w = floor
			}
			if w > n {
				w = n
			}
			out[c] = w
			sum += w
		}
		return out, sum
	}

	// Seed with the narrowest possible fit rather than the natural
	// widths: when every column is already pinned to the floor, no
	// search iteration can improve on it, and starting from the
	// overflowing naturals would leave them in place.
	lo, hi := 0.0, maxNatural
	widths, _ := widthsAt(lo)
	for i := 0; i < 64; i++ {
		mid := (lo + hi) / 2
		w, sum := widthsAt(mid)
		if sum > l.avail+0.001 {
			hi = mid
		} else {
			lo = mid
			widths = w
		}
	}

	sum := 0.0
	cappedTotal := 0.0
	for c, w := range widths {
		sum += w
		if w < natural[c] {
			cappedTotal += w
		}
	}
	if slack := l.avail - sum; slack > 0.01 && cappedTotal > 0 {
		for c, w := range widths {
			if w < natural[c] {
				widths[c] = w + slack*(w/cappedTotal)
			}
		}
	}
	return widths
}

// footers stamps the running footer onto every page once the total
// page count is known every page except the cover, which has none.
//
// With no cover (and so no FooterLeft/FooterRight/running header),
// this draws exactly the single page-number line this package always
// drew, unchanged. With a cover, the footer becomes two lines: the
// caller's FooterLeft on the left and FooterRight on the right at
// the top line, then the creation timestamp and "Page X of N" on the
// line below as two separate text draws, not one joined string, so
// a reader (and TestTableRepeatsHeaderOnEveryPage) can still find the
// exact "Page X of N" text as its own run. FooterLeft alone may carry
// an embedded newline for a second line of its own (the same
// convention Paragraph.Text already uses) a copyright notice split
// across "OSIRIS JSON" and a longer description line, say which
// then renders aligned with the right side's second line. FooterRight
// has no second line of its own: its own row is shared with the
// timestamp and "Page X of N", which the engine always draws there.
func (l *layouter) footers() {
	n := len(l.pages)
	for i, p := range l.pages {
		if p.isCover {
			continue
		}
		if !l.hasCover {
			y := l.ph - marginBottom + 16
			p.hline(marginLeft, l.pw-marginRight, y-10, 0.4, 0.8)
			label := fmt.Sprintf("Page %d of %d", i+1, n)
			p.text(l.pw-marginRight-textWidth(label, footerSize, l.face(false)), y, footerSize, false, 0.45, label)
			continue
		}

		lineGap := leadingFor(footerSize)
		top := l.ph - marginBottom - footerBandHeight + 14
		p.hline(marginLeft, l.pw-marginRight, top-footerRuleGap, 0.4, 0.8)

		if left := l.doc.FooterLeft; left != "" {
			line0, line1, _ := strings.Cut(left, "\n")
			p.text(marginLeft, top, footerSize, false, 0.45, truncateToWidth(line0, footerSize, l.face(false), l.avail*0.5))
			if line1 != "" {
				p.text(marginLeft, top+lineGap, footerSize, false, 0.45, truncateToWidth(line1, footerSize, l.face(false), l.avail*0.5))
			}
		}
		if right := l.doc.FooterRight; right != "" {
			line := truncateToWidth(right, footerSize, l.face(false), l.avail*0.65)
			p.text(l.pw-marginRight-textWidth(line, footerSize, l.face(false)), top, footerSize, false, 0.45, line)
		}

		label := fmt.Sprintf("Page %d of %d", i+1, n)
		labelW := textWidth(label, footerSize, l.face(false))
		y2 := top + lineGap
		p.text(l.pw-marginRight-labelW, y2, footerSize, false, 0.45, label)
		if !l.doc.CreationDate.IsZero() {
			ts := l.doc.CreationDate.UTC().Format(time.RFC3339)
			tsW := textWidth(ts, footerSize, l.face(false))
			const gap = 10.0
			p.text(l.pw-marginRight-labelW-gap-tsW, y2, footerSize, false, 0.45, ts)
		}
	}
}

// wrap breaks s into lines no wider than maxW, splitting on spaces and
// hard-splitting any single token too long to fit on a line of its
// own. It always returns at least one line, so an empty cell still
// occupies its row.
func wrap(s string, size float64, f face, maxW float64) []string {
	if s == "" {
		return []string{""}
	}
	if maxW <= 0 {
		return []string{s}
	}
	if textWidth(s, size, f) <= maxW {
		return []string{s}
	}

	var lines []string
	var cur string
	flush := func() {
		if cur != "" {
			lines = append(lines, cur)
			cur = ""
		}
	}
	for _, word := range strings.Fields(s) {
		candidate := word
		if cur != "" {
			candidate = cur + " " + word
		}
		if textWidth(candidate, size, f) <= maxW {
			cur = candidate
			continue
		}
		flush()
		if textWidth(word, size, f) <= maxW {
			cur = word
			continue
		}
		// Every chunk but the last becomes a line of its own; the
		// last stays in cur so the following word can share its line.
		chunks := hardSplit(word, size, f, maxW)
		for i, chunk := range chunks {
			if i == len(chunks)-1 {
				cur = chunk
				break
			}
			lines = append(lines, chunk)
		}
	}
	flush()
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// hardSplit breaks an unbreakable token into rune chunks that each fit
// maxW. The final chunk is returned last so the caller can continue
// filling the line with it.
func hardSplit(s string, size float64, f face, maxW float64) []string {
	var out []string
	var cur strings.Builder
	curW := 0.0
	for _, r := range s {
		rw := float64(runeWidth(r, f)) * size / 1000
		if curW+rw > maxW && cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
			curW = 0
		}
		cur.WriteRune(r)
		curW += rw
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// truncateToWidth shortens s with an ellipsis so it fits maxW.
func truncateToWidth(s string, size float64, f face, maxW float64) string {
	if textWidth(s, size, f) <= maxW {
		return s
	}
	const ell = "..."
	ellW := textWidth(ell, size, f)
	var b strings.Builder
	w := 0.0
	for _, r := range s {
		rw := float64(runeWidth(r, f)) * size / 1000
		if w+rw+ellW > maxW {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + ell
}
