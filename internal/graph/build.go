// This file implements Build: a single linear pass that indexes a
// document view and a defensive structural check that records anomalies
// without failing.

package graph

import (
	"context"
	"fmt"
	"sort"

	"go.osirisjson.org/consumers/internal/document"
)

// ctxCheckInterval is how often the array loops poll ctx.Err(), so
// cancellation is prompt on a large document without a per-element
// syscall.
const ctxCheckInterval = 1024

// Build indexes doc into a Graph. It never mutates doc.
//
// The only error Build returns is ctx.Err() when the context is
// cancelled during the pass; every structural problem in the document
// is reported through Graph.Anomalies instead.
// Build assumes doc already passed canonical validation, so a returned
// Graph with a non-empty Anomalies slice means either validation was
// skipped or the validator and this check disagree.
func Build(ctx context.Context, doc *document.Document) (*Graph, error) {
	if doc == nil {
		return nil, fmt.Errorf("graph: nil document")
	}
	res := doc.Topology.Resources
	conns := doc.Topology.Connections
	groups := doc.Topology.Groups

	g := &Graph{
		Resources:         make(map[string]*document.Resource, len(res)),
		Connections:       make(map[string]*document.Connection, len(conns)),
		Groups:            make(map[string]*document.Group, len(groups)),
		ResourceIDs:       make([]string, 0, len(res)),
		ConnectionIDs:     make([]string, 0, len(conns)),
		GroupIDs:          make([]string, 0, len(groups)),
		Edges:             make([]Edge, 0, len(conns)),
		OutEdges:          make(map[string][]int),
		InEdges:           make(map[string][]int),
		GroupMembers:      make(map[string][]string, len(groups)),
		GroupChildren:     make(map[string][]string, len(groups)),
		MemberOfGroups:    make(map[string][]string),
		ParentOfGroups:    make(map[string][]string),
		ByProvider:        make(map[string][]string),
		ByType:            make(map[string][]string),
		ConnectionsByType: make(map[string][]string),
	}
	var anomalies []Anomaly

	// Pass 1: resources.
	for i := range res {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		r := &res[i]
		g.ResourceIDs = append(g.ResourceIDs, r.ID)
		if _, dup := g.Resources[r.ID]; dup {
			anomalies = append(anomalies, Anomaly{
				Kind:    DuplicateResourceID,
				Subject: r.ID,
				Detail:  fmt.Sprintf("resource id %q declared more than once", r.ID),
			})
			continue
		}
		g.Resources[r.ID] = r
		g.ByProvider[r.Provider.Name] = append(g.ByProvider[r.Provider.Name], r.ID)
		g.ByType[r.Type] = append(g.ByType[r.Type], r.ID)
	}

	// Pass 2: connections and literal edges.
	for i := range conns {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		c := &conns[i]
		g.ConnectionIDs = append(g.ConnectionIDs, c.ID)
		if _, dup := g.Connections[c.ID]; dup {
			anomalies = append(anomalies, Anomaly{
				Kind:    DuplicateConnectionID,
				Subject: c.ID,
				Detail:  fmt.Sprintf("connection id %q declared more than once", c.ID),
			})
		} else {
			g.Connections[c.ID] = c
			g.ConnectionsByType[c.Type] = append(g.ConnectionsByType[c.Type], c.ID)
		}

		idx := len(g.Edges)
		g.Edges = append(g.Edges, Edge{
			ConnectionID: c.ID,
			Source:       c.Source,
			Target:       c.Target,
			Direction:    c.Direction,
			Type:         c.Type,
		})
		g.OutEdges[c.Source] = append(g.OutEdges[c.Source], idx)
		g.InEdges[c.Target] = append(g.InEdges[c.Target], idx)

		if _, ok := g.Resources[c.Source]; !ok {
			anomalies = append(anomalies, Anomaly{
				Kind:      DanglingConnectionSource,
				Subject:   c.ID,
				Reference: c.Source,
				Detail:    fmt.Sprintf("connection %q source %q is not a resource", c.ID, c.Source),
			})
		}
		if _, ok := g.Resources[c.Target]; !ok {
			anomalies = append(anomalies, Anomaly{
				Kind:      DanglingConnectionTarget,
				Subject:   c.ID,
				Reference: c.Target,
				Detail:    fmt.Sprintf("connection %q target %q is not a resource", c.ID, c.Target),
			})
		}
		if c.Source == c.Target {
			anomalies = append(anomalies, Anomaly{
				Kind:      SelfConnection,
				Subject:   c.ID,
				Reference: c.Source,
				Detail:    fmt.Sprintf("connection %q connects %q to itself", c.ID, c.Source),
			})
		}
	}

	// Pass 3: groups, membership, and the reverse directions.
	for i := range groups {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		grp := &groups[i]
		g.GroupIDs = append(g.GroupIDs, grp.ID)
		if _, dup := g.Groups[grp.ID]; dup {
			anomalies = append(anomalies, Anomaly{
				Kind:    DuplicateGroupID,
				Subject: grp.ID,
				Detail:  fmt.Sprintf("group id %q declared more than once", grp.ID),
			})
			continue
		}
		g.Groups[grp.ID] = grp
		g.GroupMembers[grp.ID] = grp.Members
		g.GroupChildren[grp.ID] = grp.Children
		for _, m := range grp.Members {
			g.MemberOfGroups[m] = append(g.MemberOfGroups[m], grp.ID)
		}
		for _, child := range grp.Children {
			g.ParentOfGroups[child] = append(g.ParentOfGroups[child], grp.ID)
		}
	}

	// Pass 4: group reference checks, over the deduplicated groups.
	for _, gid := range g.GroupIDs {
		grp, ok := g.Groups[gid]
		if !ok {
			continue
		}
		for _, m := range grp.Members {
			if !g.knownAnyID(m) {
				anomalies = append(anomalies, Anomaly{
					Kind:      DanglingGroupMember,
					Subject:   gid,
					Reference: m,
					Detail:    fmt.Sprintf("group %q member %q is not a resource, connection, or group", gid, m),
				})
			}
		}
		for _, child := range grp.Children {
			if child == gid {
				anomalies = append(anomalies, Anomaly{
					Kind:      SelfGroupChild,
					Subject:   gid,
					Reference: child,
					Detail:    fmt.Sprintf("group %q lists itself as a child", gid),
				})
				continue
			}
			if _, ok := g.Groups[child]; !ok {
				anomalies = append(anomalies, Anomaly{
					Kind:      DanglingGroupChild,
					Subject:   gid,
					Reference: child,
					Detail:    fmt.Sprintf("group %q child %q is not a group", gid, child),
				})
			}
		}
	}

	anomalies = append(anomalies, detectGroupCycles(g)...)

	sort.SliceStable(anomalies, func(i, j int) bool {
		a, b := anomalies[i], anomalies[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Subject != b.Subject {
			return a.Subject < b.Subject
		}
		return a.Reference < b.Reference
	})
	g.Anomalies = anomalies
	return g, nil
}

// knownAnyID reports whether id names any resource, connection, or group.
func (g *Graph) knownAnyID(id string) bool {
	if _, ok := g.Resources[id]; ok {
		return true
	}
	if _, ok := g.Connections[id]; ok {
		return true
	}
	_, ok := g.Groups[id]
	return ok
}

// detectGroupCycles finds cycles in the group child relation with an
// iterative three-colour DFS. It reports one GroupCycle anomaly per
// group that is on a cycle, so output is deterministic regardless of
// traversal start.
func detectGroupCycles(g *Graph) []Anomaly {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	colour := make(map[string]int, len(g.GroupIDs))
	onCycle := make(map[string]bool)

	type frame struct {
		id   string
		next int
	}

	for _, root := range g.GroupIDs {
		if colour[root] != white {
			continue
		}
		stack := []frame{{id: root}}
		colour[root] = gray
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			children := g.GroupChildren[top.id]
			if top.next < len(children) {
				child := children[top.next]
				top.next++
				if child == top.id {
					continue // self child: handled as SelfGroupChild
				}
				if _, isGroup := g.Groups[child]; !isGroup {
					continue // dangling child: handled as DanglingGroupChild
				}
				switch colour[child] {
				case white:
					colour[child] = gray
					stack = append(stack, frame{id: child})
				case gray:
					// Back edge: every gray frame is on the cycle.
					for _, f := range stack {
						onCycle[f.id] = true
					}
					onCycle[child] = true
				}
				continue
			}
			colour[top.id] = black
			stack = stack[:len(stack)-1]
		}
	}

	var out []Anomaly
	for _, gid := range g.GroupIDs {
		if onCycle[gid] {
			out = append(out, Anomaly{
				Kind:    GroupCycle,
				Subject: gid,
				Detail:  fmt.Sprintf("group %q is part of a child-relation cycle", gid),
			})
		}
	}
	return out
}
