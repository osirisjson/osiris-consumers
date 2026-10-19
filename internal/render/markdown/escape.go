// Package markdown holds the destination-aware escaping helpers for the
// Markdown renderer. The renderer itself arrives with the report
// capability; this phase provides only the escaping so untrusted
// document strings can be placed into Markdown with structure kept
// valid and content kept inert.
//
// Every helper preserves valid UTF-8 (including multilingual text and
// emoji) and normalizes only control characters. There is no
// ASCII-only fallback and no transliteration.
package markdown

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Text escapes s for use as an inline Markdown span (a name, a label, a
// one-line description). The result is a single line: tab, carriage
// return, and newline are collapsed to a single space so injected text
// cannot open a new block, list item, heading, or table row. Markdown
// active punctuation is backslash-escaped and the HTML-significant
// characters & < > are entity-encoded, so neither Markdown constructs
// nor raw HTML in s can take effect.
func Text(s string) string {
	return escapeInline(collapseSpaces(sanitizeControls(s)))
}

// TableCell escapes s for a GitHub-Flavored Markdown table cell. It is
// Text plus the guarantee that no un-escaped pipe or line break can
// break the row or column structure (both are already handled by Text;
// this name documents the intent at call sites).
func TableCell(s string) string {
	return Text(s)
}

// Link renders a Markdown link with escaped visible text and a
// conservatively encoded destination. Only http, https, and mailto
// destinations produce an actual link; anything else is rendered as
// inert text of the form "text (url)" so an unexpected scheme such as
// javascript: cannot become active.
func Link(text, url string) string {
	visible := Text(text)
	clean := sanitizeControls(url)

	lower := strings.ToLower(strings.TrimSpace(clean))
	if !(strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:")) {
		if clean == "" {
			return visible
		}
		return visible + " (" + escapeInline(collapseSpaces(clean)) + ")"
	}

	var b strings.Builder
	for _, r := range collapseSpaces(clean) {
		switch r {
		case ' ':
			b.WriteString("%20")
		case '(':
			b.WriteString("%28")
		case ')':
			b.WriteString("%29")
		case '<':
			b.WriteString("%3C")
		case '>':
			b.WriteString("%3E")
		case '"':
			b.WriteString("%22")
		case '\\':
			b.WriteString("%5C")
		default:
			b.WriteRune(r)
		}
	}
	return "[" + visible + "](" + b.String() + ")"
}

// sanitizeControls returns valid UTF-8 with every Unicode control
// character other than tab, carriage return, and newline replaced by
// U+FFFD. Carriage returns are folded into newlines first.
func sanitizeControls(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r == utf8.RuneError:
			b.WriteRune('\uFFFD')
		case unicode.IsControl(r):
			b.WriteRune('\uFFFD')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// collapseSpaces turns every run of ASCII whitespace into a single
// space and trims the ends.
func collapseSpaces(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\v' || r == '\f'
	}), " ")
}

// escapeInline backslash-escapes CommonMark ASCII punctuation and
// entity-encodes &, <, and >.
func escapeInline(s string) string {
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
			if isCommonMarkPunct(r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isCommonMarkPunct reports whether r is in the CommonMark
// backslash-escapable ASCII punctuation set, excluding &, <, and >
// which are entity-encoded instead.
func isCommonMarkPunct(r rune) bool {
	switch r {
	case '!', '"', '#', '$', '%', '\'', '(', ')', '*', '+', ',', '-', '.', '/',
		':', ';', '=', '?', '@', '[', '\\', ']', '^', '_', '`', '{', '|', '}', '~':
		return true
	default:
		return false
	}
}
