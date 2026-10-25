// Tests for the Cisco APIC fabric topology renderer: only
// compute.server and role-bearing (fabricNode) network.switch resources
// are drawn, grouped into a FABRIC CONTROLLERS swimlane and one
// SPINE POD-N / LEAF POD-N pair of swimlanes per pod, with every edge
// emitted before every container cell.

package apic

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// loadDoc mirrors apic_test.go's build helper but also returns the
// parsed document, whose Metadata a TopologyRenderer needs.
func loadDoc(t *testing.T, rel string) (*document.Document, *graph.Graph) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "testdata", rel))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", rel, err)
	}
	doc, err := document.Parse(b)
	if err != nil {
		t.Fatalf("parsing %s: %v", rel, err)
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing %s: %v", rel, err)
	}
	return doc, g
}

func TestBuildFabricMatchesTheReferenceFixture(t *testing.T) {
	g := build(t, fixture)
	controllers, pods := buildFabric(g)

	if len(controllers) != 3 {
		t.Errorf("len(controllers) = %d, want 3", len(controllers))
	}
	if len(pods) != 2 {
		t.Fatalf("len(pods) = %d, want 2", len(pods))
	}
	if pods[0].Pod != "pod-1" || pods[1].Pod != "pod-2" {
		t.Errorf("pod order = [%q, %q], want [pod-1, pod-2]", pods[0].Pod, pods[1].Pod)
	}
	wantSpines := []int{2, 2}
	wantLeafs := []int{10, 10}
	for i, p := range pods {
		if len(p.Spines) != wantSpines[i] {
			t.Errorf("pods[%d].Spines = %d, want %d", i, len(p.Spines), wantSpines[i])
		}
		if len(p.Leafs) != wantLeafs[i] {
			t.Errorf("pods[%d].Leafs = %d, want %d", i, len(p.Leafs), wantLeafs[i])
		}
	}
}

func TestBuildFabricExcludesRolelessNeighbourSwitches(t *testing.T) {
	g := build(t, fixture)
	_, pods := buildFabric(g)

	total := 0
	for _, p := range pods {
		total += len(p.Spines) + len(p.Leafs)
	}
	// The fixture declares 51 network.switch resources; 31 are
	// externally discovered neighbours with no properties.role and
	// must not appear here.
	if total != 25 {
		t.Errorf("total role-bearing switches = %d, want 20 (51 declared minus 31 role-less neighbours)", total)
	}
}

func TestPodFromNativeID(t *testing.T) {
	cases := []struct{ nativeID, want string }{
		{"topology/pod-1/node-1", "pod-1"},
		{"topology/pod-2/node-3", "pod-2"},
		{"external/00:00:5E:00:53:00", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := podFromNativeID(tc.nativeID); got != tc.want {
			t.Errorf("podFromNativeID(%q) = %q, want %q", tc.nativeID, got, tc.want)
		}
	}
}

func renderTopology(t *testing.T, doc *document.Document, g *graph.Graph) string {
	t.Helper()
	r := NewTopologyRenderer(g, doc.Metadata, time.Unix(0, 0))
	var buf bytes.Buffer
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestRenderEmitsEveryExpectedSwimlane(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	out := renderTopology(t, doc, g)

	for _, want := range []string{"FABRIC CONTROLLERS", "SPINE POD-1", "LEAF POD-1", "SPINE POD-2", "LEAF POD-2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain swimlane title %q", want)
		}
	}
}

func TestRenderEmitsEveryEdgeBeforeEveryContainer(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	out := renderTopology(t, doc, g)

	firstEdge := strings.Index(out, `edge="1"`)
	firstContainer := strings.Index(out, "FABRIC CONTROLLERS")
	if firstEdge < 0 {
		t.Fatal("output has no edge cell")
	}
	if firstContainer < 0 {
		t.Fatal("output has no FABRIC CONTROLLERS container")
	}
	if firstEdge > firstContainer {
		t.Error("an edge cell is declared after the first container cell; containers would not render on top")
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	first := renderTopology(t, doc, g)
	second := renderTopology(t, doc, g)
	if first != second {
		t.Error("Render produced different output across two runs over the same input")
	}
}

func TestRenderRespectsCancellation(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	r := NewTopologyRenderer(g, doc.Metadata, time.Unix(0, 0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := r.Render(ctx, &buf); err == nil {
		t.Fatal("Render returned nil error on a cancelled context")
	}
}
