package azure

import "testing"

func TestChoosePageIsoCandidatesStayLandscape(t *testing.T) {
	for _, tt := range []struct{ w, h int }{{100, 100}, {1600, 1100}, {4000, 3000}} {
		p := choosePage(tt.w, tt.h)
		if p.Format == "custom" {
			t.Fatalf("choosePage(%d,%d) = %+v, expected an ISO candidate for this test", tt.w, tt.h, p)
		}
		if p.Width < p.Height {
			t.Errorf("choosePage(%d,%d) = %+v, not landscape", tt.w, tt.h, p)
		}
	}
}

// TestChoosePageCustomHugsContentNoForcedAspect confirms a custom page
// beyond A0 is sized to the content's own exact bounds (rounded up to
// 10 units), with no aspect ratio imposed, including a tall, narrow
// content box that ends up a tall, narrow (non-landscape).
func TestChoosePageCustomHugsContentNoForcedAspect(t *testing.T) {
	for _, tt := range []struct{ w, h, wantW, wantH int }{
		{50, 5000, 50, 5000},
		{6000, 500, 6000, 500},
		{4685, 3305, 4690, 3310},
	} {
		p := choosePage(tt.w, tt.h)
		if p.Format != "custom" {
			t.Fatalf("choosePage(%d,%d).Format = %s, want custom", tt.w, tt.h, p.Format)
		}
		if p.Width != tt.wantW || p.Height != tt.wantH {
			t.Errorf("choosePage(%d,%d) = %+v, want %dx%d", tt.w, tt.h, p, tt.wantW, tt.wantH)
		}
	}
}

func TestChoosePageStartsAtA3AndLaddersUpWithoutClipping(t *testing.T) {
	for _, tt := range []struct {
		w, h int
		want string
	}{
		{100, 100, "A3"},
		{1600, 1100, "A3"},
		{4000, 3000, "A0"},
	} {
		p := choosePage(tt.w, tt.h)
		if p.Format != tt.want {
			t.Errorf("choosePage(%d,%d).Format = %s, want %s", tt.w, tt.h, p.Format, tt.want)
		}
		if p.Width < tt.w || p.Height < tt.h {
			t.Errorf("choosePage(%d,%d) = %+v clips the content", tt.w, tt.h, p)
		}
	}
}

func TestChoosePageCustomBeyondA0(t *testing.T) {
	p := choosePage(6000, 5000)
	if p.Format != "custom" {
		t.Fatalf("expected a custom page beyond A0, got %+v", p)
	}
	if p.Width < 6000 || p.Height < 5000 {
		t.Errorf("custom page %+v clips the content", p)
	}
}
