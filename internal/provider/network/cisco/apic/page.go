// This file chooses the draw.io page (tile) size for the Cisco APIC
// fabric topology. Unlike a single-page renderer, this topology tiles
// one page unit per pod horizontally: a document with N pods spans
// exactly N page tiles side by side ("double A3", "triple A3", ...), so
// the chosen PageInfo.Width is both one column's
// slot pitch and the mxGraphModel pageWidth. Each column's own rendered
// swimlane width is one apicColumnGap narrower than its slot (when
// there is more than one column) so a visible gap separates it from the
// next column without pushing the overall canvas past its N-tile width.
//
// Since tipically a Cisco APIC topology contain many resources the
// climb starts at A3 instead of A4 (this provider's declared default
// paper size) and goes A3 -> A2 -> A1 -> A0 -> custom.
// A tier is chosen only for its width and height fitting every pod's
// fabric stack: a pod's spine row must fit its column's rendered width
// in one row (spines never wrap to a second row it would hurt
// spine-to-leaf line readability), and the full controller/spine/leaf
// stack, using that tier's rendered column width to compute how many
// rows a pod's leafs wrap into, must fit the tier's height.
// The tier that fits is applied uniformly to every
// column and to the controller swimlane spanning all of them.
package apic

import "math"

// Layout geometry, in mxGraph user units (see the package doc comment
// in topology.go for the full placement rules this implements).
const (
	apicIconSize            = 40 // icon height, universally, and icon width for a spine or leaf switch.
	apicControllerIconWidth = 22 // a controller's icon width at 40pt height: mxgraph.cisco19.server2 is not natively square, this is its confirmed aspect-correct width.
	apicMinGap              = 80
	apicPitch               = apicIconSize + apicMinGap // 120: minimum row/column pitch for a spine or leaf switch row.
	apicMargin              = 40                        // left/right margin, and top margin below the swimlane title bar.
	apicHeaderH             = 40                        // swimlane title bar height.
	apicColumnGap           = 80                        // visible gap reserved inside each column's slot before the next column.
	apicRowStackGap         = 160                       // vertical gap between the controller/spine/leaf swimlanes.

	// apicBottomPadModel is the bottom clearance a switch row needs:
	// its 3-line label (model/name/oob_mgmt_addr) read better with 80pt
	// below the last row. apicBottomPadNoModel is the smaller clearance
	// a controller row needs, whose 2-line label omits the model line.
	apicBottomPadModel   = 80
	apicBottomPadNoModel = 40
)

// PageInfo is one chosen page (tile) size, in mxGraph user units.
// Format is an ISO 216 name ("A3".."A0") or "custom"; Width and Height
// are always landscape (Width >= Height). Content is never scaled or
// dropped to fit.
type PageInfo struct {
	Format string
	Width  int
	Height int
}

// ExceedsISOA0 reports whether the fabric was larger than landscape A0
// and therefore needed a custom tile size.
func (p PageInfo) ExceedsISOA0() bool { return p.Format == "custom" }

// apicISOLandscape is the A3..A0 progression in landscape orientation,
// at draw.io's 100-DPI convention. This provider's declared default is
// A3 (unlike internal/render/drawio and arubacentral, which start their
// own climbs at A5/A4); there is no smaller step to fall back to.
var apicISOLandscape = []PageInfo{
	{"A3", 1654, 1169},
	{"A2", 2336, 1654},
	{"A1", 3300, 2336},
	{"A0", 4681, 3300},
}

// rowCapacity returns how many iconWidth-wide icons fit in a single row
// of a container width wide, given apicMargin left/right margins and a
// minimum apicMinGap between icons. It returns 0 when not even one icon
// fits.
func rowCapacity(width, iconWidth int) int {
	inner := width - 2*apicMargin
	if inner < iconWidth {
		return 0
	}
	return (inner-iconWidth)/(iconWidth+apicMinGap) + 1
}

// minWidthForCapacity returns the smallest container width that gives
// rowCapacity at least cap for iconWidth-wide icons.
// It is rowCapacity inverse.
func minWidthForCapacity(cap, iconWidth int) int {
	if cap <= 0 {
		return 2 * apicMargin
	}
	return cap * (iconWidth + apicMinGap)
}

// colRenderWidth returns a column's own rendered swimlane width for a
// document with numPods columns at slot pitch tierWidth: one
// apicColumnGap narrower than the slot when there is more than one
// column (so a visible gap separates it from the next column), or the
// full slot when there is only one (nothing to separate it from).
func colRenderWidth(tierWidth, numPods int) int {
	if numPods > 1 {
		return tierWidth - apicColumnGap
	}
	return tierWidth
}

// rowsForCount returns how many rows count items need at the given
// per-row capacity. It returns 0 for a zero count (no row, no height
// contribution beyond the swimlane's own header and margins).
func rowsForCount(count, capacity int) int {
	if count <= 0 || capacity <= 0 {
		return 0
	}
	return (count + capacity - 1) / capacity
}

// swimlaneHeight returns the height of a swimlane holding rows of
// icons, with bottomPad clearance below the last row
// (apicBottomPadModel for a 3-line switch label, apicBottomPadNoModel
// for a 2-line controller label). A zero-row swimlane (no resources of
// that role) still reserves one row's worth of height so an
// unexpectedly empty role does not collapse the layout; callers never
// emit a swimlane with zero resources in the first place (apic never
// skips data it has, but it also never invents a swimlane for data it
// does not have).
func swimlaneHeight(rows, bottomPad int) int {
	if rows < 1 {
		rows = 1
	}
	return apicHeaderH + apicMargin + rows*apicIconSize + (rows-1)*apicMinGap + bottomPad
}

// podFabricCounts is one pod's controller-independent fabric census:
// how many spine and leaf role-bearing switches it has.
type podFabricCounts struct {
	Spines int
	Leafs  int
}

// fits reports whether every pod in pods has a spine count that fits
// one row at colWidth (a column's own rendered width, see
// colRenderWidth), so no pod's spine row would need to wrap.
func fitsSpineWidth(pods []podFabricCounts, colWidth int) bool {
	cap := rowCapacity(colWidth, apicIconSize)
	if cap == 0 {
		return false
	}
	for _, p := range pods {
		if p.Spines > cap {
			return false
		}
	}
	return true
}

// stackHeight returns the total vertical span of the controller,
// spine, and leaf swimlanes for the tallest pod, at the given column's
// own rendered width colWidth (used to compute each pod's leaf row
// count) and controller count (used with totalWidth, the full
// controller-swimlane span, to compute the controller row count).
func stackHeight(pods []podFabricCounts, colWidth, controllerCount, totalWidth int) int {
	controllerRows := rowsForCount(controllerCount, rowCapacity(totalWidth, apicControllerIconWidth))
	controllerH := swimlaneHeight(controllerRows, apicBottomPadNoModel)
	spineH := swimlaneHeight(1, apicBottomPadModel) // spines are always exactly one row.
	leafCap := rowCapacity(colWidth, apicIconSize)
	maxLeafH := 0
	for _, p := range pods {
		h := swimlaneHeight(rowsForCount(p.Leafs, leafCap), apicBottomPadModel)
		if h > maxLeafH {
			maxLeafH = h
		}
	}
	return controllerH + apicRowStackGap + spineH + apicRowStackGap + maxLeafH
}

// totalCanvasWidth returns the full controller-swimlane span for
// numPods columns at slot pitch tierWidth: the sum of every column's
// own rendered width (colRenderWidth) plus the gaps between them, one
// fewer gap than columns, since the last column has no next column to
// separate itself from.
func totalCanvasWidth(numPods, tierWidth int) int {
	if numPods <= 0 {
		return tierWidth
	}
	colW := colRenderWidth(tierWidth, numPods)
	return numPods*colW + (numPods-1)*apicColumnGap
}

// chooseAPICPage picks the page (tile) size for a document with numPods
// pod columns, controllerCount controllers spanning all of them, and
// pods' per-pod spine/leaf counts. It climbs apicISOLandscape, then
// falls back to a custom landscape tile sized to the worst pod,
// mirroring the never-clip, ISO-climb-then-custom policy
// internal/render/drawio.
func chooseAPICPage(numPods, controllerCount int, pods []podFabricCounts) PageInfo {
	for _, tier := range apicISOLandscape {
		colW := colRenderWidth(tier.Width, numPods)
		if !fitsSpineWidth(pods, colW) {
			continue
		}
		totalW := totalCanvasWidth(numPods, tier.Width)
		if stackHeight(pods, colW, controllerCount, totalW) <= tier.Height {
			return tier
		}
	}

	// Custom: widen the tile just enough that every pod's spine row
	// still fits its column's rendered width, then use whatever height
	// the resulting leaf-row count needs. If the widest pod's spine
	// count still needs more than landscape A0's column width, that
	// requirement (not A0's width) sets the floor.
	maxSpines := 0
	for _, p := range pods {
		if p.Spines > maxSpines {
			maxSpines = p.Spines
		}
	}
	tierWidth := apicISOLandscape[len(apicISOLandscape)-1].Width
	needColW := minWidthForCapacity(maxSpines, apicIconSize)
	needTier := needColW
	if numPods > 1 {
		needTier += apicColumnGap
	}
	if needTier > tierWidth {
		tierWidth = needTier
	}
	colW := colRenderWidth(tierWidth, numPods)
	totalW := totalCanvasWidth(numPods, tierWidth)
	height := stackHeight(pods, colW, controllerCount, totalW)

	// Keep the tile itself landscape-shaped (pad it to the ISO 1.414:1
	// aspect against its own height) the same way the other choosers'
	// custom fallback keeps their single page landscape.
	if lim := int(math.Ceil(float64(height) * math.Sqrt2)); lim > tierWidth {
		tierWidth = lim
	}
	return PageInfo{Format: "custom", Width: apicRoundUpTo(tierWidth, 10), Height: apicRoundUpTo(height, 10)}
}

// apicRoundUpTo rounds v up to the next multiple of step.
func apicRoundUpTo(v, step int) int {
	if step <= 1 {
		return v
	}
	if r := v % step; r != 0 {
		return v + step - r
	}
	return v
}
