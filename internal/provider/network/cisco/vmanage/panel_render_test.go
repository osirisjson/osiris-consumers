package vmanage

import "testing"

// TestPanelPortAnchor pins the derived anchor fractions for the four
// Gigabit Ethernet ports the confirmed illustration draws, against the
// coordinates worked out by composing the illustration's own SVG
// transforms (see panel_render.go's own doc comments for the
// derivation).
func TestPanelPortAnchor(t *testing.T) {
	cases := []struct {
		port  string
		wantX float64
		wantY float64
	}{
		{"GigabitEthernet0/0/0", 704.0 / panelViewW, 113.5 / panelViewH},
		{"GigabitEthernet0/0/1", 704.0 / panelViewW, 204.5 / panelViewH},
		{"GigabitEthernet0/0/2", 875.0 / panelViewW, 113.44 / panelViewH},
		{"GigabitEthernet0/0/3", 875.0 / panelViewW, 203.98 / panelViewH},
	}
	for _, c := range cases {
		t.Run(c.port, func(t *testing.T) {
			fx, fy, ok := panelPortAnchor(c.port)
			if !ok {
				t.Fatalf("panelPortAnchor(%q) ok = false, want true", c.port)
			}
			const tol = 1e-4
			if abs(fx-c.wantX) > tol || abs(fy-c.wantY) > tol {
				t.Errorf("panelPortAnchor(%q) = (%.6f, %.6f), want (%.6f, %.6f)", c.port, fx, fy, c.wantX, c.wantY)
			}
			if fx < 0 || fx > 1 || fy < 0 || fy > 1 {
				t.Errorf("panelPortAnchor(%q) = (%.6f, %.6f) is outside the [0,1] fraction range", c.port, fx, fy)
			}
		})
	}
}

func TestPanelPortAnchorRejectsUnrecognizedPorts(t *testing.T) {
	for _, port := range []string{
		"GigabitEthernet0/0/4", "GigabitEthernet0/0/2.221", "Loopback0", "NVI0",
		"Sdwan-system-intf", "Tunnel1", "vmanage-mgmt", "", "GigabitEthernet1/0/0",
	} {
		if _, _, ok := panelPortAnchor(port); ok {
			t.Errorf("panelPortAnchor(%q) ok = true, want false (only 0/0/0-0/0/3 are drawn on this illustration)", port)
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
