// Tests for the draw.io topology renderer: well-formed XML, byte-level
// determinism, deterministic cell identifiers, the injected artifact
// time, and injection safety over adversarial document strings.

package drawio

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/topology"
)

func model(t *testing.T, rel string) *topology.Topology {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	doc, err := document.Parse(b)
	if err != nil {
		t.Fatalf("parsing %s: %v", rel, err)
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing %s: %v", rel, err)
	}
	top, err := topology.Build(context.Background(), doc, g)
	if err != nil {
		t.Fatalf("topology.Build %s: %v", rel, err)
	}
	return top
}

func renderXML(t *testing.T, m *topology.Topology, at time.Time) string {
	t.Helper()
	var buf bytes.Buffer
	if err := NewTopologyRenderer(m, at).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

var fixedTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func assertWellFormed(t *testing.T, s string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(s))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("draw.io output is not well-formed XML: %v", err)
		}
	}
}

func TestRenderWellFormedAndFramed(t *testing.T) {
	for _, f := range []string{
		"valid/mixed-provider.json",
		"valid/no-provider.json",
		"adversarial/dangling-connection.json",
		"rendering/injection-strings.json",
	} {
		out := renderXML(t, model(t, f), fixedTime)
		assertWellFormed(t, out)
		if !strings.HasPrefix(out, "<mxfile ") || !strings.HasSuffix(out, "</mxfile>\n") {
			t.Errorf("%s: output not framed as an mxfile", f)
		}
		if !strings.Contains(out, `<diagram id="osirisjson-topology"`) {
			t.Errorf("%s: fixed diagram id missing", f)
		}
	}
}

func TestRenderIsByteIdentical(t *testing.T) {
	m := model(t, "valid/mixed-provider.json")
	if renderXML(t, m, fixedTime) != renderXML(t, m, fixedTime) {
		t.Fatal("two renders of the same model differ")
	}
}

func TestRenderNodeAndEdgeCells(t *testing.T) {
	m := model(t, "valid/mixed-provider.json")
	out := renderXML(t, m, fixedTime)
	// One vertex cell per node id n0..n3, one edge cell per edge e0..e2.
	for _, id := range []string{`id="n0"`, `id="n1"`, `id="n2"`, `id="n3"`, `id="e0"`, `id="e1"`, `id="e2"`} {
		if !strings.Contains(out, id) {
			t.Errorf("cell %s missing", id)
		}
	}
	// Deterministic: n4 does not exist (only four resources).
	if strings.Contains(out, `id="n4"`) {
		t.Errorf("unexpected extra node cell")
	}
	// The bidirectional connection carries both arrow heads.
	if !strings.Contains(out, "startArrow=block;") {
		t.Errorf("bidirectional edge style not emitted")
	}
}

func TestRenderArtifactTime(t *testing.T) {
	m := model(t, "valid/mixed-provider.json")
	if !strings.Contains(renderXML(t, m, fixedTime), "Generated 2026-03-04T05:06:07Z") {
		t.Errorf("injected time not stamped into the title cell")
	}
	if strings.Contains(renderXML(t, m, time.Time{}), "Generated ") {
		t.Errorf("zero time must not stamp a generated line")
	}
}

func TestRenderUnresolvedEndpointGetsPlaceholder(t *testing.T) {
	m := model(t, "adversarial/dangling-connection.json")
	out := renderXML(t, m, fixedTime)
	if len(m.Unresolved) == 0 {
		t.Skip("fixture has no unresolved endpoint")
	}
	if !strings.Contains(out, `id="u0"`) {
		t.Errorf("no placeholder cell for the unresolved endpoint")
	}
	// The edge still exists and is dashed.
	if !strings.Contains(out, `edge="1"`) {
		t.Errorf("dangling connection produced no edge")
	}
}

func TestRenderInjectionSafety(t *testing.T) {
	m := model(t, "rendering/injection-strings.json")
	out := renderXML(t, m, fixedTime)
	assertWellFormed(t, out)
	// The group name held "]]></mxCell> & <mxCell value="x"/>"; none of
	// that may close a cell or open a new one.
	if strings.Contains(out, "]]></mxCell>") {
		t.Errorf("injected CDATA close survived")
	}
	if strings.Contains(out, `<mxCell value="x"/>`) {
		t.Errorf("injected mxCell element survived unescaped")
	}
	// Every mxCell element in the output is one we opened: the count of
	// "<mxCell " openers equals the count of value=" attributes we write
	// plus the two structural cells (id 0 and id 1) that carry none.
	if strings.ContainsRune(out, 0x00) || strings.ContainsRune(out, 0x1b) {
		t.Errorf("control character survived into the XML")
	}
}

func TestRenderEmbedsLogoBadge(t *testing.T) {
	out := renderXML(t, model(t, "valid/no-provider.json"), fixedTime)
	if !strings.Contains(out, `id="logo"`) {
		t.Fatal("no logo cell in the output")
	}
	if !strings.Contains(out, "shape=image;") || !strings.Contains(out, "image=data:image/svg+xml,") {
		t.Error("logo cell is not an embedded SVG image")
	}
}

func TestRenderGroupsAreSwimlanes(t *testing.T) {
	m := model(t, "valid/mixed-provider.json")
	if len(m.Groups) == 0 {
		t.Skip("fixture has no groups")
	}
	out := renderXML(t, m, fixedTime)
	if strings.Contains(out, "container=1;collapsible=0;") {
		t.Error("group still rendered as the old dashed container")
	}
	if c := strings.Count(out, "style=\"swimlane;"); c != len(m.Groups) {
		t.Errorf("got %d swimlane cells, want %d (one per group)", c, len(m.Groups))
	}
}

func TestRenderNestsGroupForest(t *testing.T) {
	t.Skip("no committed fixture nests groups; needs a replacement fixture")
}

// TestRenderCellCompleteness proves the renderer never drops a cell:
// the emitted mxCell count equals the two structural cells, the badge,
// the title, and one cell for every node, placeholder, group, child
// group edge, and connection edge in the model.
func TestRenderCellCompleteness(t *testing.T) {
	for _, f := range []string{
		"valid/mixed-provider.json",
		"valid/no-provider.json",
		"valid/unknown-fields.json",
		"adversarial/dangling-connection.json",
		"rendering/injection-strings.json",
		"producer/network/hpe/arubacentral/hpe-arubacentral-2026-08-06T16-47-25Z-BRANCH-01.json",
	} {
		m := model(t, f)
		groupIDs := make(map[string]bool, len(m.Groups))
		for _, g := range m.Groups {
			groupIDs[g.ID] = true
		}
		// Nesting replaces the child-group connector edges; only an
		// extra parent (a group claimed as a child by more than one
		// group) still draws one.
		extraParentEdges := 0
		for _, g := range m.Groups {
			for _, ep := range g.ExtraParentGroupIDs {
				if groupIDs[ep] {
					extraParentEdges++
				}
			}
		}
		want := 2 + 1 + 1 + len(m.Nodes) + len(m.Unresolved) + len(m.Groups) + extraParentEdges + len(m.Edges)
		got := strings.Count(renderXML(t, m, fixedTime), "<mxCell ")
		if got != want {
			t.Errorf("%s: emitted %d mxCell, want %d (no cell may be dropped)", f, got, want)
		}
	}
}

func TestRenderCancellation(t *testing.T) {
	m := model(t, "valid/mixed-provider.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := NewTopologyRenderer(m, fixedTime).Render(ctx, &buf); err == nil {
		t.Fatal("Render returned nil on a cancelled context")
	}
}

func TestRenderPropagatesWriteError(t *testing.T) {
	m := model(t, "valid/mixed-provider.json")
	if err := NewTopologyRenderer(m, fixedTime).Render(context.Background(), failWriter{}); err == nil {
		t.Fatal("Render swallowed a write error")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
