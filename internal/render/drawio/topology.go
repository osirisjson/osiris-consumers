// This file is the deterministic draw.io (mxGraph) renderer for the
// generic topology model. It emits the XML entirely in code, streaming
// to the writer in one pass: a provenance badge, a title cell, one
// swimlane per group, one rectangle per resource, one placeholder per
// unresolved endpoint, and one edge per connection carrying the literal
// direction and type. Cell identifiers and geometry come from the
// model's sorted traversal order (see layout.go), never from a clock or
// a random source, so identical model plus identical artifact time
// produce byte-identical output. The page size is chosen to contain the
// whole drawing (see page.go); nothing is ever scaled or dropped to
// fit. The only variable input is the injected artifact time.

package drawio

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/topology"
)

// TopologyRenderer renders a topology.Topology as a draw.io file.
// The zero value is not usable; construct one with NewTopologyRenderer.
type TopologyRenderer struct {
	model    *topology.Topology
	artifact time.Time
}

// NewTopologyRenderer returns a renderer for model. artifactTime is
// stamped into a title cell; the zero time means "omit the generated
// line" and keeps the output free of any clock reading.
func NewTopologyRenderer(model *topology.Topology, artifactTime time.Time) *TopologyRenderer {
	return &TopologyRenderer{model: model, artifact: artifactTime.UTC()}
}

// Render writes the draw.io XML to w. It checks ctx at collection
// boundaries and returns the first write error without wrapping it
// away.
func (r *TopologyRenderer) Render(ctx context.Context, w io.Writer) error {
	cw := render.NewCheckedWriter(w)
	p := &xmlPrinter{w: cw}
	m := r.model

	ids := newCellIDs(m)
	l := buildLayout(m, ids)
	page := choosePageForExtent(l.extent())

	p.raw(`<mxfile host="osirisjson-consumer">` + "\n")
	p.raw(`  <diagram id="osirisjson-topology" name="OSIRIS JSON topology osirisjson.org">` + "\n")
	p.raw(fmt.Sprintf(
		`    <mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="0" arrows="1" fold="1" page="1" pageScale="1" pageWidth="%d" pageHeight="%d" math="0" shadow="0">`+"\n",
		page.Width, page.Height))
	p.raw(`      <root>` + "\n")
	p.raw(`        <mxCell id="0" />` + "\n")
	p.raw(`        <mxCell id="1" parent="0" />` + "\n")

	// Provenance badge and title.
	p.cell("logo", "1", LogoStyle, "", geomRect(l.logo), false, false)
	p.cell("title", "1", titleStyle, r.titleValue(m, page), geomRect(l.title), false, false)

	if err := ctx.Err(); err != nil {
		return err
	}

	// The group forest as nested swimlanes: a parent swimlane, then its
	// member nodes, then its child swimlanes, recursively.
	var emitGroup func(g placedGroup, parentID string) error
	emitGroup = func(g placedGroup, parentID string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.cell(g.id, parentID, groupStyle, g.value, geomRect(g.geom), true, false)
		for _, n := range g.members {
			p.cell(n.id, n.parent, n.style, n.value, geomRect(n.geom), false, true)
		}
		for _, c := range g.children {
			if err := emitGroup(c, g.id); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range l.roots {
		if err := emitGroup(root, "1"); err != nil {
			return err
		}
	}

	// Ungrouped nodes on the base layer.
	for _, n := range l.ungrouped {
		p.cell(n.id, "1", n.style, n.value, geomRect(n.geom), false, true)
	}

	// Placeholder nodes for unresolved endpoints.
	for _, n := range l.unresolved {
		p.cell(n.id, "1", n.style, n.value, geomRect(n.geom), false, true)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// Nesting shows the primary parent of every group. A group also
	// listed as a child by further groups keeps a dashed link from each
	// of those extra parents so the membership is not lost.
	edgeSeq := 0
	for _, grp := range m.Groups {
		for _, ep := range grp.ExtraParentGroupIDs {
			epc, ok := ids.group[ep]
			if !ok {
				continue
			}
			p.edge("gc"+strconv.Itoa(edgeSeq), epc, ids.group[grp.ID], childEdgeStyle, "child group")
			edgeSeq++
		}
	}

	// One edge per connection. The literal direction drives the arrow
	// heads (see edgeStyle); the edge carries no text label.
	for i, e := range m.Edges {
		src := endpointCell(e.Source, e.SourceResolved, ids.node, ids.unresolved)
		dst := endpointCell(e.Target, e.TargetResolved, ids.node, ids.unresolved)
		p.edge("e"+strconv.Itoa(i), src, dst, edgeStyle(e), "")
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	p.raw(`      </root>` + "\n")
	p.raw(`    </mxGraphModel>` + "\n")
	p.raw(`  </diagram>` + "\n")
	p.raw(`</mxfile>` + "\n")

	return cw.Err()
}

// titleValue builds the title-cell label: the fixed heading, the exact
// counts, the chosen page size (with an explicit note when it is a
// custom sheet), and the injected artifact time when one is set.
func (r *TopologyRenderer) titleValue(m *topology.Topology, page PageInfo) string {
	c := m.Counts()
	label := fmt.Sprintf("OSIRIS JSON topology\n%d nodes, %d edges, %d groups",
		c.Nodes, c.Edges, c.Groups)
	if c.Unresolved > 0 {
		label += fmt.Sprintf(", %d unresolved", c.Unresolved)
	}
	if page.Format == "custom" {
		label += fmt.Sprintf("\npage: custom %dx%d (exceeds ISO A0, not a standard print size)",
			page.Width, page.Height)
	} else {
		label += fmt.Sprintf("\npage: %s landscape %dx%d", page.Format, page.Width, page.Height)
	}
	if !r.artifact.IsZero() {
		label += "\nGenerated " + r.artifact.Format(time.RFC3339)
	}
	return Attr(label)
}

// --- styles ----------------------------------------------------------------

const (
	titleStyle       = "text;html=1;align=left;verticalAlign=top;whiteSpace=wrap;strokeColor=none;fillColor=none;"
	nodeStyle        = "rounded=0;whiteSpace=wrap;html=1;verticalAlign=middle;align=center;"
	groupStyle       = "swimlane;html=1;whiteSpace=wrap;rounded=0;startSize=30;collapsible=0;fillColor=none;verticalAlign=middle;align=center;fontSize=10;"
	placeholderStyle = "rounded=0;whiteSpace=wrap;html=1;dashed=1;fillColor=none;align=center;"
	childEdgeStyle   = "edgeStyle=orthogonalEdgeStyle;rounded=0;html=1;dashed=1;endArrow=open;startArrow=none;"
)

// edgeStyle maps the literal direction onto arrow heads and marks an
// edge with an unresolved endpoint dashed.
func edgeStyle(e topology.Edge) string {
	base := "edgeStyle=orthogonalEdgeStyle;rounded=0;html=1;"
	switch e.Direction {
	case "reverse":
		base += "endArrow=none;startArrow=block;"
	case "bidirectional":
		base += "endArrow=block;startArrow=block;"
	default: // "forward" or omitted
		base += "endArrow=block;startArrow=none;"
	}
	if !e.SourceResolved || !e.TargetResolved {
		base += "dashed=1;"
	}
	return base
}

// nodeValue is the resource label only: the name, or the opaque ID when
// the resource carried no name.
func nodeValue(n topology.Node) string {
	return Attr(n.Label)
}

// groupValue is the container name only.
func groupValue(g topology.Group) string {
	return Attr(g.Label)
}

func endpointCell(id string, resolved bool, nodeCell, unresolvedCell map[string]string) string {
	if resolved {
		return nodeCell[id]
	}
	return unresolvedCell[id]
}

// grid returns the column and row count for laying out n cells in as
// square a grid as possible, deterministically.
func grid(n int) (cols, rows int) {
	if n <= 0 {
		return 1, 1
	}
	cols = 1
	for cols*cols < n {
		cols++
	}
	rows = (n + cols - 1) / cols
	return cols, rows
}

func geomRect(r rect) string {
	return fmt.Sprintf(`<mxGeometry x="%d" y="%d" width="%d" height="%d" as="geometry" />`, r.x, r.y, r.w, r.h)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// xml printer
type xmlPrinter struct {
	w *render.CheckedWriter
}

func (p *xmlPrinter) raw(s string) { _, _ = p.w.WriteString(s) }

// cell writes one vertex mxCell. value is already attribute-escaped.
func (p *xmlPrinter) cell(id, parent, style, value, geometry string, isContainer, isVertexChild bool) {
	_ = isContainer
	_ = isVertexChild
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" vertex="1" parent="%s">`+"\n",
		Attr(id), value, style, Attr(parent),
	))
	p.raw("          " + geometry + "\n")
	p.raw("        </mxCell>\n")
}

// edge writes one edge mxCell. label is already attribute-escaped.
func (p *xmlPrinter) edge(id, source, target, style, label string) {
	p.raw(fmt.Sprintf(
		`        <mxCell id="%s" value="%s" style="%s" edge="1" parent="1" source="%s" target="%s">`+"\n",
		Attr(id), label, style, Attr(source), Attr(target),
	))
	p.raw(`          <mxGeometry relative="1" as="geometry" />` + "\n")
	p.raw("        </mxCell>\n")
}
