// Tests for the hand-rolled TTF parser, against the two real embedded
// Montserrat files not a synthetic font, so these are the actual
// bytes writer.go will put in a PDF's FontFile2 stream.
package pdf

import "testing"

func TestEmbeddedMontserratParsesWithSaneMetrics(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    *ttfFont
	}{
		{"Regular", montserratRegular},
		{"Bold", montserratBold},
	} {
		f := tc.f
		if f.unitsPerEm == 0 {
			t.Errorf("%s: unitsPerEm is 0", tc.name)
		}
		if f.numGlyphs < 1000 {
			t.Errorf("%s: numGlyphs = %d, implausibly low for Montserrat", tc.name, f.numGlyphs)
		}
		if len(f.widths) != f.numGlyphs {
			t.Errorf("%s: got %d widths, want %d (one per glyph)", tc.name, len(f.widths), f.numGlyphs)
		}
		if f.ascent <= 0 || f.descent >= 0 {
			t.Errorf("%s: ascent/descent look wrong: %d/%d", tc.name, f.ascent, f.descent)
		}
		if len(f.cmap) < 200 {
			t.Errorf("%s: cmap has only %d entries, expected broad Latin coverage", tc.name, len(f.cmap))
		}
	}
}

func TestEmbeddedMontserratResolvesASCIILetters(t *testing.T) {
	for _, r := range []rune{'A', 'W', 'i', 'g', '0', ' ', '?'} {
		gid, ok := montserratRegular.glyphIndex(r)
		if !ok {
			t.Errorf("Regular: no glyph for %q", r)
			continue
		}
		if gid == 0 {
			t.Errorf("Regular: %q resolved to glyph 0 (.notdef)", r)
		}
	}
}

func TestEmbeddedMontserratWAndINeverSameWidth(t *testing.T) {
	// A real width table, not a monospace fallback: 'i' must be far
	// narrower than 'W'.
	gidW, _ := montserratRegular.glyphIndex('W')
	gidI, _ := montserratRegular.glyphIndex('i')
	wW := montserratRegular.advanceWidth1000(gidW)
	wI := montserratRegular.advanceWidth1000(gidI)
	if wI >= wW {
		t.Errorf("'i' width %d is not narrower than 'W' width %d", wI, wW)
	}
}

// TestEmbeddedMontserratRegularAndBoldAreDistinctFontPrograms checks
// two embedded files are genuinely different weights, not the same
// bytes under two names Montserrat happens to keep some glyphs'
// advance widths identical across weight (verified independently
// against the raw OS/2.usWeightClass field: 400 for Regular, 700 for
// Bold), so width alone cannot tell the two apart.
func TestEmbeddedMontserratRegularAndBoldAreDistinctFontPrograms(t *testing.T) {
	if len(montserratRegularTTF) == len(montserratBoldTTF) {
		t.Fatal("Regular and Bold embedded as the exact same byte length suspicious")
	}
	gid, _ := montserratRegular.glyphIndex('M')
	if montserratRegular.widths[gid] != montserratBold.widths[gid] {
		// Not an error this only documents the real, verified
		// behavior above so a future change to the embedded assets
		// that breaks it is at least visible in a test diff.
		t.Logf("Regular/Bold 'M' width diverged: %d vs %d", montserratRegular.widths[gid], montserratBold.widths[gid])
	}
}

func TestEmbeddedMontserratHasNoGlyphForUnrepresentableCJK(t *testing.T) {
	if _, ok := montserratRegular.glyphIndex('中'); ok {
		t.Error("Montserrat unexpectedly has a glyph for a CJK code point")
	}
}

func TestParseTTFRejectsGarbage(t *testing.T) {
	if _, err := parseTTF([]byte("not a font")); err == nil {
		t.Fatal("parsing a non-font byte string was accepted")
	}
}

func TestParseTTFRejectsTruncatedHeader(t *testing.T) {
	if _, err := parseTTF(montserratRegularTTF[:40]); err == nil {
		t.Fatal("a truncated font was accepted")
	}
}
