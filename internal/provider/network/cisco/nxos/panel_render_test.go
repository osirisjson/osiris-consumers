package nxos

import "testing"

// TestPanelPortAnchor pins the derived anchor fractions for a
// representative port from each region of the front illustration
// (first and second port of a pair, across a group boundary, a QSFP
// uplink), against the coordinates worked out by hand from the
// illustration's own SVG transforms (see panel_render.go's own doc
// comments for the derivation). mgmt0 is not among these: it has no
// connector on the front illustration at all (TestRearPortAnchor
// covers it).
func TestPanelPortAnchor(t *testing.T) {
	cases := []struct {
		port  string
		wantX float64
		wantY float64
	}{
		{"Ethernet1/1", 95.0 / panelViewW, 97.41768 / panelViewH},                   // group 0, pair 0, upper.
		{"Ethernet1/2", 95.0 / panelViewW, 177.41768 / panelViewH},                  // group 0, pair 0, lower.
		{"Ethernet1/16", (95.0 + 87.142857*7) / panelViewW, 177.41768 / panelViewH}, // group 0, last pair, lower.
		{"Ethernet1/17", (95.0 + 740.03239) / panelViewW, 97.3729 / panelViewH},     // group 1, pair 0, upper.
		{"Ethernet1/33", (95.0 + 1480.0324) / panelViewW, 97.3729 / panelViewH},     // group 2, pair 0, upper.
		{"Ethernet1/48", (95.0 + 87.142857*7 + 1480.0324) / panelViewW, 177.3729 / panelViewH},
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

// TestPanelPortAnchorQSFP pins the QSFP anchors against the same
// composed-transform derivation panel_render.go's own doc comment
// describes: each cage's two ports share one local x (stacked
// vertically in the actual bezel geometry, not side by side, only the
// "49/50" text label's arrows are drawn side by side), the
// odd port on top, the even port below.
func TestPanelPortAnchorQSFP(t *testing.T) {
	cases := []struct {
		port  string
		wantX float64
		wantY float64
	}{
		{"Ethernet1/49", 0.8114035, 0.3464148},
		{"Ethernet1/50", 0.8114035, 0.6376167},
		{"Ethernet1/51", 0.8823445, 0.3473337},
		{"Ethernet1/52", 0.8823445, 0.6385356},
		{"Ethernet1/53", 0.9531947, 0.3473337},
		{"Ethernet1/54", 0.9531947, 0.6385356},
	}
	for _, c := range cases {
		t.Run(c.port, func(t *testing.T) {
			fx, fy, ok := panelPortAnchor(c.port)
			if !ok {
				t.Fatalf("panelPortAnchor(%q) ok = false, want true", c.port)
			}
			const tol = 1e-4
			if abs(fx-c.wantX) > tol || abs(fy-c.wantY) > tol {
				t.Errorf("panelPortAnchor(%q) = (%.7f, %.7f), want (%.7f, %.7f)", c.port, fx, fy, c.wantX, c.wantY)
			}
		})
	}

	// A cage's own two ports share one x (stacked, not side by side);
	// the odd port sits above the even one.
	fx49, fy49, _ := panelPortAnchor("Ethernet1/49")
	fx50, fy50, _ := panelPortAnchor("Ethernet1/50")
	if fx49 != fx50 {
		t.Errorf("port-49/50 should share the same x (stacked in one cage): got %v and %v", fx49, fx50)
	}
	if fy49 >= fy50 {
		t.Errorf("port-49 should sit above port-50: got fy49=%v fy50=%v", fy49, fy50)
	}

	// The three QSFP cages step rightward in the same order as their
	// port numbers.
	fx51, _, _ := panelPortAnchor("Ethernet1/51")
	fx53, _, _ := panelPortAnchor("Ethernet1/53")
	if !(fx49 < fx51 && fx51 < fx53) {
		t.Errorf("QSFP cages did not step rightward: 49=%v 51=%v 53=%v", fx49, fx51, fx53)
	}
}

func TestPanelPortAnchorRejectsUnrecognizedPorts(t *testing.T) {
	for _, port := range []string{"Ethernet1/0", "Ethernet1/55", "Ethernet2/1", "console", "", "loopback0", "mgmt0"} {
		if _, _, ok := panelPortAnchor(port); ok {
			t.Errorf("panelPortAnchor(%q) ok = true, want false (mgmt0 belongs on the rear illustration, not the front one)", port)
		}
	}
}

// TestRearPortAnchor pins mgmt0's position on the rear illustration's
// management block, worked out by hand from that illustration's own
// SVG transforms (see panel_render.go's own doc comment). L1, L2, and
// console-port are deliberately not covered: the rear illustration
// draws connectors for them, but no fixture's own connections name them
// yet with a confirmed OSIRIS JSON interface name to match against.
func TestRearPortAnchor(t *testing.T) {
	fx, fy, ok := rearPortAnchor("mgmt0")
	if !ok {
		t.Fatal("rearPortAnchor(\"mgmt0\") ok = false, want true")
	}
	const tol = 1e-4
	wantX, wantY := rearMgmtLocalX/panelViewW, rearMgmtLocalY/panelViewH
	if abs(fx-wantX) > tol || abs(fy-wantY) > tol {
		t.Errorf("rearPortAnchor(\"mgmt0\") = (%.6f, %.6f), want (%.6f, %.6f)", fx, fy, wantX, wantY)
	}
	if fx < 0 || fx > 1 || fy < 0 || fy > 1 {
		t.Errorf("rearPortAnchor(\"mgmt0\") = (%.6f, %.6f) is outside the [0,1] fraction range", fx, fy)
	}
}

func TestRearPortAnchorRejectsUnrecognizedPorts(t *testing.T) {
	for _, port := range []string{"Ethernet1/1", "L1", "L2", "console", "", "loopback0"} {
		if _, _, ok := rearPortAnchor(port); ok {
			t.Errorf("rearPortAnchor(%q) ok = true, want false", port)
		}
	}
}

// TestConfirmedPanelTopologyCoversEveryRealFixtureNeighbour proves the
// accuracy against the real fixture requirement directly: every local
// port the real testdata fixture's own physical.ethernet connections
// name must resolve on the front illustration, the rear illustration,
// or both must not be true simultaneously (a port drawn nowhere or
// drawn on both would be a modeling bug) so renderConfirmedPanelPage
// never silently drops a real edge, and never double-draws one.
func TestConfirmedPanelTopologyCoversEveryRealFixtureNeighbour(t *testing.T) {
	g := fixture(t)
	neighbours := buildNeighbours(g)
	if len(neighbours) != 15 {
		t.Fatalf("fixture sanity: len(neighbours) = %d, want 15", len(neighbours))
	}
	var frontCount, rearCount int
	for _, n := range neighbours {
		_, _, onFront := panelPortAnchor(n.LocalPort)
		_, _, onRear := rearPortAnchor(n.LocalPort)
		if onFront {
			frontCount++
		}
		if onRear {
			rearCount++
		}
		if onFront == onRear {
			t.Errorf("neighbour %q's local port %q: onFront=%v onRear=%v, want exactly one true", n.Label, n.LocalPort, onFront, onRear)
		}
	}
	// This fixture's own connections: 14 neighbours over Ethernet1/N
	// data ports (front), 1 over mgmt0 (rear).
	if frontCount != 14 || rearCount != 1 {
		t.Errorf("frontCount=%d rearCount=%d, want 14 and 1", frontCount, rearCount)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
