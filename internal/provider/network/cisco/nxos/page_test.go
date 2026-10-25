package nxos

import "testing"

func TestChooseNXOSPageClimbsISOSeries(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{100, 100, "A4"},
		{1600, 1100, "A3"},
		{4000, 3000, "A0"},
	}
	for _, c := range cases {
		got := chooseNXOSPage(c.w, c.h)
		if got.Format != c.want {
			t.Errorf("chooseNXOSPage(%d, %d).Format = %q, want %q", c.w, c.h, got.Format, c.want)
		}
		if got.Width < got.Height {
			t.Errorf("chooseNXOSPage(%d, %d) is not landscape: %+v", c.w, c.h, got)
		}
	}
}

func TestChooseNXOSPageFallsBackToCustom(t *testing.T) {
	got := chooseNXOSPage(10000, 3000)
	if got.Format != "custom" {
		t.Fatalf("Format = %q, want custom", got.Format)
	}
	if !got.ExceedsISOA0() {
		t.Error("ExceedsISOA0() = false, want true for a custom page")
	}
	if got.Width < got.Height {
		t.Errorf("custom page is not landscape: %+v", got)
	}
}
