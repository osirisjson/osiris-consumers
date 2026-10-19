// This file serialises laid-out pages into the PDF file structure:
// the object table, the page tree, the font dictionaries (either the
// two base-14 Type1 fonts, or, for FontMontserrat, the ten objects
// cidwriter.go builds per weight), the document information
// dictionary, one Flate-compressed content stream per page, the
// bookmark outline, and the cross-reference table and trailer that
// let a reader find all of it.
//
// Object numbers are assigned by position, never by map iteration, so
// the same Document always produces the same file: 1 catalogue,
// 2 page tree, then the font objects (2 for Helvetica, 10 for
// Montserrat, see cidwriter.go), the information dictionary, then
// two objects per page (the page and its content stream), then the
// outline root and its items.
package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Fixed object numbers that precede the font objects.
const (
	objCatalog = 1
	objPages   = 2
	fontBase   = 3
)

// writeDocument assembles and writes the complete PDF.
func writeDocument(w io.Writer, doc Document, pages []*page, outline []outlineEntry, usage *fontUsage, pw, ph float64) error {
	// objFontReg and objFontBld are the object numbers Resources
	// actually references as /F1 and /F2 the Type1 dictionary
	// itself for Helvetica, or the Type0 dictionary (whose
	// DescendantFonts points at the rest) for Montserrat.
	var objFontReg, objFontBld, objInfo, objFirst int
	if doc.FontFamily == FontMontserrat {
		objFontReg = fontBase
		objFontBld = fontBase + cidFontObjects
		objInfo = fontBase + 2*cidFontObjects
	} else {
		objFontReg = fontBase
		objFontBld = fontBase + 1
		objInfo = fontBase + 2
	}
	objFirst = objInfo + 1

	pageObj := func(i int) int { return objFirst + 2*i }
	contentObj := func(i int) int { return objFirst + 2*i + 1 }
	outlineRoot := objFirst + 2*len(pages)
	outlineItem := func(i int) int { return outlineRoot + 1 + i }

	// total is the highest object number actually written. Without an
	// outline the last object is the final page's content stream, so
	// outlineRoot is one past the end and must not be counted an
	// xref entry for an object that was never emitted makes the file
	// unreadable.
	total := outlineRoot - 1
	if len(outline) > 0 {
		total = outlineRoot + len(outline)
	}

	var buf bytes.Buffer
	offsets := make([]int, total+1)

	buf.WriteString("%PDF-1.7\n")
	// A comment of high-bit bytes marks the file as binary for tools
	// that transfer it in text mode.
	buf.Write([]byte{'%', 0xE2, 0xE3, 0xCF, 0xD3, '\n'})

	obj := func(n int, body string) {
		offsets[n] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	// objStream writes a stream object: extraDict is any dictionary
	// entries beyond /Length and /Filter, already including its own
	// trailing space when non-empty (FontFile2's /Length1, say).
	objStream := func(n int, extraDict string, raw []byte) error {
		compressed, err := deflate(raw)
		if err != nil {
			return err
		}
		offsets[n] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n<< %s/Length %d /Filter /FlateDecode >>\nstream\n", n, extraDict, len(compressed))
		buf.Write(compressed)
		buf.WriteString("\nendstream\nendobj\n")
		return nil
	}

	catalog := fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R", objPages)
	if len(outline) > 0 {
		catalog += fmt.Sprintf(" /Outlines %d 0 R /PageMode /UseOutlines", outlineRoot)
	}
	catalog += " >>"
	obj(objCatalog, catalog)

	var kids strings.Builder
	for i := range pages {
		if i > 0 {
			kids.WriteByte(' ')
		}
		fmt.Fprintf(&kids, "%d 0 R", pageObj(i))
	}
	obj(objPages, fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(pages), kids.String()))

	if doc.FontFamily == FontMontserrat {
		if err := writeCIDFont(obj, objStream, montserratFont(false), false, objFontReg, usage); err != nil {
			return err
		}
		if err := writeCIDFont(obj, objStream, montserratFont(true), true, objFontBld, usage); err != nil {
			return err
		}
	} else {
		obj(objFontReg, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
		obj(objFontBld, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	}
	obj(objInfo, infoDict(doc))

	mediaBox := fmt.Sprintf("[0 0 %s %s]", num(pw), num(ph))
	resources := fmt.Sprintf("<< /Font << /F1 %d 0 R /F2 %d 0 R >> >>", objFontReg, objFontBld)
	for i, p := range pages {
		obj(pageObj(i), fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox %s /Resources %s /Contents %d 0 R >>",
			objPages, mediaBox, resources, contentObj(i)))
		if err := objStream(contentObj(i), "", []byte(p.buf.String())); err != nil {
			return err
		}
	}

	if len(outline) > 0 {
		obj(outlineRoot, fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
			outlineItem(0), outlineItem(len(outline)-1), len(outline)))
		for i, e := range outline {
			body := fmt.Sprintf("<< /Title %s /Parent %d 0 R /Dest [%d 0 R /XYZ 0 %s 0]",
				escapeString(e.title), outlineRoot, pageObj(e.pageIdx), num(e.y))
			if i > 0 {
				body += fmt.Sprintf(" /Prev %d 0 R", outlineItem(i-1))
			}
			if i < len(outline)-1 {
				body += fmt.Sprintf(" /Next %d 0 R", outlineItem(i+1))
			}
			body += " >>"
			obj(outlineItem(i), body)
		}
	}

	startxref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", total+1)
	buf.WriteString("0000000000 65535 f \n")
	for n := 1; n <= total; n++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[n])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		total+1, objCatalog, objInfo, startxref)

	_, err := w.Write(buf.Bytes())
	return err
}

// infoDict builds the document information dictionary. The date
// entries are omitted entirely when no creation date was injected,
// which is what keeps a default render byte-identical between runs.
func infoDict(doc Document) string {
	var b strings.Builder
	b.WriteString("<< /Producer ")
	b.WriteString(escapeString("OSIRIS JSON consumers"))
	if doc.Title != "" {
		b.WriteString(" /Title ")
		b.WriteString(escapeString(doc.Title))
	}
	if doc.Subject != "" {
		b.WriteString(" /Subject ")
		b.WriteString(escapeString(doc.Subject))
	}
	if doc.Author != "" {
		b.WriteString(" /Author ")
		b.WriteString(escapeString(doc.Author))
	}
	if !doc.CreationDate.IsZero() {
		d := pdfDate(doc.CreationDate)
		fmt.Fprintf(&b, " /CreationDate %s /ModDate %s", d, d)
	}
	b.WriteString(" >>")
	return b.String()
}

// pdfDate formats t as a PDF date string in UTC.
func pdfDate(t time.Time) string {
	return "(D:" + t.UTC().Format("20060102150405") + "Z)"
}

// deflate zlib-compresses a content stream.
func deflate(in []byte) ([]byte, error) {
	var out bytes.Buffer
	zw, err := zlib.NewWriterLevel(&out, zlib.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("pdf: compressing a content stream: %w", err)
	}
	if _, err := zw.Write(in); err != nil {
		return nil, fmt.Errorf("pdf: compressing a content stream: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("pdf: compressing a content stream: %w", err)
	}
	return out.Bytes(), nil
}

// escapeString encodes s as a PDF literal string: WinAnsi bytes with
// the three structural characters escaped and anything unprintable
// written as an octal code, so a stray byte can never terminate the
// string early.
func escapeString(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, r := range s {
		c, _ := encodeRune(r)
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			if c < 32 || c > 126 {
				fmt.Fprintf(&b, "\\%03o", c)
				continue
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte(')')
	return b.String()
}

// num formats a coordinate compactly and deterministically: two
// decimal places with trailing zeros removed, and no negative zero.
func num(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}
