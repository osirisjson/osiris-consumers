// Package report builds a format-neutral inventory model of one
// canonically validated OSIRIS JSON 1.x document.
//
// The model is a faithful, vendor-neutral census: every resource,
// connection, group, provider, and type in the document is represented
// with no semantic loss.
// The model declares exactly the document fields it
// reads (see Build) and draws no compliance, maturity, or security
// conclusion from them: optional data that the document does not carry
// is reported as absent, never as a passing or failing state.
//
// Every collection the model builds is emitted in a total, documented
// order so a renderer over stable input produces byte-identical output.
package report

// A Report is the complete format-neutral inventory of one document.
//
// Field order here is also the recommended section order for a
// renderer. Every slice is sorted by the key documented on its element
// type.
type Report struct {
	// Document is the document-level provenance and validation summary.
	Document DocumentInfo
	// Counts is the exact census used for the inventory summary.
	Counts Counts
	// Disclosure records which optional document regions are present,
	// so a reader can tell absent data from empty data.
	Disclosure Disclosure
	// Providers is the per-provider resource breakdown, sorted by
	// provider name (the empty name, meaning a resource with no
	// provider block, sorts first).
	Providers []ProviderBreakdown
	// ResourceTypes is the per-type resource breakdown, sorted by type.
	ResourceTypes []TypeBreakdown
	// ConnectionTypes is the per-type connection breakdown, sorted by
	// type.
	ConnectionTypes []TypeBreakdown
	// Resources lists every resource, sorted by ID then type.
	Resources []ResourceEntry
	// Connections lists every connection, sorted by ID, type, source,
	// then target.
	Connections []ConnectionEntry
	// Groups lists every group, sorted by ID then type.
	Groups []GroupEntry
	// Anomalies are the structural inconsistencies the graph index
	// found, in the graph's deterministic order. They are defensive
	// notes about the document's own references, not canonical
	// diagnostics and not graded.
	Anomalies []AnomalyEntry
	// Truncations records any section a row bound shortened, with the
	// full count, so nothing is ever omitted silently. It is nil when
	// the model is unbounded.
	Truncations []Truncation
}

// DocumentInfo is the document header and the validation provenance the
// report was produced under.
type DocumentInfo struct {
	// OSIRISVersion is the document's declared "version" string.
	OSIRISVersion string
	// Timestamp is "metadata.timestamp" verbatim; it is not parsed.
	Timestamp string
	// GeneratorName and GeneratorVersion are "metadata.generator"
	// fields, empty when the document omits the generator.
	GeneratorName    string
	GeneratorVersion string
	// ScopeName and ScopeDescription are best-effort reads of
	// "metadata.scope"; both empty when scope is absent or carries
	// neither field.
	ScopeName        string
	ScopeDescription string
	// InputSHA256 is the lower-case hex SHA-256 of the exact document
	// bytes.
	InputSHA256 string
	// EngineName, EngineVersion, Profile, and SpecificationVersion come
	// from the validation receipt.
	EngineName           string
	EngineVersion        string
	Profile              string
	SpecificationVersion string
	// ErrorCount and WarningCount are the engine's own totals from the
	// receipt. A report is only built for an error-free receipt, so
	// ErrorCount is always zero here; it is carried for display.
	ErrorCount   int
	WarningCount int
}

// Counts is the exact inventory census. Every number is a direct count
// of document elements; none is derived or estimated.
type Counts struct {
	Resources        int
	Connections      int
	Groups           int
	Edges            int
	Providers        int
	ResourceTypes    int
	ConnectionTypes  int
	UnknownProviders int
	Anomalies        int
}

// Disclosure states, in evidence-state terms, which optional document
// regions the input carried. It exists so a reader never has to guess
// whether an empty section means "the document said nothing" or "the
// document said none".
type Disclosure struct {
	// Generator is true when "metadata.generator" was present.
	Generator bool
	// Scope is true when "metadata.scope" was present.
	Scope bool
	// UnknownTopLevelFields is true when the document carried members
	// outside the OSIRIS JSON 1.x core at the top level.
	UnknownTopLevelFields bool
	// Connections is true when the document declared a
	// "topology.connections" array (even if empty).
	Connections bool
	// Groups is true when the document declared a "topology.groups"
	// array (even if empty).
	Groups bool
}

// ProviderBreakdown is one row of the per-provider census.
type ProviderBreakdown struct {
	// Name is the provider name verbatim. The empty string means the
	// resource carried no provider block.
	Name string
	// ResourceCount is how many resources name this provider.
	ResourceCount int
	// ResourceTypes are the distinct resource types seen under this
	// provider, sorted.
	ResourceTypes []string
}

// TypeBreakdown is one row of a per-type census (resources or
// connections).
type TypeBreakdown struct {
	// Type is the type string verbatim. The empty string means the
	// element carried no type.
	Type string
	// Count is how many elements have this type.
	Count int
}

// ResourceEntry is one resource, with the generic graph facts about it.
type ResourceEntry struct {
	ID           string
	Type         string
	Name         string
	Description  string
	Provider     string
	ProviderType string
	Region       string
	Status       string
	State        string
	// NativeID is "provider.native_id" verbatim. It is never parsed or
	// interpreted.
	NativeID string
	// Tags are the resource tags, sorted by key.
	Tags []KV
	// GroupIDs are the groups that list this resource as a member,
	// sorted.
	GroupIDs []string
	// OutDegree and InDegree are the count of literal edges with this
	// resource as source and as target.
	OutDegree int
	InDegree  int
	// HasProperties and HasExtensions record that the resource carries
	// a freeform "properties" or "extensions" payload not otherwise
	// shown, so the reader knows it was not dropped.
	HasProperties bool
	HasExtensions bool
	// ExtraKeys are the names of members outside the OSIRIS JSON 1.x
	// core found on this resource, sorted.
	ExtraKeys []string
}

// ConnectionEntry is one connection rendered as its literal facts. No
// containment or hierarchy is inferred from the type.
type ConnectionEntry struct {
	ID          string
	Type        string
	Source      string
	Target      string
	Direction   string
	Name        string
	Description string
	Status      string
	State       string
	// SourceMissing and TargetMissing are true when the endpoint names
	// no resource in the document.
	SourceMissing bool
	TargetMissing bool
	Tags          []KV
	HasProperties bool
	HasExtensions bool
	ExtraKeys     []string
}

// GroupEntry is one group with its literal membership lists.
type GroupEntry struct {
	ID          string
	Type        string
	Name        string
	Description string
	// Members and Children are the identifier lists exactly as written
	// in the document, in document order.
	Members  []string
	Children []string
	// MemberCount and ChildCount are len(Members) and len(Children).
	MemberCount   int
	ChildCount    int
	Tags          []KV
	HasProperties bool
	HasExtensions bool
	ExtraKeys     []string
}

// AnomalyEntry is one structural inconsistency carried from the graph
// index, verbatim.
type AnomalyEntry struct {
	Kind      string
	Subject   string
	Reference string
	Detail    string
}

// Truncation records that a bounded section kept only the first Shown
// rows of Total, in sort order.
type Truncation struct {
	Section string
	Shown   int
	Total   int
}

// KV is one sorted key/value tag pair.
type KV struct {
	Key   string
	Value string
}

// Options tune Build. The zero Options produce an unbounded model.
type Options struct {
	// MaxRows bounds each list section (resources, connections,
	// groups). Zero or less means unbounded: no row is ever omitted.
	// When a section is longer, Build keeps the first MaxRows rows in
	// sort order and appends a Truncation entry with the full count.
	MaxRows int
}
