// This file builds the per-type interface-detail page model: every
// gateway or switch resource in the document, paired with the port
// resources the document itself reports it contains. It is entirely
// data-driven, a device with four reported ports produces four port
// cells, never a fixed hardware-model port count.

package arubacentral

import (
	"encoding/json"

	"go.osirisjson.org/consumers/internal/graph"
)

// portResourceType is the OSIRIS JSON resource type a device's
// physical port shows up as in an Aruba Central snapshot, linked to
// its device by a literal "contains" connection.
const portResourceType = "network.interface"

// containsType is the OSIRIS JSON connection type a device-to-port
// relationship is asserted with.
const containsType = "contains"

// InterfacePage is one type-detail page: every resource of one
// recognized type in the document, each with its reported ports.
type InterfacePage struct {
	ResourceType string
	Devices      []Device
}

// Device is one gateway or switch resource and the ports the document
// reports it contains, in declaration order.
type Device struct {
	ResourceID string
	Name       string
	Ports      []Port
}

// Port is one network.interface child resource of a Device, found via
// a literal "contains" edge from the device.
type Port struct {
	ResourceID string
	Name       string
	// Status is the port resource's own Status field, verbatim
	// (evidence-state: observed exactly as the document asserts it,
	// including "unknown" when the document says so).
	Status string
	// PropertiesJSON is the port resource's "properties" object,
	// deterministically re-encoded (sorted keys, two-space indent) for
	// display. Empty when the document carried none. No fixed field
	// set is assumed: a gateway port and a switch port carry different
	// vocabularies, and this shows whatever the document actually
	// asserts for that one port.
	PropertiesJSON string
}

// BuildInterfacePage collects every resource of resourceType and its
// reported ports, in the graph's declaration order (deterministic).
// A resource with no reported ports still appears, with an empty
// Ports slice, an unreported port is not evidence the device has none.
func BuildInterfacePage(g *graph.Graph, resourceType string) InterfacePage {
	page := InterfacePage{ResourceType: resourceType}
	for _, id := range g.ByType[resourceType] {
		res := g.Resources[id]
		page.Devices = append(page.Devices, Device{
			ResourceID: id,
			Name:       res.Name,
			Ports:      ports(g, id),
		})
	}
	return page
}

// ports finds every resource a device "contains" that is itself a
// portResourceType, via the graph's literal edges. Order follows the
// device's own connection declaration order.
func ports(g *graph.Graph, deviceID string) []Port {
	var out []Port
	for _, idx := range g.OutEdges[deviceID] {
		e := g.Edges[idx]
		if e.Type != containsType {
			continue
		}
		target, ok := g.Resources[e.Target]
		if !ok || target.Type != portResourceType {
			continue
		}
		out = append(out, Port{
			ResourceID:     e.Target,
			Name:           target.Name,
			Status:         target.Status,
			PropertiesJSON: formatProperties(target.Properties),
		})
	}
	return out
}

// formatProperties re-encodes a resource's raw "properties" object
// with sorted keys and two-space indent, for a stable, readable
// display. A non-object or malformed value falls back to the raw
// bytes rather than inventing structure; encoding/json already sorts
// map keys, so this is deterministic across runs.
func formatProperties(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return string(raw)
	}
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(b)
}
