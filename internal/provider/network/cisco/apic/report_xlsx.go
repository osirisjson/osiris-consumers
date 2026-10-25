// This file is the .xlsx companion renderer for the Cisco APIC report
// model: one sheet each for faults, tenants, VRFs, EPGs, bridge
// domains, subnets, ports, and externally discovered neighbours, the
// eight categories NeedsXLSXCompanion (report_model.go) judges against
// the 50-row threshold.
//
// No third-party dependency: internal/render/xlsx is a minimal,
// standard-library-only .xlsx writer with no styling or autofilter
// deliberately out of scope.
package apic

import (
	"context"
	"io"
	"strconv"

	"go.osirisjson.org/consumers/internal/render/xlsx"
)

// XLSXRenderer renders a ReportModel's eight enumerable categories as
// a multi-sheet .xlsx workbook.
//
// The zero value is not usable; construct one with NewXLSXRenderer.
type XLSXRenderer struct {
	model ReportModel
}

// NewXLSXRenderer returns a renderer for model.
func NewXLSXRenderer(model ReportModel) *XLSXRenderer {
	return &XLSXRenderer{model: model}
}

// Render writes the .xlsx workbook to w.
func (r *XLSXRenderer) Render(ctx context.Context, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return xlsx.Write(w, buildXLSXSheets(r.model))
}

func buildXLSXSheets(m ReportModel) []xlsx.Sheet {
	return []xlsx.Sheet{
		faultsSheet(m.Faults),
		tenantsSheet(m.Tenants),
		groupRollupSheet("VRFs", m.VRFs),
		groupMembersSheet("VRF members", m.VRFMembers),
		groupRollupSheet("EPGs", m.EPGs),
		groupMembersSheet("EPG members", m.EPGMembers),
		bridgeDomainsSheet(m.BridgeDomains),
		subnetsSheet(m.Subnets),
		portsSheet(m.Ports.Rows),
		neighboursSheet(m.Neighbours.Rows),
	}
}

func faultsSheet(rows []FaultRecord) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Faults",
		Header: []string{"Node", "Severity", "Cause", "Description", "Domain", "Lifecycle", "Created", "Last transition"},
	}
	for _, f := range rows {
		s.Rows = append(s.Rows, []string{f.Node, f.Severity, f.Cause, f.Description, f.Domain, f.Lifecycle, f.Created, f.LastTransition})
	}
	return s
}

func tenantsSheet(rows []TenantInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Tenants",
		Header: []string{"Name", "VRFs", "EPGs", "Direct members"},
	}
	for _, t := range rows {
		s.Rows = append(s.Rows, []string{t.Name, strconv.Itoa(t.VRFs), strconv.Itoa(t.EPGs), strconv.Itoa(t.DirectMembers)})
	}
	return s
}

func groupRollupSheet(name string, rows []GroupRollup) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   name,
		Header: []string{"Name", "Tenant", "Members"},
	}
	for _, g := range rows {
		s.Rows = append(s.Rows, []string{g.Name, g.Tenant, strconv.Itoa(g.Members)})
	}
	return s
}

func groupMembersSheet(name string, rows []GroupMemberInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   name,
		Header: []string{"Group", "Tenant", "Member"},
	}
	for _, m := range rows {
		s.Rows = append(s.Rows, []string{m.Group, m.Tenant, m.Member})
	}
	return s
}

func bridgeDomainsSheet(rows []BridgeDomainInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Bridge domains",
		Header: []string{"Name", "Tenant", "MAC", "Unicast routing", "ARP flood", "L2 unknown unicast"},
	}
	for _, b := range rows {
		s.Rows = append(s.Rows, []string{b.Name, b.Tenant, b.MAC, b.UnicastRouting, b.ArpFlood, b.L2UnknownUnicast})
	}
	return s
}

func subnetsSheet(rows []SubnetInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Subnets",
		Header: []string{"CIDR", "Tenant", "Gateway IP", "ACI scope", "Preferred"},
	}
	for _, sn := range rows {
		s.Rows = append(s.Rows, []string{sn.CIDR, sn.Tenant, sn.GatewayIP, sn.ACIScope, sn.Preferred})
	}
	return s
}

func portsSheet(rows []PortInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Ports",
		Header: []string{"Node", "Name", "Admin status", "Oper status", "Usage", "Speed", "Port mode", "Layer", "MTU", "Duplex"},
	}
	for _, p := range rows {
		s.Rows = append(s.Rows, []string{p.Node, p.Name, p.AdminStatus, p.OperStatus, p.Usage, p.Speed, p.PortMode, p.Layer, p.Mtu, p.Duplex})
	}
	return s
}

func neighboursSheet(rows []NeighbourInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Neighbours",
		Header: []string{"Chassis ID", "Discovered by", "Connected to node", "Connected to port"},
	}
	for _, n := range rows {
		s.Rows = append(s.Rows, []string{n.ChassisID, n.DiscoveredBy, n.ConnectedToNode, n.ConnectedToPort})
	}
	return s
}
