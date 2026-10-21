// This file builds the format-neutral Cisco NX-OS report model: the
// switch's own chassis inventory, a port census, the SVI/loopback/
// management interface inventory, the port-channel (LAG) inventory,
// the VLAN inventory, the VRF inventory (from the document's groups),
// a CDP/LLDP-discovered neighbour census, and the document's own
// signature. It is read-only over the generic graph, never re-parsing
// the document; see report_render.go for the Markdown emission and
// nxos.go for the Applies trigger this shares.
//
// A single NX-OS document describes exactly one switch, so
// every enumerable category here is a real per-item table by default;
// only the categories a single switch can plausibly grow very large
// (ports, and CDP/LLDP-discovered neighbours, both bounded by physical
// port count rather than logical configuration) stay aggregate-only in
// the Markdown report, with their full per-item detail moved to the
// .xlsx companion once NeedsXLSXCompanion is true.
package nxos

import (
	"encoding/json"
	"sort"
	"strconv"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// ReportModel is the complete, format-neutral Cisco NX-OS report.
type ReportModel struct {
	Signature  SignatureInfo
	Switch     SwitchInfo
	Ports      PortSummary
	Interfaces InterfaceSummary
	LAGs       []LAGInfo
	VLANs      VLANSummary
	VRFs       []VRFInfo
	Neighbours NeighbourSummary
}

// nxosXLSXRowThreshold is the row count, per enumerable category, that
// triggers the .xlsx companion workbook, same used by other providers
// we set the default value to 50 raw after that the file is generated.
const nxosXLSXRowThreshold = 50

// NeedsXLSXCompanion reports whether any of the six enumerable
// categories (ports, interfaces, LAGs, VLANs, VRFs, neighbours)
// exceeds nxosXLSXRowThreshold rows. Once true, every one of the six
// gets a full sheet in the companion workbook regardless of its own
// individual size.
func (m ReportModel) NeedsXLSXCompanion() bool {
	return len(m.Ports.Rows) > nxosXLSXRowThreshold ||
		len(m.Interfaces.Rows) > nxosXLSXRowThreshold ||
		len(m.LAGs) > nxosXLSXRowThreshold ||
		len(m.VLANs.Rows) > nxosXLSXRowThreshold ||
		len(m.VRFs) > nxosXLSXRowThreshold ||
		len(m.Neighbours.Rows) > nxosXLSXRowThreshold
}

// SignatureInfo is the document's own provenance: the exact input hash
// plus every "metadata" field the document carries. Every string is
// "(absent)" when the document didn't carry it, never fabricated.
type SignatureInfo struct {
	InputSHA256                     string
	Timestamp                       string
	GeneratorName, GeneratorVersion string
	GeneratorURL                    string // empty, not "(absent)", when the render should omit the link
	ScopeName, ScopeDescription     string
	ScopePurpose                    string
	ScopeProviders                  []string
	ExtraMetadataKeys               []string
}

// SwitchInfo is the document's single network.switch resource: chassis
// identity and inventory. Zero value (every field "(absent)") when the
// document carries no such resource under ProviderName.
type SwitchInfo struct {
	ID, Name, NativeID   string
	ModelDescription     string // properties.model, the free-text chassis description.
	ChassisProductID     string // extensions.osiris.cisco.inventory "Chassis" entry's product_id, e.g. "N9K-C93180YC-FX".
	Manufacturer, Serial string
	SoftwareVersion      string
	BiosVersion          string
	ManagementIP         string
	MemoryMB             string
	PortCount            string
}

// PortSummary is the network.switch.port census: aggregate counts for
// the Markdown report (a chassis can carry many dozens of ports, too
// many to list inline), plus Rows, every port individually, for the
// .xlsx companion when NeedsXLSXCompanion is true.
type PortSummary struct {
	Total              int
	AdminUp, AdminDown int
	ByPortMode         []KV // port_mode string -> count, sorted by mode.
	Rows               []PortInfo
}

// PortInfo is one network.switch.port resource. IPAddress and
// SpeedMbps are carried by mgmt0 and other non-switched ports as well
// as ordinary data ports (panel_ports.go's own panelPort reads the
// same portProperties fields for the confirmed-panel edge label).
type PortInfo struct {
	Name, AdminStatus, PortMode string
	NativeVLAN, AccessVLAN      string
	IPAddress, SpeedMbps        string
}

// InterfaceSummary is the network.interface census restricted to this
// switch's own interfaces (SVIs, loopbacks, the management interface
// never the CDP/LLDP-discovered neighbour stubs, see Neighbours
// below). A full table when small; ByAdminStatus plus Rows moved to
// the .xlsx companion once the category crosses
// nxosXLSXRowThreshold.
type InterfaceSummary struct {
	Total         int
	ByAdminStatus []KV
	Rows          []InterfaceInfo
}

// InterfaceInfo is one of this switch's own network.interface
// resources.
type InterfaceInfo struct {
	Name, AdminStatus, IPAddress string
}

// LAGInfo is one network.interface.lag (port-channel) resource, with
// its member ports read from the document's own "contains" connections
// (report_model.go's buildLAGs), never inferred from naming.
type LAGInfo struct {
	Name, AdminStatus, Protocol, PortMode string
	NativeVLAN, AccessVLAN                string
	MemberCount                           string
	Members                               []string
}

// VLANSummary is the network.vlan census. A full table when small;
// ByAdminState plus Rows moved to the .xlsx companion once the
// category crosses nxosXLSXRowThreshold.
type VLANSummary struct {
	Total        int
	ByAdminState []KV
	Rows         []VLANInfo
}

// VLANInfo is one network.vlan resource.
type VLANInfo struct {
	ID, Name, AdminState string
}

// VRFInfo is one logical.vrf group.
type VRFInfo struct {
	Name, RouteDistinguisher, VRFID, State string
	Members                                int
}

// NeighbourSummary is the CDP/LLDP-discovered neighbour census:
// aggregate counts for the Markdown report, plus Rows, every
// neighbour individually, for the .xlsx companion when
// NeedsXLSXCompanion is true.
type NeighbourSummary struct {
	Total           int
	ByDiscoveredVia []KV
	Rows            []NeighbourInfo
}

// NeighbourInfo is one CDP/LLDP-discovered neighbour resource (see
// nxos.isExternallyDiscovered), with the local port
// topology.connections says it was seen on.
type NeighbourInfo struct {
	RemoteSystem, RemotePort, RemoteMgmtAddr string
	DiscoveredVia                            string
	LocalPort                                string
}

// KV is one sorted-order key/count pair for a breakdown table.
type KV struct {
	Key   string
	Count int
}

// BuildReportModel builds the report over g. md and inputSHA carry the
// document's own provenance into the Signature section; neither is
// re-derived from g.
func BuildReportModel(g *graph.Graph, md document.Metadata, inputSHA string) ReportModel {
	return ReportModel{
		Signature:  buildSignature(md, inputSHA),
		Switch:     buildSwitch(g),
		Ports:      buildPortSummary(g),
		Interfaces: buildInterfaceSummary(g),
		LAGs:       buildLAGs(g),
		VLANs:      buildVLANSummary(g),
		VRFs:       buildVRFs(g),
		Neighbours: buildNeighbourSummary(g),
	}
}

// reportScope is the subset of "metadata.scope" the Signature section
// discloses.
type reportScope struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Purpose     string   `json:"purpose"`
	Providers   []string `json:"providers"`
}

func decodeReportScope(raw json.RawMessage) reportScope {
	var s reportScope
	if len(raw) == 0 {
		return s
	}
	_ = json.Unmarshal(raw, &s)
	return s
}

func buildSignature(md document.Metadata, inputSHA string) SignatureInfo {
	s := SignatureInfo{
		InputSHA256: inputSHA,
		Timestamp:   stringOrAbsent(md.Timestamp),
	}
	if md.Generator != nil {
		s.GeneratorName = stringOrAbsent(md.Generator.Name)
		s.GeneratorVersion = stringOrAbsent(md.Generator.Version)
		s.GeneratorURL = md.Generator.URL
	} else {
		s.GeneratorName = "(absent)"
		s.GeneratorVersion = "(absent)"
	}
	scope := decodeReportScope(md.Scope)
	s.ScopeName = stringOrAbsent(scope.Name)
	s.ScopeDescription = stringOrAbsent(scope.Description)
	s.ScopePurpose = stringOrAbsent(scope.Purpose)
	s.ScopeProviders = scope.Providers
	for k := range md.Extra {
		s.ExtraMetadataKeys = append(s.ExtraMetadataKeys, k)
	}
	sort.Strings(s.ExtraMetadataKeys)
	return s
}

// switchProperties is the subset of the switch resource's "properties"
// this report reads.
type switchProperties struct {
	ManagementIP string `json:"management_ip"`
	Manufacturer string `json:"manufacturer"`
	MemoryMB     *int64 `json:"memory_mb"`
	Model        string `json:"model"`
	PortCount    *int64 `json:"port_count"`
	SerialNumber string `json:"serial_number"`
	Version      string `json:"version"`
}

// ciscoInventoryEntry is one entry of the switch resource's
// "extensions.osiris.cisco.inventory" array.
type ciscoInventoryEntry struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	ProductID   string `json:"product_id"`
	VendorID    string `json:"vendor_id"`
}

// switchExtensions is the subset of the switch resource's
// "extensions.osiris.cisco" this report reads.
type switchExtensions struct {
	BiosVersion string                `json:"bios_version"`
	Inventory   []ciscoInventoryEntry `json:"inventory"`
}

func decodeSwitchExtensions(raw json.RawMessage) switchExtensions {
	var e struct {
		Cisco switchExtensions `json:"osiris.cisco"`
	}
	if len(raw) == 0 {
		return e.Cisco
	}
	_ = json.Unmarshal(raw, &e)
	return e.Cisco
}

// chassisProductID returns the "Chassis"-named inventory entry's
// product_id, or "" when the extensions carry no such entry.
func chassisProductID(ext switchExtensions) string {
	for _, entry := range ext.Inventory {
		if entry.Name == "Chassis" {
			return entry.ProductID
		}
	}
	return ""
}

// buildSwitch reads the document's single network.switch resource
// under ProviderName the switch this NX-OS document describes. The
// zero SwitchInfo (every field "(absent)") is returned when the
// document carries none, rather than failing the render: a malformed
// or incomplete document still gets a report, evidence-state
// throughout.
func buildSwitch(g *graph.Graph) SwitchInfo {
	var id string
	for _, candidate := range g.ByType["network.switch"] {
		r := g.Resources[candidate]
		if r != nil && r.Provider.Name == ProviderName {
			id = candidate
			break
		}
	}
	if id == "" {
		return SwitchInfo{
			ID: "(absent)", Name: "(absent)", NativeID: "(absent)",
			ModelDescription: "(absent)", ChassisProductID: "(absent)",
			Manufacturer: "(absent)", Serial: "(absent)", SoftwareVersion: "(absent)",
			BiosVersion: "(absent)", ManagementIP: "(absent)", MemoryMB: "(absent)", PortCount: "(absent)",
		}
	}
	r := g.Resources[id]
	var props switchProperties
	_ = json.Unmarshal(r.Properties, &props)
	ext := decodeSwitchExtensions(r.Extensions)
	return SwitchInfo{
		ID: id, Name: labelOr(r.Name, id), NativeID: stringOrAbsent(r.Provider.NativeID),
		ModelDescription: stringOrAbsent(props.Model), ChassisProductID: stringOrAbsent(chassisProductID(ext)),
		Manufacturer: stringOrAbsent(props.Manufacturer), Serial: stringOrAbsent(props.SerialNumber),
		SoftwareVersion: stringOrAbsent(props.Version), BiosVersion: stringOrAbsent(ext.BiosVersion),
		ManagementIP: stringOrAbsent(props.ManagementIP),
		MemoryMB:     int64OrAbsent(props.MemoryMB), PortCount: int64OrAbsent(props.PortCount),
	}
}

// portProperties is the subset of a network.switch.port resource's
// properties this report reads. IPAddress and SpeedMbps are carried by
// mgmt0 and other non-switched ports as well as ordinary data ports,
// panel_ports.go's own panelPort reads them too, for the confirmed-
// panel edge label.
type portProperties struct {
	AdminStatus   string `json:"admin_status"`
	InterfaceName string `json:"interface_name"`
	IPAddress     string `json:"ip_address"`
	NativeVlan    *int64 `json:"native_vlan"`
	PortMode      string `json:"port_mode"`
	SpeedMbps     *int64 `json:"speed_mbps"`
	Vlan          *int64 `json:"vlan"`
}

// buildPortSummary reads every network.switch.port resource under
// ProviderName once, sorted by ID, building both the Markdown report's
// aggregate counts and (for the .xlsx companion) one PortInfo row per
// port.
func buildPortSummary(g *graph.Graph) PortSummary {
	var s PortSummary
	byMode := map[string]int{}
	for _, id := range sortedByType(g, "network.switch.port") {
		r := g.Resources[id]
		if r.Provider.Name != ProviderName {
			continue
		}
		var p portProperties
		_ = json.Unmarshal(r.Properties, &p)
		s.Total++
		switch p.AdminStatus {
		case "up":
			s.AdminUp++
		case "down":
			s.AdminDown++
		}
		if p.PortMode != "" {
			byMode[p.PortMode]++
		}
		name := p.InterfaceName
		if name == "" {
			name = labelOr(r.Name, id)
		}
		s.Rows = append(s.Rows, PortInfo{
			Name: name, AdminStatus: stringOrAbsent(p.AdminStatus), PortMode: stringOrAbsent(p.PortMode),
			NativeVLAN: int64OrAbsent(p.NativeVlan), AccessVLAN: int64OrAbsent(p.Vlan),
			IPAddress: stringOrAbsent(p.IPAddress), SpeedMbps: int64OrAbsent(p.SpeedMbps),
		})
	}
	s.ByPortMode = sortedKV(byMode)
	return s
}

// interfaceProperties is the subset of a network.interface resource's
// properties this report reads.
type interfaceProperties struct {
	AdminStatus string `json:"admin_status"`
	IPAddress   string `json:"ip_address"`
}

// buildInterfaceSummary reads every network.interface resource under
// ProviderName once, SVIs, loopbacks, and the management interface,
// never the CDP/LLDP-discovered neighbour stubs (those carry
// provider.name "unknown"/"", isExternallyDiscovered, and are counted
// by buildNeighbourSummary instead).
func buildInterfaceSummary(g *graph.Graph) InterfaceSummary {
	var s InterfaceSummary
	byStatus := map[string]int{}
	for _, id := range sortedByType(g, "network.interface") {
		r := g.Resources[id]
		if r.Provider.Name != ProviderName {
			continue
		}
		var p interfaceProperties
		_ = json.Unmarshal(r.Properties, &p)
		s.Total++
		if p.AdminStatus != "" {
			byStatus[p.AdminStatus]++
		}
		s.Rows = append(s.Rows, InterfaceInfo{
			Name: labelOr(r.Name, id), AdminStatus: stringOrAbsent(p.AdminStatus),
			IPAddress: stringOrAbsent(p.IPAddress),
		})
	}
	s.ByAdminStatus = sortedKV(byStatus)
	return s
}

// lagProperties is the subset of a network.interface.lag resource's
// properties this report reads. NativeVlan and Vlan are the same
// switchport-mode fields a network.switch.port resource carries
// (portProperties): a port-channel is itself a switched interface with
// its own trunk/access configuration, not merely a bundle of member
// ports.
type lagProperties struct {
	AdminStatus string `json:"admin_status"`
	MemberCount *int64 `json:"member_count"`
	NativeVlan  *int64 `json:"native_vlan"`
	PortMode    string `json:"port_mode"`
	Protocol    string `json:"protocol"`
	Vlan        *int64 `json:"vlan"`
}

// buildLAGs reads every network.interface.lag resource under
// ProviderName once, sorted by ID, resolving each one's member ports
// from the document's own "contains" connections (source the LAG,
// target the member port), never inferred from naming.
func buildLAGs(g *graph.Graph) []LAGInfo {
	var out []LAGInfo
	for _, id := range sortedByType(g, "network.interface.lag") {
		r := g.Resources[id]
		if r.Provider.Name != ProviderName {
			continue
		}
		var p lagProperties
		_ = json.Unmarshal(r.Properties, &p)
		var members []string
		for _, idx := range g.OutEdges[id] {
			e := g.Edges[idx]
			if e.Type != "contains" {
				continue
			}
			if mr, ok := g.Resources[e.Target]; ok {
				members = append(members, labelOr(mr.Name, e.Target))
			}
		}
		sort.Strings(members)
		out = append(out, LAGInfo{
			Name: labelOr(r.Name, id), AdminStatus: stringOrAbsent(p.AdminStatus),
			Protocol: stringOrAbsent(p.Protocol), PortMode: stringOrAbsent(p.PortMode),
			NativeVLAN: int64OrAbsent(p.NativeVlan), AccessVLAN: int64OrAbsent(p.Vlan),
			MemberCount: int64OrAbsent(p.MemberCount), Members: members,
		})
	}
	return out
}

// vlanProperties is the subset of a network.vlan resource's properties
// this report reads.
type vlanProperties struct {
	AdminState string `json:"admin_state"`
	VlanID     *int64 `json:"vlan_id"`
	VlanName   string `json:"vlan_name"`
}

// buildVLANSummary reads every network.vlan resource under
// ProviderName once, sorted by ID.
func buildVLANSummary(g *graph.Graph) VLANSummary {
	var s VLANSummary
	byState := map[string]int{}
	for _, id := range sortedByType(g, "network.vlan") {
		r := g.Resources[id]
		if r.Provider.Name != ProviderName {
			continue
		}
		var p vlanProperties
		_ = json.Unmarshal(r.Properties, &p)
		s.Total++
		if p.AdminState != "" {
			byState[p.AdminState]++
		}
		s.Rows = append(s.Rows, VLANInfo{
			ID: int64OrAbsent(p.VlanID), Name: stringOrAbsent(p.VlanName), AdminState: stringOrAbsent(p.AdminState),
		})
	}
	s.ByAdminState = sortedKV(byState)
	return s
}

// vrfProperties is the subset of a logical.vrf group's properties this
// report reads.
type vrfProperties struct {
	RouteDistinguisher string `json:"route_distinguisher"`
	State              string `json:"state"`
	VRFID              string `json:"vrf_id"`
}

// buildVRFs lists every logical.vrf group, sorted by name then ID.
func buildVRFs(g *graph.Graph) []VRFInfo {
	type keyed struct {
		id string
		v  VRFInfo
	}
	var rows []keyed
	for _, id := range g.GroupIDs {
		grp := g.Groups[id]
		if grp == nil || grp.Type != "logical.vrf" {
			continue
		}
		var p vrfProperties
		_ = json.Unmarshal(grp.Properties, &p)
		rows = append(rows, keyed{id: id, v: VRFInfo{
			Name: labelOr(grp.Name, id), RouteDistinguisher: stringOrAbsent(p.RouteDistinguisher),
			VRFID: stringOrAbsent(p.VRFID), State: stringOrAbsent(p.State), Members: len(grp.Members),
		}})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].v.Name != rows[j].v.Name {
			return rows[i].v.Name < rows[j].v.Name
		}
		return rows[i].id < rows[j].id
	})
	out := make([]VRFInfo, len(rows))
	for i, r := range rows {
		out[i] = r.v
	}
	return out
}

// neighbourProperties is the subset of a CDP/LLDP-discovered
// neighbour resource's properties this report reads.
type neighbourProperties struct {
	DiscoveredVia  []string `json:"discovered_via"`
	RemoteMgmtAddr string   `json:"remote_mgmt_addr"`
	RemotePort     string   `json:"remote_port"`
	RemoteSystem   string   `json:"remote_system"`
}

// localPortFor returns the local port name topology.connections says
// neighbourID was seen on, the first physical.ethernet connection
// naming it, in edge-declaration order, or "(absent)" when the
// document asserts none.
func localPortFor(g *graph.Graph, neighbourID string) string {
	for _, idx := range g.InEdges[neighbourID] {
		e := g.Edges[idx]
		if e.Type != "physical.ethernet" {
			continue
		}
		if r, ok := g.Resources[e.Source]; ok {
			return labelOr(r.Name, e.Source)
		}
	}
	return "(absent)"
}

// buildNeighbourSummary reads every CDP/LLDP-discovered neighbour
// resource (isExternallyDiscovered) once, sorted by ID, building both
// the Markdown report's aggregate counts and (for the .xlsx companion)
// one NeighbourInfo row per neighbour. Not restricted to
// network.interface: any resource type this producer ever marks
// externally discovered is counted here.
func buildNeighbourSummary(g *graph.Graph) NeighbourSummary {
	var ids []string
	for _, id := range g.ResourceIDs {
		r := g.Resources[id]
		if r != nil && isExternallyDiscovered(r.Provider.Name) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	var s NeighbourSummary
	s.Total = len(ids)
	byMethod := map[string]int{}
	for _, id := range ids {
		r := g.Resources[id]
		var n neighbourProperties
		_ = json.Unmarshal(r.Properties, &n)
		method := "(absent)"
		if len(n.DiscoveredVia) > 0 {
			method = n.DiscoveredVia[0]
			byMethod[method]++
		}
		s.Rows = append(s.Rows, NeighbourInfo{
			RemoteSystem: stringOrAbsent(n.RemoteSystem), RemotePort: stringOrAbsent(n.RemotePort),
			RemoteMgmtAddr: stringOrAbsent(n.RemoteMgmtAddr), DiscoveredVia: method,
			LocalPort: localPortFor(g, id),
		})
	}
	s.ByDiscoveredVia = sortedKV(byMethod)
	return s
}

// sortedByType returns g.ByType[typ], copied and sorted by ID, so
// every builder that reads one resource type gets the same
// deterministic order without repeating the copy-then-sort.
func sortedByType(g *graph.Graph, typ string) []string {
	ids := append([]string(nil), g.ByType[typ]...)
	sort.Strings(ids)
	return ids
}

// labelOr returns name, or id when name is empty, every resource's
// own display fallback.
func labelOr(name, id string) string {
	if name == "" {
		return id
	}
	return name
}

func stringOrAbsent(s string) string {
	if s == "" {
		return "(absent)"
	}
	return s
}

// int64OrAbsent formats v, or "(absent)" when the document's own
// properties carried no such field, distinguished from a genuine 0 by
// v being a nil pointer, never by treating 0 as missing.
func int64OrAbsent(v *int64) string {
	if v == nil {
		return "(absent)"
	}
	return strconv.FormatInt(*v, 10)
}

// sortedKV turns a key-to-count map into a deterministic, key-sorted
// slice.
func sortedKV(m map[string]int) []KV {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]KV, len(keys))
	for i, k := range keys {
		out[i] = KV{Key: k, Count: m[k]}
	}
	return out
}
