// Tests for the embedded logo asset and its SVG path parser, against
// the real file not a synthetic example.
package brand

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestEmbeddedLogoParsesWithExpectedShape(t *testing.T) {
	if len(Logo) != 8 {
		t.Fatalf("got %d paths, want 8 (one per glyph/bracket in the mark)", len(Logo))
	}
	if !almostEqual(LogoViewBoxWidth, 38.364581) || !almostEqual(LogoViewBoxHeight, 9.2604207) {
		t.Errorf("viewBox = %v x %v, want 38.364581 x 9.2604207", LogoViewBoxWidth, LogoViewBoxHeight)
	}

	colors := map[[3]float64]int{}
	for _, p := range Logo {
		colors[[3]float64{p.FillR, p.FillG, p.FillB}]++
		if len(p.Subpaths) == 0 {
			t.Error("a path has no subpaths")
		}
	}
	if len(colors) != 2 {
		t.Errorf("got %d distinct fill colors, want exactly 2 (near-black letters, gold brackets)", len(colors))
	}
}

// TestIBarIsAnExactRectangle hand-verifies the simplest path in the
// mark (the "I" letter, d="M 17.197917,1.3229162 V 7.937497 h
// 1.322917 V 1.3229162 Z") against coordinates worked out by hand
// from that same string, so this is checking the parser's own
// correctness, not just that it ran without error.
func TestIBarIsAnExactRectangle(t *testing.T) {
	var ops []LogoOp
	for _, p := range Logo {
		if len(p.Subpaths) == 1 && len(p.Subpaths[0].Ops) == 4 {
			first := p.Subpaths[0].Ops[0]
			if almostEqual(first.X, 17.197917) && almostEqual(first.Y, 1.3229162) {
				ops = p.Subpaths[0].Ops
				if !p.Subpaths[0].Closed {
					t.Error("the I-bar subpath should be closed (the d-string ends in Z)")
				}
			}
		}
	}
	if ops == nil {
		t.Fatal("could not find the I-bar path among the parsed paths")
	}

	want := []struct {
		kind LogoOpKind
		x, y float64
	}{
		{LogoMoveTo, 17.197917, 1.3229162},
		{LogoLineTo, 17.197917, 7.937497},  // V 7.937497 - x unchanged
		{LogoLineTo, 18.520834, 7.937497},  // h 1.322917 - relative, y unchanged
		{LogoLineTo, 18.520834, 1.3229162}, // V 1.3229162 - x unchanged
	}
	if len(ops) != len(want) {
		t.Fatalf("got %d ops, want %d", len(ops), len(want))
	}
	for i, w := range want {
		if ops[i].Kind != w.kind || !almostEqual(ops[i].X, w.x) || !almostEqual(ops[i].Y, w.y) {
			t.Errorf("op %d = %+v, want kind=%v x=%v y=%v", i, ops[i], w.kind, w.x, w.y)
		}
	}
}

// TestMoveToRepeatBecomesLineTo covers the real quirk three of the
// eight paths rely on: a second coordinate pair right after M, with
// no command letter between them, is an implicit lineto - not a
// second moveto starting a new subpath.
func TestMoveToRepeatBecomesLineTo(t *testing.T) {
	subpaths, err := parsePathData("M 2.6460932,9.2578499 2.6458334,8.2020803 Z")
	if err != nil {
		t.Fatalf("parsePathData: %v", err)
	}
	if len(subpaths) != 1 {
		t.Fatalf("got %d subpaths, want 1 (the implicit lineto must not start a new one)", len(subpaths))
	}
	ops := subpaths[0].Ops
	if len(ops) != 2 {
		t.Fatalf("got %d ops, want 2 (moveto + implicit lineto)", len(ops))
	}
	if ops[0].Kind != LogoMoveTo {
		t.Errorf("op 0 kind = %v, want LogoMoveTo", ops[0].Kind)
	}
	if ops[1].Kind != LogoLineTo || !almostEqual(ops[1].X, 2.6458334) || !almostEqual(ops[1].Y, 8.2020803) {
		t.Errorf("op 1 = %+v, want an implicit lineto to (2.6458334, 8.2020803)", ops[1])
	}
	if !subpaths[0].Closed {
		t.Error("subpath should be closed (the d-string ends in Z)")
	}
}

// TestScientificNotationCoordinate covers the right-curly-bracket
// path's "2.6e-4" coordinate - the only place in the real asset
// exponent notation appears.
func TestScientificNotationCoordinate(t *testing.T) {
	subpaths, err := parsePathData("m 35.718621,0.00128169 2.6e-4,1.05576951 Z")
	if err != nil {
		t.Fatalf("parsePathData: %v", err)
	}
	got := subpaths[0].Ops[1]
	wantX, wantY := 35.718621+2.6e-4, 0.00128169+1.05576951
	if !almostEqual(got.X, wantX) || !almostEqual(got.Y, wantY) {
		t.Errorf("got (%v, %v), want (%v, %v)", got.X, got.Y, wantX, wantY)
	}
}

func TestParseFillColor(t *testing.T) {
	r, g, b, err := parseFillColor("font-weight:bold;stroke-width:0.262276;fill:#ffbe19;fill-opacity:1")
	if err != nil {
		t.Fatalf("parseFillColor: %v", err)
	}
	if !almostEqual(r, 1.0) || !almostEqual(g, 190.0/255) || !almostEqual(b, 25.0/255) {
		t.Errorf("got (%v, %v, %v), want #ffbe19 as 0..1 RGB", r, g, b)
	}
}

func TestParseFillColorRejectsMissingFill(t *testing.T) {
	if _, _, _, err := parseFillColor("font-weight:bold"); err == nil {
		t.Fatal("a style with no fill declaration was accepted")
	}
}

func TestParsePathDataRejectsUnsupportedCommand(t *testing.T) {
	if _, err := parsePathData("M 0,0 A 1,1 0 0 1 2,2"); err == nil {
		t.Fatal("an arc command (A) was silently accepted")
	}
}

func TestParsePathDataRejectsMissingArguments(t *testing.T) {
	if _, err := parsePathData("M 0,0 L 1"); err == nil {
		t.Fatal("a lineto with only one argument was accepted")
	}
}

func TestParseViewBoxRejectsNonzeroOrigin(t *testing.T) {
	if _, _, err := parseViewBox("1 0 10 10"); err == nil {
		t.Fatal("a viewBox with a nonzero origin was accepted")
	}
}
