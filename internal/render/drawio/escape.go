// Package drawio holds the destination-aware escaping helpers for the
// draw.io (mxGraph) XML renderer. The renderer itself arrives with the
// topology capability; this phase provides only the escaping so
// untrusted document strings can be placed into draw.io XML with the
// document kept well-formed and label content kept inert.
//
// Every helper preserves valid UTF-8 and removes only characters that
// XML 1.0 cannot represent. There is no ASCII-only fallback.
package drawio

import (
	"strings"
	"unicode/utf8"
)

// Attr escapes s for an XML attribute value such as an mxCell "value".
// The five XML metacharacters are entity-encoded and tab, newline, and
// carriage return are numeric-encoded so a multi-line label cannot end
// the attribute or the element early. draw.io renders "&#10;" as a line
// break in a label.
func Attr(s string) string {
	s = sanitizeXML(s)
	var b strings.Builder
	b.Grow(len(s) + len(s)/8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		case '\n':
			b.WriteString("&#10;")
		case '\r':
			b.WriteString("&#13;")
		case '\t':
			b.WriteString("&#9;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Text escapes s for XML character data (an element body). The three
// data-significant metacharacters are entity-encoded; tab, newline, and
// carriage return are kept literally.
func Text(s string) string {
	s = sanitizeXML(s)
	var b strings.Builder
	b.Grow(len(s) + len(s)/8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizeXML returns valid UTF-8 with every code point that is not a
// legal XML 1.0 character replaced by U+FFFD. The legal set is
// #x9 | #xA | #xD | [#x20-#xD7FF] | [#xE000-#xFFFD] |
// [#x10000-#x10FFFF].
func sanitizeXML(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isXMLChar(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('\uFFFD')
		}
	}
	return b.String()
}

func isXMLChar(r rune) bool {
	switch {
	case r == 0x9, r == 0xA, r == 0xD:
		return true
	case r >= 0x20 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return true
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	default:
		return false
	}
}
