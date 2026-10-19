package drawio

import (
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAttrEscapesMetacharactersAndBreaks(t *testing.T) {
	got := Attr(`a "b" & <c> 'd'` + "\n\t")
	for _, bad := range []string{`"`, `<`, `>`, "\n", "\t"} {
		if strings.Contains(got, bad) {
			t.Errorf("unescaped %q in %q", bad, got)
		}
	}
	for _, want := range []string{"&quot;", "&amp;", "&lt;", "&gt;", "&#39;", "&#10;", "&#9;"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestAttrProducesParseableXML(t *testing.T) {
	payload := `]]></mxCell><script>x</script> & "quote" <b>` + "\x00\x1b"
	doc := `<mxCell value="` + Attr(payload) + `"/>`
	var v struct {
		Value string `xml:"value,attr"`
	}
	if err := xml.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("escaped attribute is not well-formed XML: %v\n%s", err, doc)
	}
	// The decoded attribute value carries the original text (that is
	// the point of escaping): the structure held, the content did not
	// leak as markup.
	if !strings.Contains(v.Value, "script") {
		t.Errorf("payload text lost on round-trip: %q", v.Value)
	}
}

func TestTextEscapesDataMetacharacters(t *testing.T) {
	got := Text("a & b < c > d")
	if strings.ContainsAny(got, "<>") {
		t.Errorf("unescaped angle bracket in %q", got)
	}
	if !strings.Contains(got, "&amp;") || !strings.Contains(got, "&lt;") || !strings.Contains(got, "&gt;") {
		t.Errorf("metacharacters not entity-encoded: %q", got)
	}
	doc := "<node>" + got + "</node>"
	var v struct {
		Data string `xml:",chardata"`
	}
	if err := xml.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("not well-formed: %v", err)
	}
	if v.Data != "a & b < c > d" {
		t.Errorf("chardata round-trip = %q", v.Data)
	}
}

func TestSanitizeDropsIllegalXMLChars(t *testing.T) {
	got := Attr("keep\x00me\x08here\x1ffine")
	if strings.ContainsAny(got, "\x00\x08\x1f") {
		t.Errorf("illegal XML chars survived: %q", got)
	}
	if !strings.Contains(got, "keep") || !strings.Contains(got, "fine") {
		t.Errorf("legal text lost: %q", got)
	}
}

func TestPreservesMultilingualUTF8(t *testing.T) {
	in := "家 مرحبا Omega test smile"
	got := Text(in)
	if !utf8.ValidString(got) {
		t.Fatalf("not valid UTF-8: %q", got)
	}
	for _, want := range []string{"家", "مرحبا", "Omega"} {
		if !strings.Contains(got, want) {
			t.Errorf("dropped %q", want)
		}
	}
}

func TestRepairsInvalidUTF8(t *testing.T) {
	if got := Text("ok\xffbad"); !utf8.ValidString(got) {
		t.Errorf("invalid UTF-8 not repaired: %q", got)
	}
}
