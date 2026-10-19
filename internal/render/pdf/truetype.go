// This file is a minimal, hand-rolled TrueType (sfnt) table parser
// no third-party dependency, same decision already taken for the PDF
// and .xlsx writers. It reads exactly the tables font.go needs to
// embed a TrueType font in a PDF as a CIDFontType2: the table
// directory, "head" (units per em, the glyph bounding box), "hhea"
// and "maxp" (ascent, descent, glyph count), "hmtx" (per-glyph advance
// widths) and "cmap" (a Unicode-to-glyph-index subtable, format 4,
// which is what every font this package embeds carries under
// platform 3 encoding 1 Windows, Unicode BMP).
//
// This is read-only, build-time use over a font file this package
// controls (see montserrat.go); it does not aim to parse an arbitrary
// caller-supplied font, so it rejects anything the embedded assets
// themselves do not need (a composite glyf outline, a variable font's
// "fvar", CFF/OpenType-CFF2 outlines, a format-12 cmap subtable for
// characters outside the Basic Multilingual Plane).
package pdf

import (
	"encoding/binary"
	"fmt"
)

// ttfFont is the subset of a TrueType font's own metrics this package
// needs: nothing here is a rendering engine, only the numbers a PDF
// FontDescriptor and a CIDFontType2's width array require.
type ttfFont struct {
	raw        []byte // the complete, unmodified font file, embedded as-is (FontFile2)
	unitsPerEm uint16
	ascent     int16
	descent    int16
	xMin, yMin int16
	xMax, yMax int16
	numGlyphs  int
	widths     []uint16        // advance width per glyph index, in font units
	cmap       map[rune]uint16 // Unicode code point -> glyph index; absent means no glyph
}

// parseTTF parses a complete sfnt-wrapped TrueType font.
func parseTTF(data []byte) (*ttfFont, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("pdf: truetype data too short (%d bytes)", len(data))
	}
	version := binary.BigEndian.Uint32(data[0:4])
	if version != 0x00010000 && version != 0x74727565 { // 1.0, or "true"
		return nil, fmt.Errorf("pdf: unsupported sfnt version %#08x", version)
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))

	tables := make(map[string][2]uint32, numTables)
	for i := 0; i < numTables; i++ {
		rec := data[12+16*i:]
		if len(rec) < 16 {
			return nil, fmt.Errorf("pdf: truncated table directory entry %d", i)
		}
		tag := string(rec[0:4])
		off := binary.BigEndian.Uint32(rec[8:12])
		length := binary.BigEndian.Uint32(rec[12:16])
		tables[tag] = [2]uint32{off, length}
	}

	required := []string{"head", "hhea", "maxp", "hmtx", "cmap"}
	for _, t := range required {
		if _, ok := tables[t]; !ok {
			return nil, fmt.Errorf("pdf: font is missing the required %q table", t)
		}
	}

	f := &ttfFont{raw: data}

	head := tableBytes(data, tables["head"])
	if len(head) < 44 {
		return nil, fmt.Errorf("pdf: truncated head table")
	}
	f.unitsPerEm = binary.BigEndian.Uint16(head[18:20])
	f.xMin = int16(binary.BigEndian.Uint16(head[36:38]))
	f.yMin = int16(binary.BigEndian.Uint16(head[38:40]))
	f.xMax = int16(binary.BigEndian.Uint16(head[40:42]))
	f.yMax = int16(binary.BigEndian.Uint16(head[42:44]))

	hhea := tableBytes(data, tables["hhea"])
	if len(hhea) < 36 {
		return nil, fmt.Errorf("pdf: truncated hhea table")
	}
	f.ascent = int16(binary.BigEndian.Uint16(hhea[4:6]))
	f.descent = int16(binary.BigEndian.Uint16(hhea[6:8]))
	numHMetrics := int(binary.BigEndian.Uint16(hhea[34:36]))

	maxp := tableBytes(data, tables["maxp"])
	if len(maxp) < 6 {
		return nil, fmt.Errorf("pdf: truncated maxp table")
	}
	f.numGlyphs = int(binary.BigEndian.Uint16(maxp[4:6]))

	widths, err := parseHmtx(tableBytes(data, tables["hmtx"]), numHMetrics, f.numGlyphs)
	if err != nil {
		return nil, err
	}
	f.widths = widths

	cmap, err := parseCmap(tableBytes(data, tables["cmap"]))
	if err != nil {
		return nil, err
	}
	f.cmap = cmap

	return f, nil
}

// tableBytes returns the slice of data a table directory entry
// describes, or nil if it runs past the end of the file.
func tableBytes(data []byte, rec [2]uint32) []byte {
	off, length := int(rec[0]), int(rec[1])
	if off < 0 || length < 0 || off+length > len(data) {
		return nil
	}
	return data[off : off+length]
}

// parseHmtx reads the per-glyph advance widths. A font may give an
// explicit entry to only its first numHMetrics glyphs and have every
// later glyph repeat the last one (the usual case for a monospaced
// tail of combining marks); this expands that out to one width per
// glyph so later lookups are a plain index.
func parseHmtx(hmtx []byte, numHMetrics, numGlyphs int) ([]uint16, error) {
	if numHMetrics <= 0 || numHMetrics*4 > len(hmtx) {
		return nil, fmt.Errorf("pdf: truncated hmtx table")
	}
	widths := make([]uint16, numGlyphs)
	last := uint16(0)
	for i := 0; i < numGlyphs; i++ {
		if i < numHMetrics {
			last = binary.BigEndian.Uint16(hmtx[4*i : 4*i+2])
		}
		widths[i] = last
	}
	return widths, nil
}

// parseCmap finds the font's Windows Unicode BMP subtable (platform 3,
// encoding 1) the one every embedded asset in this package carries
// and parses it as format 4. A subtable in any other format, or the
// subtable's own absence, is reported rather than silently producing
// an empty map: a font this package cannot read correctly must fail
// loudly, never render with wrong or missing glyphs.
func parseCmap(cmap []byte) (map[rune]uint16, error) {
	if len(cmap) < 4 {
		return nil, fmt.Errorf("pdf: truncated cmap table")
	}
	numSubtables := int(binary.BigEndian.Uint16(cmap[2:4]))
	var subOff uint32
	found := false
	for i := 0; i < numSubtables; i++ {
		rec := cmap[4+8*i:]
		if len(rec) < 8 {
			return nil, fmt.Errorf("pdf: truncated cmap encoding record %d", i)
		}
		platformID := binary.BigEndian.Uint16(rec[0:2])
		encodingID := binary.BigEndian.Uint16(rec[2:4])
		if platformID == 3 && encodingID == 1 {
			subOff = binary.BigEndian.Uint32(rec[4:8])
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("pdf: font has no Windows Unicode BMP cmap subtable")
	}
	if int(subOff)+2 > len(cmap) {
		return nil, fmt.Errorf("pdf: cmap subtable offset out of range")
	}
	sub := cmap[subOff:]
	format := binary.BigEndian.Uint16(sub[0:2])
	if format != 4 {
		return nil, fmt.Errorf("pdf: unsupported cmap subtable format %d", format)
	}
	return parseCmapFormat4(sub)
}

// parseCmapFormat4 parses a format-4 segment-mapping subtable per the
// OpenType specification: a sorted list of code-point ranges, each
// mapped to a glyph index either by a constant offset (idDelta) or
// through an explicit per-code-point array (idRangeOffset).
func parseCmapFormat4(sub []byte) (map[rune]uint16, error) {
	if len(sub) < 14 {
		return nil, fmt.Errorf("pdf: truncated cmap format 4 header")
	}
	segCount := int(binary.BigEndian.Uint16(sub[6:8])) / 2
	headerEnd := 14
	endCodeOff := headerEnd
	startCodeOff := endCodeOff + 2*segCount + 2 // +2 skips reservedPad
	idDeltaOff := startCodeOff + 2*segCount
	idRangeOff := idDeltaOff + 2*segCount
	glyphArrayOff := idRangeOff + 2*segCount
	if glyphArrayOff > len(sub) {
		return nil, fmt.Errorf("pdf: truncated cmap format 4 segment arrays")
	}

	out := make(map[rune]uint16)
	for i := 0; i < segCount; i++ {
		endCode := binary.BigEndian.Uint16(sub[endCodeOff+2*i:])
		startCode := binary.BigEndian.Uint16(sub[startCodeOff+2*i:])
		idDelta := int16(binary.BigEndian.Uint16(sub[idDeltaOff+2*i:]))
		idRangeOffset := binary.BigEndian.Uint16(sub[idRangeOff+2*i:])
		if startCode == 0xFFFF && endCode == 0xFFFF {
			continue // the mandatory terminator segment; carries no real mapping
		}
		for c := uint32(startCode); c <= uint32(endCode); c++ {
			var gid uint16
			if idRangeOffset == 0 {
				gid = uint16(uint32(int32(c) + int32(idDelta)))
			} else {
				// The pointer is relative to its own position in the
				// table, per spec - the classic "glyphIndexAddress"
				// calculation.
				addr := idRangeOff + 2*i + int(idRangeOffset) + 2*int(c-uint32(startCode))
				if addr+2 > len(sub) {
					continue
				}
				raw := binary.BigEndian.Uint16(sub[addr:])
				if raw == 0 {
					continue
				}
				gid = uint16(uint32(int32(raw) + int32(idDelta)))
			}
			if gid != 0 {
				out[rune(c)] = gid
			}
			if c == 0xFFFF {
				break // guard against uint32 wraparound on the rare segment ending there
			}
		}
	}
	return out, nil
}

// glyphIndex returns r's glyph index and whether the font has one.
func (f *ttfFont) glyphIndex(r rune) (uint16, bool) {
	gid, ok := f.cmap[r]
	return gid, ok
}

// advanceWidth1000 returns gid's advance width scaled to 1000 units
// per em the text-space convention this package's layout engine
// already uses for the base-14 metrics in font.go.
func (f *ttfFont) advanceWidth1000(gid uint16) int {
	if int(gid) >= len(f.widths) {
		return int(f.widths[0]) * 1000 / int(f.unitsPerEm)
	}
	return int(f.widths[gid]) * 1000 / int(f.unitsPerEm)
}
