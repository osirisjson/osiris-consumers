// Tests for the embedded device icon data URIs: each decodes to
// well-formed SVG.

package arubacentral

import (
	"encoding/base64"
	"encoding/xml"
	"strings"
	"testing"
)

func TestIconDataURIsAreWellFormedSVG(t *testing.T) {
	for name, uri := range map[string]string{
		"gatewayIconDataURI":     gatewayIconDataURI,
		"switchIconDataURI":      switchIconDataURI,
		"switchStackIconDataURI": switchStackIconDataURI,
	} {
		t.Run(name, func(t *testing.T) {
			const prefix = "data:image/svg+xml,"
			if !strings.HasPrefix(uri, prefix) {
				t.Fatalf("missing %q prefix", prefix)
			}
			b64 := strings.TrimPrefix(uri, prefix)
			raw, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				t.Fatalf("base64 decode: %v", err)
			}
			var v struct {
				XMLName xml.Name `xml:"svg"`
			}
			if err := xml.Unmarshal(raw, &v); err != nil {
				t.Fatalf("not well-formed SVG XML: %v", err)
			}
		})
	}
}
