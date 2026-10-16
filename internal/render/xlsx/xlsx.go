// Package xlsx writes a minimal, valid multi-sheet .xlsx workbook
// (OOXML SpreadsheetML) using for simplicity only the standard library
// without third-party dependency.
//
// An .xlsx file is a zip archive of small XML parts. Write emits
// exactly the parts a reader needs: content types, package and
// workbook relationships, the workbook itself, one worksheet per
// Sheet, and a bare-minimum styles part (some readers expect one to
// exist even when nothing is styled). Every cell is a plain inline
// string; there is no shared-string table, no formula, and no cell
// formatting.
package xlsx

import (
	"archive/zip"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Sheet is one worksheet: Name (truncated and sanitized to fit
// Excel's 31-character, punctuation-restricted sheet name rule),
// Header, and Rows. A row may have fewer cells than Header (the rest
// render blank) but never more, Write returns an error rather than
// silently dropping a value.
type Sheet struct {
	Name   string
	Header []string
	Rows   [][]string
}

// Write generate a workbook containing sheets, in order, to w.
func Write(w io.Writer, sheets []Sheet) error {
	for _, s := range sheets {
		for _, row := range s.Rows {
			if len(row) > len(s.Header) {
				return fmt.Errorf("xlsx: sheet %q has a row with %d cells, more than its %d header columns", s.Name, len(row), len(s.Header))
			}
		}
	}

	zw := zip.NewWriter(w)

	if err := writePart(zw, "[Content_Types].xml", contentTypesXML(len(sheets))); err != nil {
		return err
	}
	if err := writePart(zw, "_rels/.rels", rootRelsXML); err != nil {
		return err
	}
	if err := writePart(zw, "xl/workbook.xml", workbookXML(sheets)); err != nil {
		return err
	}
	if err := writePart(zw, "xl/_rels/workbook.xml.rels", workbookRelsXML(len(sheets))); err != nil {
		return err
	}
	if err := writePart(zw, "xl/styles.xml", stylesXML); err != nil {
		return err
	}
	for i, s := range sheets {
		if err := writePart(zw, fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), worksheetXML(s)); err != nil {
			return err
		}
	}

	return zw.Close()
}

func writePart(zw *zip.Writer, name, content string) error {
	f, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("xlsx: creating %s: %w", name, err)
	}
	if _, err := io.WriteString(f, content); err != nil {
		return fmt.Errorf("xlsx: writing %s: %w", name, err)
	}
	return nil
}

const xmlProlog = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

func contentTypesXML(numSheets int) string {
	var b strings.Builder
	b.WriteString(xmlProlog)
	b.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	b.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)
	for i := 1; i <= numSheets; i++ {
		fmt.Fprintf(&b, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i)
	}
	b.WriteString(`</Types>`)
	return b.String()
}

const rootRelsXML = xmlProlog +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
	`</Relationships>`

func workbookXML(sheets []Sheet) string {
	var b strings.Builder
	b.WriteString(xmlProlog)
	b.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	b.WriteString(`<sheets>`)
	for i, s := range sheets {
		n := i + 1
		fmt.Fprintf(&b, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, escapeAttr(sheetName(s.Name, i)), n, n)
	}
	b.WriteString(`</sheets>`)
	b.WriteString(`</workbook>`)
	return b.String()
}

func workbookRelsXML(numSheets int) string {
	var b strings.Builder
	b.WriteString(xmlProlog)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 1; i <= numSheets; i++ {
		fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i, i)
	}
	fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, numSheets+1)
	b.WriteString(`</Relationships>`)
	return b.String()
}

// stylesXML is the bare minimum styles part: one default font, fill,
// border, and cell format. Nothing in this package ever references a
// non-default style, but some readers expect this part to exist.
const stylesXML = xmlProlog +
	`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
	`<fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>` +
	`<fills count="1"><fill><patternFill patternType="none"/></fill></fills>` +
	`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
	`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
	`<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>` +
	`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
	`</styleSheet>`

func worksheetXML(s Sheet) string {
	var b strings.Builder
	b.WriteString(xmlProlog)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	b.WriteString(`<sheetData>`)
	writeRow(&b, 1, s.Header)
	for i, row := range s.Rows {
		writeRow(&b, i+2, row)
	}
	b.WriteString(`</sheetData>`)
	b.WriteString(`</worksheet>`)
	return b.String()
}

func writeRow(b *strings.Builder, rowNum int, cells []string) {
	fmt.Fprintf(b, `<row r="%d">`, rowNum)
	for i, v := range cells {
		if v == "" {
			continue // an empty cell needs no element at all.
		}
		ref := columnLetter(i+1) + strconv.Itoa(rowNum)
		space := ""
		if strings.TrimSpace(v) != v {
			space = ` xml:space="preserve"`
		}
		fmt.Fprintf(b, `<c r="%s" t="inlineStr"><is><t%s>%s</t></is></c>`, ref, space, escapeText(v))
	}
	b.WriteString(`</row>`)
}

// columnLetter returns the spreadsheet column letters for a 1-indexed
// column number (1 -> "A", 26 -> "Z", 27 -> "AA").
func columnLetter(n int) string {
	var b []byte
	for n > 0 {
		n--
		b = append([]byte{byte('A' + n%26)}, b...)
		n /= 26
	}
	return string(b)
}

// sheetName returns name truncated to Excel's 31-character sheet-name
// limit with its reserved characters (: \ / ? * [ ]) replaced by a
// space, or "SheetN" (1-indexed) when that leaves nothing usable.
func sheetName(name string, index int) string {
	var b strings.Builder
	for _, r := range name {
		switch r {
		case ':', '\\', '/', '?', '*', '[', ']':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 31 {
		out = strings.TrimSpace(string([]rune(out)[:31]))
	}
	if out == "" {
		return "Sheet" + strconv.Itoa(index+1)
	}
	return out
}

// escapeText escapes s for XML 1.0 character data (a <t> element's
// body): the three data-significant metacharacters are entity-
// encoded, and any code point illegal in XML 1.0 is replaced with
// U+FFFD so an untrusted document string can never produce a
// malformed part.
func escapeText(s string) string {
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

// escapeAttr escapes s for an XML attribute value (a sheet name).
func escapeAttr(s string) string {
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
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizeXML returns valid UTF-8 with every code point illegal in
// XML 1.0 replaced by U+FFFD (mirrors internal/render/drawio's own
// sanitizeXML: same legal set, same rationale a spreadsheet cell is
// exactly as untrusted as a diagram label).
func sanitizeXML(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isXMLChar(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('�')
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
