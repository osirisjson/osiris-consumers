// Tests for the Montserrat glyph-resolution, width-measurement, and
// Identity-H encoding path (cidfont.go), and the resulting document
// structure writer.go builds from it (cidwriter.go).
package pdf

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func montserratDoc(blocks ...Block) Document {
	return Document{Title: "Montserrat test", FontFamily: FontMontserrat, Blocks: blocks}
}

func TestMontserratGlyphControlCharacterBeatsTheFontsOwnCmap(t *testing.T) {
	// Regression: Montserrat's own cmap maps U+000D (carriage return)
	// to a real, non-notdef glyph. Letting that win over the
	// control-character rule put an invisible-but-"real" glyph with
	// ToUnicode U+000D into the output a reader's text extraction
	// then treats it as a line break instead of as plain whitespace,
	// which is what font.go's WinAnsi path already guarantees for the
	// same input. Both paths must resolve a control character to the
	// space glyph before ever consulting the font's own cmap.
	spaceGid, _ := montserratRegular.glyphIndex(' ')
	for _, r := range []rune{'\t', '\r', '\n', 0, 27} {
		gid, resolved := montserratGlyph(r, false)
		if resolved != ' ' {
			t.Errorf("montserratGlyph(%q) resolved to %q, want ' '", r, resolved)
		}
		if gid != spaceGid {
			t.Errorf("montserratGlyph(%q) used glyph %d, want the space glyph %d", r, gid, spaceGid)
		}
	}
}

func TestMontserratGlyphUnrepresentableBecomesQuestionMark(t *testing.T) {
	gid, resolved := montserratGlyph('中', false)
	wantGid, _ := montserratRegular.glyphIndex('?')
	if resolved != '?' || gid != wantGid {
		t.Errorf("montserratGlyph(CJK) = (%d, %q), want (%d, '?')", gid, resolved, wantGid)
	}
}

func TestMontserratRuneWidthMatchesWhatGetsDrawn(t *testing.T) {
	// Measurement and drawing must resolve a rune to the exact same
	// glyph, otherwise column fitting and line wrapping would be
	// computed against a different shape than what actually appears.
	for _, r := range []rune{'A', 'i', '\t', '中'} {
		gid, _ := montserratGlyph(r, false)
		want := montserratRegular.advanceWidth1000(gid)
		if got := montserratRuneWidth(r, false); got != want {
			t.Errorf("montserratRuneWidth(%q) = %d, want %d (the drawn glyph's own width)", r, got, want)
		}
	}
}

func TestMontserratHexStringRecordsUsage(t *testing.T) {
	usage := newFontUsage()
	hex := montserratHexString(usage, "Ai", false)
	if !strings.HasPrefix(hex, "<") || !strings.HasSuffix(hex, ">") {
		t.Fatalf("hex string %q is not angle-bracket framed", hex)
	}
	if len(hex) != 1+4+4+1 { // '<' + 2 glyphs * 4 hex digits + '>'
		t.Errorf("hex string %q has an unexpected length for 2 glyphs", hex)
	}
	if len(usage.glyphs(false)) != 2 {
		t.Errorf("usage recorded %d glyphs, want 2", len(usage.glyphs(false)))
	}
	gidA, _ := montserratRegular.glyphIndex('A')
	if usage.glyphs(false)[gidA] != 'A' {
		t.Errorf("usage did not record 'A' against its own glyph")
	}
}

func TestMontserratDocumentValidates(t *testing.T) {
	out := render(t, montserratDoc(
		Heading{Level: 1, Text: "Montserrat title"},
		Paragraph{Text: "test €120 - résumé"},
		Heading{Level: 2, Text: "Section"},
		Table{Columns: []string{"Name", "Value"}, Rows: [][]string{{"a\tb\rc", "x"}}},
	))

	for _, want := range []string{
		"/Subtype /Type0", "/Encoding /Identity-H", "/Subtype /CIDFontType2",
		"/CIDToGIDMap /Identity", "/FontFile2", "/BaseFont /Montserrat", "/BaseFont /Montserrat-Bold",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("Montserrat document is missing %q", want)
		}
	}
	if bytes.Contains(out, []byte("/BaseFont /Helvetica")) {
		t.Error("a FontMontserrat document should not reference Helvetica at all")
	}
}

func TestMontserratDocumentIsLargerThanTheEquivalentHelveticaOne(t *testing.T) {
	// The whole, unsubsetted font program is embedded (montserrat.go's
	// documented tradeoff) a Montserrat document is expected to be
	// substantially larger than the same content under Helvetica, not
	// a rounding difference.
	blocks := []Block{Heading{Level: 1, Text: "Compare"}, Paragraph{Text: "Some body text."}}
	helv := render(t, Document{Title: "Compare", Blocks: blocks})
	mont := render(t, montserratDoc(blocks...))
	if len(mont) < len(helv)+200000 {
		t.Errorf("Montserrat doc is %d bytes, Helvetica %d bytes expected at least 200KB from the two embedded (Flate-compressed) font programs",
			len(mont), len(helv))
	}
}

func TestMontserratDocumentIsDeterministic(t *testing.T) {
	doc := montserratDoc(Heading{Level: 1, Text: "Stable"}, Paragraph{Text: "Body."})
	if a, b := render(t, doc), render(t, doc); !bytes.Equal(a, b) {
		t.Error("two renders of the same Montserrat document differ")
	}
}

func TestMontserratWidthArrayAndToUnicodeAreSortedAndWellFormed(t *testing.T) {
	out := render(t, montserratDoc(Heading{Level: 1, Text: "AbC"}))

	// The array nests one bracket pair per glyph ("cid [w]"), so the
	// outer bracket cannot be found by stopping at the first "]"
	// match the whole repeated "cid [w]" run instead.
	wRe := regexp.MustCompile(`/W \[((?:\d+ \[\d+\] ?)+)\]`)
	m := wRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no /W array found")
	}
	entryRe := regexp.MustCompile(`(\d+) \[(\d+)\]`)
	entries := entryRe.FindAllStringSubmatch(m[1], -1)
	if len(entries) == 0 {
		t.Fatal("/W array has no entries")
	}
	last := -1
	for _, e := range entries {
		gid, err := strconv.Atoi(e[1])
		if err != nil {
			t.Fatalf("unparsable CID %q", e[1])
		}
		if gid <= last {
			t.Errorf("/W array is not sorted ascending: %d after %d", gid, last)
		}
		last = gid
	}
}
