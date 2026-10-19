// Tests for the page-size ladder: every chosen page is landscape, the
// ladder climbs A5 -> A0 as the drawing grows, a drawing larger than
// landscape A0 falls to a custom landscape page, the chosen page always
// contains the whole layout (nothing is scaled or dropped to fit), and
// the size Render stamps into the mxGraphModel header matches
// ChoosePage.

package drawio

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"go.osirisjson.org/consumers/internal/topology"
)

// synthTopology returns a model of n ungrouped generic nodes.
func synthTopology(n int) *topology.Topology {
	m := &topology.Topology{}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("r%06d", i)
		m.Nodes = append(m.Nodes, topology.Node{ID: id, Label: id, Type: "generic.thing"})
	}
	return m
}

func ladderIndex(format string) int {
	for i, p := range isoLandscape {
		if p.Format == format {
			return i
		}
	}
	return len(isoLandscape) // "custom" sorts last
}

func TestChoosePageAlwaysLandscape(t *testing.T) {
	for _, n := range []int{0, 1, 4, 20, 100, 500, 2000, 8000} {
		p := ChoosePage(synthTopology(n))
		if p.Width < p.Height {
			t.Errorf("n=%d: page %s %dx%d is portrait", n, p.Format, p.Width, p.Height)
		}
	}
}

func TestChoosePageLaddersUpAndNeverClips(t *testing.T) {
	prev := -1
	for _, n := range []int{0, 1, 4, 12, 30, 80, 200, 600, 1500, 6000} {
		m := synthTopology(n)
		p := ChoosePage(m)
		w, h := buildLayout(m, newCellIDs(m)).extent()
		if w > p.Width || h > p.Height {
			t.Errorf("n=%d: layout %dx%d does not fit page %s %dx%d", n, w, h, p.Format, p.Width, p.Height)
		}
		if idx := ladderIndex(p.Format); idx < prev {
			t.Errorf("n=%d: ladder went backwards to %s", n, p.Format)
		} else {
			prev = idx
		}
	}
}

func TestChoosePageCustomBeyondA0(t *testing.T) {
	m := synthTopology(6000)
	p := ChoosePage(m)
	if p.Format != "custom" {
		t.Fatalf("expected a custom page for a very large topology, got %s", p.Format)
	}
	if !p.ExceedsISOA0() {
		t.Errorf("ExceedsISOA0 should be true for a custom page")
	}
	if p.Width < p.Height {
		t.Errorf("custom page %dx%d is portrait", p.Width, p.Height)
	}
	w, h := buildLayout(m, newCellIDs(m)).extent()
	if w > p.Width || h > p.Height {
		t.Errorf("custom page %dx%d clips layout %dx%d", p.Width, p.Height, w, h)
	}
}

func TestChoosePageMatchesRenderHeader(t *testing.T) {
	re := regexp.MustCompile(`pageWidth="(\d+)" pageHeight="(\d+)"`)
	for _, f := range []string{
		"valid/mixed-provider.json",
		"valid/no-provider.json",
		"adversarial/dangling-connection.json",
		"producer/network/hpe/arubacentral/hpe-arubacentral-2026-08-06T16-47-25Z-BRANCH-01.json",
	} {
		m := model(t, f)
		want := ChoosePage(m)
		out := renderXML(t, m, fixedTime)
		mm := re.FindStringSubmatch(out)
		if mm == nil {
			t.Fatalf("%s: no pageWidth/pageHeight in header", f)
		}
		gotW, _ := strconv.Atoi(mm[1])
		gotH, _ := strconv.Atoi(mm[2])
		if gotW != want.Width || gotH != want.Height {
			t.Errorf("%s: header page %dx%d, ChoosePage %dx%d", f, gotW, gotH, want.Width, want.Height)
		}
	}
}
