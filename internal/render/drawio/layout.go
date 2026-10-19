// This file computes the deterministic placement of a topology once, so
// the page-size decision (ChoosePage) and XML emission (Render) work
// from exactly the same geometry. Placement depends only on the model
// and the layout constants below, never on a clock or a random source.
//
// The layout is a vertical stack: a provenance badge and a title cell,
// then the group forest as nested swimlanes (a parent group physically
// contains its child group swimlanes and its primary member nodes,
// recursively), then ungrouped nodes, then a placeholder per unresolved
// endpoint. Groups with no parent are the roots; a document with no
// groups (a single switch, for example) has no root swimlane at all.

package drawio

import (
	"strconv"

	"go.osirisjson.org/consumers/internal/topology"
)

// Layout geometry, in mxGraph user units.
const (
	nodeW        = 200
	nodeH        = 60
	hGap         = 40
	vGap         = 40
	pad          = 24
	titleH       = 30 // swimlane title-bar height; also the member inset
	sectionGap   = 60
	pageMargin   = 20
	logoGap      = 8
	titleW       = 480
	titleCellH   = 48
	placeholderW = 200
	placeholderH = 60
)

// rect is an axis-aligned cell rectangle in mxGraph user units.
type rect struct{ x, y, w, h int }

func (r rect) right() int  { return r.x + r.w }
func (r rect) bottom() int { return r.y + r.h }

// placedNode is one resource or placeholder cell. parent is the id of
// the containing cell; geom is relative to that parent when parent is a
// swimlane, absolute when parent is the base layer.
type placedNode struct {
	id     string
	value  string
	style  string
	geom   rect
	parent string
}

// placedGroup is one swimlane with its nested member nodes and child
// swimlanes. geom is absolute for a root and relative to the parent
// swimlane for a nested group.
type placedGroup struct {
	id       string
	value    string
	geom     rect
	members  []placedNode
	children []placedGroup
}

// layout is the full deterministic placement of a topology.
type layout struct {
	logo       rect
	title      rect
	roots      []placedGroup
	ungrouped  []placedNode
	unresolved []placedNode
}

// extent returns the content bounding box measured from the origin plus
// a right and bottom quiet zone of one page margin. Nested cells are
// bounded by their root swimlane, so only roots and base-layer cells
// are measured.
func (l layout) extent() (w, h int) {
	acc := func(r rect) {
		if r.right() > w {
			w = r.right()
		}
		if r.bottom() > h {
			h = r.bottom()
		}
	}
	acc(l.logo)
	acc(l.title)
	for _, g := range l.roots {
		acc(g.geom)
	}
	for _, n := range l.ungrouped {
		acc(n.geom)
	}
	for _, n := range l.unresolved {
		acc(n.geom)
	}
	return w + pageMargin, h + pageMargin
}

// cellIDs holds the deterministic cell identifiers, derived from the
// model's sorted traversal order.
type cellIDs struct {
	node       map[string]string
	unresolved map[string]string
	group      map[string]string
}

func newCellIDs(m *topology.Topology) cellIDs {
	ids := cellIDs{
		node:       make(map[string]string, len(m.Nodes)),
		unresolved: make(map[string]string, len(m.Unresolved)),
		group:      make(map[string]string, len(m.Groups)),
	}
	for i, n := range m.Nodes {
		ids.node[n.ID] = "n" + strconv.Itoa(i)
	}
	for i, id := range m.Unresolved {
		ids.unresolved[id] = "u" + strconv.Itoa(i)
	}
	for i, g := range m.Groups {
		ids.group[g.ID] = "g" + strconv.Itoa(i)
	}
	return ids
}

// buildLayout places every cell of m: the badge and title top-left,
// then each root swimlane stacked downward, then ungrouped nodes, then
// unresolved placeholders.
func buildLayout(m *topology.Topology, ids cellIDs) layout {
	var l layout
	l.logo = rect{pageMargin, pageMargin, LogoIntrinsicW, LogoIntrinsicH}

	cursorY := pageMargin + LogoIntrinsicH + logoGap
	l.title = rect{pageMargin, cursorY, titleW, titleCellH}
	cursorY += titleCellH + sectionGap

	byID := make(map[string]*topology.Group, len(m.Groups))
	childIDs := make(map[string][]string) // parent id -> child ids, in sorted (m.Groups) order
	for i := range m.Groups {
		g := &m.Groups[i]
		byID[g.ID] = g
		if g.ParentGroupID != "" {
			childIDs[g.ParentGroupID] = append(childIDs[g.ParentGroupID], g.ID)
		}
	}
	primaryOf := make(map[string][]topology.Node) // group id -> its primary members, in sorted node order
	for _, n := range m.Nodes {
		if n.PrimaryGroup != "" {
			primaryOf[n.PrimaryGroup] = append(primaryOf[n.PrimaryGroup], n)
		}
	}

	seen := make(map[string]bool)
	var place func(gid string) placedGroup
	place = func(gid string) placedGroup {
		pg := placedGroup{id: ids.group[gid]}
		if g := byID[gid]; g != nil {
			pg.value = groupValue(*g)
		}
		if seen[gid] {
			pg.geom = rect{0, 0, nodeW + 2*pad, titleH + 2*pad}
			return pg
		}
		seen[gid] = true

		y := titleH + pad
		innerW := nodeW

		mem := primaryOf[gid]
		if len(mem) > 0 {
			cols, rows := grid(len(mem))
			for j, n := range mem {
				col, row := j%cols, j/cols
				pg.members = append(pg.members, placedNode{
					id:     ids.node[n.ID],
					value:  nodeValue(n),
					style:  nodeStyle,
					geom:   rect{pad + col*(nodeW+hGap), y + row*(nodeH+vGap), nodeW, nodeH},
					parent: pg.id,
				})
			}
			if blockW := cols*nodeW + maxInt(cols-1, 0)*hGap; blockW > innerW {
				innerW = blockW
			}
			y += rows*nodeH + maxInt(rows-1, 0)*vGap + vGap
		}

		for _, cid := range childIDs[gid] {
			cpg := place(cid)
			cpg.geom.x = pad
			cpg.geom.y = y
			if cpg.geom.w > innerW {
				innerW = cpg.geom.w
			}
			pg.children = append(pg.children, cpg)
			y += cpg.geom.h + vGap
		}

		if len(mem) == 0 && len(pg.children) == 0 {
			pg.geom = rect{0, 0, nodeW + 2*pad, titleH + 2*pad}
			return pg
		}
		pg.geom = rect{0, 0, innerW + 2*pad, y - vGap + pad}
		return pg
	}

	placeRoot := func(gid string) {
		root := place(gid)
		root.geom.x = pageMargin
		root.geom.y = cursorY
		l.roots = append(l.roots, root)
		cursorY += root.geom.h + sectionGap
	}

	for i := range m.Groups {
		if m.Groups[i].ParentGroupID == "" {
			placeRoot(m.Groups[i].ID)
		}
	}
	// Defence in depth: a group left unreached by any root (only
	// possible through a residual cycle) is promoted to a root so no
	// swimlane is dropped.
	for i := range m.Groups {
		if !seen[m.Groups[i].ID] {
			placeRoot(m.Groups[i].ID)
		}
	}

	var ungrouped []topology.Node
	for _, n := range m.Nodes {
		if n.PrimaryGroup == "" {
			ungrouped = append(ungrouped, n)
		}
	}
	if len(ungrouped) > 0 {
		cols, rows := grid(len(ungrouped))
		for j, n := range ungrouped {
			col, row := j%cols, j/cols
			l.ungrouped = append(l.ungrouped, placedNode{
				id:    ids.node[n.ID],
				value: nodeValue(n),
				style: nodeStyle,
				geom:  rect{pageMargin + col*(nodeW+hGap), cursorY + row*(nodeH+vGap), nodeW, nodeH},
			})
		}
		cursorY += rows*(nodeH+vGap) + sectionGap
	}

	if len(m.Unresolved) > 0 {
		cols, _ := grid(len(m.Unresolved))
		for j, id := range m.Unresolved {
			col, row := j%cols, j/cols
			l.unresolved = append(l.unresolved, placedNode{
				id:    ids.unresolved[id],
				value: Attr("unresolved\n" + id),
				style: placeholderStyle,
				geom:  rect{pageMargin + col*(placeholderW+hGap), cursorY + row*(placeholderH+vGap), placeholderW, placeholderH},
			})
		}
	}

	return l
}
