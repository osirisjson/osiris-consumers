// This file defines the structural anomaly vocabulary Build reports.
// Anomalies are evidence of an inconsistency in the document's own
// references; they are not compliance findings and carry no severity
// grade.

package graph

import "fmt"

// An AnomalyKind classifies a structural inconsistency. The iota order
// is also the sort order Build emits anomalies in, so output is
// deterministic.
type AnomalyKind int

const (
	// DuplicateResourceID: two resources share an identifier.
	DuplicateResourceID AnomalyKind = iota
	// DuplicateConnectionID: two connections share an identifier.
	DuplicateConnectionID
	// DuplicateGroupID: two groups share an identifier.
	DuplicateGroupID
	// DanglingConnectionSource: a connection source names no resource.
	DanglingConnectionSource
	// DanglingConnectionTarget: a connection target names no resource.
	DanglingConnectionTarget
	// SelfConnection: a connection's source equals its target.
	SelfConnection
	// DanglingGroupMember: a group member names no resource, group, or
	// connection in the document.
	DanglingGroupMember
	// DanglingGroupChild: a group child names no group.
	DanglingGroupChild
	// SelfGroupChild: a group lists itself as a child.
	SelfGroupChild
	// GroupCycle: the group child relation contains a cycle.
	GroupCycle
)

// String returns a stable lower-case slug for the kind.
func (k AnomalyKind) String() string {
	switch k {
	case DuplicateResourceID:
		return "duplicate-resource-id"
	case DuplicateConnectionID:
		return "duplicate-connection-id"
	case DuplicateGroupID:
		return "duplicate-group-id"
	case DanglingConnectionSource:
		return "dangling-connection-source"
	case DanglingConnectionTarget:
		return "dangling-connection-target"
	case SelfConnection:
		return "self-connection"
	case DanglingGroupMember:
		return "dangling-group-member"
	case DanglingGroupChild:
		return "dangling-group-child"
	case SelfGroupChild:
		return "self-group-child"
	case GroupCycle:
		return "group-cycle"
	default:
		return fmt.Sprintf("anomaly(%d)", int(k))
	}
}

// An Anomaly is one structural inconsistency found while indexing.
type Anomaly struct {
	// Kind classifies the inconsistency.
	Kind AnomalyKind
	// Subject is the identifier at fault (the duplicate id, the
	// connection with the dangling endpoint, the group with the cycle).
	Subject string
	// Reference is the identifier Subject pointed at, when the anomaly
	// is about a reference (the missing endpoint, the missing child).
	// It is empty for the duplicate-id and self-* kinds.
	Reference string
	// Detail is a short human-readable explanation.
	Detail string
}
