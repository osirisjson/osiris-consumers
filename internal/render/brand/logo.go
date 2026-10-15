// Package brand carries the OSIRIS JSON word-mark logo as a single,
// build-time embedded asset shared by every renderer that wants it.
// internal/render/drawio (as a base64 data URI) and internal/render/pdf
// (as real vector path operators, since a PDF has no mechanism to embed
// an SVG directly). go:embed requires the embedded file to live in the
// same directory tree as the embedding source file, so the asset and
// its one parse live here rather than in either renderer; each
// renderer imports the result it needs instead of re-embedding.
package brand

import (
	_ "embed"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

//go:embed assets/logo/osiris-logo.svg
var SVG []byte

// LogoOpKind is one subpath operation's kind.
type LogoOpKind int

// The operation kinds a parsed logo subpath carries exactly the
// ones the SVG path mini-language this package parses can produce
// (svgpath.go): a moveto, a lineto (absorbing L, H, and V, which all
// resolve to "go in a straight line to this point"), and a cubic
// Bezier curveto.
const (
	LogoMoveTo LogoOpKind = iota
	LogoLineTo
	LogoCurveTo
)

// LogoOp is one operation in a subpath, with every coordinate already
// resolved to absolute values in the SVG's own coordinate space. X1/Y1
// and X2/Y2 (the two Bezier control points) are meaningful only for
// LogoCurveTo.
type LogoOp struct {
	Kind           LogoOpKind
	X, Y           float64
	X1, Y1, X2, Y2 float64
}

// LogoSubpath is one moveto-started run of operations. Closed records
// whether the source path explicitly closed it (SVG "Z"/"z") a
// renderer decides for itself whether an unclosed subpath should still
// be closed before filling (PDF's fill operators do this implicitly;
// this field exists so a renderer that cares can tell the difference).
type LogoSubpath struct {
	Ops    []LogoOp
	Closed bool
}

// LogoPath is one SVG <path> element: its subpaths (a single element
// commonly carries more than one, using winding direction to cut a
// hole the logo's "O" and "o" letters work this way) and the solid
// fill color the whole element shares, as 0..1 RGB components.
type LogoPath struct {
	Subpaths            []LogoSubpath
	FillR, FillG, FillB float64
}

// Logo is the parsed logo: every <path> element in SVG, in document
// order, ready for a renderer to place and scale. LogoViewBoxWidth and
// LogoViewBoxHeight are the coordinate space every LogoPath's numbers
// are expressed in parsed from the SVG's own viewBox, not
// hand-transcribed, so they can never drift from the asset.
var (
	Logo              []LogoPath
	LogoViewBoxWidth  float64
	LogoViewBoxHeight float64
)

func init() {
	w, h, paths, err := parseLogoSVG(SVG)
	if err != nil {
		panic("brand: embedded logo asset failed to parse: " + err.Error())
	}
	LogoViewBoxWidth, LogoViewBoxHeight = w, h
	Logo = paths
}

// svgRoot and svgGroup mirror only the elements and attributes this
// package needs out of the embedded SVG encoding/xml silently
// ignores everything else, which is fine for a build-time asset this
// package owns and controls.
type svgRoot struct {
	XMLName xml.Name   `xml:"svg"`
	ViewBox string     `xml:"viewBox,attr"`
	Groups  []svgGroup `xml:"g"`
}

type svgGroup struct {
	Groups []svgGroup    `xml:"g"`
	Paths  []svgPathElem `xml:"path"`
}

type svgPathElem struct {
	D     string `xml:"d,attr"`
	Style string `xml:"style,attr"`
}

// parseLogoSVG parses raw SVG bytes into the viewBox size and every
// <path> element's geometry, at whatever nesting depth it appears.
func parseLogoSVG(raw []byte) (w, h float64, paths []LogoPath, err error) {
	var root svgRoot
	if err := xml.Unmarshal(raw, &root); err != nil {
		return 0, 0, nil, fmt.Errorf("decoding SVG XML: %w", err)
	}
	w, h, err = parseViewBox(root.ViewBox)
	if err != nil {
		return 0, 0, nil, err
	}

	var elems []svgPathElem
	var walk func(groups []svgGroup)
	walk = func(groups []svgGroup) {
		for _, g := range groups {
			elems = append(elems, g.Paths...)
			walk(g.Groups)
		}
	}
	walk(root.Groups)

	for i, e := range elems {
		subpaths, err := parsePathData(e.D)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("path element %d: %w", i, err)
		}
		r, g, b, err := parseFillColor(e.Style)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("path element %d: %w", i, err)
		}
		paths = append(paths, LogoPath{Subpaths: subpaths, FillR: r, FillG: g, FillB: b})
	}
	if len(paths) == 0 {
		return 0, 0, nil, fmt.Errorf("found no <path> element")
	}
	return w, h, paths, nil
}

// parseViewBox parses an SVG viewBox attribute's "minX minY width
// height" into its width and height; minX/minY are required to be
// zero, which is all this package's own asset ever needs to support.
func parseViewBox(vb string) (w, h float64, err error) {
	fields := strings.Fields(vb)
	if len(fields) != 4 {
		return 0, 0, fmt.Errorf("viewBox %q does not have 4 fields", vb)
	}
	nums := make([]float64, 4)
	for i, f := range fields {
		n, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("viewBox %q: %w", vb, err)
		}
		nums[i] = n
	}
	if nums[0] != 0 || nums[1] != 0 {
		return 0, 0, fmt.Errorf("viewBox %q has a nonzero origin, which this package does not support", vb)
	}
	return nums[2], nums[3], nil
}

// parseFillColor extracts the "fill:#RRGGBB" declaration from an SVG
// style attribute's semicolon-separated list and returns it as 0..1
// RGB components.
func parseFillColor(style string) (r, g, b float64, err error) {
	for _, decl := range strings.Split(style, ";") {
		k, v, ok := strings.Cut(decl, ":")
		if !ok || strings.TrimSpace(k) != "fill" {
			continue
		}
		hex := strings.TrimSpace(v)
		hex = strings.TrimPrefix(hex, "#")
		if len(hex) != 6 {
			return 0, 0, 0, fmt.Errorf("unsupported fill value %q", v)
		}
		raw, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("unsupported fill value %q: %w", v, err)
		}
		r = float64((raw>>16)&0xFF) / 255
		g = float64((raw>>8)&0xFF) / 255
		b = float64(raw&0xFF) / 255
		return r, g, b, nil
	}
	return 0, 0, 0, fmt.Errorf("style %q has no fill declaration", style)
}
