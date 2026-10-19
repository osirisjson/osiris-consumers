// Tests for the cover page, running header, and running footer
// (layout.go's addCoverPage/drawHeaderBand/footers) and the logo
// vector drawing they share (logo.go). The actual shape/placement of
// the logo was verified visually during development (rasterized and
// compared against the source SVG); these tests check the structural
// guarantees what's present on which page, and what is not.
package pdf

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func coverDoc(extra func(*Document)) Document {
	d := Document{
		Title:        "Cover test report",
		Subject:      "A test subject line",
		CreationDate: time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC),
		FooterLeft:   "(c) 2026 https://osirisjson.org",
		FooterRight:  "generator-x v1.2.3 - purpose: testing",
		Blocks:       []Block{Heading{Level: 2, Text: "Section"}, Paragraph{Text: "Body text."}},
	}
	if extra != nil {
		extra(&d)
	}
	return d
}

func TestCoverPageAddsExactlyOnePage(t *testing.T) {
	blocks := []Block{Heading{Level: 2, Text: "Section"}, Paragraph{Text: "Body."}}
	withoutTitle := render(t, Document{Blocks: blocks})
	withTitle := render(t, Document{Title: "Has a cover", Blocks: blocks})

	n1 := len(contentStreams(t, withoutTitle))
	n2 := len(contentStreams(t, withTitle))
	if n2 != n1+1 {
		t.Errorf("setting Title changed page count from %d to %d, want exactly +1 (the cover)", n1, n2)
	}
}

func TestNoCoverWhenTitleEmpty(t *testing.T) {
	out := render(t, Document{Blocks: []Block{Paragraph{Text: "no title here"}}})
	streams := contentStreams(t, out)
	if len(streams) != 1 {
		t.Fatalf("got %d pages, want 1", len(streams))
	}
	// Logo fill colors (see TestLogoFillColorsAppearOnCoverAndHeader)
	// must not appear anywhere when there is no cover and no header.
	if strings.Contains(streams[0], goldFillOperand()) {
		t.Error("a Title-less document drew the logo, but it should have neither a cover nor a header")
	}
}

func goldFillOperand() string {
	return num(1) + " " + num(190.0/255) + " " + num(25.0/255) + " rg"
}

func blackFillOperand() string {
	return num(8.0/255) + " " + num(8.0/255) + " " + num(8.0/255) + " rg"
}

func TestLogoFillColorsAppearOnCoverAndHeader(t *testing.T) {
	out := render(t, coverDoc(nil))
	streams := contentStreams(t, out)
	if len(streams) < 2 {
		t.Fatalf("got %d pages, want at least 2 (cover + one content page)", len(streams))
	}
	for i, s := range streams[:2] {
		if !strings.Contains(s, goldFillOperand()) || !strings.Contains(s, blackFillOperand()) {
			t.Errorf("page %d is missing one of the logo's two fill colors", i+1)
		}
	}
}

func TestCoverShowsTitleSubjectAndTimestamp(t *testing.T) {
	out := render(t, coverDoc(nil))
	streams := contentStreams(t, out)
	cover := streams[0]
	for _, want := range []string{"(Cover test report) Tj", "(A test subject line) Tj", "(Generated 2026-10-14T09:30:00Z) Tj"} {
		if !strings.Contains(cover, want) {
			t.Errorf("cover page is missing %q", want)
		}
	}
}

func TestCoverTitleTwoRowsAtDifferentSizes(t *testing.T) {
	out := render(t, coverDoc(func(d *Document) {
		d.Title = "OSIRIS JSON Example report for\ntest-abc1234"
	}))
	cover := contentStreams(t, out)[0]
	row1 := fmt.Sprintf("BT /F2 %s Tf", num(coverTitleSize))
	row2 := fmt.Sprintf("BT /F2 %s Tf", num(coverTitleSize2))
	if !strings.Contains(cover, row1) {
		t.Errorf("cover is missing the row-1 title at coverTitleSize (%v)", coverTitleSize)
	}
	if !strings.Contains(cover, row2) {
		t.Errorf("cover is missing the row-2 title at coverTitleSize2 (%v)", coverTitleSize2)
	}
	for _, want := range []string{"(OSIRIS JSON Example report for) Tj", "(test-abc1234) Tj"} {
		if !strings.Contains(cover, want) {
			t.Errorf("cover is missing %q", want)
		}
	}
}

func TestCoverOmitsSubjectAndTimestampWhenUnset(t *testing.T) {
	out := render(t, coverDoc(func(d *Document) {
		d.Subject = ""
		d.CreationDate = time.Time{}
	}))
	cover := contentStreams(t, out)[0]
	if strings.Contains(cover, "(A test subject line) Tj") {
		t.Error("cover drew a subject line that was never set")
	}
	if strings.Contains(cover, "Generated") {
		t.Error("cover drew a timestamp line although CreationDate was zero")
	}
}

func TestCoverHasNoHeaderOrFooterContent(t *testing.T) {
	out := render(t, coverDoc(nil))
	cover := contentStreams(t, out)[0]
	for _, mustNotContain := range []string{"(c) 2026 https://osirisjson.org", "generator-x v1.2.3", "Page 1 of"} {
		if strings.Contains(cover, mustNotContain) {
			t.Errorf("cover page unexpectedly contains %q (that belongs on content pages only)", mustNotContain)
		}
	}
}

func TestContentPagesShowHeaderTitleAndFooterContent(t *testing.T) {
	out := render(t, coverDoc(nil))
	streams := contentStreams(t, out)
	if len(streams) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(streams))
	}
	page2 := streams[1]
	for _, want := range []string{
		"(Cover test report) Tj",                     // header title
		`(\(c\) 2026 https://osirisjson.org) Tj`,     // footer left - '(' ')' are PDF-escaped
		"(generator-x v1.2.3 - purpose: testing) Tj", // footer right, line 1
		"(2026-10-14T09:30:00Z) Tj",                  // footer right, line 2 (timestamp)
		"(Page 2 of",                                 // footer right, line 2 (pagination)
	} {
		if !strings.Contains(page2, want) {
			t.Errorf("content page is missing %q", want)
		}
	}
}

func TestHeaderTitleNewlineBecomesTwoRows(t *testing.T) {
	out := render(t, coverDoc(func(d *Document) {
		d.Title = "OSIRIS JSON Example report for\ntest-abc1234"
	}))
	page2 := contentStreams(t, out)[1]
	for _, want := range []string{"(OSIRIS JSON Example report for) Tj", "(test-abc1234) Tj"} {
		if !strings.Contains(page2, want) {
			t.Errorf("header is missing %q", want)
		}
	}
	// The full, un-split string must never appear as a single run
	// that would mean the newline leaked into one Tj instead of
	// becoming two rows.
	if strings.Contains(page2, "(OSIRIS JSON Example report for\\012test") {
		t.Error("header drew the title as one run instead of two rows")
	}
}

func TestHeaderTitleSingleLineUnaffectedByTwoRowSupport(t *testing.T) {
	// coverDoc's Title has no newline; the header must still render it
	// as one row, exactly as before two-row support existed.
	out := render(t, coverDoc(nil))
	page2 := contentStreams(t, out)[1]
	if !strings.Contains(page2, "(Cover test report) Tj") {
		t.Error("single-line header title regressed")
	}
}

func TestFooterOmitsTimestampWhenCreationDateZero(t *testing.T) {
	out := render(t, coverDoc(func(d *Document) { d.CreationDate = time.Time{} }))
	page2 := contentStreams(t, out)[1]
	if !strings.Contains(page2, "(Page 2 of") {
		t.Error("pagination should still render even with no timestamp")
	}
	if strings.Contains(page2, "2026-10-14") {
		t.Error("footer drew a timestamp although CreationDate was zero")
	}
}

func TestFooterLeftAndRightAreIndependentlyOptional(t *testing.T) {
	out := render(t, coverDoc(func(d *Document) { d.FooterLeft = ""; d.FooterRight = "" }))
	page2 := contentStreams(t, out)[1]
	if strings.Contains(page2, "https://osirisjson.org") || strings.Contains(page2, "generator-x") {
		t.Error("footer drew left/right content that was never set")
	}
	// Pagination must still be present regardless.
	if !strings.Contains(page2, "(Page 2 of") {
		t.Error("pagination is missing even though it does not depend on FooterLeft/FooterRight")
	}
}

func TestFooterLeftNewlineBecomesASecondLine(t *testing.T) {
	out := render(t, coverDoc(func(d *Document) {
		d.FooterLeft = "OSIRIS JSON\nOpen Specification for Infrastructure Resource Interchange Schema."
	}))
	page2 := contentStreams(t, out)[1]
	for _, want := range []string{"(OSIRIS JSON) Tj", "(Open Specification for Infrastructure Resource Interchange Schema.) Tj"} {
		if !strings.Contains(page2, want) {
			t.Errorf("content page is missing %q", want)
		}
	}
}

func TestFooterRightIgnoresNewlineSinceItsOwnSecondLineIsFixed(t *testing.T) {
	// FooterRight's own second row always belongs to the timestamp and
	// "Page X of N" an embedded newline must not try to claim it.
	out := render(t, coverDoc(func(d *Document) { d.FooterRight = "line one\nline two" }))
	page2 := contentStreams(t, out)[1]
	if strings.Contains(page2, "(line two) Tj") {
		t.Error("FooterRight's newline produced a second line, colliding with the timestamp/pagination row")
	}
}

func TestDocumentWithNoTitleKeepsTheOriginalMinimalFooter(t *testing.T) {
	// The exact pre-cover-feature behavior: no title on the left, just
	// "Page X of N" on the right, single line.
	out := render(t, Document{
		FooterLeft:  "should be ignored, no cover",
		FooterRight: "should also be ignored",
		Blocks:      []Block{Paragraph{Text: "body"}},
	})
	page := contentStreams(t, out)[0]
	if strings.Contains(page, "should be ignored") || strings.Contains(page, "should also be ignored") {
		t.Error("FooterLeft/FooterRight rendered on a Document with no Title (no cover)")
	}
	if !strings.Contains(page, "(Page 1 of 1) Tj") {
		t.Error("the minimal page-number footer is missing")
	}
}

func TestInfoDictStillCarriesAuthorRegardlessOfCover(t *testing.T) {
	out := render(t, coverDoc(func(d *Document) { d.Author = "Tia Zanella" }))
	if !bytes.Contains(out, []byte("/Author (Tia Zanella)")) {
		t.Error("Info dictionary is missing /Author")
	}
}
