// This file draws the OSIRIS JSON word mark (internal/render/brand)
// as real PDF path operators a PDF has no mechanism to embed an
// SVG directly, unlike draw.io's data-URI approach, so the mark's
// vector geometry is drawn with the same fill/path operators any
// other PDF content uses, scaled and placed at the point of use.
//
// SVG's coordinate space has Y increasing downward from the top of
// its viewBox; PDF's has Y increasing upward from the bottom of the
// page. Rather than negate every point by hand, logoMatrix builds one
// PDF "cm" transform that does both the SVG-to-PDF flip and the
// uniform scale-to-target-height in a single step, then the raw
// SVG-space coordinates are written out completely unchanged.
package pdf

import (
	"fmt"
	"strings"

	"go.osirisjson.org/consumers/internal/render/brand"
)

// logoAspect is the mark's own width-to-height ratio, from its SVG
// viewBox used to compute a placement's width once its height is
// chosen, so the mark is never drawn stretched.
func logoAspect() float64 {
	return brand.LogoViewBoxWidth / brand.LogoViewBoxHeight
}

// drawLogo places the mark at x, with its top edge yTop points below
// the page top, scaled to height points tall (width follows from the
// mark's own aspect ratio). It returns the width actually used, so a
// caller can position whatever sits to the mark's right.
func drawLogo(buf *strings.Builder, ph, x, yTop, height float64) float64 {
	scale := height / brand.LogoViewBoxHeight
	width := brand.LogoViewBoxWidth * scale

	// (px, py) = (scale*sx + x, -scale*sy + (ph - yTop)) for an SVG
	// point (sx, sy) - the PDF cm matrix [a b c d e f] that produces
	// exactly this is a = scale, d = -scale, e = x, f = ph - yTop.
	fmt.Fprintf(buf, "q %s 0 0 %s %s %s cm\n", num(scale), num(-scale), num(x), num(ph-yTop))
	for _, p := range brand.Logo {
		fmt.Fprintf(buf, "%s %s %s rg\n", num(p.FillR), num(p.FillG), num(p.FillB))
		for _, sp := range p.Subpaths {
			writeLogoSubpath(buf, sp)
		}
		buf.WriteString("f\n")
	}
	buf.WriteString("Q\n")
	return width
}

func writeLogoSubpath(buf *strings.Builder, sp brand.LogoSubpath) {
	for _, op := range sp.Ops {
		switch op.Kind {
		case brand.LogoMoveTo:
			fmt.Fprintf(buf, "%s %s m\n", num(op.X), num(op.Y))
		case brand.LogoLineTo:
			fmt.Fprintf(buf, "%s %s l\n", num(op.X), num(op.Y))
		case brand.LogoCurveTo:
			fmt.Fprintf(buf, "%s %s %s %s %s %s c\n",
				num(op.X1), num(op.Y1), num(op.X2), num(op.Y2), num(op.X), num(op.Y))
		}
	}
	if sp.Closed {
		buf.WriteString("h\n")
	}
}
