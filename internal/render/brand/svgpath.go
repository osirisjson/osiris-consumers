// This file is a minimal, hand-rolled parser for the SVG path "d"
// mini-language without require third-party dependency.
// It supports exactly the six command letters this package's one
// embedded asset uses (M m L l H h V v C c Z z: moveto, lineto,
// horizontal/vertical lineto, cubic Bezier curveto, closepath) and
// nothing else, it does not aim to parse an arbitrary caller-supplied
// path, only the build-time-fixed logo this package controls (logo.go).
//
// Two real SVG quirks this parser MUST get right against the actual
// asset, not a textbook example: a moveto followed by a second
// coordinate pair with no command letter between them is an implicit
// lineto, not a second moveto (three of the logo's eight paths rely
// on this); and a coordinate may use exponent notation ("2.6e-4", in
// the right-curly-bracket path).
package brand

import (
	"fmt"
	"regexp"
	"strconv"
)

// svgTokenRe matches either a command letter or a signed decimal
// number (with an optional exponent) the only two token kinds a "d"
// attribute from this asset's own export pipeline (Inkscape) ever
// produces.
var svgTokenRe = regexp.MustCompile(`[MmLlHhVvCcZz]|-?\d*\.?\d+(?:[eE][+-]?\d+)?`)

type svgToken struct {
	isCmd bool
	cmd   byte
	num   float64
}

func tokenizeSVGPath(d string) ([]svgToken, error) {
	raw := svgTokenRe.FindAllString(d, -1)
	toks := make([]svgToken, len(raw))
	for i, s := range raw {
		if len(s) == 1 && isPathCommandByte(s[0]) {
			toks[i] = svgToken{isCmd: true, cmd: s[0]}
			continue
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("brand: unparsable number %q in path data", s)
		}
		toks[i] = svgToken{num: n}
	}
	return toks, nil
}

func isPathCommandByte(b byte) bool {
	switch b {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'Z', 'z':
		return true
	}
	return false
}

// parsePathData parses one SVG <path> element's "d" attribute into
// its subpaths, with every coordinate resolved to absolute values in
// the source SVG's own coordinate space (Y increasing downward, as
// SVG itself defines it converting to PDF's bottom-left origin is
// logo.go's job, not this parser's).
//
// A repeated group of numbers after a command letter repeats that
// same command, per the SVG grammar the loop below needs no special
// code for this, since leaving activeCmd unchanged and not consuming
// a letter naturally re-enters the same case. The one real exception
// is M/m: a repeat after the first moveto is defined as an implicit
// lineto, so the moveto case switches activeCmd to L/l immediately
// after placing its own point.
func parsePathData(d string) ([]LogoSubpath, error) {
	toks, err := tokenizeSVGPath(d)
	if err != nil {
		return nil, err
	}

	var subpaths []LogoSubpath
	var cur LogoSubpath
	haveSubpath := false
	var cx, cy, startX, startY float64
	var activeCmd byte

	flush := func() {
		if haveSubpath {
			subpaths = append(subpaths, cur)
		}
		cur = LogoSubpath{}
		haveSubpath = false
	}

	need := func(i, n int) error {
		if i+n > len(toks) {
			return fmt.Errorf("brand: path command %q is missing an argument", activeCmd)
		}
		return nil
	}

	i := 0
	for i < len(toks) {
		if toks[i].isCmd {
			activeCmd = toks[i].cmd
			i++
			if activeCmd == 'Z' || activeCmd == 'z' {
				cur.Closed = true
				cx, cy = startX, startY
				flush()
				continue
			}
		} else if activeCmd == 0 {
			return nil, fmt.Errorf("brand: path data does not start with a command")
		} else if activeCmd == 'Z' || activeCmd == 'z' {
			// Z takes no arguments and is never repeated by a bare
			// number group; a number here means malformed data.
			return nil, fmt.Errorf("brand: unexpected number after closepath")
		}

		switch activeCmd {
		case 'M', 'm':
			if err := need(i, 2); err != nil {
				return nil, err
			}
			x, y := toks[i].num, toks[i+1].num
			i += 2
			if activeCmd == 'm' {
				x, y = cx+x, cy+y
			}
			flush()
			cx, cy, startX, startY = x, y, x, y
			cur.Ops = append(cur.Ops, LogoOp{Kind: LogoMoveTo, X: x, Y: y})
			haveSubpath = true
			if activeCmd == 'M' {
				activeCmd = 'L'
			} else {
				activeCmd = 'l'
			}

		case 'L', 'l':
			if err := need(i, 2); err != nil {
				return nil, err
			}
			x, y := toks[i].num, toks[i+1].num
			i += 2
			if activeCmd == 'l' {
				x, y = cx+x, cy+y
			}
			cx, cy = x, y
			cur.Ops = append(cur.Ops, LogoOp{Kind: LogoLineTo, X: x, Y: y})

		case 'H', 'h':
			if err := need(i, 1); err != nil {
				return nil, err
			}
			x := toks[i].num
			i++
			if activeCmd == 'h' {
				x += cx
			}
			cx = x
			cur.Ops = append(cur.Ops, LogoOp{Kind: LogoLineTo, X: cx, Y: cy})

		case 'V', 'v':
			if err := need(i, 1); err != nil {
				return nil, err
			}
			y := toks[i].num
			i++
			if activeCmd == 'v' {
				y += cy
			}
			cy = y
			cur.Ops = append(cur.Ops, LogoOp{Kind: LogoLineTo, X: cx, Y: cy})

		case 'C', 'c':
			if err := need(i, 6); err != nil {
				return nil, err
			}
			x1, y1 := toks[i].num, toks[i+1].num
			x2, y2 := toks[i+2].num, toks[i+3].num
			x, y := toks[i+4].num, toks[i+5].num
			i += 6
			if activeCmd == 'c' {
				x1, y1 = cx+x1, cy+y1
				x2, y2 = cx+x2, cy+y2
				x, y = cx+x, cy+y
			}
			cur.Ops = append(cur.Ops, LogoOp{Kind: LogoCurveTo, X1: x1, Y1: y1, X2: x2, Y2: y2, X: x, Y: y})
			cx, cy = x, y

		default:
			return nil, fmt.Errorf("brand: unsupported path command %q", activeCmd)
		}
	}
	flush()
	return subpaths, nil
}
