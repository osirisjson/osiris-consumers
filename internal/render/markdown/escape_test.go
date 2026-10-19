package markdown

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextNeutralisesMarkdownAndHTML(t *testing.T) {
	in := "<script>alert(1)</script> & pipe | star * under _ back ` brack ] hash # bang !"
	got := Text(in)
	if strings.Contains(got, "<script>") || strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("angle brackets not neutralised: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected entity-encoded tag, got %q", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("ampersand not entity-encoded: %q", got)
	}
	for _, sub := range []string{`\|`, `\*`, `\_`, "\\`", `\]`, `\#`, `\!`} {
		if !strings.Contains(got, sub) {
			t.Errorf("expected %q escaped in %q", sub, got)
		}
	}
}

func TestTextCollapsesLineBreaks(t *testing.T) {
	got := Text("line one\nline two\r\nline three\tafter tab")
	if strings.ContainsAny(got, "\n\r\t") {
		t.Errorf("line breaks / tabs survived: %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Errorf("whitespace not collapsed: %q", got)
	}
}

func TestTextPreservesMultilingualUTF8(t *testing.T) {
	in := "duo lingua test Omega c smile 家 مرحبا"
	got := Text(in)
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8: %q", got)
	}
	for _, want := range []string{"家", "مرحبا", "Omega", "test"} {
		if !strings.Contains(got, want) {
			t.Errorf("dropped %q from %q", want, got)
		}
	}
}

func TestTextNormalisesControlCharsOnly(t *testing.T) {
	in := "a\x00b\x07c\x1bd\x7fe"
	got := Text(in)
	if strings.ContainsAny(got, "\x00\x07\x1b\x7f") {
		t.Errorf("control chars survived: %q", got)
	}
	if !strings.Contains(got, "a") || !strings.Contains(got, "e") {
		t.Errorf("surrounding text lost: %q", got)
	}
	if !strings.Contains(got, "�") {
		t.Errorf("expected U+FFFD replacement, got %q", got)
	}
}

func TestTextRepairsInvalidUTF8(t *testing.T) {
	got := Text("good\xffbad")
	if !utf8.ValidString(got) {
		t.Errorf("invalid UTF-8 not repaired: %q", got)
	}
}

func TestTableCellHasNoRawPipeOrBreak(t *testing.T) {
	got := TableCell("a | b\nc | d")
	if strings.Contains(got, "|") && !strings.Contains(got, `\|`) {
		t.Errorf("raw pipe in cell: %q", got)
	}
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("line break in cell: %q", got)
	}
}

func TestLinkOnlyAllowsSafeSchemes(t *testing.T) {
	if got := Link("click", "https://example.com/a b"); got != "[click](https://example.com/a%20b)" {
		t.Errorf("https link = %q", got)
	}
	if got := Link("mail", "mailto:admin@example.com"); got != "[mail](mailto:admin@example.com)" {
		t.Errorf("mailto link = %q", got)
	}
	got := Link("evil", "javascript:alert(1)")
	if strings.HasPrefix(got, "[") {
		t.Errorf("javascript: was rendered as a link: %q", got)
	}
	if !strings.Contains(got, "evil") {
		t.Errorf("visible text lost: %q", got)
	}
}

func TestLinkEncodesParensInURL(t *testing.T) {
	got := Link("x", "https://example.com/foo(bar)")
	if strings.ContainsAny(strings.TrimPrefix(got, "[x]("), "()"[:1]) {
		// crude: ensure "(" inside the url part is percent-encoded
	}
	if !strings.Contains(got, "%28") || !strings.Contains(got, "%29") {
		t.Errorf("parens not encoded: %q", got)
	}
}
