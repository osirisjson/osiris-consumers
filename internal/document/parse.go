// This file turns exact document bytes into the read-only Doc view.
// It exposes the known OSIRIS JSON 1.x fields, keeps freeform and
// unknown content as raw JSON, and rejects a non-1 major version
// defensively. It performs no provider-specific interpretation.

package document

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Parsing error categories. ErrUnsupportedMajor is the only one a
// caller maps to exit code 1 (a document-level fault the canonical
// validator would also reject); the rest indicate malformed structure
// reaching a parser that should have seen validated input.
var (
	// ErrNotObject means the top-level JSON value is not an object.
	ErrNotObject = errors.New("document: top-level value is not a JSON object")
	// ErrMalformedDocument means a known OSIRIS JSON field had the
	// wrong JSON shape.
	ErrMalformedDocument = errors.New("document: malformed OSIRIS JSON structure")
	// ErrUnsupportedMajor means the document declares an OSIRIS JSON
	// major version other than 1.
	ErrUnsupportedMajor = errors.New("document: unsupported OSIRIS JSON major version")
)

// Parse builds a read-only Document from the exact bytes of one JSON
// value. The bytes are retained verbatim and hashed; callers normally
// obtain them from Read so the hash matches what the validator saw.
//
// Parse assumes the input has already passed canonical validation. It
// still guards defensively: it requires a top-level object and a "1.x.y"
// version, and it returns a typed error otherwise. Unknown fields at
// every level are preserved, and "properties" and "extensions" are kept
// as raw JSON.
func Parse(input []byte) (*Document, error) {
	sum := sha256.Sum256(input)

	var top map[string]json.RawMessage
	if err := json.Unmarshal(input, &top); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return nil, fmt.Errorf("%w", ErrNotObject)
		}
		return nil, fmt.Errorf("%w: %v", ErrMalformedDocument, err)
	}

	doc := &Document{
		raw:    append([]byte(nil), input...),
		sha256: hex.EncodeToString(sum[:]),
	}

	if err := json.Unmarshal(rawOr(top["version"], "null"), &doc.Version); err != nil {
		return nil, fmt.Errorf("%w: \"version\" is not a string", ErrMalformedDocument)
	}
	if err := checkMajor(doc.Version); err != nil {
		return nil, err
	}

	md, err := parseMetadata(top["metadata"])
	if err != nil {
		return nil, err
	}
	doc.Metadata = md

	tp, err := parseTopology(top["topology"])
	if err != nil {
		return nil, err
	}
	doc.Topology = tp

	delete(top, "version")
	delete(top, "metadata")
	delete(top, "topology")
	if len(top) > 0 {
		doc.Extra = top
	}
	return doc, nil
}

// checkMajor accepts a "1.x.y" version and rejects any other major.
func checkMajor(version string) error {
	major, _, found := strings.Cut(version, ".")
	if !found || major != "1" {
		return fmt.Errorf("%w: got %q, this build consumes OSIRIS JSON 1.x", ErrUnsupportedMajor, version)
	}
	return nil
}

func parseMetadata(raw json.RawMessage) (Metadata, error) {
	var md Metadata
	if len(raw) == 0 {
		return md, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return md, fmt.Errorf("%w: \"metadata\" is not an object", ErrMalformedDocument)
	}

	if v, ok := obj["timestamp"]; ok {
		if err := json.Unmarshal(v, &md.Timestamp); err != nil {
			return md, fmt.Errorf("%w: \"metadata.timestamp\" is not a string", ErrMalformedDocument)
		}
	}
	if v, ok := obj["generator"]; ok && !isJSONNull(v) {
		g, err := parseGenerator(v)
		if err != nil {
			return md, err
		}
		md.Generator = &g
	}
	if v, ok := obj["scope"]; ok && !isJSONNull(v) {
		md.Scope = append(json.RawMessage(nil), v...)
	}

	delete(obj, "timestamp")
	delete(obj, "generator")
	delete(obj, "scope")
	if len(obj) > 0 {
		md.Extra = obj
	}
	return md, nil
}

func parseGenerator(raw json.RawMessage) (Generator, error) {
	var g Generator
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return g, fmt.Errorf("%w: \"metadata.generator\" is not an object", ErrMalformedDocument)
	}
	_ = json.Unmarshal(rawOr(obj["name"], `""`), &g.Name)
	_ = json.Unmarshal(rawOr(obj["version"], `""`), &g.Version)
	_ = json.Unmarshal(rawOr(obj["url"], `""`), &g.URL)
	delete(obj, "name")
	delete(obj, "version")
	delete(obj, "url")
	if len(obj) > 0 {
		g.Extra = obj
	}
	return g, nil
}

func parseTopology(raw json.RawMessage) (Topology, error) {
	var tp Topology
	if len(raw) == 0 {
		return tp, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return tp, fmt.Errorf("%w: \"topology\" is not an object", ErrMalformedDocument)
	}

	if v, ok := obj["resources"]; ok && !isJSONNull(v) {
		var rawList []json.RawMessage
		if err := json.Unmarshal(v, &rawList); err != nil {
			return tp, fmt.Errorf("%w: \"topology.resources\" is not an array", ErrMalformedDocument)
		}
		tp.Resources = make([]Resource, 0, len(rawList))
		for i, rr := range rawList {
			res, err := parseResource(rr)
			if err != nil {
				return tp, fmt.Errorf("topology.resources[%d]: %w", i, err)
			}
			tp.Resources = append(tp.Resources, res)
		}
	}
	if v, ok := obj["connections"]; ok && !isJSONNull(v) {
		var rawList []json.RawMessage
		if err := json.Unmarshal(v, &rawList); err != nil {
			return tp, fmt.Errorf("%w: \"topology.connections\" is not an array", ErrMalformedDocument)
		}
		tp.Connections = make([]Connection, 0, len(rawList))
		for i, rc := range rawList {
			c, err := parseConnection(rc)
			if err != nil {
				return tp, fmt.Errorf("topology.connections[%d]: %w", i, err)
			}
			tp.Connections = append(tp.Connections, c)
		}
	}
	if v, ok := obj["groups"]; ok && !isJSONNull(v) {
		var rawList []json.RawMessage
		if err := json.Unmarshal(v, &rawList); err != nil {
			return tp, fmt.Errorf("%w: \"topology.groups\" is not an array", ErrMalformedDocument)
		}
		tp.Groups = make([]Group, 0, len(rawList))
		for i, rg := range rawList {
			g, err := parseGroup(rg)
			if err != nil {
				return tp, fmt.Errorf("topology.groups[%d]: %w", i, err)
			}
			tp.Groups = append(tp.Groups, g)
		}
	}

	delete(obj, "resources")
	delete(obj, "connections")
	delete(obj, "groups")
	if len(obj) > 0 {
		tp.Extra = obj
	}
	return tp, nil
}

func parseResource(raw json.RawMessage) (Resource, error) {
	var r Resource
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return r, fmt.Errorf("%w: resource is not an object", ErrMalformedDocument)
	}
	strField(obj, "id", &r.ID)
	strField(obj, "type", &r.Type)
	strField(obj, "name", &r.Name)
	strField(obj, "description", &r.Description)
	strField(obj, "status", &r.Status)
	strField(obj, "state", &r.State)
	if v, ok := obj["provider"]; ok && !isJSONNull(v) {
		p, err := parseProvider(v)
		if err != nil {
			return r, err
		}
		r.Provider = p
	}
	rawField(obj, "properties", &r.Properties)
	rawField(obj, "extensions", &r.Extensions)
	tagsField(obj, "tags", &r.Tags)

	for _, k := range []string{"id", "type", "name", "description", "status", "state", "provider", "properties", "extensions", "tags"} {
		delete(obj, k)
	}
	if len(obj) > 0 {
		r.Extra = obj
	}
	return r, nil
}

func parseProvider(raw json.RawMessage) (Provider, error) {
	var p Provider
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return p, fmt.Errorf("%w: \"provider\" is not an object", ErrMalformedDocument)
	}
	strField(obj, "name", &p.Name)
	strField(obj, "namespace", &p.Namespace)
	strField(obj, "native_id", &p.NativeID)
	strField(obj, "account", &p.Account)
	strField(obj, "tenant", &p.Tenant)
	strField(obj, "type", &p.Type)
	strField(obj, "region", &p.Region)
	strField(obj, "zone", &p.Zone)
	strField(obj, "subscription", &p.Subscription)
	strField(obj, "project", &p.Project)
	strField(obj, "site", &p.Site)
	strField(obj, "system", &p.System)
	strField(obj, "source", &p.Source)
	strField(obj, "version", &p.Version)
	for _, k := range []string{"name", "namespace", "native_id", "account", "tenant", "type", "region", "zone", "subscription", "project", "site", "system", "source", "version"} {
		delete(obj, k)
	}
	if len(obj) > 0 {
		p.Extra = obj
	}
	return p, nil
}

func parseConnection(raw json.RawMessage) (Connection, error) {
	var c Connection
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return c, fmt.Errorf("%w: connection is not an object", ErrMalformedDocument)
	}
	strField(obj, "id", &c.ID)
	strField(obj, "type", &c.Type)
	strField(obj, "source", &c.Source)
	strField(obj, "target", &c.Target)
	strField(obj, "direction", &c.Direction)
	strField(obj, "name", &c.Name)
	strField(obj, "description", &c.Description)
	strField(obj, "status", &c.Status)
	strField(obj, "state", &c.State)
	rawField(obj, "properties", &c.Properties)
	rawField(obj, "extensions", &c.Extensions)
	tagsField(obj, "tags", &c.Tags)
	for _, k := range []string{"id", "type", "source", "target", "direction", "name", "description", "status", "state", "properties", "extensions", "tags"} {
		delete(obj, k)
	}
	if len(obj) > 0 {
		c.Extra = obj
	}
	return c, nil
}

func parseGroup(raw json.RawMessage) (Group, error) {
	var g Group
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return g, fmt.Errorf("%w: group is not an object", ErrMalformedDocument)
	}
	strField(obj, "id", &g.ID)
	strField(obj, "type", &g.Type)
	strField(obj, "name", &g.Name)
	strField(obj, "description", &g.Description)
	strList(obj, "members", &g.Members)
	strList(obj, "children", &g.Children)
	rawField(obj, "properties", &g.Properties)
	rawField(obj, "extensions", &g.Extensions)
	tagsField(obj, "tags", &g.Tags)
	for _, k := range []string{"id", "type", "name", "description", "members", "children", "properties", "extensions", "tags"} {
		delete(obj, k)
	}
	if len(obj) > 0 {
		g.Extra = obj
	}
	return g, nil
}

// strField copies obj[key] into dst when it is present and a string.
// A present non-string value is left as the zero string; the raw value
// still survives in the object's Extra map because the key is only
// deleted by the caller after this call.
func strField(obj map[string]json.RawMessage, key string, dst *string) {
	if v, ok := obj[key]; ok {
		_ = json.Unmarshal(v, dst)
	}
}

func strList(obj map[string]json.RawMessage, key string, dst *[]string) {
	if v, ok := obj[key]; ok && !isJSONNull(v) {
		_ = json.Unmarshal(v, dst)
	}
}

func rawField(obj map[string]json.RawMessage, key string, dst *json.RawMessage) {
	if v, ok := obj[key]; ok && !isJSONNull(v) {
		*dst = append(json.RawMessage(nil), v...)
	}
}

func tagsField(obj map[string]json.RawMessage, key string, dst *map[string]string) {
	if v, ok := obj[key]; ok && !isJSONNull(v) {
		_ = json.Unmarshal(v, dst)
	}
}

func rawOr(v json.RawMessage, fallback string) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(fallback)
	}
	return v
}

func isJSONNull(v json.RawMessage) bool {
	return string(v) == "null"
}
