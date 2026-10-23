// Tests for the Aruba Central page-size choice: climbs the ISO 216
// landscape series from A4 through A0, then falls back to a custom
// landscape page never scales or clips content to fit.

package arubacentral

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestChooseACPageStartsAtA4(t *testing.T) {
	p := chooseACPage(10, 10)
	if p.Format != "A4" {
		t.Errorf("Format = %q, want A4 for trivially small content", p.Format)
	}
}

func TestChooseACPageClimbsTheISOSeries(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{1169, 826, "A4"},
		{1170, 826, "A3"}, // one unit over A4's width forces the next size
		{1654, 1169, "A3"},
		{1655, 1169, "A2"},
		{2336, 1654, "A2"},
		{3300, 2336, "A1"},
		{4681, 3300, "A0"},
	}
	for _, tc := range cases {
		got := chooseACPage(tc.w, tc.h)
		if got.Format != tc.want {
			t.Errorf("chooseACPage(%d, %d) = %q, want %q", tc.w, tc.h, got.Format, tc.want)
		}
	}
}

func TestChooseACPageNeverClips(t *testing.T) {
	for _, tc := range []struct{ w, h int }{
		{10, 10}, {1169, 826}, {2000, 1900}, {5000, 4000}, {9000, 100},
	} {
		got := chooseACPage(tc.w, tc.h)
		if got.Width < tc.w || got.Height < tc.h {
			t.Errorf("chooseACPage(%d, %d) = %dx%d, clips the content", tc.w, tc.h, got.Width, got.Height)
		}
		if got.Width < got.Height {
			t.Errorf("chooseACPage(%d, %d) = %dx%d is not landscape", tc.w, tc.h, got.Width, got.Height)
		}
	}
}

func TestChooseACPageFallsBackToCustomAboveA0(t *testing.T) {
	got := chooseACPage(5000, 4000)
	if got.Format != "custom" {
		t.Errorf("Format = %q, want custom for content beyond landscape A0", got.Format)
	}
	if !got.ExceedsISOA0() {
		t.Error("ExceedsISOA0() = false, want true")
	}
	if got.Width < 5000 || got.Height < 4000 {
		t.Errorf("custom page %dx%d clips the 5000x4000 content", got.Width, got.Height)
	}
}

func TestChooseACPageTallCustomStaysLandscape(t *testing.T) {
	// Content taller than it is wide must still get a landscape sheet:
	// the width is padded to the ISO 1.414:1 aspect.
	got := chooseACPage(1000, 6000)
	if got.Format != "custom" {
		t.Fatalf("Format = %q, want custom", got.Format)
	}
	if got.Width < got.Height {
		t.Errorf("%dx%d is not landscape for a 1000x6000 content box", got.Width, got.Height)
	}
}

func TestChooseTopologyPageMatchesRender(t *testing.T) {
	top, doc := buildModel(t, fixture)
	g := build(t, fixture)
	page := ChooseTopologyPage(top)

	r := NewTopologyRenderer(top, g, doc.Metadata, time.Time{})
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := fmt.Sprintf(`pageWidth="%d" pageHeight="%d"`, page.Width, page.Height)
	if !strings.Contains(buf.String(), want) {
		t.Errorf("ChooseTopologyPage = %+v, but Render's header does not contain %q", page, want)
	}
}
