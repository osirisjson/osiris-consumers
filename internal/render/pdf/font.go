// This file holds the text metrics the layout engine needs: the
// WinAnsiEncoding byte mapping used for every string written into a
// content stream, and the Adobe Core 14 AFM advance widths for
// Helvetica and Helvetica-Bold, the only two fonts this package uses.
//
// Widths are per 1000 units of text space, so a run's width in points
// is sum(width)/1000 * fontSize. Codes 32..126 carry their exact AFM
// value; codes outside that range fall back to a representative
// lowercase width, which can only make a wrap or a column fit slightly
// loose, never produce an invalid file.
package pdf

// Code points 32..126, in order, for Helvetica.
var helveticaASCII = [95]int{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556,
	278, 278, 584, 584, 584, 556, 1015,
	667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778, 667,
	778, 722, 667, 611, 722, 667, 944, 667, 667, 611,
	278, 278, 278, 469, 556, 333,
	556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556, 556,
	556, 333, 500, 278, 556, 500, 722, 500, 500, 500,
	334, 260, 334, 584,
}

// Code points 32..126, in order, for Helvetica-Bold.
var helveticaBoldASCII = [95]int{
	278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556,
	333, 333, 584, 584, 584, 611, 975,
	722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833, 722, 778, 667,
	778, 722, 667, 611, 722, 667, 944, 667, 667, 611,
	333, 278, 333, 584, 556, 333,
	556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, 611, 611, 611,
	611, 389, 556, 333, 611, 556, 778, 556, 556, 500,
	389, 280, 389, 584,
}

// Fallback advance for a code outside 32..126.
const (
	fallbackWidth     = 556
	fallbackBoldWidth = 611
)

// winAnsiHigh maps the runes that WinAnsiEncoding places in 0x80..0x9F
// (the range where CP1252 departs from Latin-1) to their byte. Every
// other rune in 0xA0..0xFF encodes as itself.
var winAnsiHigh = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84,
	'…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88,
	'‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C,
	'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93,
	'”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97,
	'˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B,
	'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// encodeRune returns r's WinAnsiEncoding byte, and whether r is
// representable at all. An unrepresentable rune becomes '?' rather
// than being dropped: a reader must never silently lose a character.
//
// An ASCII control character becomes a space instead. No font has a
// glyph for one, and a tab or a stray carriage return inside a
// document's own string value is whitespace in intent turning it
// into a space keeps words apart, where '?' would read as data the
// document never carried.
func encodeRune(r rune) (byte, bool) {
	switch {
	case r < 32 || r == 127:
		return ' ', true
	case r <= 126:
		return byte(r), true
	case r >= 0xA0 && r <= 0xFF:
		return byte(r), true
	}
	if b, ok := winAnsiHigh[r]; ok {
		return b, true
	}
	return '?', false
}

// runeWidth returns r's advance width in 1/1000 units for f. A
// FontMontserrat face is measured against the embedded font's own
// cmap/hmtx (montserrat.go, cidfont.go); every other value measures
// against the base-14 Helvetica AFM tables below.
func runeWidth(r rune, f face) int {
	if f.family == FontMontserrat {
		return montserratRuneWidth(r, f.bold)
	}
	b, _ := encodeRune(r)
	if b >= 32 && b <= 126 {
		if f.bold {
			return helveticaBoldASCII[b-32]
		}
		return helveticaASCII[b-32]
	}
	if f.bold {
		return fallbackBoldWidth
	}
	return fallbackWidth
}

// textWidth returns s's rendered width in points at size for f.
func textWidth(s string, size float64, f face) float64 {
	total := 0
	for _, r := range s {
		total += runeWidth(r, f)
	}
	return float64(total) * size / 1000
}
