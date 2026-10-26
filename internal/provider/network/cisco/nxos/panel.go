// This file maps a Cisco Nexus switch chassis's product ID (see
// icons.go's chassisProductID) to a confirmed, hand-drawn front/rear
// device-panel illustration, the same "fixed, confirmed lookup; an
// unconfirmed model gets nothing rather than a guess" pattern icons.go
// already uses for the Cisco19 prIcon mapping.
//
// The full per-port dynamic assembly algorithm (placing an
// assets.RJ45Port/SFPPort/QSFPPort icon at each of this switch's own
// ports, front and rear, for a chassis with no confirmed illustration)
// is tracked as a later increment, not built here yet: this file only
// resolves whether a confirmed illustration exists for a given model
// and exposes it, ready for the topology renderer to place on its own
// page once that page exists.
package nxos

import (
	_ "embed"
	"encoding/base64"

	"go.osirisjson.org/consumers/internal/provider/network/assets"
)

//go:embed assets/drawio/nexus-9k/C93180YC-FX/osiris-json-cisco-N9K-C93180YC-FX-front.svg
var n9kC93180YCFXFrontSVG []byte

//go:embed assets/drawio/nexus-9k/C93180YC-FX/osiris-json-cisco-N9K-C93180YC-FX-rear.svg
var n9kC93180YCFXRearSVG []byte

// ConfirmedPanel is one confirmed model's hand-drawn front and rear
// device-panel illustration, each a complete assets.Icon (data URI
// plus the SVG's own intrinsic geometry) rather than an assembly of
// the shared connector/component icons.
type ConfirmedPanel struct {
	Front, Rear assets.Icon
}

// confirmedPanels maps a chassis product ID to its ConfirmedPanel.
// Adding a model here means adding its own front/rear SVG pair under
// assets/drawio/<family>/<model>/ (see
// internal/provider/network/cisco/nxos/assets/README.md) and one entry
// below, never a guessed or generated illustration.
var confirmedPanels = map[string]ConfirmedPanel{
	"N9K-C93180YC-FX": {
		Front: newIcon(n9kC93180YCFXFrontSVG, 2964, 274),
		Rear:  newIcon(n9kC93180YCFXRearSVG, 2964, 274),
	},
}

// newIcon mirrors assets.newIcon (unexported there): this package
// builds its own Icon values from its own embedded bytes rather than
// depending on assets for anything but the Icon type itself.
func newIcon(svg []byte, w, h float64) assets.Icon {
	return assets.Icon{DataURI: "data:image/svg+xml," + base64.StdEncoding.EncodeToString(svg), Width: w, Height: h}
}

// ConfirmedPanelForModel returns the hand-drawn front/rear illustration
// for productID, or ok=false when no model in confirmedPanels matches
// the caller then falls back to the generic, per-port assembly instead
// of guessing a chassis it has never been shown.
func ConfirmedPanelForModel(productID string) (panel ConfirmedPanel, ok bool) {
	panel, ok = confirmedPanels[productID]
	return panel, ok
}
