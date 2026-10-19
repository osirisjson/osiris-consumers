// Tests for the shared Markdown table of contents: the anchor slug
// algorithm matches GitHub/GitLab/VS Code's own heading-anchor
// convention, and duplicate titles get disambiguated the same way.

package markdown

import "testing"

func TestHeadingAnchor(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Document", "document"},
		{"Fabric summary", "fabric-summary"},
		{"Tenants, VRFs, and EPGs", "tenants-vrfs-and-epgs"},
		{"Out of scope for this report", "out-of-scope-for-this-report"},
		{"L3outs", "l3outs"},
		{"  leading and trailing spaces  ", "leading-and-trailing-spaces"},
		{"under_score kept", "under_score-kept"},
	}
	for _, tc := range cases {
		if got := HeadingAnchor(tc.title); got != tc.want {
			t.Errorf("HeadingAnchor(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestTableOfContentsLinksAndOrder(t *testing.T) {
	got := TableOfContents([]string{"Document", "Fabric summary"})
	want := "- [Document](#document)\n- [Fabric summary](#fabric-summary)\n"
	if got != want {
		t.Errorf("TableOfContents = %q, want %q", got, want)
	}
}

func TestTableOfContentsDisambiguatesDuplicateTitles(t *testing.T) {
	got := TableOfContents([]string{"Spine", "Spine", "Spine"})
	want := "- [Spine](#spine)\n- [Spine](#spine-1)\n- [Spine](#spine-2)\n"
	if got != want {
		t.Errorf("TableOfContents = %q, want %q", got, want)
	}
}

func TestTableOfContentsSectionIncludesHeading(t *testing.T) {
	got := TableOfContentsSection([]string{"Document", "Disclosure"})
	want := "## Table of Content\n\n" +
		"- [Document](#document)\n" +
		"- [Disclosure](#disclosure)\n" +
		"\n"
	if got != want {
		t.Errorf("TableOfContentsSection = %q, want %q", got, want)
	}
}

func TestTableOfContentsWritesTitleVerbatim(t *testing.T) {
	// Titles are developer-authored, not document content: the link
	// text must match a real heading (e.g. "## Tenants, VRFs, and
	// EPGs") exactly, with no escaping the heading itself doesn't have.
	got := TableOfContents([]string{"Tenants, VRFs, and EPGs"})
	want := "- [Tenants, VRFs, and EPGs](#tenants-vrfs-and-epgs)\n"
	if got != want {
		t.Errorf("TableOfContents = %q, want %q", got, want)
	}
}
