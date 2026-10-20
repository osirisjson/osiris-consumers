// This file builds the format-neutral Aruba Central report model: the
// site header, a fleet health snapshot for every gateway, switch, and
// access point the document reports, and client connectivity (real
// when client resources exist, an explicit "none observed" when they
// do not, never silently omitted). It is read-only over the generic
// graph, never re-parsing the document; see report_render.go for the
// Markdown emission and topology.go for the Applies trigger this
// shares.

package arubacentral

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// siteResourceType, clientResourceType, and clientConnectionType are
// the OSIRIS JSON types this report reads. A document without a
// resource of siteResourceType or clientResourceType simply has an
// absent site or zero clients; neither is an error.
const (
	siteResourceType     = "osiris.hpe.arubacentral.site"
	clientResourceType   = "osiris.hpe.arubacentral.client"
	clientConnectionType = "network"
)

// interfaceResourceType and bssidResourceType are two more OSIRIS JSON
// types this report reads, xlsx-only: a
// document's physical switch/gateway ports, and a document's
// broadcast wireless BSSIDs. Neither gets a Markdown section, a
// document with thousands of access points can carry tens of
// thousands of BSSID resources, which is exactly the kind of bulk
// data the .xlsx companion exists for; see report_xlsx.go.
const (
	interfaceResourceType = "network.interface"
	bssidResourceType     = "osiris.hpe.arubacentral.bssid"
)

// ReportModel is the complete, format-neutral Aruba Central report.
type ReportModel struct {
	Signature  SignatureInfo
	Site       SiteInfo
	Fleet      []DeviceHealth
	Clients    []ClientInfo
	Interfaces []InterfaceInfo
	BSSIDs     []BSSIDInfo
}

// acXLSXRowThreshold is the row count, per enumerable category, above
// that value it triggers the .xlsx companion workbook.
const acXLSXRowThreshold = 50

// NeedsXLSXCompanion reports whether any tracked category exceeds
// acXLSXRowThreshold rows, the trigger for producing the .xlsx
// companion workbook. Once true, every tracked category (Fleet,
// Clients, Interfaces, and BSSIDs) gets a full sheet in that workbook
// regardless of its own individual size, one consistent companion file
// rather than a patchwork of which category happened to be large in a
// given document. Device properties is deliberately not a tracked
// category (see report_xlsx.go's doc comment), it stays Markdown-only.
func (m ReportModel) NeedsXLSXCompanion() bool {
	return len(m.Fleet) > acXLSXRowThreshold || len(m.Clients) > acXLSXRowThreshold ||
		len(m.Interfaces) > acXLSXRowThreshold || len(m.BSSIDs) > acXLSXRowThreshold
}

// PropertyRow is one field of a table dump of a resource's (or the
// document's own metadata's) fields, in sorted key order. Key is the
// raw OSIRIS JSON field name; Label is what the render actually shows
// a human label for a well-known key (see propertyDisplayName),
// falling back to a humanized form of Key itself.
type PropertyRow struct {
	Key, Label, Value string
}

// SignatureInfo is the document's own provenance: the exact input hash
// plus every "metadata" field the document carries. Every string is
// "(absent)" when the document didn't carry it, never fabricated.
type SignatureInfo struct {
	InputSHA256                     string
	Timestamp                       string
	GeneratorName, GeneratorVersion string
	GeneratorURL                    string // empty, not "(absent)", when the render should omit the link
	ScopePurpose                    string
	ScopeProviders, ScopeSites      []string
	ScopeClusters                   []string
	ExtraMetadataKeys               []string
}

// SiteInfo is the osiris.hpe.arubacentral.site resource's disclosed
// fields. Present is false when the document carries no such
// resource; every string field is "(absent)" when the document didn't
// carry it, never fabricated.
type SiteInfo struct {
	Present                  bool
	Name                     string
	Address, City, State     string
	Zipcode, Country         string
	TimezoneID, TimezoneName string
	Latitude, Longitude      string
	DeviceCount              string
}

// DeviceHealth is one gateway, switch, or access point resource's
// point-in-time snapshot values, exactly as the document asserts them
// "(absent)" for anything the document didn't carry, never a live
// reading. Properties carries every "properties" field the document
// reports for this device, sorted, generic, no fixed field set.
// NativeID, ProviderSite, and ProviderSource are the rest of the
// resource's own "provider" block; Description, Tags, ExtensionKeys,
// and ExtraKeys disclose everything else the resource carries outside
// "properties" (loss-aware, a key is named even when this package
// does not interpret its value).
type DeviceHealth struct {
	ResourceID, Name, Type string
	Status, State          string
	CPUPercent, MemPercent string
	PowerWatts, TempC      string
	UptimeSeconds          string
	Model, FirmwareVersion string
	NativeID               string
	ProviderSite           string
	ProviderSource         string
	Description            string
	Tags                   string
	ExtensionKeys          []string
	ExtraKeys              []string
	Properties             []PropertyRow
}

// ClientInfo is one osiris.hpe.arubacentral.client resource, with the
// device and port it connects to when the document carries a
// connection asserting that.
type ClientInfo struct {
	ResourceID, Name                   string
	MAC, Manufacturer, OS              string
	Category, ConnectionType, Function string
	VLANID, VLANName, ConnectedAt      string
	ConnectedToName, Port              string
}

// InterfaceInfo is one network.interface resource: a physical port on
// a gateway, switch, or access point. Device is the owning resource's
// display name, resolved via the port's own provider.native_id against
// deviceNameIndex, the document does not need a "contains" connection
// for this, since a port's provider block already names its own
// device. Every string is "(absent)" when the document didn't carry
// that field, never fabricated; AllowedVLANs joins a reported VLAN
// list, "(absent)" when the document carried none.
type InterfaceInfo struct {
	Device, Port, Description string
	Status                    string
	AdminStatus, OperStatus   string
	Connector, Duplex         string
	SpeedMbps, MTU            string
	NativeVLAN, AllowedVLANs  string
	PoEStatus, LAG, Module    string
}

// BSSIDInfo is one osiris.hpe.arubacentral.bssid resource: one
// broadcast BSSID of one radio on one access point. Device is the
// owning access point's display name, resolved the same way as
// InterfaceInfo.Device. RadioNumber and ClientCount are "(absent)"
// when the document didn't report them for this BSSID, not every
// BSSID resource in a real document carries both.
type BSSIDInfo struct {
	Device, BSSID, MACAddress string
	WLANName                  string
	RadioNumber, ClientCount  string
}

// BuildReportModel builds the report over g. md and inputSHA carry the
// document's own provenance into the Signature section; neither is
// re-derived from g (the graph never keeps a copy of metadata or the
// input hash).
func BuildReportModel(g *graph.Graph, md document.Metadata, inputSHA string) ReportModel {
	names := deviceNameIndex(g)
	return ReportModel{
		Signature:  buildSignature(md, inputSHA),
		Site:       buildSiteInfo(g),
		Fleet:      buildFleet(g),
		Clients:    buildClients(g),
		Interfaces: buildInterfaces(g, names),
		BSSIDs:     buildBSSIDs(g, names),
	}
}

// deviceNameIndex maps every gateway, switch, and access point
// resource's own provider.native_id to its display name, so a
// dependent resource (a port, a BSSID) that only carries the device's
// native_id in its own provider block can still show a human name.
func deviceNameIndex(g *graph.Graph) map[string]string {
	names := make(map[string]string)
	for _, t := range acColumnOrder {
		for _, id := range g.ByType[t] {
			res := g.Resources[id]
			if res.Provider.NativeID != "" {
				names[res.Provider.NativeID] = res.Name
			}
		}
	}
	return names
}

// deviceNameFor resolves nativeID to its display name via names,
// falling back to the raw native_id when the document carries no
// matching device resource, and to "(absent)" when there is no
// native_id at all.
func deviceNameFor(names map[string]string, nativeID string) string {
	if nativeID == "" {
		return "(absent)"
	}
	if name, ok := names[nativeID]; ok && name != "" {
		return name
	}
	return nativeID
}

func buildInterfaces(g *graph.Graph, names map[string]string) []InterfaceInfo {
	var out []InterfaceInfo
	for _, id := range g.ByType[interfaceResourceType] {
		res := g.Resources[id]
		props := decodeProps(res.Properties)
		out = append(out, InterfaceInfo{
			Device:       deviceNameFor(names, res.Provider.NativeID),
			Port:         res.Name,
			Description:  stringOrAbsent(res.Description),
			Status:       stringOrAbsent(res.Status),
			AdminStatus:  propString(props, "admin_status"),
			OperStatus:   propString(props, "oper_status"),
			Connector:    propString(props, "connector"),
			Duplex:       propString(props, "duplex"),
			SpeedMbps:    propString(props, "speed_mbps"),
			MTU:          propString(props, "mtu"),
			NativeVLAN:   propString(props, "native_vlan"),
			AllowedVLANs: joinVLANList(props["allowed_vlans"]),
			PoEStatus:    propString(props, "poe_status"),
			LAG:          propString(props, "lag"),
			Module:       propString(props, "module"),
		})
	}
	return out
}

func buildBSSIDs(g *graph.Graph, names map[string]string) []BSSIDInfo {
	var out []BSSIDInfo
	for _, id := range g.ByType[bssidResourceType] {
		res := g.Resources[id]
		props := decodeProps(res.Properties)
		out = append(out, BSSIDInfo{
			Device:      deviceNameFor(names, res.Provider.NativeID),
			BSSID:       res.Name,
			MACAddress:  propString(props, "mac_address"),
			WLANName:    propString(props, "wlan_name"),
			RadioNumber: propString(props, "radio_number"),
			ClientCount: propString(props, "client_count"),
		})
	}
	return out
}

// joinVLANList renders a reported VLAN-list property (a JSON array of
// strings) as a comma-joined string, or "(absent)" for an empty or
// missing list. A malformed or non-array value falls back to its raw
// scalar text rather than inventing structure.
func joinVLANList(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "(absent)"
	}
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return scalarOrRaw(raw)
	}
	var kept []string
	for _, v := range items {
		if v != "" {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		return "(absent)"
	}
	return strings.Join(kept, ", ")
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
	scope := decodeScope(md.Scope)
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

func buildSiteInfo(g *graph.Graph) SiteInfo {
	ids := g.ByType[siteResourceType]
	if len(ids) == 0 {
		return SiteInfo{}
	}
	res := g.Resources[ids[0]]
	props := decodeProps(res.Properties)
	return SiteInfo{
		Present:      true,
		Name:         res.Name,
		Address:      propString(props, "address"),
		City:         propString(props, "city"),
		State:        propString(props, "state"),
		Zipcode:      propString(props, "zipcode"),
		Country:      propString(props, "country"),
		TimezoneID:   propString(props, "timezone_id"),
		TimezoneName: propString(props, "timezone_name"),
		Latitude:     propString(props, "latitude"),
		Longitude:    propString(props, "longitude"),
		DeviceCount:  propString(props, "device_count"),
	}
}

func buildFleet(g *graph.Graph) []DeviceHealth {
	var out []DeviceHealth
	for _, t := range acColumnOrder {
		for _, id := range g.ByType[t] {
			res := g.Resources[id]
			props := decodeProps(res.Properties)
			out = append(out, DeviceHealth{
				ResourceID:      id,
				Name:            res.Name,
				Type:            t,
				Status:          stringOrAbsent(res.Status),
				State:           stringOrAbsent(res.State),
				CPUPercent:      propString(props, "cpu_utilization_pct"),
				MemPercent:      propString(props, "memory_utilization_pct"),
				PowerWatts:      propString(props, "power_consumption_watts"),
				TempC:           propString(props, "system_temperature"),
				UptimeSeconds:   propertyDisplayValue("uptime_seconds", props["uptime_seconds"]),
				Model:           stringOrAbsent(res.Provider.Type),
				FirmwareVersion: stringOrAbsent(res.Provider.Version),
				NativeID:        stringOrAbsent(res.Provider.NativeID),
				ProviderSite:    stringOrAbsent(res.Provider.Site),
				ProviderSource:  stringOrAbsent(res.Provider.Source),
				Description:     stringOrAbsent(res.Description),
				Tags:            joinTags(res.Tags),
				ExtensionKeys:   sortedRawKeys(res.Extensions),
				ExtraKeys:       sortedExtraKeys(res.Extra),
				Properties:      propertyRows(res.Properties),
			})
		}
	}
	return out
}

func buildClients(g *graph.Graph) []ClientInfo {
	var out []ClientInfo
	for _, id := range g.ByType[clientResourceType] {
		res := g.Resources[id]
		props := decodeProps(res.Properties)
		c := ClientInfo{
			ResourceID:     id,
			Name:           res.Name,
			MAC:            propString(props, "mac_address"),
			Manufacturer:   propString(props, "client_manufacturer"),
			OS:             propString(props, "client_operating_system"),
			Category:       propString(props, "client_category"),
			ConnectionType: propString(props, "client_connection_type"),
			Function:       propString(props, "client_function"),
			VLANID:         propString(props, "vlan_id"),
			VLANName:       propString(props, "vlan_name"),
			ConnectedAt:    propString(props, "connected_at"),
		}
		// The document's own connection says what this client plugs
		// into, when it carries one (type "network", source = this
		// client) never inferred from the client's own fields.
		for _, idx := range g.OutEdges[id] {
			e := g.Edges[idx]
			if e.Type != clientConnectionType {
				continue
			}
			if target, ok := g.Resources[e.Target]; ok {
				c.ConnectedToName = target.Name
			}
			if conn, ok := g.Connections[e.ConnectionID]; ok {
				c.Port = propString(decodeProps(conn.Properties), "port")
			}
			break
		}
		out = append(out, c)
	}
	return out
}

// decodeProps decodes a resource or connection's raw "properties"
// object best-effort; a malformed or absent value yields nil, never an
// error (the receipt already gates malformed input at the document
// level).
func decodeProps(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(raw, &obj)
	return obj
}

// joinTags renders a resource's "tags" map as "key=value" pairs,
// sorted by key, or "(absent)" when there are none.
func joinTags(tags map[string]string) string {
	if len(tags) == 0 {
		return "(absent)"
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := keys[0] + "=" + tags[keys[0]]
	for _, k := range keys[1:] {
		out += ", " + k + "=" + tags[k]
	}
	return out
}

// sortedRawKeys lists the member names of a raw JSON object, sorted
// used to disclose that a resource carries an "extensions" object
// without asserting any understanding of its structure.
func sortedRawKeys(raw json.RawMessage) []string {
	props := decodeProps(raw)
	if len(props) == 0 {
		return nil
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedExtraKeys lists the names of unrecognized top-level resource
// fields, sorted, loss-aware disclosure, same idea as sortedRawKeys.
func sortedExtraKeys(extra map[string]json.RawMessage) []string {
	if len(extra) == 0 {
		return nil
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// propertyRows decodes raw into sorted, human-labeled rows for a
// table dump. A non-object or malformed value yields a single "value"
// row holding the raw bytes, rather than inventing structure.
func propertyRows(raw json.RawMessage) []PropertyRow {
	props := decodeProps(raw)
	if props == nil {
		if len(raw) > 0 {
			return []PropertyRow{{Key: "value", Label: "Value", Value: string(raw)}}
		}
		return nil
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var rows []PropertyRow
	for _, k := range keys {
		rows = append(rows, flattenProperty(k, displayNameFor(k), props[k])...)
	}
	return rows
}

// flattenProperty expands one property's value into one or more rows.
// A scalar, an array, or a malformed value still stays exactly one row
// only a genuine nested object is expanded.
func flattenProperty(key, label string, raw json.RawMessage) []PropertyRow {
	nested := decodeProps(raw)
	if nested == nil {
		return []PropertyRow{{Key: key, Label: label, Value: propertyDisplayValue(key, raw)}}
	}
	subKeys := make([]string, 0, len(nested))
	for k := range nested {
		subKeys = append(subKeys, k)
	}
	sort.Strings(subKeys)
	var rows []PropertyRow
	for _, sk := range subKeys {
		rows = append(rows, flattenProperty(key+"."+sk, label+" - "+displayNameFor(sk), nested[sk])...)
	}
	return rows
}

// propertyDisplayName maps a well-known "properties" key to a human
// label. A key not listed here falls back to humanizeKey, so a future
// or unrecognized field still reads better than raw snake_case without
// needing a dictionary entry for every possible key.
var propertyDisplayName = map[string]string{
	"cpu_utilization_pct":     "CPU Percentage utilization",
	"deployment":              "Deployment",
	"ipv4":                    "IPv4 Address",
	"mac_address":             "MAC Address",
	"memory_utilization_pct":  "RAM Percentage utilization",
	"power_consumption_watts": "PWR Consumption watt",
	"public_ip":               "Public IP address",
	"site_id":                 "Site ID",
	"switch_role":             "Switch role",
	"switch_type":             "Switch class",
	"uptime_seconds":          "Uptime",
}

// displayNameFor returns key's human label.
func displayNameFor(key string) string {
	if name, ok := propertyDisplayName[key]; ok {
		return name
	}
	return humanizeKey(key)
}

// humanizeKey turns "some_field_name" into "Some field name": replace
// underscores with spaces, capitalize only the first letter (matching
// the explicit labels above, e.g. "deployment" -> "Deployment").
// knownAcronyms upper-cases whole underscore-separated words that are
// networking acronyms, wherever they occur in a key, so a sibling key
// this package has no explicit label for (e.g. "ip_address" alongside
// the listed "ipv4", or "mac_range" alongside "mac_address") still
// reads as "IP address" / "MAC range", not "Ip address" / "Mac range".
var knownAcronyms = map[string]string{
	"mac": "MAC", "ip": "IP", "id": "ID", "cpu": "CPU", "ram": "RAM",
	"pwr": "PWR", "url": "URL", "os": "OS", "vlan": "VLAN", "lag": "LAG",
	"mtu": "MTU", "isl": "ISL",
}

func humanizeKey(key string) string {
	words := strings.Split(key, "_")
	for i, w := range words {
		if up, ok := knownAcronyms[strings.ToLower(w)]; ok {
			words[i] = up
		} else if i == 0 && w != "" {
			r := []rune(w)
			r[0] = unicode.ToUpper(r[0])
			words[i] = string(r)
		}
	}
	return strings.Join(words, " ")
}

// propertyDisplayValue returns key's display value: "(absent)" when
// raw is empty, formatUptimeSeconds's rendering for "uptime_seconds"
// when raw parses as a number, and the plain scalar-or-raw text
// otherwise.
func propertyDisplayValue(key string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return "(absent)"
	}
	if key == "uptime_seconds" {
		if s, ok := formatUptimeSeconds(raw); ok {
			return s
		}
	}
	return scalarOrRaw(raw)
}

// formatUptimeSeconds renders a seconds count as "<n>d <n>h <n>m <n>s"
// (dropping leading zero units), so a real value like 6229243 reads
// as "72d 2h 20m 43s" instead of a bare number. false when raw is not
// a non-negative number, so the caller falls back to the raw value
// rather than inventing a duration.
func formatUptimeSeconds(raw json.RawMessage) (string, bool) {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil || f < 0 {
		return "", false
	}
	total := int64(f)
	days := total / 86400
	rem := total % 86400
	hh := rem / 3600
	rem %= 3600
	mm := rem / 60
	ss := rem % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm %ds", days, hh, mm, ss), true
	case hh > 0:
		return fmt.Sprintf("%dh %dm %ds", hh, mm, ss), true
	case mm > 0:
		return fmt.Sprintf("%dm %ds", mm, ss), true
	default:
		return fmt.Sprintf("%ds", ss), true
	}
}

// scalarOrRaw returns raw's display text: the plain string when raw
// is a JSON string, or raw's literal bytes for any other JSON value
// (number, bool, array, object), never invents structure.
func scalarOrRaw(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// propString returns key's value as display text, or "(absent)" when
// the document did not carry that key, evidence-state, never
// fabricated. Both a quoted string and a bare JSON scalar (number,
// bool) are handled, since Aruba Central properties mix both.
func propString(props map[string]json.RawMessage, key string) string {
	raw, ok := props[key]
	if !ok || len(raw) == 0 {
		return "(absent)"
	}
	return scalarOrRaw(raw)
}
