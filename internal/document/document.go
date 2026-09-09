// Package document provides an exact-byte, loss-aware, read-only view
// of an OSIRIS JSON 1.x document.
//
// The package keeps the original input bytes and their SHA-256 hash
// untouched alongside a parsed view of the known OSIRIS 1.x fields.
// Every object level also retains its unknown fields as raw JSON, and
// the freeform "properties" and "extensions" payloads are held verbatim,
// so nothing in the input is silently discarded.
//
// Resource, connection, and group identifiers are treated as opaque and
// case-sensitive. This package performs no structural inference: it
// does not parse native identifiers, does not recognise any vendor
// filename grammar, and does not project provider account, subscription
// tenant, or management-group hierarchy.
// Provider-specific interpretation lives in an adapter, never here.
package document

import "encoding/json"

// A Document is a read-only view of one OSIRIS JSON 1.x document.
//
// Raw and SHA256 describe the exact input bytes. The remaining fields
// are the parsed known-field view. Extra holds any top-level members
// that are not part of the OSIRIS JSON 1.x core, preserved as raw JSON
// for later exact comparison.
type Document struct {
	raw    []byte
	sha256 string

	// Version is the document's declared OSIRIS JSON specification
	// version (a "1.x.y" SemVer string).
	Version string
	// Metadata is the document-level metadata block.
	Metadata Metadata
	// Topology is the resource, connection, and group snapshot.
	Topology Topology
	// Extra holds unrecognised top-level fields verbatim.
	Extra map[string]json.RawMessage
}

// Raw returns a copy of the exact input bytes the document was parsed
// from. The returned slice is safe for the caller to modify.
func (d *Document) Raw() []byte {
	out := make([]byte, len(d.raw))
	copy(out, d.raw)
	return out
}

// SHA256 returns the lower-case hex SHA-256 of the exact input bytes.
func (d *Document) SHA256() string { return d.sha256 }

// Metadata is the OSIRIS JSON "metadata" object.
type Metadata struct {
	// Timestamp is the snapshot time as written in the document
	// (an RFC 3339 date-time string). It is not parsed into a time
	// value here; consumers that need a time.Time parse it themselves.
	Timestamp string
	// Generator identifies the producer that emitted the document, when
	// present. It is nil when the document omits "metadata.generator".
	Generator *Generator
	// Scope is the optional "metadata.scope" object, kept as raw JSON.
	Scope json.RawMessage
	// Extra holds unrecognised "metadata" members verbatim.
	Extra map[string]json.RawMessage
}

// Generator is the OSIRIS JSON "metadata.generator" object.
// It is read for provenance display only; no behaviour keys off it.
type Generator struct {
	Name    string
	Version string
	URL     string
	Extra   map[string]json.RawMessage
}

// Topology is the OSIRIS JSON "topology" object.
type Topology struct {
	Resources   []Resource
	Connections []Connection
	Groups      []Group
	// Extra holds unrecognised "topology" members verbatim.
	Extra map[string]json.RawMessage
}

// Resource is one entry of "topology.resources". ID and Type are kept
// exactly as written. Properties and Extensions are held as raw JSON so
// no nested field is lost.
type Resource struct {
	ID          string
	Type        string
	Name        string
	Description string
	Provider    Provider
	Status      string
	State       string
	Properties  json.RawMessage
	Extensions  json.RawMessage
	Tags        map[string]string
	Extra       map[string]json.RawMessage
}

// Provider is the OSIRIS JSON "provider" provenance object.
// Every field is exposed as written; NativeID in particular is never
// parsed or interpreted in this package.
type Provider struct {
	Name         string
	Namespace    string
	NativeID     string
	Account      string
	Tenant       string
	Type         string
	Region       string
	Zone         string
	Subscription string
	Project      string
	Site         string
	System       string
	Source       string
	Version      string
	Extra        map[string]json.RawMessage
}

// Connection is one entry of "topology.connections". Source, Target,
// Direction, and Type are kept literally; this package draws no
// containment or hierarchy conclusion from a connection type.
type Connection struct {
	ID          string
	Type        string
	Source      string
	Target      string
	Direction   string
	Name        string
	Description string
	Status      string
	State       string
	Properties  json.RawMessage
	Extensions  json.RawMessage
	Tags        map[string]string
	Extra       map[string]json.RawMessage
}

// Group is one entry of "topology.groups". Members and Children are the
// literal identifier lists as written.
type Group struct {
	ID          string
	Type        string
	Name        string
	Description string
	Members     []string
	Children    []string
	Properties  json.RawMessage
	Extensions  json.RawMessage
	Tags        map[string]string
	Extra       map[string]json.RawMessage
}
