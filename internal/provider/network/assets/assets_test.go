package assets

import (
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// svgTagAttr extracts one attribute's value from an SVG's own <svg ...>
// tag, so this test can pin Icon.Width/Height against the source file's
// own viewBox rather than a second hand-copied number that could drift
// from it unnoticed.
func svgTagAttr(svg []byte, attr string) (string, bool) {
	tag := regexp.MustCompile(`(?s)<svg[^>]*>`).Find(svg)
	if tag == nil {
		return "", false
	}
	m := regexp.MustCompile(attr + `="([^"]*)"`).FindSubmatch(tag)
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

func TestIconsDecodeToWellFormedSVG(t *testing.T) {
	cases := []struct {
		name string
		icon Icon
		raw  []byte
	}{
		{"RJ45Port", RJ45Port, rj45PortSVG},
		{"SFPPort", SFPPort, sfpPortSVG},
		{"QSFPPort", QSFPPort, qsfpPortSVG},
		{"USBVerticalPort", USBVerticalPort, usbVerticalPortSVG},
		{"Fan", Fan, fanSVG},
		{"PSU", PSU, psuSVG},
		{"PortNumbers", PortNumbers, portNumbersSVG},
		{"SwitchBody1U", SwitchBody1U, switchBody1USVG},
		{"SwitchBody2U", SwitchBody2U, switchBody2USVG},
		{"SwitchBody4U", SwitchBody4U, switchBody4USVG},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if len(c.raw) == 0 {
				t.Fatal("embedded SVG is empty")
			}
			const prefix = "data:image/svg+xml,"
			if !strings.HasPrefix(c.icon.DataURI, prefix) {
				t.Fatalf("DataURI does not start with %q: %s", prefix, firstN(c.icon.DataURI, 40))
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(c.icon.DataURI, prefix))
			if err != nil {
				t.Fatalf("DataURI payload does not decode as base64: %v", err)
			}
			if string(decoded) != string(c.raw) {
				t.Error("decoded DataURI does not round-trip to the embedded source bytes")
			}
			if !strings.Contains(string(decoded), "<svg") {
				t.Error("decoded payload does not look like an SVG")
			}

			wantW, ok := svgTagAttr(c.raw, "width")
			if !ok {
				t.Fatal("source SVG has no width attribute")
			}
			wantH, ok := svgTagAttr(c.raw, "height")
			if !ok {
				t.Fatal("source SVG has no height attribute")
			}
			if got := formatDim(c.icon.Width); got != wantW {
				t.Errorf("Icon.Width = %s, want %s (the source SVG's own width)", got, wantW)
			}
			if got := formatDim(c.icon.Height); got != wantH {
				t.Errorf("Icon.Height = %s, want %s (the source SVG's own height)", got, wantH)
			}
		})
	}
}

// formatDim mirrors how encoding/xml would have rendered the same
// float back as the SVG author's own attribute string, for the exact
// values this package's icons carry (no more precision than the
// source SVGs themselves use).
func formatDim(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func TestIconGeometryIsPositive(t *testing.T) {
	icons := map[string]Icon{
		"RJ45Port": RJ45Port, "SFPPort": SFPPort, "QSFPPort": QSFPPort,
		"USBVerticalPort": USBVerticalPort, "Fan": Fan, "PSU": PSU,
		"PortNumbers": PortNumbers, "SwitchBody1U": SwitchBody1U,
		"SwitchBody2U": SwitchBody2U, "SwitchBody4U": SwitchBody4U,
	}
	for name, icon := range icons {
		if icon.Width <= 0 || icon.Height <= 0 {
			t.Errorf("%s geometry = %vx%v, want both positive", name, icon.Width, icon.Height)
		}
	}
}

func TestSwitchBodyHeightsClimbWithRackUnits(t *testing.T) {
	// Sanity check on the three rack-unit bodies sharing this package:
	// each additional U roughly doubles the previous height, the same
	// physical relationship real 1U/2U/4U chassis have.
	if !(SwitchBody1U.Height < SwitchBody2U.Height && SwitchBody2U.Height < SwitchBody4U.Height) {
		t.Errorf("switch body heights do not climb with rack units: 1U=%v 2U=%v 4U=%v",
			SwitchBody1U.Height, SwitchBody2U.Height, SwitchBody4U.Height)
	}
}
