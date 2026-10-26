// This file chooses the draw.io page size for the Cisco vManage
// confirmed-panel topology, mirroring internal/render/drawio/page.go's
// (and internal/provider/network/cisco/nxos/page.go's) never-clip,
// ISO-climb-then-custom policy: climb the ISO 216 landscape series
// until the content fits, fall back to a custom landscape page above
// A0 page format.
package vmanage

import "math"

// PageInfo is the chosen page size, in mxGraph user units. Format is
// an ISO 216 name ("A4".."A0") or "custom"; Width and Height are
// always landscape (Width >= Height). Content is never scaled or
// dropped to fit.
type PageInfo struct {
	Format string
	Width  int
	Height int
}

// ExceedsISOA0 reports whether the content was larger than landscape
// A0 and therefore needed a custom page.
func (p PageInfo) ExceedsISOA0() bool { return p.Format == "custom" }

// vmanageISOLandscape is the A4..A0 progression in landscape
// orientation, at draw.io's 100-DPI convention.
var vmanageISOLandscape = []PageInfo{
	{"A4", 1169, 826},
	{"A3", 1654, 1169},
	{"A2", 2336, 1654},
	{"A1", 3300, 2336},
	{"A0", 4681, 3300},
}

// choosePage returns the smallest landscape ISO page (A4..A0) that
// contains a w x h content box, or a custom landscape page, width
// padded to the ISO 1.414:1 aspect so the sheet stays landscape even
// when the content is taller than it is wide, when the content
// exceeds landscape A0.
func choosePage(w, h int) PageInfo {
	for _, p := range vmanageISOLandscape {
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
