// This file embeds the Montserrat Regular and Bold static TrueType
// instances this package can select as an alternative to the base-14
// Helvetica faces (see Document.FontFamily in pdf.go).
//
// Only the static Regular and Bold instances are kept not Google
// Fonts' variable-weight files, which a PDF viewer has no mechanism
// to interpret (OpenType variation axes are meaningless outside an
// application that understands "fvar"/"gvar"), and not the other
// sixteen static weights, since this package exposes exactly two
// faces, matching the existing Helvetica/Helvetica-Bold design.
//
// Unlike the base-14 faces, embedding Montserrat carries a real,
// visible cost: the complete, unsubsetted font program for each
// weight (roughly 325KB and 329KB) is written into every PDF that
// selects it, regardless of how much text that document actually
// uses. Subsetting (keeping only the glyphs a document references,
// renumbering and rewriting "glyf"/"loca") is deliberately out of
// scope here, the same kind of simplicity-over-size tradeoff already
// made for internal/render/xlsx, a real font looks right; trimming
// its file size is a later optimization, not a correctness
// requirement.
package pdf

import _ "embed"

//go:embed font/montserrat/Montserrat-Regular.ttf
var montserratRegularTTF []byte

//go:embed font/montserrat/Montserrat-Bold.ttf
var montserratBoldTTF []byte

// montserratRegular and montserratBold are parsed once, at package
// initialization, from the embedded files above. A parse failure here
// can only mean one of those two compile-time assets is corrupt or
// was replaced with something this package's hand-rolled parser
// cannot read a build-time defect, not a runtime condition any
// caller can recover from, so it panics immediately rather than
// surfacing as a mysterious failure the first time a caller asks for
// FontMontserrat.
var montserratRegular = mustParseTTF(montserratRegularTTF)
var montserratBold = mustParseTTF(montserratBoldTTF)

func mustParseTTF(data []byte) *ttfFont {
	f, err := parseTTF(data)
	if err != nil {
		panic("pdf: embedded Montserrat asset failed to parse: " + err.Error())
	}
	return f
}
