// This file maps a Cisco Nexus switch chassis's product ID (its
// extensions.osiris.cisco.inventory "Chassis" entry, e.g.
// "N9K-C93180YC-FX") to the draw.io Cisco19 stencil's prIcon value.
package nxos

// iconIntrinsic is the icon width and height, in mxGraph user units.
const iconIntrinsic = 40

// nexusPrIcons maps the leading "N<digit>K" prefix of a Nexus product
// ID to its Cisco19 prIcon value.
var nexusPrIcons = map[byte]string{
	'1': "nexus_1k",
	'2': "nexus_2k",
	'3': "nexus_3k",
	'4': "nexus_4k",
	'5': "nexus_5k",
	'7': "nexus_7k",
	'9': "nexus_9300",
}

// prIconForModel returns the Cisco19 prIcon value for a chassis whose
// product ID is model, or "" when the product ID does not match a
// confirmed "N<digit>K..." prefix, the caller then omits prIcon and
// falls back to the bare shape rather than guessing.
func prIconForModel(model string) string {
	if len(model) < 3 || (model[0] != 'N' && model[0] != 'n') || (model[2] != 'K' && model[2] != 'k') {
		return ""
	}
	return nexusPrIcons[model[1]]
}
