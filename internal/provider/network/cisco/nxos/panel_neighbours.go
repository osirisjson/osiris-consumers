// This file builds the confirmed-panel pages own richer reading of
// each CDP/LLDP-discovered neighbour: topology.go's buildNeighbours
// stays the generic fallback page's own reading (a bare
// label and local port, deliberately minimal, see that page's own doc
// comment on what it leaves out); a document whose chassis has a
// confirmed panel never renders that fallback page at all, so enriching
// this reading never changes the fallback page's own behavior.
package nxos

import (
	"encoding/json"
	"sort"
	"strings"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// panelTransceiver is the "source_transceiver" object a physical.ethernet
// connection's own properties may carry, this switch's own transceiver
// in that link, never the remote end's (no confirmed OSIRIS JSON field
// for that exists yet in the NX-OS producer, so none is read here).
type panelTransceiver struct {
	Model      string `json:"model"`
	PartNumber string `json:"part_number"`
	Vendor     string `json:"vendor"`
}

// panelEdgeProperties is the subset of a physical.ethernet connection's
// own properties the confirmed-panel edge label shows.
type panelEdgeProperties struct {
	DiscoveredVia     []string          `json:"discovered_via"`
	SourceTransceiver *panelTransceiver `json:"source_transceiver"`
}

// panelNeighbour is one CDP/LLDP-discovered neighbour as the confirmed-
// panel pages need it: topoNeighbour own ID and LocalPort, a native-
// ID-derived Label (neighbourNativeLabel), the physical.ethernet
// connection's own discovery method and transceiver, and the local
// port's own mode/VLAN properties (panelPort), every field the edge
// label and neighbour box show, literal from the document.
type panelNeighbour struct {
	ID, Label, LocalPort string
	DiscoveredVia        string
	Transceiver          *panelTransceiver
	Port                 panelPort
}

// buildPanelNeighbours returns every CDP/LLDP-discovered neighbour
// resource, sorted by ID for deterministic placement, each carrying its
// own physical.ethernet connection's discovery method and transceiver
// plus its local port's own properties (ports indexes buildPanelPorts's
// own result by name).
func buildPanelNeighbours(g *graph.Graph, ports map[string]panelPort) []panelNeighbour {
	var ids []string
	for _, id := range g.ResourceIDs {
		r := g.Resources[id]
		if r != nil && isExternallyDiscovered(r.Provider.Name) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	out := make([]panelNeighbour, 0, len(ids))
	for _, id := range ids {
		localPort := "(absent)"
		var props panelEdgeProperties
		for _, idx := range g.InEdges[id] {
			e := g.Edges[idx]
			if e.Type != "physical.ethernet" {
				continue
			}
			if r, ok := g.Resources[e.Source]; ok {
				localPort = labelOr(r.Name, e.Source)
			}
			if c, ok := g.Connections[e.ConnectionID]; ok {
				_ = json.Unmarshal(c.Properties, &props)
			}
			break
		}
		method := "(absent)"
		if len(props.DiscoveredVia) > 0 {
			method = props.DiscoveredVia[0]
		}
		out = append(out, panelNeighbour{
			ID: id, Label: neighbourNativeLabel(id, g.Resources[id]), LocalPort: localPort,
			DiscoveredVia: method, Transceiver: props.SourceTransceiver,
			Port: ports[localPort],
		})
	}
	return out
}

// neighbourMarker is the substring a CDP/LLDP-discovered neighbour
// resource's own ID always carries, right before the remote system and
// port this switch's own producer embedded in it verbatim (e.g.
// "cisco.nxos::ABC00001DE2/neighbor/MXP-SW-123.example.com/Eth1/7").
const neighbourMarker = "/neighbor/"

// neighbourNativeLabel returns the exact substring of id after its own
// "/neighbor/" marker, the remote system and port precisely as this
// switch's own producer named them. Falls back to r's own Name, then id
// when the marker is absent, a resource this switch marks externally
// discovered but that carries no such marker in its own ID never
// renders as an empty label.
func neighbourNativeLabel(id string, r *document.Resource) string {
	if i := strings.LastIndex(id, neighbourMarker); i >= 0 {
		if rest := id[i+len(neighbourMarker):]; rest != "" {
			return rest
		}
	}
	if r != nil {
		return labelOr(r.Name, id)
	}
	return id
}
