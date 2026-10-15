// Package assets carries the shared, vendor-neutral device-panel
// icons used to assemble a switch's physical front/rear diagram:
// physical connector glyphs (RJ45, SFP, QSFP, a vertical USB port),
// chassis components (a fan, a PSU, a small port-number label strip),
// and plain 1U/2U/4U chassis-body backgrounds. None of these depict a
// specific vendor's product, a connector or a rack-unit body looks
// the same regardless of who made the switch so this package lives
// at the network category level (internal/provider/network/), a peer
// of internal/provider/network/cisco/ and .../hpe/, not nested under
// any one vendor.
//
// Every element is self-authored artwork from original vendor appliance
// never copied from a vendor's own branded material).
//
// go:embed requires the embedded files to live in the same directory
// tree as the embedding source file (no ".." patterns), so this
// package, not each vendor package, owns the embed and exports the
// resulting Icon values for vendor packages to import.
package assets

import (
	_ "embed"
	"encoding/base64"
)

//go:embed drawio/shared/rj45-port.svg
var rj45PortSVG []byte

//go:embed drawio/shared/sfp-port.svg
var sfpPortSVG []byte

//go:embed drawio/shared/qsfp-port.svg
var qsfpPortSVG []byte

//go:embed drawio/shared/usb-vertical-port.svg
var usbVerticalPortSVG []byte

//go:embed drawio/shared/fan.svg
var fanSVG []byte

//go:embed drawio/shared/PSU.svg
var psuSVG []byte

//go:embed drawio/shared/port-numbers.svg
var portNumbersSVG []byte

//go:embed drawio/shared/1u-switch-body.svg
var switchBody1USVG []byte

//go:embed drawio/shared/2u-switch-body.svg
var switchBody2USVG []byte

//go:embed drawio/shared/4u-switch-body.svg
var switchBody4USVG []byte

// Icon is one embedded SVG asset: a draw.io-ready data URI plus the SVG
// own intrinsic width and height, in the same units draw.io places the
// icon at 1:1 scale callers size their mxCell geometry from these,
// not a guessed aspect ratio.
type Icon struct {
	// DataURI is "data:image/svg+xml,<base64>" no ";base64" marker
	// Callers compose it into a full mxCell style string themselves
	// (e.g. "shape=image;...;image=" + DataURI + ";"),
	// since different render contexts need different style flags.
	DataURI       string
	Width, Height float64
}

func newIcon(svg []byte, w, h float64) Icon {
	return Icon{DataURI: "data:image/svg+xml," + base64.StdEncoding.EncodeToString(svg), Width: w, Height: h}
}

// Connector and component icons. Width/Height mirror each SVG's own
// viewBox exactly (checked by assets_test.go).
var (
	RJ45Port        = newIcon(rj45PortSVG, 74, 70)
	SFPPort         = newIcon(sfpPortSVG, 74, 70)
	QSFPPort        = newIcon(qsfpPortSVG, 150.86852, 70)
	USBVerticalPort = newIcon(usbVerticalPortSVG, 26.25, 70)
	Fan             = newIcon(fanSVG, 309, 209)
	PSU             = newIcon(psuSVG, 594, 209)
	PortNumbers     = newIcon(portNumbersSVG, 30.493034, 10.164574)
	SwitchBody1U    = newIcon(switchBody1USVG, 2964, 274)
	SwitchBody2U    = newIcon(switchBody2USVG, 2965.6567, 545.65686)
	SwitchBody4U    = newIcon(switchBody4USVG, 2968, 1088)
)
