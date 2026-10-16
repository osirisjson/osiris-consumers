// This file implements Build: it walks the read-only document view and
// the generic graph index once and assembles the literal Topology,
// sorting every collection by a total, documented key.

package topology

import (
	"context"
	"sort"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// ctxCheckInterval is how often the element loops poll ctx.Err().
const ctxCheckInterval = 1024

// Build assembles the Topology from a validated document view and the
// generic graph index over the same document.
//
// It reads per resource: id, type, name, provider.name; per connection:
// id, type, source, target, direction; per group: id, type, name,
// members, children. Nothing else.
//
// The only error Build returns is ctx.Err() on cancellation.
func Build(ctx context.Context, doc *document.Document, g *graph.Graph) (*Topology, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t := &Topology{}

	// Groups first, sorted by ID, so the parent forest and PrimaryGroup
	// are computed in a single total order.
	groups := append([]document.Group(nil), doc.Topology.Groups...)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })

	isGroup := make(map[string]bool, len(groups))
	for i := range groups {
		isGroup[groups[i].ID] = true
	}

	// childToParents: for each group, every group that lists it as a
	// child, in sorted-parent order (groups is already sorted).
	childToParents := make(map[string][]string)
	for i := range groups {
		grp := &groups[i]
		for _, c := range grp.Children {
			if c == grp.ID || !isGroup[c] {
				continue
			}
			childToParents[c] = append(childToParents[c], grp.ID)
		}
	}

	// parentOf picks the first parent that does not close a cycle; the
	// rest become ExtraParentGroupIDs. Children are resolved in sorted
	// order so the forest is deterministic.
	parentOf := make(map[string]string)
	extraParents := make(map[string][]string)
	sortedChildren := make([]string, 0, len(childToParents))
	for c := range childToParents {
		sortedChildren = append(sortedChildren, c)
	}
	sort.Strings(sortedChildren)
	for _, c := range sortedChildren {
		for _, p := range childToParents[c] {
			if _, set := parentOf[c]; !set && !reachesVia(parentOf, p, c) {
				parentOf[c] = p
			} else {
				extraParents[c] = append(extraParents[c], p)
			}
		}
	}

	// depth is the length of the parent chain, memoized and cycle-safe.
	depthMemo := make(map[string]int)
	var depthOf func(id string) int
	depthOf = func(id string) int {
		if d, ok := depthMemo[id]; ok {
			return d
		}
		depthMemo[id] = 0 // break any residual cycle
		p, ok := parentOf[id]
		if !ok {
			return 0
		}
		d := 1 + depthOf(p)
		depthMemo[id] = d
		return d
	}

	primaryGroup := make(map[string]string) // resource ID -> deepest claiming group
	primaryDepth := make(map[string]int)
	for i := range groups {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		grp := &groups[i]
		gd := depthOf(grp.ID)
		var memberNodes []string
		other := 0
		for _, m := range grp.Members {
			if _, ok := g.Resources[m]; ok {
				memberNodes = append(memberNodes, m)
				if _, seen := primaryGroup[m]; !seen || gd > primaryDepth[m] {
					primaryGroup[m] = grp.ID
					primaryDepth[m] = gd
				}
			} else {
				other++
			}
		}
		var childGroups []string
		for _, c := range grp.Children {
			if _, ok := g.Groups[c]; ok {
				childGroups = append(childGroups, c)
			}
		}
		t.Groups = append(t.Groups, Group{
			ID:                  grp.ID,
			Label:               labelOr(grp.Name, grp.ID),
			Type:                grp.Type,
			MemberNodeIDs:       memberNodes,
			MemberOther:         other,
			ChildGroupIDs:       childGroups,
			ParentGroupID:       parentOf[grp.ID],
			ExtraParentGroupIDs: extraParents[grp.ID],
		})
	}

	// Nodes, sorted by ID.
	res := doc.Topology.Resources
	for i := range res {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		r := &res[i]
		t.Nodes = append(t.Nodes, Node{
			ID:           r.ID,
			Label:        labelOr(r.Name, r.ID),
			Type:         r.Type,
			Provider:     r.Provider.Name,
			Known:        false,
			PrimaryGroup: primaryGroup[r.ID],
		})
	}
	sort.SliceStable(t.Nodes, func(i, j int) bool { return t.Nodes[i].ID < t.Nodes[j].ID })

	// Edges, one per connection, sorted by ID, source, target, type.
	unresolved := make(map[string]struct{})
	conns := doc.Topology.Connections
	for i := range conns {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		c := &conns[i]
		_, srcOK := g.Resources[c.Source]
		_, dstOK := g.Resources[c.Target]
		if !srcOK {
			unresolved[c.Source] = struct{}{}
		}
		if !dstOK {
			unresolved[c.Target] = struct{}{}
		}
		t.Edges = append(t.Edges, Edge{
			ID:             c.ID,
			Source:         c.Source,
			Target:         c.Target,
			Direction:      c.Direction,
			Type:           c.Type,
			SourceResolved: srcOK,
			TargetResolved: dstOK,
			Label:          labelOr(c.Type, c.ID),
		})
	}
	sort.SliceStable(t.Edges, func(i, j int) bool {
		a, b := t.Edges[i], t.Edges[j]
		switch {
		case a.ID != b.ID:
			return a.ID < b.ID
		case a.Source != b.Source:
			return a.Source < b.Source
		case a.Target != b.Target:
			return a.Target < b.Target
		default:
			return a.Type < b.Type
		}
	})

	for id := range unresolved {
		t.Unresolved = append(t.Unresolved, id)
	}
	sort.Strings(t.Unresolved)

	return t, nil
}

// reachesVia reports whether following parentOf upward from start
// arrives at target, so a candidate parent assignment that would close
// a cycle can be refused. It stops after len(parentOf)+1 hops as a
// backstop against a pre-existing cycle in parentOf.
func reachesVia(parentOf map[string]string, start, target string) bool {
	cur := start
	for hops := 0; hops <= len(parentOf); hops++ {
		if cur == target {
			return true
		}
		next, ok := parentOf[cur]
		if !ok {
			return false
		}
		cur = next
	}
	return false
}

// labelOr returns primary when it is non-empty, otherwise fallback.
func labelOr(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
