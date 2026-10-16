// This file implements Build: it walks the read-only document view and
// the generic graph index once and assembles the format-neutral Report,
// sorting every collection by a total, documented key.

package report

import (
	"context"
	"encoding/json"
	"sort"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/validation"
)

// ctxCheckInterval is how often the element loops poll ctx.Err().
const ctxCheckInterval = 1024

// Build assembles the Report from a validated document view, the
// generic graph index over the same document, the validation receipt
// the run used, and Options.
//
// It reads these OSIRIS JSON document fields and nothing else:
//   - version;
//   - metadata.timestamp, metadata.generator.{name,version},
//     metadata.scope.{name,description};
//   - per resource: id, type, name, description, status, state,
//     provider.{name,type,region,native_id}, tags, and the presence of
//     properties, extensions, and unknown members;
//   - per connection: id, type, source, target, direction, name,
//     description, status, state, tags, and the presence of properties,
//     extensions, and unknown members;
//   - per group: id, type, name, description, members, children, tags,
//     and the presence of properties, extensions, and unknown members.
//
// The only error Build returns is ctx.Err() on cancellation.
func Build(ctx context.Context, doc *document.Document, g *graph.Graph, receipt validation.Receipt, opts Options) (*Report, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r := &Report{
		Document: buildDocumentInfo(doc, receipt),
		Disclosure: Disclosure{
			Generator:             doc.Metadata.Generator != nil,
			Scope:                 len(doc.Metadata.Scope) > 0,
			UnknownTopLevelFields: len(doc.Extra) > 0,
			Connections:           doc.Topology.Connections != nil,
			Groups:                doc.Topology.Groups != nil,
		},
	}

	r.Providers = buildProviders(g)
	r.ResourceTypes = countTypes(g.ByType)
	r.ConnectionTypes = countTypes(g.ConnectionsByType)

	resources, err := buildResources(ctx, doc, g)
	if err != nil {
		return nil, err
	}
	connections, err := buildConnections(ctx, doc, g)
	if err != nil {
		return nil, err
	}
	groups, err := buildGroups(ctx, doc)
	if err != nil {
		return nil, err
	}

	r.Counts = Counts{
		Resources:        len(doc.Topology.Resources),
		Connections:      len(doc.Topology.Connections),
		Groups:           len(doc.Topology.Groups),
		Edges:            len(g.Edges),
		Providers:        len(r.Providers),
		ResourceTypes:    len(r.ResourceTypes),
		ConnectionTypes:  len(r.ConnectionTypes),
		UnknownProviders: countUnknownProviders(r.Providers),
		Anomalies:        len(g.Anomalies),
	}

	r.Anomalies = make([]AnomalyEntry, 0, len(g.Anomalies))
	for _, a := range g.Anomalies {
		r.Anomalies = append(r.Anomalies, AnomalyEntry{
			Kind:      a.Kind.String(),
			Subject:   a.Subject,
			Reference: a.Reference,
			Detail:    a.Detail,
		})
	}

	r.Resources, r.Truncations = boundResources(resources, opts.MaxRows, r.Truncations)
	r.Connections, r.Truncations = boundConnections(connections, opts.MaxRows, r.Truncations)
	r.Groups, r.Truncations = boundGroups(groups, opts.MaxRows, r.Truncations)

	return r, nil
}

func buildDocumentInfo(doc *document.Document, receipt validation.Receipt) DocumentInfo {
	info := DocumentInfo{
		OSIRISVersion:        doc.Version,
		Timestamp:            doc.Metadata.Timestamp,
		InputSHA256:          doc.SHA256(),
		EngineName:           receipt.EngineName,
		EngineVersion:        receipt.EngineVersion,
		Profile:              receipt.Profile,
		SpecificationVersion: receipt.SpecificationVersion,
		ErrorCount:           receipt.ErrorCount,
		WarningCount:         receipt.WarningCount,
	}
	if g := doc.Metadata.Generator; g != nil {
		info.GeneratorName = g.Name
		info.GeneratorVersion = g.Version
	}
	info.ScopeName, info.ScopeDescription = scopeStrings(doc.Metadata.Scope)
	return info
}

// scopeStrings pulls the display "name" and "description" out of the
// raw metadata.scope object, best effort. A malformed or absent scope
// yields two empty strings and is not an error.
func scopeStrings(raw json.RawMessage) (name, description string) {
	if len(raw) == 0 {
		return "", ""
	}
	var scope struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(raw, &scope)
	return scope.Name, scope.Description
}

// buildProviders turns the graph's provider buckets into sorted rows,
// each with its distinct resource types.
func buildProviders(g *graph.Graph) []ProviderBreakdown {
	names := make([]string, 0, len(g.ByProvider))
	for name := range g.ByProvider {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ProviderBreakdown, 0, len(names))
	for _, name := range names {
		ids := g.ByProvider[name]
		typeSet := make(map[string]struct{})
		for _, id := range ids {
			if res, ok := g.Resources[id]; ok {
				typeSet[res.Type] = struct{}{}
			}
		}
		types := make([]string, 0, len(typeSet))
		for t := range typeSet {
			types = append(types, t)
		}
		sort.Strings(types)
		out = append(out, ProviderBreakdown{
			Name:          name,
			ResourceCount: len(ids),
			ResourceTypes: types,
		})
	}
	return out
}

// countTypes turns a type bucket map into sorted {type, count} rows.
func countTypes(buckets map[string][]string) []TypeBreakdown {
	types := make([]string, 0, len(buckets))
	for t := range buckets {
		types = append(types, t)
	}
	sort.Strings(types)
	out := make([]TypeBreakdown, 0, len(types))
	for _, t := range types {
		out = append(out, TypeBreakdown{Type: t, Count: len(buckets[t])})
	}
	return out
}

func countUnknownProviders(providers []ProviderBreakdown) int {
	n := 0
	for _, p := range providers {
		if p.Name == "" || p.Name == "unknown" {
			n += p.ResourceCount
		}
	}
	return n
}

func buildResources(ctx context.Context, doc *document.Document, g *graph.Graph) ([]ResourceEntry, error) {
	res := doc.Topology.Resources
	out := make([]ResourceEntry, 0, len(res))
	for i := range res {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		r := &res[i]
		out = append(out, ResourceEntry{
			ID:            r.ID,
			Type:          r.Type,
			Name:          r.Name,
			Description:   r.Description,
			Provider:      r.Provider.Name,
			ProviderType:  r.Provider.Type,
			Region:        r.Provider.Region,
			Status:        r.Status,
			State:         r.State,
			NativeID:      r.Provider.NativeID,
			Tags:          sortedTags(r.Tags),
			GroupIDs:      sortedCopy(g.MemberOfGroups[r.ID]),
			OutDegree:     len(g.OutEdges[r.ID]),
			InDegree:      len(g.InEdges[r.ID]),
			HasProperties: len(r.Properties) > 0,
			HasExtensions: len(r.Extensions) > 0,
			ExtraKeys:     sortedKeys(r.Extra),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}

func buildConnections(ctx context.Context, doc *document.Document, g *graph.Graph) ([]ConnectionEntry, error) {
	conns := doc.Topology.Connections
	out := make([]ConnectionEntry, 0, len(conns))
	for i := range conns {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		c := &conns[i]
		_, srcOK := g.Resources[c.Source]
		_, dstOK := g.Resources[c.Target]
		out = append(out, ConnectionEntry{
			ID:            c.ID,
			Type:          c.Type,
			Source:        c.Source,
			Target:        c.Target,
			Direction:     c.Direction,
			Name:          c.Name,
			Description:   c.Description,
			Status:        c.Status,
			State:         c.State,
			SourceMissing: !srcOK,
			TargetMissing: !dstOK,
			Tags:          sortedTags(c.Tags),
			HasProperties: len(c.Properties) > 0,
			HasExtensions: len(c.Extensions) > 0,
			ExtraKeys:     sortedKeys(c.Extra),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.ID != b.ID:
			return a.ID < b.ID
		case a.Type != b.Type:
			return a.Type < b.Type
		case a.Source != b.Source:
			return a.Source < b.Source
		default:
			return a.Target < b.Target
		}
	})
	return out, nil
}

func buildGroups(ctx context.Context, doc *document.Document) ([]GroupEntry, error) {
	groups := doc.Topology.Groups
	out := make([]GroupEntry, 0, len(groups))
	for i := range groups {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		grp := &groups[i]
		out = append(out, GroupEntry{
			ID:            grp.ID,
			Type:          grp.Type,
			Name:          grp.Name,
			Description:   grp.Description,
			Members:       append([]string(nil), grp.Members...),
			Children:      append([]string(nil), grp.Children...),
			MemberCount:   len(grp.Members),
			ChildCount:    len(grp.Children),
			Tags:          sortedTags(grp.Tags),
			HasProperties: len(grp.Properties) > 0,
			HasExtensions: len(grp.Extensions) > 0,
			ExtraKeys:     sortedKeys(grp.Extra),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}

func boundResources(in []ResourceEntry, max int, trunc []Truncation) ([]ResourceEntry, []Truncation) {
	if max <= 0 || len(in) <= max {
		return in, trunc
	}
	return in[:max], append(trunc, Truncation{Section: "resources", Shown: max, Total: len(in)})
}

func boundConnections(in []ConnectionEntry, max int, trunc []Truncation) ([]ConnectionEntry, []Truncation) {
	if max <= 0 || len(in) <= max {
		return in, trunc
	}
	return in[:max], append(trunc, Truncation{Section: "connections", Shown: max, Total: len(in)})
}

func boundGroups(in []GroupEntry, max int, trunc []Truncation) ([]GroupEntry, []Truncation) {
	if max <= 0 || len(in) <= max {
		return in, trunc
	}
	return in[:max], append(trunc, Truncation{Section: "groups", Shown: max, Total: len(in)})
}

// sortedTags returns the map as KV pairs sorted by key.
func sortedTags(m map[string]string) []KV {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]KV, 0, len(keys))
	for _, k := range keys {
		out = append(out, KV{Key: k, Value: m[k]})
	}
	return out
}

// sortedKeys returns the map keys sorted. It is used for unknown-field
// name lists, so the report can disclose them without showing values.
func sortedKeys[V any](m map[string]V) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedCopy returns a sorted copy of s, leaving the input untouched.
func sortedCopy(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}
