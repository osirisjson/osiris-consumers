// Package graph builds generic, vendor-neutral indexes and projections
// over a read-only OSIRIS JSON document view.
//
// Everything here is literal. Edges carry the connection's own source,
// target, direction, and type with no interpretation: a "contains"
// connection is an edge of type "contains", not a parent-child
// hierarchy. Identifiers are opaque and case-sensitive.
// Unknown resource and connection types are indexed like any other.
// A mixed-provider or wholly unrecognised document produces a complete
// graph with nothing dropped.
//
// Build runs in time linear in the number of resources, connections,
// groups, and group-membership entries. It assumes its input already
// passed canonical validation, but it still records structural
// anomalies (duplicate ids, dangling endpoints, self references, group
// cycles) defensively rather than trusting that validation ran.
package graph

import "go.osirisjson.org/consumers/internal/document"

// A Graph is the generic index over one document. All exported maps are
// read-only once Build returns; callers must not mutate them.
type Graph struct {
	// Resources, Connections, and Groups map an identifier to the
	// first element declared with that identifier. A duplicate
	// identifier is recorded as an Anomaly and does not overwrite the
	// first element.
	Resources   map[string]*document.Resource
	Connections map[string]*document.Connection
	Groups      map[string]*document.Group

	// ResourceIDs, ConnectionIDs, and GroupIDs preserve declaration
	// order for deterministic traversal. They include every element,
	// duplicates too.
	ResourceIDs   []string
	ConnectionIDs []string
	GroupIDs      []string

	// Edges holds one literal directed edge per connection, in
	// declaration order.
	Edges []Edge
	// OutEdges and InEdges map a resource identifier to indexes into
	// Edges where it is the source or the target. An identifier that
	// appears only as a dangling endpoint still gets an entry.
	OutEdges map[string][]int
	InEdges  map[string][]int

	// GroupMembers and GroupChildren are the literal identifier lists
	// from each group, keyed by group identifier.
	GroupMembers  map[string][]string
	GroupChildren map[string][]string
	// MemberOfGroups and ParentOfGroups are the reverse directions:
	// referenced identifier to the groups that name it.
	MemberOfGroups map[string][]string
	ParentOfGroups map[string][]string

	// ByProvider buckets resource identifiers by their provider name,
	// verbatim. ByType buckets resource identifiers by resource type.
	// ConnectionsByType buckets connection identifiers by connection
	// type. Bucket order follows resource/connection declaration order.
	ByProvider        map[string][]string
	ByType            map[string][]string
	ConnectionsByType map[string][]string

	// Anomalies are structural problems found while indexing, in a
	// deterministic order (see AnomalyKind). An empty slice means the
	// document's internal references are all consistent.
	Anomalies []Anomaly
}

// An Edge is one connection rendered as a literal directed edge.
// Direction is kept exactly as written; an empty Direction means the
// connection omitted the field (OSIRIS JSON consumers may treat that as
// "bidirectional", but Build does not rewrite it).
type Edge struct {
	ConnectionID string
	Source       string
	Target       string
	Direction    string
	Type         string
}

// ResourceExists reports whether id was declared as a resource.
func (g *Graph) ResourceExists(id string) bool {
	_, ok := g.Resources[id]
	return ok
}
