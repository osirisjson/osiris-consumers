// This file reads the confirmed-panel pages own port-level data: the
// full network.switch.port census (every physical port's own resource-
// level status, never just the CDP/LLDP-discovered subset) and the
// per-port properties (native VLAN, port mode, access VLAN) the
// physical.ethernet edge label shows alongside the transceiver. Kept
// separate from report_model.go's own portProperties reader: that one
// builds the Markdown/xlsx port table, this one builds the panel
// pages status markers and edge labels, but both read the same
// network.switch.port resources and the same json shape, so the
// portProperties type itself is shared (defined in report_model.go).
package nxos

import (
	"encoding/json"

	"go.osirisjson.org/consumers/internal/graph"
)

// panelPort is one network.switch.port resource as the confirmed-panel
// pages need it: its own name (to resolve an anchor), its resource-
// level Status (never its properties.admin_status that is the
// operator's configured intent, this is the document's own observed
// state), and the subset of properties the physical.ethernet edge
// label shows.
type panelPort struct {
	LocalPort              string
	Status                 string
	PortMode               string
	NativeVLAN, AccessVLAN string
	IPAddress, SpeedMbps   string
}

// buildPanelPorts reads every network.switch.port resource under
// ProviderName once, sorted by ID for deterministic marker placement.
func buildPanelPorts(g *graph.Graph) []panelPort {
	var out []panelPort
	for _, id := range sortedByType(g, "network.switch.port") {
		r := g.Resources[id]
		if r.Provider.Name != ProviderName {
			continue
		}
		var p portProperties
		_ = json.Unmarshal(r.Properties, &p)
		name := p.InterfaceName
		if name == "" {
			name = labelOr(r.Name, id)
		}
		out = append(out, panelPort{
			LocalPort: name, Status: r.Status, PortMode: p.PortMode,
			NativeVLAN: int64OrAbsent(p.NativeVlan), AccessVLAN: int64OrAbsent(p.Vlan),
			IPAddress: p.IPAddress, SpeedMbps: int64OrAbsent(p.SpeedMbps),
		})
	}
	return out
}

// panelPortsByName indexes ports (buildPanelPorts's own result) by
// LocalPort, for the edge-label lookup below, a port name is unique
// within one switch's own network.switch.port census.
func panelPortsByName(ports []panelPort) map[string]panelPort {
	out := make(map[string]panelPort, len(ports))
	for _, p := range ports {
		out[p.LocalPort] = p
	}
	return out
}

// portMarkerStyleForStatus returns the confirmed-panel marker color for
// a network.switch.port resource's own Status: green for "active", red
// for "inactive", and the neutral portMarkerStyle (see panel_render.go)
// for anything else, an empty or unrecognized status is left neutral
// rather than guessed either way (evidence-state language:
// observed active, observed inactive, or unknown, never a fabricated
// default).
func portMarkerStyleForStatus(status string) string {
	switch status {
	case "active":
		return portMarkerActiveStyle
	case "inactive":
		return portMarkerInactiveStyle
	default:
		return portMarkerStyle
	}
}

const (
	portMarkerActiveStyle   = "ellipse;whiteSpace=wrap;html=1;fillColor=#d5e8d4;strokeColor=#82b366;strokeWidth=2;"
	portMarkerInactiveStyle = "ellipse;whiteSpace=wrap;html=1;fillColor=#f8cecc;strokeColor=#b85450;strokeWidth=2;"
)
