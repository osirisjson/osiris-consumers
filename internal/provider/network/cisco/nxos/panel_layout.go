// This file places each confirmed-panel neighbour box and sizes the
// page around them. A neighbour's column position comes from its own
// local port's physical anchor on the illustration whenever that fits
// the panel's own width at the preferred spacing, so a reader can see
// at a glance which real port a box belongs to, falling back to an
// even left-to-right distribution across the panel's own width
// (preserving physical order, never fidelity to the exact anchor once
// compression is already unavoidable) only when it does not; a
// neighbour goes above the panel when its port sits in the
// illustration's own upper half, below when it sits in the lower half
// (panelAnchorFunc's own fy already carries that distinction).
//
// Growing the page past the fixed A3 landscape baseline is the last
// resort, not the first response to crowding: two immediate-neighbour
// SFP ports sit only a few page units apart at A3, so a page that grew
// every time two neighbours needed pushing apart would rarely stay at
// A3 at all. Compressing down to panelNeighbourMinPitch first (still
// comfortably readable, just tighter than the preferred
// panelNeighbourPitch) keeps every real fixture seen so far on a single
// A3 page; only a chassis with enough simultaneously active ports that
// even the minimum pitch cannot fit grows the page, and by how much is
// then a closed-form calculation, not a search.
package nxos

import "sort"

// panelNeighbourGap, panelNeighbourPitch, panelNeighbourMinPitch, and
// panelNeighbourBoxW are layout constants, in mxGraph user units:
// panelNeighbourGap is the clearance kept between the panel image's own
// top/bottom edge and a neighbour box placed above or below it;
// panelNeighbourPitch is the preferred center-to-center spacing between
// two neighbour boxes on the same side; panelNeighbourMinPitch is the
// floor that spacing may compress to, when the preferred pitch would
// not fit the panel's own width, before the page itself needs to grow;
// panelNeighbourBoxW is the neighbour box's own rendered width, wider
// than the iconIntrinsic-square Cisco19 stencil it holds (aspect=fixed
// keeps the stencil itself square, centered in the extra width) so a
// long CDP/LLDP remote system name has real room to wrap
// (neighbourSwitchStyle's own whiteSpace=wrap) instead of rendering as
// one unbroken line that can run off the page's own left edge.
// panelNeighbourMinPitch must stay above panelNeighbourBoxW or the
// compression fallback would overlap adjacent boxes.
const (
	panelNeighbourGap      = 160
	panelNeighbourPitch    = 160
	panelNeighbourMinPitch = 150
	panelNeighbourBoxW     = 140
)

// panelPlacement is one neighbour's computed box position (top-left, in
// mxGraph user units) and which side of the panel it landed on.
type panelPlacement struct {
	X, Y  int
	Above bool
}

// placePanelNeighbours computes neighbours box positions against a
// panel image already positioned at (panelX, panelY) sized
// panelW x panelH. Only neighbours anchor resolves are placed; the
// caller (renderPanelDiagram) has already filtered to those.
func placePanelNeighbours(neighbours []panelNeighbour, anchor panelAnchorFunc, panelX, panelY, panelW, panelH int) []panelPlacement {
	type candidate struct {
		index  int
		idealX int
	}
	var above, below []candidate
	for i, n := range neighbours {
		fx, fy, ok := anchor(n.LocalPort)
		if !ok {
			continue
		}
		idealX := panelX + int(fx*float64(panelW)) - panelNeighbourBoxW/2
		c := candidate{index: i, idealX: idealX}
		if fy < 0.5 {
			above = append(above, c)
		} else {
			below = append(below, c)
		}
	}

	out := make([]panelPlacement, len(neighbours))
	place := func(cands []candidate, y int, isAbove bool) {
		// The pitch-enforcement sweep below only works left to right:
		// cands arrives in neighbour-ID order (buildPanelNeighbours),
		// not physical position, so it must be sorted by idealX first
		// otherwise a port near the panel's left edge processed after
		// one near its right edge gets pushed arbitrarily far right of
		// its own true anchor.
		sort.Slice(cands, func(i, j int) bool { return cands[i].idealX < cands[j].idealX })
		idealXs := make([]int, len(cands))
		for i, c := range cands {
			idealXs[i] = c.idealX
		}
		xs := pitchEnforcedPositions(idealXs, panelNeighbourPitch)
		if n := len(xs); n > 0 && (xs[0] < panelX || xs[n-1]+panelNeighbourBoxW > panelX+panelW) {
			// The preferred pitch does not fit the panel's own width
			// (a busy run of adjacent ports): fall back to an even
			// spread across the full panel width, preserving the same
			// left-to-right physical order the anchors already gave.
			xs = distributeEvenly(n, panelX, panelW, panelNeighbourBoxW, panelNeighbourMinPitch)
		}
		for i, c := range cands {
			out[c.index] = panelPlacement{X: xs[i], Y: y, Above: isAbove}
		}
	}
	place(above, panelY-panelNeighbourGap-iconIntrinsic, true)
	place(below, panelY+panelH+panelNeighbourGap, false)
	return out
}

// pitchEnforcedPositions pushes each sorted idealX position right just
// enough to keep at least pitch between it and the previous one.
func pitchEnforcedPositions(idealX []int, pitch int) []int {
	out := make([]int, len(idealX))
	last := -1 << 62
	for i, x := range idealX {
		if last != -1<<62 && x-last < pitch {
			x = last + pitch
		}
		last = x
		out[i] = x
	}
	return out
}

// distributeEvenly spaces n items' own left edges starting at start,
// using the largest pitch that keeps the whole row within width
// (falling back to minPitch, and overflowing width only if even that
// does not fit, the caller is then past what this panel width can
// hold and must grow the page instead, see chooseConfirmedPanelPage).
func distributeEvenly(n, start, width, itemWidth, minPitch int) []int {
	out := make([]int, n)
	if n == 0 {
		return out
	}
	if n == 1 {
		out[0] = start
		return out
	}
	pitch := (width - itemWidth) / (n - 1)
	if pitch < minPitch {
		pitch = minPitch
	}
	for i := range out {
		out[i] = start + i*pitch
	}
	return out
}

// sideCounts reports how many neighbours anchor places above and below
// the panel, without computing their actual positions, the count
// alone is enough to size the page (chooseConfirmedPanelPage).
func sideCounts(neighbours []panelNeighbour, anchor panelAnchorFunc) (above, below int) {
	for _, n := range neighbours {
		_, fy, ok := anchor(n.LocalPort)
		if !ok {
			continue
		}
		if fy < 0.5 {
			above++
		} else {
			below++
		}
	}
	return above, below
}

// chooseConfirmedPanelPage returns the page size one confirmed-panel
// diagram (front or rear) needs: the fixed A3 landscape baseline
// (nxosA3Width x nxosA3Height), unless the busiest side's own neighbour
// count would need more than the panel's own A3 width even at
// panelNeighbourMinPitch, in which case placePanelNeighbours's own
// compression is not enough, and the page grows to exactly the width
// that count needs at that same minimum pitch (a closed-form
// calculation, since compression already absorbs every case that does
// not strictly need more room), climbing the ISO series and, past
// landscape A0, into a custom page (page.go's own never-clip policy).
// Height never grows from neighbour count alone: neighbours stack at
// most one row deep above and one row below the panel, so only width
// is content-fit here.
func chooseConfirmedPanelPage(neighbours []panelNeighbour, anchor panelAnchorFunc) PageInfo {
	above, below := sideCounts(neighbours, anchor)
	maxCount := above
	if below > maxCount {
		maxCount = below
	}
	neededPanelW := panelNeighbourBoxW
	if maxCount > 1 {
		neededPanelW = (maxCount-1)*panelNeighbourMinPitch + panelNeighbourBoxW
	}
	neededPageW := neededPanelW + 2*nxosPanelMarginX
	if neededPageW < nxosA3Width {
		neededPageW = nxosA3Width
	}
	return chooseNXOSPage(neededPageW, nxosA3Height)
}
