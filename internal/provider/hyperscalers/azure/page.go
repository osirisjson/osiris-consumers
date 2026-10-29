// This file chooses the draw.io page size for the Azure topology
// overview: climb the ISO 216 landscape series A3, A2, A1, A0, then
// fall back to a custom page above A0 sized to the content's own exact
// bounds, with no imposed aspect ratio.
// Content is never scaled or dropped to fit; the A3..A0 ladder itself
// stays unchanged and is landscape by its own fixed
// definition either way.

package azure

// PageInfo is the chosen page size, in mxGraph user units. Format is an
// ISO 216 name ("A3".."A0") or "custom". Width and Height are landscape
// (Width >= Height) for every ISO format, by that format's own fixed
// definition; a "custom" page beyond A0 hugs the content's own bounds
// exactly and is not guaranteed to be landscape.
type PageInfo struct {
	Format string
	Width  int
	Height int
}

// isoLandscape is the A3..A0 progression in landscape orientation, at
// draw.io's 100-DPI convention, the same dimensions
// internal/render/drawio/page.go uses for its own A4..A0 steps.
var isoLandscape = []PageInfo{
	{"A3", 1654, 1169},
	{"A2", 2336, 1654},
	{"A1", 3300, 2336},
	{"A0", 4681, 3300},
}

// choosePage returns the smallest ISO page (A3..A0) that contains a
// w x h content box, or when the content exceeds even A0, a custom
// page sized to exactly that content box, rounded up to the nearest 10
// units. No aspect ratio is imposed on the custom page: a tall, narrow
// document gets a tall, narrow custom page, not one padded wide purely
// to stay landscape-shaped.
func choosePage(w, h int) PageInfo {
	for _, p := range isoLandscape {
		if w <= p.Width && h <= p.Height {
			return p
		}
	}
	return PageInfo{Format: "custom", Width: roundUpTo(w, 10), Height: roundUpTo(h, 10)}
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
