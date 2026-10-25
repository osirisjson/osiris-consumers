// This file is the .xlsx companion renderer for the Cisco NX-OS report
// model: one sheet each for ports, interfaces, port-channels, VLANs,
// VRFs, and CDP/LLDP-discovered neighbours, the six categories
// NeedsXLSXCompanion (report_model.go) judges against the 50-row
// threshold. It renders unconditionally when asked (deciding *whether*
// to render it is the caller's job, generally gated on
// NeedsXLSXCompanion); every one of the six always gets a full sheet,
// regardless of that category's own size, so the companion file is
// consistent regardless of which category happened to trigger it.
//
// No third-party dependency: internal/render/xlsx is a minimal,
// standard-library-only .xlsx writer with no styling or autofilter.
package nxos

import (
	"context"
	"io"
	"strconv"
	"strings"

	"go.osirisjson.org/consumers/internal/render/xlsx"
)

// XLSXRenderer renders a ReportModel's six enumerable categories as a
// multi-sheet .xlsx workbook.
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
		portsSheet(m.Ports.Rows),
		interfacesSheet(m.Interfaces.Rows),
		lagsSheet(m.LAGs),
		vlansSheet(m.VLANs.Rows),
		vrfsSheet(m.VRFs),
		neighboursSheet(m.Neighbours.Rows),
	}
}

func portsSheet(rows []PortInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Ports",
		Header: []string{"Name", "Admin status", "Port mode", "Native VLAN", "Access VLAN", "IP address", "Speed (Mbps)"},
	}
	for _, p := range rows {
		s.Rows = append(s.Rows, []string{p.Name, p.AdminStatus, p.PortMode, p.NativeVLAN, p.AccessVLAN, p.IPAddress, p.SpeedMbps})
	}
	return s
}

func interfacesSheet(rows []InterfaceInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Interfaces",
		Header: []string{"Name", "Admin status", "IP address"},
	}
	for _, i := range rows {
		s.Rows = append(s.Rows, []string{i.Name, i.AdminStatus, i.IPAddress})
	}
	return s
}

func lagsSheet(rows []LAGInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Port-channels",
		Header: []string{"Name", "Admin status", "Protocol", "Port mode", "Native VLAN", "Access VLAN", "Member count", "Members"},
	}
	for _, l := range rows {
		s.Rows = append(s.Rows, []string{l.Name, l.AdminStatus, l.Protocol, l.PortMode, l.NativeVLAN, l.AccessVLAN, l.MemberCount, strings.Join(l.Members, ", ")})
	}
	return s
}

func vlansSheet(rows []VLANInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "VLANs",
		Header: []string{"VLAN ID", "Name", "Admin state"},
	}
	for _, v := range rows {
		s.Rows = append(s.Rows, []string{v.ID, v.Name, v.AdminState})
	}
	return s
}

func vrfsSheet(rows []VRFInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "VRFs",
		Header: []string{"Name", "VRF ID", "Route distinguisher", "State", "Members"},
	}
	for _, v := range rows {
		s.Rows = append(s.Rows, []string{v.Name, v.VRFID, v.RouteDistinguisher, v.State, strconv.Itoa(v.Members)})
	}
	return s
}

func neighboursSheet(rows []NeighbourInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "Neighbours",
		Header: []string{"Remote system", "Remote port", "Remote mgmt addr", "Discovered via", "Local port"},
	}
	for _, n := range rows {
		s.Rows = append(s.Rows, []string{n.RemoteSystem, n.RemotePort, n.RemoteMgmtAddr, n.DiscoveredVia, n.LocalPort})
	}
	return s
}
