package vmanage

import "testing"

func TestConfirmedPanelForModel(t *testing.T) {
	panel, ok := ConfirmedPanelForModel("vedge-C8200L-1N-4T")
	if !ok {
		t.Fatal("ConfirmedPanelForModel(\"vedge-C8200L-1N-4T\") ok = false, want true")
	}
	if panel.Front.DataURI == "" {
		t.Error("confirmed panel's own Front icon has no data URI")
	}
	if panel.Front.Width != 2964 || panel.Front.Height != 274 {
		t.Errorf("confirmed panel's own Front icon size = %vx%v, want 2964x274", panel.Front.Width, panel.Front.Height)
	}
}

func TestConfirmedPanelForModelRejectsUnconfirmedModel(t *testing.T) {
	if _, ok := ConfirmedPanelForModel("vedge-C8000L"); ok {
		t.Error("ConfirmedPanelForModel(\"vedge-C8000L\") ok = true, want false (not a confirmed model)")
	}
	if _, ok := ConfirmedPanelForModel(""); ok {
		t.Error("ConfirmedPanelForModel(\"\") ok = true, want false")
	}
}

func TestConfirmedTopologyAppliesOnRealFixture(t *testing.T) {
	g := fixture(t)
	if !ConfirmedTopologyApplies(g) {
		t.Fatal("expected the fixture's own router model (vedge-C8200L-1N-4T) to resolve to a confirmed panel")
	}
}

func TestConfirmedTopologyAppliesRejectsUnconfirmedModel(t *testing.T) {
	g := fixtureWithModel(t, "vedge-C8000L")
	if ConfirmedTopologyApplies(g) {
		t.Error("ConfirmedTopologyApplies fired on a router model with no confirmed panel")
	}
}
