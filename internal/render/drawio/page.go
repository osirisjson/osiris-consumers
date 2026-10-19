// This file chooses the draw.io page size for a rendered topology. The
// renderer never scales, samples, or drops content to make a drawing
// fit: the page is chosen large enough to contain the whole
// deterministic layout. Sizes climb the ISO 216 A series (A5 first) in
// landscape orientation; a drawing larger than landscape A0 is emitted
// on a custom landscape page whose width is padded to the ISO 1.414:1
// aspect so the sheet stays landscape even when the drawing itself is
// taller than it is wide.

package drawio

import (
	"math"

	"go.osirisjson.org/consumers/internal/topology"
)

// A PageInfo is the draw.io page size chosen for one topology. Format
// is ISO 216 name ("A5" through "A0") or "custom". Width and Height are
// the emitted page dimensions in mxGraph user units and are always
// landscape (Width >= Height). No topology content is ever omitted or
// scaled to fit.
type PageInfo struct {
	Format string
	Width  int
	Height int
}

// ExceedsISOA0 reports whether the layout was larger than landscape A0
// and therefore needed a custom page.
func (p PageInfo) ExceedsISOA0() bool { return p.Format == "custom" }

// isoLandscape is the A5..A0 progression in landscape orientation, at
// draw.io's 100-DPI convention for ISO paper, smallest first.
var isoLandscape = []PageInfo{
	{"A5", 826, 583},
	{"A4", 1169, 826},
	{"A3", 1654, 1169},
	{"A2", 2336, 1654},
	{"A1", 3300, 2336},
	{"A0", 4681, 3300},
}

// ChoosePage returns the smallest landscape ISO page that contains the
// deterministic layout of m, or a custom landscape page when the layout
// exceeds landscape A0. It is a pure function of the model and the
// layout constants and always agrees with Render.
func ChoosePage(m *topology.Topology) PageInfo {
	w, h := buildLayout(m, newCellIDs(m)).extent()
	return choosePageForExtent(w, h)
}

// choosePageForExtent maps a content bounding box to a page size.
func choosePageForExtent(w, h int) PageInfo {
	for _, p := range isoLandscape {
		if w <= p.Width && h <= p.Height {
			return p
		}
	}
	pw := w
	if lim := int(math.Ceil(float64(h) * math.Sqrt2)); lim > pw {
		pw = lim
	}
	return PageInfo{Format: "custom", Width: roundUpTo(pw, 10), Height: roundUpTo(h, 10)}
}

// roundUpTo rounds v up to the next multiple of step.
func roundUpTo(v, step int) int {
	if step <= 1 {
		return v
	}
	if r := v % step; r != 0 {
		return v + step - r
	}
	return v
}
