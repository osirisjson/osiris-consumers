// This file carries the OSIRIS JSON word mark as a self-contained data
// URI, so the draw.io topology renderer can place a provenance badge on
// every diagram without a vendored asset tree. The source SVG
// itself lives in internal/render/brand (shared with internal/render/pdf,
// which needs the same mark's real vector geometry, not a raster or a
// data URI), this package only base64-encodes brand.SVG, the same bytes
// it embedded directly before brand existed. The image is the project's
// own mark (positive black-and-white word mark), an SVG whose intrinsic
// size is LogoIntrinsicW by LogoIntrinsicH draw.io user units. draw.io
// stores an image style as image=data:<mediatype>,<b64> with the base64
// payload placed directly after the comma (no ";base64" marker, draw.io
// accepts this form, and TestRenderEmbedsLogoBadge pins it) and the
// trailing semicolon closing the style key.
//
// LogoStyle and the geometry constants are exported so a per-vendor
// renderer outside this package (internal/provider/*) can place the
// same badge without duplicating the mark.

package drawio

import (
	"encoding/base64"

	"go.osirisjson.org/consumers/internal/render/brand"
)

// Logo geometry, in mxGraph user units.
const (
	LogoIntrinsicW = 145
	LogoIntrinsicH = 35
)

// LogoStyle is the mxCell style for the OSIRIS JSON word-mark badge.
// editableCssRules is kept so a user can recolour the mark in draw.io.
var LogoStyle = "shape=image;verticalLabelPosition=bottom;labelBackgroundColor=default;verticalAlign=top;aspect=fixed;imageAspect=0;editableCssRules=.*;image=data:image/svg+xml," +
	base64.StdEncoding.EncodeToString(brand.SVG) + ";"
