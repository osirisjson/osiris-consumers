// Tests for the Cisco APIC page (tile) size choice: one page unit per
// pod column, climbing A3 -> A2 -> A1 -> A0 -> custom, driven by each
// pod's spine row (which never wraps) and leaf row count (which can).

package apic

import "testing"

func TestRowCapacityMatchesTheReferenceMockup(t *testing.T) {
	// The mockup packs 13 icons into a leaf row at a
	// column width close to landscape A3's 1654.
	if got := rowCapacity(1654, apicIconSize); got != 13 {
		t.Errorf("rowCapacity(1654, apicIconSize) = %d, want 13", got)
	}
}

func TestSwimlaneHeightMatchesTheReferenceMockup(t *testing.T) {
	cases := []struct {
		rows, bottomPad, want int
	}{
		{1, apicBottomPadModel, 200},   // one-row switch swimlane (e.g. SPINE POD-1).
		{2, apicBottomPadModel, 320},   // two-row switch swimlane (e.g. LEAF POD-1).
		{1, apicBottomPadNoModel, 160}, // one-row controller swimlane.
	}
	for _, tc := range cases {
		if got := swimlaneHeight(tc.rows, tc.bottomPad); got != tc.want {
			t.Errorf("swimlaneHeight(%d, %d) = %d, want %d", tc.rows, tc.bottomPad, got, tc.want)
		}
	}
}

func TestChooseAPICPageStartsAtA3ForTheReferenceFabric(t *testing.T) {
	// Mirrors the mockup: 2 pods, 3 controllers, 2 spines and
	// 10/10 leafs per pod.
	pods := []podFabricCounts{{Spines: 2, Leafs: 10}, {Spines: 2, Leafs: 10}}
	got := chooseAPICPage(2, 3, pods)
	if got.Format != "A3" {
		t.Errorf("Format = %q, want A3 for the reference fabric", got.Format)
	}
	if got.ExceedsISOA0() {
		t.Error("ExceedsISOA0() = true, want false")
	}
}

func TestChooseAPICPageEscalatesWidthWhenASpineRowWouldWrap(t *testing.T) {
	// 14 spines in one pod cannot fit landscape A3's 13-icon row
	// capacity, even though the leaf count is trivial, width, not
	// height, must drive the escalation here.
	pods := []podFabricCounts{{Spines: 14, Leafs: 1}}
	got := chooseAPICPage(1, 2, pods)
	if got.Format == "A3" {
		t.Errorf("Format = A3, want a wider tier: 14 spines exceed A3's row capacity")
	}
	colW := colRenderWidth(got.Width, 1)
	if rowCapacity(colW, apicIconSize) < 14 {
		t.Errorf("chosen width %d (column render width %d) gives capacity %d, want >= 14", got.Width, colW, rowCapacity(colW, apicIconSize))
	}
}

func TestChooseAPICPageEscalatesHeightWhenLeafsWrapTooManyRows(t *testing.T) {
	// A huge leaf count in one pod, at A3's width, needs far more rows
	// than A3's height allows.
	pods := []podFabricCounts{{Spines: 2, Leafs: 500}}
	got := chooseAPICPage(1, 2, pods)
	if got.Format == "A3" {
		t.Errorf("Format = A3, want a taller tier: 500 leafs at A3 width need more rows than A3 height allows")
	}
}

func TestChooseAPICPageNeverClipsAndStaysLandscape(t *testing.T) {
	for _, pods := range [][]podFabricCounts{
		{{Spines: 2, Leafs: 10}, {Spines: 2, Leafs: 10}},
		{{Spines: 40, Leafs: 5000}},
		{{Spines: 1, Leafs: 1}},
	} {
		got := chooseAPICPage(len(pods), 3, pods)
		colW := colRenderWidth(got.Width, len(pods))
		if !fitsSpineWidth(pods, colW) {
			t.Errorf("chooseAPICPage(%v) chose width %d (column render width %d), a pod's spine row does not fit", pods, got.Width, colW)
		}
		totalW := totalCanvasWidth(len(pods), got.Width)
		if h := stackHeight(pods, colW, 3, totalW); h > got.Height {
			t.Errorf("chooseAPICPage(%v) chose height %d, content needs %d", pods, got.Height, h)
		}
		if got.Width < got.Height {
			t.Errorf("chooseAPICPage(%v) = %dx%d is not landscape", pods, got.Width, got.Height)
		}
	}
}

func TestChooseAPICPageFallsBackToCustomAboveA0(t *testing.T) {
	pods := []podFabricCounts{{Spines: 100, Leafs: 20000}}
	got := chooseAPICPage(1, 3, pods)
	if got.Format != "custom" {
		t.Errorf("Format = %q, want custom for a fabric this large", got.Format)
	}
	if !got.ExceedsISOA0() {
		t.Error("ExceedsISOA0() = false, want true")
	}
}
