// This file maps a Cisco vManage router's own model string (see
// panel_render.go's routerModel) to a confirmed, hand-drawn front-panel
// device illustration, the same "fixed, confirmed lookup; an
// unconfirmed model gets nothing rather than a guess" pattern
// internal/provider/network/cisco/nxos's own panel.go uses.
//
// The key is the router's own "properties.model" string, exactly as
// the document carries it (e.g. "vedge-C8200L-1N-4T"), an exact match,
// never a family prefix: a document naming a model this map does not
// carry gets no confirmed panel, rather than this package guessing that
// a same-family model (say a "vedge-C8200L-1N-2T" with fewer ports)
// shares this exact illustration's own port layout.
//
// Rear illustration not built yet: ConfirmedPanel carries only Front
// for now. Add Rear here once that illustration exists, following the
// same review-then-embed process this doc comment describes for Front.
package vmanage

import (
	_ "embed"
	"encoding/base64"
)

//go:embed assets/drawio/catalyst-8200/cisco-catalyst-C8200L-1N-4T-front.svg
var catalystC8200LFrontSVG []byte

// ConfirmedPanel is one confirmed model's hand-drawn front device-panel
// illustration, a complete Icon (data URI plus the SVG's own intrinsic
// geometry) rather than an assembly of shared connector/component
// icons.
type ConfirmedPanel struct {
	Front Icon
}

// Icon is an embedded SVG plus its own intrinsic width and height, in
// the SVG's own viewBox user units, mirrors
// internal/provider/network/assets.Icon, but this package embeds its
// own bytes directly rather than depending on that package for
// anything but the shape of this type.
type Icon struct {
	DataURI       string
	Width, Height float64
}

// newIcon mirrors internal/provider/network/assets's own unexported
// newIcon: "data:image/svg+xml,<base64>" with no ";base64" marker, see
// that package's own Icon.DataURI doc comment for why.
func newIcon(svg []byte, w, h float64) Icon {
	return Icon{DataURI: "data:image/svg+xml," + base64.StdEncoding.EncodeToString(svg), Width: w, Height: h}
}

// confirmedPanels maps a router's own "properties.model" string to its
// ConfirmedPanel. Adding a model here means adding its own front SVG
// under assets/drawio/<family>/<model>/ and one entry below, never a
// guessed or generated illustration.
var confirmedPanels = map[string]ConfirmedPanel{
	"vedge-C8200L-1N-4T": {
		Front: newIcon(catalystC8200LFrontSVG, 2964, 274),
	},
}

// ConfirmedPanelForModel returns the hand-drawn front illustration for
// a router whose own "properties.model" is model, or ok=false when no
// model in confirmedPanels matches, the caller then falls back to the
// generic topology path instead of guessing a chassis it has never
// been shown.
func ConfirmedPanelForModel(model string) (panel ConfirmedPanel, ok bool) {
	panel, ok = confirmedPanels[model]
	return panel, ok
}
