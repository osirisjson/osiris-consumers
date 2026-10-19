// This file is the Montserrat-specific half of text handling: glyph
// lookup and width measurement over the embedded ttfFont values
// (montserrat.go), the Identity-H hex-string encoding a CIDFontType2
// content stream operand requires in place of a WinAnsi literal
// string, and the usage tracker writer.go reads to build a per-font
// width array and ToUnicode CMap from only the glyphs a document
// actually drew.
//
// Identity-H uses the glyph index itself as the character ID (CID),
// two bytes each, so a run of text becomes a hex string of 4-digit
// glyph indices rather than a string of WinAnsi bytes. This is also
// why resolving a rune's glyph is table.go's business for width, and
// this file's business for drawing: both must resolve the exact same
// rune to the exact same glyph, or the widths used to lay the text out
// would not match what gets drawn.
package pdf

import (
	"fmt"
	"strings"
)

// montserratFont returns the embedded Regular or Bold instance.
func montserratFont(bold bool) *ttfFont {
	if bold {
		return montserratBold
	}
	return montserratRegular
}

// montserratRuneWidth returns r's advance width in 1/1000 units for
// the given weight, resolved through the exact same montserratGlyph
// substitution rule drawing uses (control character to space,
// unrepresentable to '?') measurement and drawing must never
// disagree about which glyph, and so which width, a rune becomes.
func montserratRuneWidth(r rune, bold bool) int {
	gid, _ := montserratGlyph(r, bold)
	return montserratFont(bold).advanceWidth1000(gid)
}

// montserratGlyph resolves r to the glyph actually drawn for it: a
// space's glyph for an ASCII control character, r's own glyph
// otherwise when the font has one, or '?' for anything else the font
// cannot represent. It returns the resolved rune alongside the glyph
// index, since that is the Unicode value a ToUnicode CMap must report
// for this glyph what is visually on the page, not what the caller
// originally asked for.
//
// The control-character check runs before the font's own cmap lookup,
// not after: a real TrueType cmap can carry a genuine, non-notdef
// glyph for a C0 control code (Montserrat itself maps U+000D to one),
// and letting that win would put an invisible-but-"real" character
// into the output text stream a reader's text extraction then
// treats it as a line break or is dropped outright rather than as the
// plain whitespace font.go's encodeRune already treats it as for the
// WinAnsi path. Both paths must agree on this rule.
func montserratGlyph(r rune, bold bool) (gid uint16, resolved rune) {
	f := montserratFont(bold)
	if r < 32 || r == 127 {
		if g, ok := f.glyphIndex(' '); ok {
			return g, ' '
		}
	}
	if g, ok := f.glyphIndex(r); ok {
		return g, r
	}
	if g, ok := f.glyphIndex('?'); ok {
		return g, '?'
	}
	return 0, r
}

// montserratHexString returns s's Identity-H content-stream operand
// a hex string of one 2-byte glyph index per rune, framed in angle
// brackets and records each glyph it used in usage so writer.go can
// build that weight's width array and ToUnicode CMap afterward. usage
// may be nil (a face.family of FontHelvetica never calls this), in
// which case nothing is recorded.
func montserratHexString(usage *fontUsage, s string, bold bool) string {
	var b strings.Builder
	b.WriteByte('<')
	for _, r := range s {
		gid, resolved := montserratGlyph(r, bold)
		fmt.Fprintf(&b, "%04X", gid)
		if usage != nil {
			usage.record(bold, gid, resolved)
		}
	}
	b.WriteByte('>')
	return b.String()
}

// fontUsage accumulates, per weight, which glyphs a FontMontserrat
// document actually drew and which Unicode value each one represents.
// writer.go uses this to build a CIDFontType2's /W width array and its
// ToUnicode CMap from only what the document references the font
// program itself is still embedded whole (montserrat.go), but the
// bookkeeping around it stays proportional to the document's own text,
// not the font's full few-thousand-glyph repertoire.
type fontUsage struct {
	glyphUnicode [2]map[uint16]rune
}

// newFontUsage returns an empty tracker. Always allocated, even for a
// FontHelvetica document, so layout.go's page type never needs a nil
// check before handing it to page.text.
func newFontUsage() *fontUsage {
	return &fontUsage{glyphUnicode: [2]map[uint16]rune{{}, {}}}
}

func weightIndex(bold bool) int {
	if bold {
		return 1
	}
	return 0
}

func (u *fontUsage) record(bold bool, gid uint16, r rune) {
	u.glyphUnicode[weightIndex(bold)][gid] = r
}

// glyphs returns the weight's recorded glyph-to-Unicode map, never
// nil (possibly empty, when that weight was never drawn).
func (u *fontUsage) glyphs(bold bool) map[uint16]rune {
	return u.glyphUnicode[weightIndex(bold)]
}
