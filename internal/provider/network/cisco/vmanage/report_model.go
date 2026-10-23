// This file builds the format-neutral Cisco vManage report model: the
// document's own signature, every network.router this snapshot
// describes (a vManage document may carry more than one, unlike Cisco
// NX-OS's single switch), a router.port census attributed back to its
// own owning router, and the site/room groups this fleet belongs to.
// It is read-only over the generic graph, never re-parsing the
// document; see report_render.go for the Markdown emission and
// vmanage.go for the Applies trigger this shares.
package vmanage

import (
	"encoding/json"
	"sort"
	"strconv"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// ReportModel is the complete, format-neutral Cisco vManage report.
type ReportModel struct {
	Signature SignatureInfo
	Routers   []RouterInfo
	Ports     PortSummary
	Sites     []SiteInfo
}

// vmanageXLSXRowThreshold is the row count, per enumerable category,
// triggering the .xlsx companion workbook, same used by other providers
// we set the default value to 50 raw after that the file is generated.
const vmanageXLSXRowThreshold = 50

// NeedsXLSXCompanion reports whether either enumerable category
// (routers, ports) exceeds vmanageXLSXRowThreshold rows.
func (m ReportModel) NeedsXLSXCompanion() bool {
	return len(m.Routers) > vmanageXLSXRowThreshold || len(m.Ports.Rows) > vmanageXLSXRowThreshold
}

// SignatureInfo is the document's own provenance: the exact input hash
// plus every "metadata" field the document carries. Every string is
// "(absent)" when the document didn't carry it, never fabricated.
// ScopeSites and ScopeClusters are carried by this producer's own
// "metadata.scope" (the fleet's site codes and the vManage cluster
// that manages them) beyond the generic name/description/purpose/
// providers every producer's scope may carry.
type SignatureInfo struct {
	InputSHA256                     string
	Timestamp                       string
	GeneratorName, GeneratorVersion string
	GeneratorURL                    string // empty, not "(absent)", when the render should omit the link
	ScopeName, ScopeDescription     string
	ScopePurpose                    string
	ScopeProviders                  []string
	ScopeSites, ScopeClusters       []string
	ExtraMetadataKeys               []string
}

// reportScope is the subset of "metadata.scope" the Signature section
// discloses.
type reportScope struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Purpose     string   `json:"purpose"`
	Providers   []string `json:"providers"`
	Sites       []string `json:"sites"`
	Clusters    []string `json:"clusters"`
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
	s.ScopeSites = scope.Sites
	s.ScopeClusters = scope.Clusters
	for k := range md.Extra {
		s.ExtraMetadataKeys = append(s.ExtraMetadataKeys, k)
	}
	sort.Strings(s.ExtraMetadataKeys)
	return s
}

// RouterInfo is one network.router resource, one of possibly several
// edge routers a vManage document describes for its own site.
type RouterInfo struct {
	ID, Name, NativeID            string
	Model, Manufacturer, Serial   string
	ManagementIP, SoftwareVersion string
	Status, State                 string // resource-level, literal ("active"/"degraded"/... ; "reachable"/...).
	StateDescription              string // extensions' own human-readable elaboration on State (e.g. "All daemons up").
	Health                        string
	CertificateValidity           string
	ChassisNumber                 string
	SiteID                        string
	UpSince, LastUpdate           string
	ConnectedManager              []string // extensions' own "connected_manager" list, the vManage controller(s) this router reports being managed by.
	Site                          string   // the name of the group (e.g. a physical.room) this router is a member of, never the raw numeric SiteID.
}

// routerProperties is the subset of a network.router resource's
// properties this report reads.
type routerProperties struct {
	ManagementIP string `json:"management_ip"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	SerialNumber string `json:"serial_number"`
	Version      string `json:"version"`
}

// routerExtensions is the subset of a network.router resource's
// "extensions.osiris.cisco.vmanage" object this report reads.
type routerExtensions struct {
	CertificateValidity string   `json:"certificate_validity"`
	ChassisNumber       string   `json:"chassis_number"`
	ConnectedManager    []string `json:"connected_manager"`
	Health              string   `json:"health"`
	LastUpdate          string   `json:"last_update"`
	SiteID              string   `json:"site_id"`
	StateDescription    string   `json:"state_description"`
	UpSince             string   `json:"up_since"`
}

func decodeRouterExtensions(raw json.RawMessage) routerExtensions {
	var e struct {
		VManage routerExtensions `json:"osiris.cisco.vmanage"`
	}
	if len(raw) == 0 {
		return e.VManage
	}
	_ = json.Unmarshal(raw, &e)
	return e.VManage
}

// siteNameFor returns the name of the first (sorted) group routerID is
// a member of, or "(absent)" when it belongs to none, the document's
// own human-readable site grouping (e.g. physical.room 523 on floor 12
// "MXP-F12-R523"), never the router's own raw numeric SiteID extension
// field, which names the same site far less legibly.
func siteNameFor(g *graph.Graph, routerID string) string {
	groups := append([]string(nil), g.MemberOfGroups[routerID]...)
	if len(groups) == 0 {
		return "(absent)"
	}
	sort.Strings(groups)
	grp := g.Groups[groups[0]]
	if grp == nil {
		return "(absent)"
	}
	return labelOr(grp.Name, groups[0])
}

// buildRouters reads every network.router resource under ProviderName
// once, sorted by ID, one RouterInfo per router never assuming
// exactly one the way Cisco NX-OS's single-switch buildSwitch does.
func buildRouters(g *graph.Graph) []RouterInfo {
	var out []RouterInfo
	for _, id := range sortedByType(g, "network.router") {
		r := g.Resources[id]
		if r.Provider.Name != ProviderName {
			continue
		}
		var props routerProperties
		_ = json.Unmarshal(r.Properties, &props)
		ext := decodeRouterExtensions(r.Extensions)
		out = append(out, RouterInfo{
			ID: id, Name: labelOr(r.Name, id), NativeID: stringOrAbsent(r.Provider.NativeID),
			Model: stringOrAbsent(props.Model), Manufacturer: stringOrAbsent(props.Manufacturer),
			Serial: stringOrAbsent(props.SerialNumber), ManagementIP: stringOrAbsent(props.ManagementIP),
			SoftwareVersion: stringOrAbsent(props.Version),
			Status:          stringOrAbsent(r.Status), State: stringOrAbsent(r.State),
			StateDescription: stringOrAbsent(ext.StateDescription),
			Health:           stringOrAbsent(ext.Health), CertificateValidity: stringOrAbsent(ext.CertificateValidity),
			ChassisNumber: stringOrAbsent(ext.ChassisNumber), SiteID: stringOrAbsent(ext.SiteID),
			UpSince: stringOrAbsent(ext.UpSince), LastUpdate: stringOrAbsent(ext.LastUpdate),
			ConnectedManager: ext.ConnectedManager, Site: siteNameFor(g, id),
		})
	}
	return out
}

// PortSummary is the network.router.port census across every router
// this document describes: aggregate counts for the Markdown report,
// plus Rows every port individually, each naming its own owning
// router for the .xlsx companion when NeedsXLSXCompanion is true.
type PortSummary struct {
	Total         int
	ByAdminStatus []KV
	ByStatus      []KV // resource-level status: active/degraded/inactive/...
	Rows          []PortInfo
}

// PortInfo is one network.router.port resource. RouterName is resolved
// from the document's own "contains" connection (source the router,
// target the port), never inferred from naming. PublicIP and PrivateIP
// come from "properties.ip_addresses" (a router.port may carry more
// than one private address; joined for display, never truncated).
type PortInfo struct {
	RouterName            string
	Name                  string
	Status, AdminStatus   string
	OperStatus            string
	SpeedMbps, MTU, VRFID string
	MACAddress            string
	PublicIP              string
	PrivateIP             []string
	Color, NATType        string
}

// portProperties is the subset of a network.router.port resource's
// properties this report reads.
type portProperties struct {
	AdminStatus   string          `json:"admin_status"`
	InterfaceName string          `json:"interface_name"`
	IPAddresses   portIPAddresses `json:"ip_addresses"`
	MACAddress    string          `json:"mac_address"`
	MTU           *int64          `json:"mtu"`
	OperStatus    string          `json:"oper_status"`
	SpeedMbps     *int64          `json:"speed_mbps"`
	VRFID         string          `json:"vrf_id"`
}

type portIPAddresses struct {
	PublicIP  string   `json:"public_ip"`
	PrivateIP []string `json:"private_ip"`
}

// portExtensions is the subset of a network.router.port resource's
// "extensions.osiris.cisco.vmanage" object this report reads: the
// SD-WAN transport color and the NAT type this producer's own vendor
// extension namespace carries, never a generic OSIRIS JSON field.
type portExtensions struct {
	Color   string `json:"color"`
	NATType string `json:"nat_type"`
}

func decodePortExtensions(raw json.RawMessage) portExtensions {
	var e struct {
		VManage portExtensions `json:"osiris.cisco.vmanage"`
	}
	if len(raw) == 0 {
		return e.VManage
	}
	_ = json.Unmarshal(raw, &e)
	return e.VManage
}

// routerNameFor returns the name of the router that "contains" portID
// (the document's own "contains" connection, source the router, target
// the port), or "(absent)" when no such connection resolves never
// inferred from the port's own native-ID prefix.
func routerNameFor(g *graph.Graph, portID string) string {
	for _, idx := range g.InEdges[portID] {
		e := g.Edges[idx]
		if e.Type != "contains" {
			continue
		}
		if r, ok := g.Resources[e.Source]; ok {
			return labelOr(r.Name, e.Source)
		}
	}
	return "(absent)"
}

// buildPortSummary reads every network.router.port resource under
// ProviderName once, sorted by ID, across every router in the
// document.
func buildPortSummary(g *graph.Graph) PortSummary {
	var s PortSummary
	byAdmin := map[string]int{}
	byStatus := map[string]int{}
	for _, id := range sortedByType(g, "network.router.port") {
		r := g.Resources[id]
		if r.Provider.Name != ProviderName {
			continue
		}
		var p portProperties
		_ = json.Unmarshal(r.Properties, &p)
		ext := decodePortExtensions(r.Extensions)
		s.Total++
		if p.AdminStatus != "" {
			byAdmin[p.AdminStatus]++
		}
		if r.Status != "" {
			byStatus[r.Status]++
		}
		name := p.InterfaceName
		if name == "" {
			name = labelOr(r.Name, id)
		}
		s.Rows = append(s.Rows, PortInfo{
			RouterName: routerNameFor(g, id), Name: name,
			Status: stringOrAbsent(r.Status), AdminStatus: stringOrAbsent(p.AdminStatus),
			OperStatus: stringOrAbsent(p.OperStatus),
			SpeedMbps:  int64OrAbsent(p.SpeedMbps), MTU: int64OrAbsent(p.MTU), VRFID: stringOrAbsent(p.VRFID),
			MACAddress: stringOrAbsent(p.MACAddress), PublicIP: stringOrAbsent(p.IPAddresses.PublicIP),
			PrivateIP: p.IPAddresses.PrivateIP,
			Color:     stringOrAbsent(ext.Color), NATType: stringOrAbsent(ext.NATType),
		})
	}
	s.ByAdminStatus = sortedKV(byAdmin)
	s.ByStatus = sortedKV(byStatus)
	return s
}

// SiteInfo is one group this document's routers belong to (this
// producer's own fixtures use "physical.room", but nothing here
// assumes that type every group is read the same way, literally).
type SiteInfo struct {
	ID, Name, Type      string
	Latitude, Longitude string
	Members             int
}

type siteProperties struct {
	GeoLocation struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"geo_location"`
}

// buildSites lists every group in the document, sorted by name then
// ID, with its own geo-location when present and its own member count
// never filtered by group type, since this package does not assume
// every vManage document's own site group carries the same type its
// one reviewed fixture happens to use.
func buildSites(g *graph.Graph) []SiteInfo {
	type keyed struct {
		id string
		v  SiteInfo
	}
	var rows []keyed
	for _, id := range g.GroupIDs {
		grp := g.Groups[id]
		if grp == nil {
			continue
		}
		var p siteProperties
		_ = json.Unmarshal(grp.Properties, &p)
		v := SiteInfo{ID: id, Name: labelOr(grp.Name, id), Type: stringOrAbsent(grp.Type), Members: len(grp.Members)}
		v.Latitude, v.Longitude = "(absent)", "(absent)"
		if p.GeoLocation.Latitude != nil {
			v.Latitude = strconv.FormatFloat(*p.GeoLocation.Latitude, 'f', -1, 64)
		}
		if p.GeoLocation.Longitude != nil {
			v.Longitude = strconv.FormatFloat(*p.GeoLocation.Longitude, 'f', -1, 64)
		}
		rows = append(rows, keyed{id: id, v: v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].v.Name != rows[j].v.Name {
			return rows[i].v.Name < rows[j].v.Name
		}
		return rows[i].id < rows[j].id
	})
	out := make([]SiteInfo, len(rows))
	for i, r := range rows {
		out[i] = r.v
	}
	return out
}

// BuildReportModel builds the report over g. md and inputSHA carry the
// document's own provenance into the Signature section; neither is
// re-derived from g.
func BuildReportModel(g *graph.Graph, md document.Metadata, inputSHA string) ReportModel {
	return ReportModel{
		Signature: buildSignature(md, inputSHA),
		Routers:   buildRouters(g),
		Ports:     buildPortSummary(g),
		Sites:     buildSites(g),
	}
}

// KV is one sorted-order key/count pair for a breakdown table.
type KV struct {
	Key   string
	Count int
}

// sortedByType returns g.ByType[typ], copied and sorted by ID, so
// every builder that reads one resource type gets the same
// deterministic order without repeating the copy-then-sort.
func sortedByType(g *graph.Graph, typ string) []string {
	ids := append([]string(nil), g.ByType[typ]...)
	sort.Strings(ids)
	return ids
}

// labelOr returns name, or id when name is empty every resource's
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
// properties carried no such field distinguished from a genuine 0 by
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
