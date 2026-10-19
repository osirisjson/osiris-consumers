// Package pdf writes a deterministic, paginated PDF 1.7 document using
// only the standard library, no third-party dependency, same
// for internal/render/xlsx.
//
// The caller describes a document as an ordered list of blocks
// (headings, paragraphs, tables, explicit page breaks) and this
// package does the rest: it measures text against the Adobe Core 14
// Helvetica metrics, wraps it, fits table columns to the printable
// width, breaks pages, repeats a table's header row after every break,
// numbers the pages, and emits a bookmark outline for the headings.
//
// Two base-14 faces are available by default, Helvetica and
// Helvetica-Bold, referenced rather than embedded, which keeps the
// output small and free of font-licensing concerns. Text is encoded as
// WinAnsiEncoding; a rune outside that repertoire is written as '?'
// rather than dropped (see font.go).
//
// A Document may instead select FontMontserrat, which embeds the
// Montserrat Regular/Bold TrueType instances as CIDFontType2 fonts
// (see montserrat.go, cidfont.go) and is encoded as Identity-H, giving
// broader Unicode coverage at the cost of roughly 650KB of embedded
// font data per document regardless of how much text it uses.
//
// A Document whose Title is non-empty gets a cover page (its own,
// centered layout: the OSIRIS JSON word mark, the title, the subject,
// and the creation timestamp) plus a running header (the mark on the
// left, the title on the right) and footer (a caller-supplied left
// line, and a caller-supplied right line followed by the timestamp
// and "Page X of N") on every page after it. A Document with no title
// gets none of this a single content flow with no cover, no header,
// and a minimal page-number-only footer, exactly as this package
// behaved before the cover/header/footer feature existed.
//
// Determinism: the output is a pure function of the Document value.
// Nothing here reads the wall clock a creation date is an injected
// field, omitted entirely when zero and every object number, page
// break and column width follows from the content alone.
package pdf

import (
	"fmt"
	"io"
	"time"
)

// Align is a table column's horizontal alignment.
type Align int

// The supported column alignments.
const (
	AlignLeft Align = iota
	AlignRight
)

// A Block is one renderable element of a Document. The implementations
// are Heading, Paragraph, Table and PageBreak.
type Block interface {
	block()
}

// Heading is a section title. Level 1 is the document title, drawn
// once at the top with a rule beneath it; level 2 is a section
// heading. Every level-2 heading becomes an entry in the PDF bookmark
// outline, in emission order. PageBreakBefore forces a new page ahead
// of this heading, the same way an explicit PageBreak block would,
// rather than only breaking when the heading does not otherwise fit
// (ensure's orphan guard) a no-op on a page that is already blank.
type Heading struct {
	Level           int
	Text            string
	PageBreakBefore bool
}

// Paragraph is a run of body text, wrapped to the printable width. A
// newline in Text forces a line break; the text is not otherwise
// interpreted, so it carries no markup of any kind.
type Paragraph struct {
	Text string
}

// Table is a grid of strings. Columns is the header row and may be nil
// for a headerless table (a two-column key/value block, for example).
// Align may be nil, shorter than the column count, or full length; a
// column it does not cover is left-aligned. A row may be shorter than
// the header (the remaining cells render blank) but never longer
// Write returns an error rather than silently dropping a value.
type Table struct {
	Columns []string
	Align   []Align
	Rows    [][]string
}

// PageBreak starts the following block on a new page.
type PageBreak struct{}

func (Heading) block()   {}
func (Paragraph) block() {}
func (Table) block()     {}
func (PageBreak) block() {}

// FontFamily selects which face set a Document is measured, laid out,
// and embedded with.
type FontFamily int

// The supported font families.
const (
	// FontHelvetica uses the base-14 Helvetica / Helvetica-Bold faces
	// (font.go): referenced, never embedded, so this is the zero
	// value a Document that does not set FontFamily renders exactly
	// as it did before FontMontserrat existed.
	FontHelvetica FontFamily = iota
	// FontMontserrat embeds the Montserrat Regular / Bold TrueType
	// instances (montserrat.go) as CIDFontType2 fonts (cidfont.go,
	// writer.go), Identity-H encoded for broad Unicode coverage.
	FontMontserrat
)

// face bundles a font family with a weight for text measurement and
// glyph lookup; the zero value (FontHelvetica, not bold) is what every
// measurement call site used before FontMontserrat existed, so
// introducing this type changed no existing behavior.
type face struct {
	family FontFamily
	bold   bool
}

// Document is a complete PDF: its catalogue metadata plus the blocks
// to lay out.
//
// Title is used for the PDF Info dictionary, the cover page, and the
// running header a generic string, not an OSIRIS JSON field name,
// since this package carries no document-model assumptions. An
// embedded newline splits the running header into two rows instead
// of one (see drawHeaderBand); the cover wraps the same string
// naturally by width regardless, and the Info dictionary's /Title
// renders it as a single line (a control character, which a newline
// is, becomes a space there font.go's encodeRune). A document that
// wants an in-flow title block in addition to (or instead of) the
// cover starts its Blocks with a level-1 Heading, unaffected by
// Title. Subject appears on the cover,
// under the title, and in the Info dictionary. Landscape swaps the A4
// page dimensions, which suits a wide inventory table. CreationDate is
// an injected artifact time; the zero time omits it from the cover,
// the footer, and the Info dictionary, keeping the output byte-
// identical across runs. FontFamily defaults to FontHelvetica.
//
// FooterLeft and FooterRight are caller-supplied, generic lines for
// the running footer (a copyright notice, generator name and version,
// a purpose string whatever the caller's own document model wants
// to disclose); this package only places them, never interprets their
// content, with one exception: an embedded newline in FooterLeft (not
// FooterRight, whose own second line is always the timestamp and page
// number) starts a second footer line, the same convention
// Paragraph.Text already uses. They render under Title != "" the same
// as the cover and header; a Document with no title keeps the minimal
// page-number-only footer this package always had, and FooterLeft/
// FooterRight are ignored.
type Document struct {
	Title        string
	Subject      string
	Author       string
	Landscape    bool
	FontFamily   FontFamily
	CreationDate time.Time
	FooterLeft   string
	FooterRight  string
	Blocks       []Block
}

// A4 page dimensions in points.
const (
	a4Width  = 595.28
	a4Height = 841.89
)

// Write lays doc out and writes the resulting PDF to w.
func Write(w io.Writer, doc Document) error {
	for i, b := range doc.Blocks {
		t, ok := b.(Table)
		if !ok {
			continue
		}
		if len(t.Columns) == 0 {
			continue
		}
		for j, row := range t.Rows {
			if len(row) > len(t.Columns) {
				return fmt.Errorf("pdf: block %d row %d has %d cells, more than its %d header columns", i, j, len(row), len(t.Columns))
			}
		}
	}

	pw, ph := a4Width, a4Height
	if doc.Landscape {
		pw, ph = ph, pw
	}

	pages, outline, usage := layout(doc, pw, ph)
	return writeDocument(w, doc, pages, outline, usage, pw, ph)
}
