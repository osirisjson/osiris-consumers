// This file maps a Cisco APIC fabric node's properties.model to the
// draw.io Cisco19 stencil's prIcon value. The mapping is a fixed,
// lookup: the Cisco19 library has no "nexus_9k" glyph, so a 9000-series
// model gets the literal "nexus_9300" value instead of following the
// "nexus_<n>k" shorthand the 1000/2000/3000/4000/5000/7000 series use.
// A model outside this confirmed set yields no icon at all rather than
// a guessed one.
package apic

// iconIntrinsic is the icon width and height, in mxGraph user units.
const iconIntrinsic = 40

// nexusPrIcons maps the leading "N<digit>K" prefix of a Nexus model
// string to its Cisco19 prIcon value.
var nexusPrIcons = map[byte]string{
	'1': "nexus_1k",
	'2': "nexus_2k",
	'3': "nexus_3k",
	'4': "nexus_4k",
	'5': "nexus_5k",
	'7': "nexus_7k",
	'9': "nexus_9300",
}

// prIconForModel returns the Cisco19 prIcon value for a fabric node
// whose properties.model is model, or "" when the model does not match
// a confirmed "N<digit>K..." prefix, the caller then omits prIcon and
// falls back to the bare shape rather than guessing.
func prIconForModel(model string) string {
	if len(model) < 3 || (model[0] != 'N' && model[0] != 'n') || (model[2] != 'K' && model[2] != 'k') {
		return ""
	}
	return nexusPrIcons[model[1]]
}
