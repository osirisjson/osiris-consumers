package nxos

import (
	"testing"

	"go.osirisjson.org/consumers/internal/document"
)

func TestConfirmedPanelForModel(t *testing.T) {
	panel, ok := ConfirmedPanelForModel("N9K-C93180YC-FX")
	if !ok {
		t.Fatal("N9K-C93180YC-FX should resolve to a confirmed panel")
	}
	if panel.Front.DataURI == "" || panel.Rear.DataURI == "" {
		t.Fatal("confirmed panel has an empty front or rear DataURI")
	}
	if panel.Front.Width != 2964 || panel.Front.Height != 274 {
		t.Errorf("Front geometry = %vx%v, want 2964x274 (the source SVG's own viewBox)", panel.Front.Width, panel.Front.Height)
	}
	if panel.Rear.Width != 2964 || panel.Rear.Height != 274 {
		t.Errorf("Rear geometry = %vx%v, want 2964x274 (the source SVG's own viewBox)", panel.Rear.Width, panel.Rear.Height)
	}
}

func TestConfirmedPanelForModelRejectsUnconfirmedModel(t *testing.T) {
	for _, model := range []string{
		"N9K-C9336C-FX2", // a real, but not-yet-confirmed, Nexus 9000 model which need a specific SVG illustration saved under /assets folder.
		"WS-C3850-24T-E", // a Catalyst model, not even a Nexus.
		"",               // no model asserted at all.
	} {
		if _, ok := ConfirmedPanelForModel(model); ok {
			t.Errorf("ConfirmedPanelForModel(%q) = ok, want false: no confirmed illustration exists for this model", model)
		}
	}
}

// TestConfirmedPanelIdentifiesRealFixtureSwitch proves the
// identification path end to end: the real NX-OS testdata fixture's own
// switch resource carries the exact chassis product ID confirmedPanels
// has an entry for, so building the report model over it and looking
// that ID up resolves to the hand-drawn C93180YC-FX illustration, not
// the generic fallback.
func TestConfirmedPanelIdentifiesRealFixtureSwitch(t *testing.T) {
	g := fixture(t)
	m := BuildReportModel(g, document.Metadata{}, "deadbeef")

	if m.Switch.ChassisProductID != "N9K-C93180YC-FX" {
		t.Fatalf("fixture sanity: Switch.ChassisProductID = %q, want N9K-C93180YC-FX", m.Switch.ChassisProductID)
	}
	panel, ok := ConfirmedPanelForModel(m.Switch.ChassisProductID)
	if !ok {
		t.Fatalf("the real fixture's own chassis product ID %q was not identified as a confirmed panel", m.Switch.ChassisProductID)
	}
	if panel.Front.Width != panel.Rear.Width || panel.Front.Height != panel.Rear.Height {
		t.Errorf("front/rear geometry mismatch: front=%vx%v rear=%vx%v",
			panel.Front.Width, panel.Front.Height, panel.Rear.Width, panel.Rear.Height)
	}
}
