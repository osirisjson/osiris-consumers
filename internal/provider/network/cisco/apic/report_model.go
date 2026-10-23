// This file builds the format-neutral Cisco APIC report model: a
// fabric summary (pod, controller, spine, leaf, tenant, and port
// census), one inventory row per controller and role-bearing switch
// (with its own point-in-time fault severity counts), a tenant/VRF/EPG
// rollup from the document's groups, aggregate bridge-domain, subnet,
// L3out, port, and externally discovered neighbour counts, and the
// document's own signature. It is read-only over the generic graph,
// never re-parsing the document; see report_render.go for the
// Markdown emission and topology.go for the Applies trigger this
// shares, and buildFabric/podFromNativeID/labelOr for the pod and role
// extraction this reuses.
//
// Unlike the topology page a curated device-level diagram this
// report accounts for every resource type the fixture declares:
// role-bearing fabric nodes get a full inventory row, and every other
// type (ports, bridge domains, subnets, L3outs, role-less discovered
// neighbours, tenants/VRFs/EPGs) gets at least an aggregate count, so
// nothing the document carries goes unacknowledged even where listing
// every individual instance would be unreadable.
package apic

import (
	"encoding/json"
	"sort"
	"strings"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// ReportModel is the complete, format-neutral Cisco APIC report.
type ReportModel struct {
	Signature     SignatureInfo
	Faults        []FaultRecord // every fault any controller or role-bearing switch asserts, sorted critical-first.
	Fabric        FabricSummary
	Nodes         []NodeInfo // every controller and role-bearing switch, sorted by pod then band then ID.
	Tenants       []TenantInfo
	VRFs          []GroupRollup     // every logical.vrf group, sorted by tenant then name.
	VRFMembers    []GroupMemberInfo // every logical.vrf group's direct members, one row each, sorted by VRF then member.
	EPGs          []GroupRollup     // every osiris.cisco.epg group, sorted by tenant then name.
	EPGMembers    []GroupMemberInfo // every osiris.cisco.epg group's direct members, one row each, sorted by EPG then member.
	L3Outs        []L3OutInfo
	BridgeDomains []BridgeDomainInfo
	Subnets       []SubnetInfo
	Ports         PortSummary
	Neighbours    NeighbourSummary
}

// apicXLSXRowThreshold is the row count, per enumerable category, that
// triggers the .xlsx companion workbook, same used by other providers
// we set the default value to 50 raw after that the file is generated.
const apicXLSXRowThreshold = 50

// NeedsXLSXCompanion reports whether any of the eight enumerable
// categories (faults, tenants, VRFs, EPGs, bridge domains, subnets,
// ports, neighbours) exceeds apicXLSXRowThreshold rows. Once true,
// every one of the eight gets a full sheet in the companion workbook
// regardless of its own individual size.
func (m ReportModel) NeedsXLSXCompanion() bool {
	return len(m.Faults) > apicXLSXRowThreshold ||
		len(m.Tenants) > apicXLSXRowThreshold ||
		len(m.VRFs) > apicXLSXRowThreshold ||
		len(m.EPGs) > apicXLSXRowThreshold ||
		len(m.BridgeDomains) > apicXLSXRowThreshold ||
		len(m.Subnets) > apicXLSXRowThreshold ||
		len(m.Ports.Rows) > apicXLSXRowThreshold ||
		len(m.Neighbours.Rows) > apicXLSXRowThreshold
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

// FabricSummary is the fabric-wide census: every count a reader would
// want before drilling into the inventory tables.
type FabricSummary struct {
	Domain        string // properties.fabric_domain, "(absent)" if no node carried one.
	Pods          int
	Controllers   int
	Spines        int
	Leafs         int
	Neighbours    int // role-less externally discovered switches.
	Tenants       int
	VRFs          int
	EPGs          int
	BridgeDomains int
	Subnets       int
	L3Outs        int
	Ports         int
}

// FaultCounts is one resource's own asserted fault severities, exactly
// as the document's extensions.osiris.cisco.faults carries them
// evidence-state counts, never a derived health verdict.
type FaultCounts struct {
	Critical, Major, Minor, Warning int
}

// Total returns the sum of every severity.
func (f FaultCounts) Total() int { return f.Critical + f.Major + f.Minor + f.Warning }

// FaultRecord is one fault, exactly as a controller or role-bearing
// switch's own extensions.osiris.cisco.faults entry asserts it, with
// the node it was observed on. Code and Subject are not carried here:
// Cause and Description already identify the fault, and the generic
// inventory keeps every field with no semantic loss for anyone who
// needs them.
type FaultRecord struct {
	Node, Severity, Cause, Description string
	Domain, Lifecycle                  string
	Created, LastTransition            string
}

// NodeInfo is one controller or role-bearing switch's inventory row.
type NodeInfo struct {
	ID, Name, NativeID       string
	Band                     fabricBand // bandController, bandSpine, or bandLeaf.
	Pod                      string     // "" for a controller.
	Model, Serial, Firmware  string
	OobMgmtAddr, InbMgmtAddr string
	SystemState, Uptime      string
	Faults                   FaultCounts
}

// TenantInfo is one logical.tenant group's VRF, EPG, and direct-member
// rollup, a count, not a listing, since a fabric can carry hundreds
// of EPGs.
type TenantInfo struct {
	Name          string
	VRFs, EPGs    int
	DirectMembers int
}

// L3OutInfo is one osiris.cisco.l3out resource.
type L3OutInfo struct {
	Name, NativeID, Tenant string
}

// GroupRollup is one VRF or EPG group's name, owning tenant, and
// direct member count, both group types share this shape, so one
// builder (buildGroupRollups) and one type serve both.
type GroupRollup struct {
	Name, Tenant string
	Members      int
}

// GroupMemberInfo is one direct member of a VRF or EPG group, flattened
// to one row per member (a VRF can have dozens; buildGroupRollups'
// Members count alone does not disclose which resources they are).
// Member is the member identifier with its "<provider>::" prefix
// stripped (every member in a Cisco APIC document shares the same
// provider, so the prefix is redundant noise, not information).
type GroupMemberInfo struct {
	Group, Tenant, Member string
}

// BridgeDomainInfo is one osiris.cisco.domain.bridge resource.
type BridgeDomainInfo struct {
	Name, Tenant                                    string
	MAC, UnicastRouting, ArpFlood, L2UnknownUnicast string
}

// SubnetInfo is one network.subnet resource.
type SubnetInfo struct {
	CIDR, Tenant, GatewayIP, ACIScope, Preferred string
}

// PortSummary is the fabric-wide network.switch.port census: aggregate
// counts for the Markdown report, plus Rows every port individually
// for the .xlsx companion when NeedsXLSXCompanion is true (a real
// fabric carries thousands, far too many for a Markdown table).
type PortSummary struct {
	Total                         int
	AdminUp, AdminDown            int
	OperUp, OperDown, OperUnknown int
	ByUsage                       []KV // usage string -> count, sorted by usage.
	Rows                          []PortInfo
}

// PortInfo is one network.switch.port resource.
type PortInfo struct {
	Node, Name                          string
	AdminStatus, OperStatus, Usage      string
	Speed, PortMode, Layer, Mtu, Duplex string
}

// NeighbourSummary is the fabric-wide role-less (externally discovered)
// switch census: aggregate counts for the Markdown report, plus Rows
// every neighbour individually, for the .xlsx companion when
// NeedsXLSXCompanion is true.
type NeighbourSummary struct {
	Total          int
	ByDiscoveredBy []KV // discovery method -> count, sorted by method.
	Rows           []NeighbourInfo
}

// NeighbourInfo is one externally discovered (role-less) network.switch
// resource, with the fabric node and port topology.connections says it
// was seen on, when the document asserts one.
type NeighbourInfo struct {
	ChassisID, DiscoveredBy          string
	ConnectedToNode, ConnectedToPort string
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
	controllers, pods := buildFabric(g)
	nodes := buildNodes(g, controllers, pods)
	tenants := buildTenants(g)
	vrfs := buildGroupRollups(g, "logical.vrf")
	vrfMembers := buildGroupMembers(g, "logical.vrf")
	epgs := buildGroupRollups(g, "osiris.cisco.epg")
	epgMembers := buildGroupMembers(g, "osiris.cisco.epg")
	l3outs := buildL3Outs(g)
	bridgeDomains := buildBridgeDomains(g)
	subnets := buildSubnets(g)
	nodeNames := buildNodeNameIndex(g)
	ports := buildPortSummary(g, nodeNames)
	neighbours := buildNeighbourSummary(g)

	spines, leafs := 0, 0
	for _, p := range pods {
		spines += len(p.Spines)
		leafs += len(p.Leafs)
	}

	return ReportModel{
		Signature: buildSignature(md, inputSHA),
		Faults:    buildFaults(g, controllers, pods),
		Fabric: FabricSummary{
			Domain:        fabricDomain(g),
			Pods:          len(pods),
			Controllers:   len(controllers),
			Spines:        spines,
			Leafs:         leafs,
			Neighbours:    neighbours.Total,
			Tenants:       len(tenants),
			VRFs:          len(vrfs),
			EPGs:          len(epgs),
			BridgeDomains: len(bridgeDomains),
			Subnets:       len(subnets),
			L3Outs:        len(l3outs),
			Ports:         ports.Total,
		},
		Nodes:         nodes,
		Tenants:       tenants,
		VRFs:          vrfs,
		VRFMembers:    vrfMembers,
		EPGs:          epgs,
		EPGMembers:    epgMembers,
		L3Outs:        l3outs,
		BridgeDomains: bridgeDomains,
		Subnets:       subnets,
		Ports:         ports,
		Neighbours:    neighbours,
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

// nodeProperties is the subset of a fabric node's "properties" the
// report reads a superset of topology.go's fabricProperties, since
// the report's inventory rows carry more than a diagram label needs.
type nodeProperties struct {
	Model        string `json:"model"`
	Role         string `json:"role"`
	Serial       string `json:"serial"`
	FirmwareVer  string `json:"firmware_version"`
	OobMgmtAddr  string `json:"oob_mgmt_addr"`
	InbMgmtAddr  string `json:"inb_mgmt_addr"`
	SystemState  string `json:"system_state"`
	Uptime       string `json:"uptime"`
	FabricDomain string `json:"fabric_domain"`
	NodeID       string `json:"node_id"`
}

func decodeNodeProperties(raw json.RawMessage) nodeProperties {
	var p nodeProperties
	if len(raw) == 0 {
		return p
	}
	_ = json.Unmarshal(raw, &p)
	return p
}

// ciscoFault is one entry of a resource's "extensions.osiris.cisco.faults"
// array, exactly as the document asserts it evidence-state fields,
// never a derived verdict.
type ciscoFault struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	Cause          string `json:"cause"`
	Description    string `json:"description"`
	Created        string `json:"created"`
	LastTransition string `json:"last_transition"`
	Lifecycle      string `json:"lifecycle"`
	Domain         string `json:"domain"`
	Subject        string `json:"subject"`
}

// ciscoExtensions is the subset of a resource's "extensions.osiris.cisco"
// the report reads.
type ciscoExtensions struct {
	Faults []ciscoFault `json:"faults"`
}

func decodeCiscoExtensions(raw json.RawMessage) ciscoExtensions {
	var e struct {
		Cisco ciscoExtensions `json:"osiris.cisco"`
	}
	if len(raw) == 0 {
		return e.Cisco
	}
	_ = json.Unmarshal(raw, &e)
	return e.Cisco
}

func faultCountsFor(raw json.RawMessage) FaultCounts {
	var f FaultCounts
	for _, fault := range decodeCiscoExtensions(raw).Faults {
		switch fault.Severity {
		case "critical":
			f.Critical++
		case "major":
			f.Major++
		case "minor":
			f.Minor++
		case "warning":
			f.Warning++
		}
	}
	return f
}

// buildNodes builds one inventory row per controller and role-bearing
// switch buildFabric already identified and grouped, re-reading each
// resource's full properties and fault extensions (buildFabric's own
// fabricNode only carries what the topology diagram's label needs).
// Sorted controllers first, then by ascending pod number, spines
// before leafs within a pod, each by resource ID a total,
// deterministic order.
func buildNodes(g *graph.Graph, controllers []fabricNode, pods []podFabric) []NodeInfo {
	var out []NodeInfo
	row := func(n fabricNode, pod string) NodeInfo {
		r := g.Resources[n.ID]
		props := decodeNodeProperties(r.Properties)
		return NodeInfo{
			ID: n.ID, Name: n.Label, NativeID: stringOrAbsent(r.Provider.NativeID),
			Band: n.Band, Pod: pod,
			Model: stringOrAbsent(props.Model), Serial: stringOrAbsent(props.Serial),
			Firmware:    stringOrAbsent(props.FirmwareVer),
			OobMgmtAddr: stringOrAbsent(props.OobMgmtAddr), InbMgmtAddr: stringOrAbsent(props.InbMgmtAddr),
			SystemState: stringOrAbsent(props.SystemState), Uptime: stringOrAbsent(props.Uptime),
			Faults: faultCountsFor(r.Extensions),
		}
	}
	for _, c := range controllers {
		out = append(out, row(c, ""))
	}
	for _, p := range pods {
		for _, s := range p.Spines {
			out = append(out, row(s, p.Pod))
		}
		for _, l := range p.Leafs {
			out = append(out, row(l, p.Pod))
		}
	}
	return out
}

// severityRank orders fault severities critical-first for buildFaults'
// sort; an unrecognized or missing severity sorts last, after warning.
func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 0
	case "major":
		return 1
	case "minor":
		return 2
	case "warning":
		return 3
	default:
		return 4
	}
}

// buildFaults collects every fault any controller or role-bearing
// switch's own extensions.osiris.cisco.faults asserts, sorted
// critical-first, then by node name, then by created timestamp, then
// by cause, a total, deterministic order over what is otherwise a
// flat, unordered list spread across many resources.
func buildFaults(g *graph.Graph, controllers []fabricNode, pods []podFabric) []FaultRecord {
	var out []FaultRecord
	collect := func(n fabricNode) {
		r := g.Resources[n.ID]
		for _, fault := range decodeCiscoExtensions(r.Extensions).Faults {
			out = append(out, FaultRecord{
				Node: n.Label, Severity: stringOrAbsent(fault.Severity),
				Cause: stringOrAbsent(fault.Cause), Description: stringOrAbsent(fault.Description),
				Domain: stringOrAbsent(fault.Domain), Lifecycle: stringOrAbsent(fault.Lifecycle),
				Created: stringOrAbsent(fault.Created), LastTransition: stringOrAbsent(fault.LastTransition),
			})
		}
	}
	for _, c := range controllers {
		collect(c)
	}
	for _, p := range pods {
		for _, s := range p.Spines {
			collect(s)
		}
		for _, l := range p.Leafs {
			collect(l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := severityRank(a.Severity), severityRank(b.Severity); ra != rb {
			return ra < rb
		}
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		if a.Created != b.Created {
			return a.Created < b.Created
		}
		return a.Cause < b.Cause
	})
	return out
}

// fabricDomain returns the first non-empty properties.fabric_domain
// any compute.server or network.switch resource under ProviderName
// carries, or "(absent)" when none do, a single fabric normally
// reports the same domain on every node, so the first one found in
// declaration order is representative, not an assumption this package
// verifies.
func fabricDomain(g *graph.Graph) string {
	for _, typ := range [2]string{"compute.server", "network.switch"} {
		for _, id := range g.ByType[typ] {
			r := g.Resources[id]
			if r == nil || r.Provider.Name != ProviderName {
				continue
			}
			if d := decodeNodeProperties(r.Properties).FabricDomain; d != "" {
				return d
			}
		}
	}
	return "(absent)"
}

// buildTenants rolls up each logical.tenant group's direct VRF and EPG
// children plus its own direct member count, sorted by tenant name
// then ID for a total order.
func buildTenants(g *graph.Graph) []TenantInfo {
	type keyed struct {
		id string
		t  TenantInfo
	}
	var rows []keyed
	for _, id := range g.GroupIDs {
		grp := g.Groups[id]
		if grp == nil || grp.Type != "logical.tenant" {
			continue
		}
		vrfs, epgs := 0, 0
		for _, childID := range g.GroupChildren[id] {
			child := g.Groups[childID]
			if child == nil {
				continue
			}
			switch child.Type {
			case "logical.vrf":
				vrfs++
			case "osiris.cisco.epg":
				epgs++
			}
		}
		rows = append(rows, keyed{id: id, t: TenantInfo{
			Name: labelOr(grp.Name, id), VRFs: vrfs, EPGs: epgs, DirectMembers: len(grp.Members),
		}})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].t.Name != rows[j].t.Name {
			return rows[i].t.Name < rows[j].t.Name
		}
		return rows[i].id < rows[j].id
	})
	out := make([]TenantInfo, len(rows))
	for i, r := range rows {
		out[i] = r.t
	}
	return out
}

// tenantFromNativeID extracts the "tn-<name>" segment of a Cisco APIC
// native_id (e.g. "uni/tn-common/out-default" -> "common"), or ""
// when no such segment is present.
func tenantFromNativeID(nativeID string) string {
	for _, seg := range strings.Split(nativeID, "/") {
		if strings.HasPrefix(seg, "tn-") {
			return strings.TrimPrefix(seg, "tn-")
		}
	}
	return ""
}

// tenantNameForGroup returns the name of the logical.tenant group that
// declares groupID as a child (the smallest-ID one, for determinism,
// on the rare document that lists more than one), or "(absent)" when
// no tenant does.
func tenantNameForGroup(g *graph.Graph, groupID string) string {
	parents := append([]string(nil), g.ParentOfGroups[groupID]...)
	sort.Strings(parents)
	for _, pid := range parents {
		if grp := g.Groups[pid]; grp != nil && grp.Type == "logical.tenant" {
			return labelOr(grp.Name, pid)
		}
	}
	return "(absent)"
}

// buildGroupRollups lists every group of groupType ("logical.vrf" or
// "osiris.cisco.epg"), sorted by owning tenant, then name, then ID
// both group types share the same {name, tenant, member count} shape.
func buildGroupRollups(g *graph.Graph, groupType string) []GroupRollup {
	type keyed struct {
		id string
		r  GroupRollup
	}
	var rows []keyed
	for _, id := range g.GroupIDs {
		grp := g.Groups[id]
		if grp == nil || grp.Type != groupType {
			continue
		}
		rows = append(rows, keyed{id: id, r: GroupRollup{
			Name: labelOr(grp.Name, id), Tenant: tenantNameForGroup(g, id), Members: len(grp.Members),
		}})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].r.Tenant != rows[j].r.Tenant {
			return rows[i].r.Tenant < rows[j].r.Tenant
		}
		if rows[i].r.Name != rows[j].r.Name {
			return rows[i].r.Name < rows[j].r.Name
		}
		return rows[i].id < rows[j].id
	})
	out := make([]GroupRollup, len(rows))
	for i, r := range rows {
		out[i] = r.r
	}
	return out
}

// stripProviderPrefix removes a leading "<provider>::" from an opaque
// OSIRIS JSON identifier (e.g. "cisco.apic::uni/tn-infra/BD-ave-ctrl"
// -> "uni/tn-infra/BD-ave-ctrl"), every member in a Cisco APIC-only
// document shares the same provider prefix, so keeping it on every row
// is redundant noise, not information.
func stripProviderPrefix(id string) string {
	if _, rest, ok := strings.Cut(id, "::"); ok {
		return rest
	}
	return id
}

// buildGroupMembers flattens every group of groupType's direct members
// into one row each, sorted by owning tenant, then group name, then
// member, a VRF can have dozens of members, disclosed individually
// here since buildGroupRollups' Members is only a count.
func buildGroupMembers(g *graph.Graph, groupType string) []GroupMemberInfo {
	var out []GroupMemberInfo
	var ids []string
	for _, id := range g.GroupIDs {
		if grp := g.Groups[id]; grp != nil && grp.Type == groupType {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		grp := g.Groups[id]
		name := labelOr(grp.Name, id)
		tenant := tenantNameForGroup(g, id)
		members := append([]string(nil), grp.Members...)
		sort.Strings(members)
		for _, m := range members {
			out = append(out, GroupMemberInfo{Group: name, Tenant: tenant, Member: stripProviderPrefix(m)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tenant != out[j].Tenant {
			return out[i].Tenant < out[j].Tenant
		}
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Member < out[j].Member
	})
	return out
}

// bridgeDomainProperties is the subset of an
// osiris.cisco.domain.bridge resource's properties the report reads.
type bridgeDomainProperties struct {
	MAC              string `json:"mac"`
	UnicastRouting   string `json:"unicast_routing"`
	ArpFlood         string `json:"arp_flood"`
	L2UnknownUnicast string `json:"l2_unknown_unicast"`
}

// buildBridgeDomains lists every osiris.cisco.domain.bridge resource,
// sorted by ID.
func buildBridgeDomains(g *graph.Graph) []BridgeDomainInfo {
	ids := append([]string(nil), g.ByType["osiris.cisco.domain.bridge"]...)
	sort.Strings(ids)
	out := make([]BridgeDomainInfo, 0, len(ids))
	for _, id := range ids {
		r := g.Resources[id]
		var p bridgeDomainProperties
		_ = json.Unmarshal(r.Properties, &p)
		out = append(out, BridgeDomainInfo{
			Name: labelOr(r.Name, id), Tenant: stringOrAbsent(tenantFromNativeID(r.Provider.NativeID)),
			MAC: stringOrAbsent(p.MAC), UnicastRouting: stringOrAbsent(p.UnicastRouting),
			ArpFlood: stringOrAbsent(p.ArpFlood), L2UnknownUnicast: stringOrAbsent(p.L2UnknownUnicast),
		})
	}
	return out
}

// subnetProperties and subnetExtensions are the subsets of a
// network.subnet resource's properties and osiris.cisco extensions the
// report reads.
type subnetProperties struct {
	CIDR      string `json:"cidr"`
	GatewayIP string `json:"gateway_ip"`
}

type subnetExtensions struct {
	ACIScope  string `json:"aci_scope"`
	Preferred string `json:"preferred"`
}

func decodeSubnetExtensions(raw json.RawMessage) subnetExtensions {
	var e struct {
		Cisco subnetExtensions `json:"osiris.cisco"`
	}
	if len(raw) == 0 {
		return e.Cisco
	}
	_ = json.Unmarshal(raw, &e)
	return e.Cisco
}

// buildSubnets lists every network.subnet resource, sorted by ID.
func buildSubnets(g *graph.Graph) []SubnetInfo {
	ids := append([]string(nil), g.ByType["network.subnet"]...)
	sort.Strings(ids)
	out := make([]SubnetInfo, 0, len(ids))
	for _, id := range ids {
		r := g.Resources[id]
		var p subnetProperties
		_ = json.Unmarshal(r.Properties, &p)
		ext := decodeSubnetExtensions(r.Extensions)
		out = append(out, SubnetInfo{
			CIDR: stringOrAbsent(p.CIDR), Tenant: stringOrAbsent(tenantFromNativeID(r.Provider.NativeID)),
			GatewayIP: stringOrAbsent(p.GatewayIP), ACIScope: stringOrAbsent(ext.ACIScope),
			Preferred: stringOrAbsent(ext.Preferred),
		})
	}
	return out
}

// buildNodeNameIndex maps a fabric node's own properties.node_id to
// its display name, for resolving a port's owning switch (which
// carries only the node_id, not the switch's name) to something
// readable. Built from every compute.server and network.switch
// resource under ProviderName, role-bearing or not.
func buildNodeNameIndex(g *graph.Graph) map[string]string {
	idx := map[string]string{}
	for _, typ := range [2]string{"compute.server", "network.switch"} {
		for _, id := range g.ByType[typ] {
			r := g.Resources[id]
			if r == nil || r.Provider.Name != ProviderName {
				continue
			}
			props := decodeNodeProperties(r.Properties)
			if props.NodeID == "" {
				continue
			}
			idx[props.NodeID] = labelOr(r.Name, id)
		}
	}
	return idx
}

// buildL3Outs lists every osiris.cisco.l3out resource, sorted by ID.
func buildL3Outs(g *graph.Graph) []L3OutInfo {
	ids := append([]string(nil), g.ByType["osiris.cisco.l3out"]...)
	sort.Strings(ids)
	out := make([]L3OutInfo, 0, len(ids))
	for _, id := range ids {
		r := g.Resources[id]
		out = append(out, L3OutInfo{
			Name: labelOr(r.Name, id), NativeID: stringOrAbsent(r.Provider.NativeID),
			Tenant: stringOrAbsent(tenantFromNativeID(r.Provider.NativeID)),
		})
	}
	return out
}

// portProperties is the subset of a network.switch.port resource's
// properties the report reads.
type portProperties struct {
	AdminStatus   string `json:"admin_status"`
	OperStatus    string `json:"oper_status"`
	Usage         string `json:"usage"`
	Speed         string `json:"speed"`
	PortMode      string `json:"port_mode"`
	Layer         string `json:"layer"`
	Mtu           string `json:"mtu"`
	Duplex        string `json:"duplex"`
	NodeID        string `json:"node_id"`
	InterfaceName string `json:"interface_name"`
}

// buildPortSummary reads every network.switch.port resource once,
// sorted by ID, building both the Markdown report's aggregate counts
// and (for the .xlsx companion) one PortInfo row per port. nodeNames
// resolves a port's own node_id to its owning switch's display name
// (see buildNodeNameIndex).
func buildPortSummary(g *graph.Graph, nodeNames map[string]string) PortSummary {
	ids := append([]string(nil), g.ByType["network.switch.port"]...)
	sort.Strings(ids)
	var s PortSummary
	s.Total = len(ids)
	usage := map[string]int{}
	rows := make([]PortInfo, 0, len(ids))
	for _, id := range ids {
		r := g.Resources[id]
		var p portProperties
		_ = json.Unmarshal(r.Properties, &p)
		switch p.AdminStatus {
		case "up":
			s.AdminUp++
		case "down":
			s.AdminDown++
		}
		switch p.OperStatus {
		case "up":
			s.OperUp++
		case "down":
			s.OperDown++
		default:
			s.OperUnknown++
		}
		if p.Usage != "" {
			usage[p.Usage]++
		}
		node, ok := nodeNames[p.NodeID]
		if !ok {
			node = "(absent)"
		}
		name := p.InterfaceName
		if name == "" {
			name = labelOr(r.Name, id)
		}
		rows = append(rows, PortInfo{
			Node: node, Name: name,
			AdminStatus: stringOrAbsent(p.AdminStatus), OperStatus: stringOrAbsent(p.OperStatus),
			Usage: stringOrAbsent(p.Usage), Speed: stringOrAbsent(p.Speed),
			PortMode: stringOrAbsent(p.PortMode), Layer: stringOrAbsent(p.Layer),
			Mtu: stringOrAbsent(p.Mtu), Duplex: stringOrAbsent(p.Duplex),
		})
	}
	s.ByUsage = sortedKV(usage)
	s.Rows = rows
	return s
}

// neighbourProperties is the subset of a role-less network.switch
// resource's properties the report reads.
type neighbourProperties struct {
	ChassisID    string `json:"chassis_id"`
	DiscoveredBy string `json:"discovered_by"`
}

// connTargetPort is the subset of a physical.ethernet connection's
// properties the report reads.
type connTargetPort struct {
	TargetPort string `json:"target_port"`
}

// connectedFabricNode returns the fabric node and port
// topology.connections says neighbourID (an externally discovered
// switch) was seen on, the first physical.ethernet connection naming
// it, in edge-declaration order, or "(absent)", "(absent)" when the
// document asserts none.
func connectedFabricNode(g *graph.Graph, neighbourID string) (node, port string) {
	idxs := append(append([]int(nil), g.OutEdges[neighbourID]...), g.InEdges[neighbourID]...)
	for _, idx := range idxs {
		e := g.Edges[idx]
		if e.Type != "physical.ethernet" {
			continue
		}
		other := e.Target
		if other == neighbourID {
			other = e.Source
		}
		r, ok := g.Resources[other]
		if !ok {
			continue
		}
		p := ""
		if conn := g.Connections[e.ConnectionID]; conn != nil {
			var tp connTargetPort
			_ = json.Unmarshal(conn.Properties, &tp)
			p = tp.TargetPort
		}
		return labelOr(r.Name, other), stringOrAbsent(p)
	}
	return "(absent)", "(absent)"
}

// buildNeighbourSummary reads every role-less (externally discovered)
// network.switch resource once, sorted by ID, building both the
// Markdown report's aggregate counts and (for the .xlsx companion) one
// NeighbourInfo row per neighbour.
func buildNeighbourSummary(g *graph.Graph) NeighbourSummary {
	var ids []string
	for _, id := range g.ByType["network.switch"] {
		r := g.Resources[id]
		if r == nil || r.Provider.Name != ProviderName {
			continue
		}
		if decodeNodeProperties(r.Properties).Role != "" {
			continue // role-bearing: counted in Fabric.Spines/Leafs, not here.
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var s NeighbourSummary
	s.Total = len(ids)
	byMethod := map[string]int{}
	rows := make([]NeighbourInfo, 0, len(ids))
	for _, id := range ids {
		r := g.Resources[id]
		var neigh neighbourProperties
		_ = json.Unmarshal(r.Properties, &neigh)
		if neigh.DiscoveredBy != "" {
			byMethod[neigh.DiscoveredBy]++
		}
		connNode, connPort := connectedFabricNode(g, id)
		rows = append(rows, NeighbourInfo{
			ChassisID: stringOrAbsent(neigh.ChassisID), DiscoveredBy: stringOrAbsent(neigh.DiscoveredBy),
			ConnectedToNode: connNode, ConnectedToPort: connPort,
		})
	}
	s.ByDiscoveredBy = sortedKV(byMethod)
	s.Rows = rows
	return s
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
