// Package topology builds a format-neutral topology model of one
// canonically validated OSIRIS JSON 1.x document.
//
// The model is literal and complete: every resource becomes exactly one
// node, every connection becomes exactly one edge carrying the
// connection's own source, target, direction, and type, and every group
// is represented with its literal membership. Nothing is inferred.
// Unrecognised resource types are represented the same as any other
// (the Node.Known flag is the hook a later provider enrichment uses to
// swap in a vendor shape without changing counts). Identifiers are
// opaque and case-sensitive.
//
// Every collection is emitted in a total, documented order so a
// renderer over stable input produces byte-identical output.
package topology

// A Topology is the complete literal graph of one document.
type Topology struct {
	// Nodes has one entry per resource, sorted by ID.
	Nodes []Node
	// Edges has one entry per connection, sorted by ID, then source,
	// then target, then type.
	Edges []Edge
	// Groups has one entry per group, sorted by ID.
	Groups []Group
	// Unresolved lists every distinct identifier used as a connection
	// endpoint that names no resource, sorted. A renderer draws these
	// as placeholder nodes so no edge is dropped; they are not
	// resources and are excluded from Counts.Nodes.
	Unresolved []string
}

// A Node is one resource.
type Node struct {
	// ID is the opaque resource identifier.
	ID string
	// Label is the display label: the resource name, or the ID when the
	// resource has no name.
	Label string
	// Type is the resource type verbatim, empty when the resource
	// carried none.
	Type string
	// Provider is the provider name verbatim, empty when the resource
	// carried no provider block.
	Provider string
	// Known reports whether a provider enrichment recognised this
	// resource's type. The generic model always sets it false; the
	// field exists so enrichment can flip it without altering the node
	// set. Counts never depend on it.
	Known bool
	// PrimaryGroup is the ID of the first group (in sorted group order)
	// that lists this node as a member, or empty when the node is in no
	// group. A renderer uses it to place the node inside one container.
	PrimaryGroup string
}

// An Edge is one connection rendered literally.
type Edge struct {
	// ID is the opaque connection identifier.
	ID string
	// Source and Target are the endpoint identifiers verbatim.
	Source string
	Target string
	// Direction is the connection direction verbatim: "forward",
	// "reverse", "bidirectional", or empty when the document omitted
	// it. It is not rewritten.
	Direction string
	// Type is the connection type verbatim, empty when the connection
	// carried none.
	Type string
	// SourceResolved and TargetResolved report whether each endpoint
	// names a resource in the document.
	SourceResolved bool
	TargetResolved bool
	// Label is the edge display label: the connection type, or the ID
	// when the connection has no type.
	Label string
}

// A Group is one group with its literal membership.
type Group struct {
	// ID is the opaque group identifier.
	ID string
	// Label is the group name, or the ID when the group has no name.
	Label string
	// Type is the group type verbatim, empty when the group carried
	// none.
	Type string
	// MemberNodeIDs are the member identifiers that name a resource, in
	// document order. Members that name a connection or another group,
	// or nothing, are not included here (MemberOther keeps their
	// count).
	MemberNodeIDs []string
	// MemberOther is the count of member identifiers that do not name a
	// resource, so a renderer can disclose that the group has more
	// members than it draws.
	MemberOther int
	// ChildGroupIDs are the child identifiers that name a group, in
	// document order.
	ChildGroupIDs []string
	// ParentGroupID is the ID of the group that visually contains this
	// one: the first group (in sorted ID order) that lists this group
	// in its children and does not thereby form a cycle. It is empty
	// when the group is a forest root. A renderer nests each group
	// inside its ParentGroupID.
	ParentGroupID string
	// ExtraParentGroupIDs are any further groups that also list this
	// group as a child, beyond ParentGroupID, in sorted ID order.
	// Nesting uses ParentGroupID only; a renderer may draw a link from
	// each extra parent so the additional membership is not lost.
	ExtraParentGroupIDs []string
}

// Counts is the invariant census a provider enrichment MUST NOT change.
type Counts struct {
	Nodes      int
	Edges      int
	Groups     int
	Unresolved int
}

// Counts returns the census of t.
func (t *Topology) Counts() Counts {
	return Counts{
		Nodes:      len(t.Nodes),
		Edges:      len(t.Edges),
		Groups:     len(t.Groups),
		Unresolved: len(t.Unresolved),
	}
}
