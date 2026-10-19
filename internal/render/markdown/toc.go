// This file implements a Markdown table of contents: one bullet per
// top-level report section, linking to that section's own heading via
// the same anchor-slug convention GitHub, GitLab, and VS Codium own
// Markdown previews all generate automatically for a heading. Shared
// by every Markdown report renderer (the generic inventory brief and
// every per-provider enrichment) so a reader gets consistent, working
// navigation regardless of which renderer produced the file.
package markdown

import (
	"strconv"
	"strings"
)

// TOCHeading is the exact heading text every Markdown report renderer
// in this repository uses for its table of contents, exported so a
// caller never has to retype (and risk drifting from) this literal
// string.
const TOCHeading = "Table of content"

// TableOfContentsSection renders the complete table-of-contents
// section a renderer writes right after its metadata table: the
// TOCHeading heading, TableOfContents' bullet list, and a trailing
// blank line. TOCHeading itself is not one of titles (a table of
// contents does not link to itself).
func TableOfContentsSection(titles []string) string {
	var b strings.Builder
	b.WriteString("## " + TOCHeading + "\n\n")
	b.WriteString(TableOfContents(titles))
	b.WriteString("\n")
	return b.String()
}

// TableOfContents renders one Markdown bullet per title, in order,
// linking to that heading's anchor. titles must be exactly the
// heading text each section renders (case and punctuation included)
// the caller is responsible for keeping the two in sync, since a
// heading with no corresponding entry here (or vice versa) is a dead
// link either way. Two identical titles get "-1", "-2", ... suffixes
// on their anchors, matching how a duplicate heading's anchor is
// disambiguated.
//
// titles is developer-authored section-title text, not document
// content, so it is written verbatim rather than through Text(): a
// heading itself (e.g. "## Tenants, VRFs, and EPGs") is written
// literally by every renderer in this repository, and escaping only
// the table of contents' copy of that same string would print a
// visible backslash here that the real heading right below it does
// not have. A caller must never pass untrusted, document-derived text
// as a title.
func TableOfContents(titles []string) string {
	var b strings.Builder
	seen := map[string]int{}
	for _, title := range titles {
		anchor := HeadingAnchor(title)
		if n, ok := seen[anchor]; ok {
			seen[anchor] = n + 1
			anchor = anchor + "-" + strconv.Itoa(n)
		} else {
			seen[anchor] = 1
		}
		b.WriteString("- [")
		b.WriteString(title)
		b.WriteString("](#")
		b.WriteString(anchor)
		b.WriteString(")\n")
	}
	return b.String()
}

// HeadingAnchor slugifies title the way GitHub, GitLab, and VS Codium
// Markdown previews all generate a heading's anchor: lower-case,
// letters, digits, and underscores are kept, and any run of anything
// else (spaces, punctuation) collapses to a single hyphen, with no
// leading or trailing hyphen. Exported so a renderer can build an
// explicit link to one of its own headings (not just a table of
// contents) without duplicating this algorithm.
func HeadingAnchor(title string) string {
	var b strings.Builder
	pendingHyphen := false
	for _, r := range strings.ToLower(title) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_':
			if pendingHyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingHyphen = false
			b.WriteRune(r)
		default:
			pendingHyphen = true
		}
	}
	return b.String()
}
