// This file emits the CIDFontType2 font objects a FontMontserrat
// Document needs: the Type0 dictionary Resources actually references,
// its CIDFontType2 descendant, a FontDescriptor, the embedded
// FontFile2 stream (the complete, unsubsetted TrueType font program
// see montserrat.go's header for why this package does not subset),
// and a ToUnicode CMap built from exactly the glyphs usage recorded
// (cidfont.go) during layout.
//
// Five objects per weight, at a fixed offset from the base object
// number writer.go allocates it: +0 Type0, +1 CIDFontType2,
// +2 FontDescriptor, +3 FontFile2, +4 ToUnicode.
package pdf

import (
	"fmt"
	"sort"
	"strings"
)

// cidFontObjects is how many PDF objects one weight's CIDFontType2
// font occupies see the file comment for what each one is.
const cidFontObjects = 5

// writeCIDFont emits one weight's font objects at base..base+4, via
// obj for the dictionary objects and objStream for the two stream
// objects (FontFile2 and the ToUnicode CMap).
func writeCIDFont(obj func(int, string), objStream func(int, string, []byte) error, f *ttfFont, bold bool, base int, usage *fontUsage) error {
	type0 := base
	cidFont := base + 1
	descriptor := base + 2
	fontFile := base + 3
	toUnicode := base + 4

	baseName := "Montserrat"
	if bold {
		baseName = "Montserrat-Bold"
	}

	scale := func(v int16) int { return int(v) * 1000 / int(f.unitsPerEm) }
	// StemV and CapHeight have no exact source in the tables this
	// package parses (no OS/2); these are the same kind of
	// commonly-used approximation most minimal PDF writers fall back
	// to when the real values are not at hand. Neither feeds glyph
	// rendering a reader draws the embedded outlines regardless
	// they only inform font substitution if the embedded font is
	// ever unavailable, which an embedded FontFile2 never is.
	stemV := 80
	if bold {
		stemV = 120
	}
	ascent := scale(f.ascent)
	descent := scale(f.descent)
	capHeight := int(float64(ascent) * 0.72)

	obj(descriptor, fmt.Sprintf(
		"<< /Type /FontDescriptor /FontName /%s /Flags 32 /FontBBox [%d %d %d %d] "+
			"/ItalicAngle 0 /Ascent %d /Descent %d /CapHeight %d /StemV %d /FontFile2 %d 0 R >>",
		baseName, scale(f.xMin), scale(f.yMin), scale(f.xMax), scale(f.yMax),
		ascent, descent, capHeight, stemV, fontFile))

	obj(cidFont, fmt.Sprintf(
		"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /%s "+
			"/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> "+
			"/FontDescriptor %d 0 R /CIDToGIDMap /Identity /DW %d /W %s >>",
		baseName, descriptor, f.advanceWidth1000(0), widthArray(f, usage.glyphs(bold))))

	obj(type0, fmt.Sprintf(
		"<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H "+
			"/DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>",
		baseName, cidFont, toUnicode))

	if err := objStream(fontFile, fmt.Sprintf("/Length1 %d ", len(f.raw)), f.raw); err != nil {
		return fmt.Errorf("pdf: embedding the %s font program: %w", baseName, err)
	}
	if err := objStream(toUnicode, "", []byte(toUnicodeCMap(usage.glyphs(bold)))); err != nil {
		return fmt.Errorf("pdf: writing the %s ToUnicode CMap: %w", baseName, err)
	}
	return nil
}

// widthArray builds a CIDFontType2's /W array from exactly the
// recorded glyphs: one "cid [w]" entry per glyph, sorted by CID for
// determinism (map iteration order is not).
func widthArray(f *ttfFont, glyphs map[uint16]rune) string {
	gids := sortedGIDs(glyphs)
	var b strings.Builder
	b.WriteByte('[')
	for i, gid := range gids {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%d [%d]", gid, f.advanceWidth1000(gid))
	}
	b.WriteByte(']')
	return b.String()
}

// toUnicodeCMap builds a ToUnicode CMap stream mapping each recorded
// glyph (its CID, since CIDToGIDMap is Identity) back to the Unicode
// value it represents what lets a reader extract or search the text
// a Montserrat-encoded page actually shows. Entries are chunked at
// 100 per beginbfchar/endbfchar block, the conventional limit several
// real-world readers expect even though the spec itself does not
// mandate one.
func toUnicodeCMap(glyphs map[uint16]rune) string {
	gids := sortedGIDs(glyphs)

	var b strings.Builder
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n" +
		"/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n" +
		"1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")

	for start := 0; start < len(gids); start += 100 {
		end := start + 100
		if end > len(gids) {
			end = len(gids)
		}
		fmt.Fprintf(&b, "%d beginbfchar\n", end-start)
		for _, gid := range gids[start:end] {
			fmt.Fprintf(&b, "<%04X> <%04X>\n", gid, glyphs[gid])
		}
		b.WriteString("endbfchar\n")
	}

	b.WriteString("endcmap\nCMapType 1 currentdict /CMap defineresource pop\nend\nend\n")
	return b.String()
}

// sortedGIDs returns glyphs' keys in ascending order, for a
// deterministic /W array and ToUnicode CMap regardless of Go's
// unordered map iteration.
func sortedGIDs(glyphs map[uint16]rune) []uint16 {
	gids := make([]uint16, 0, len(glyphs))
	for gid := range glyphs {
		gids = append(gids, gid)
	}
	sort.Slice(gids, func(i, j int) bool { return gids[i] < gids[j] })
	return gids
}
