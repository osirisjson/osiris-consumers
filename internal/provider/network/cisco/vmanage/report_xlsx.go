// This file is the .xlsx companion renderer for the Cisco vManage
// report model: one sheet each for routers and router.ports, the two
// categories NeedsXLSXCompanion (report_model.go) judges against the
// 50-row threshold. It renders unconditionally when asked (deciding
// whether to render it is the caller's job, generally gated on
// NeedsXLSXCompanion); both sheets always get full rows, regardless of
// which category triggered the companion file.
//
// No third-party dependency: internal/render/xlsx is a minimal,
// standard-library-only .xlsx writer with no styling or autofilter.
package vmanage

import (
	"context"
	"io"
	"strings"

	"go.osirisjson.org/consumers/internal/render/xlsx"
)

// XLSXRenderer renders a ReportModel's two enumerable categories as a
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
		routersSheet(m.Routers),
		portsSheet(m.Ports.Rows),
	}
}

func routersSheet(rows []RouterInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name: "Routers",
		Header: []string{
			"Name", "Site", "Model", "Manufacturer", "Serial", "Management IP", "Software version",
			"Status", "State", "State description", "Health", "Certificate validity", "Chassis number",
			"Site ID", "Connected manager", "Up since", "Last update",
		},
	}
	for _, r := range rows {
		s.Rows = append(s.Rows, []string{
			r.Name, r.Site, r.Model, r.Manufacturer, r.Serial, r.ManagementIP, r.SoftwareVersion,
			r.Status, r.State, r.StateDescription, r.Health, r.CertificateValidity, r.ChassisNumber,
			r.SiteID, strings.Join(r.ConnectedManager, ", "), r.UpSince, r.LastUpdate,
		})
	}
	return s
}

func portsSheet(rows []PortInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name: "Ports",
		Header: []string{
			"Router", "Name", "Status", "Admin status", "Oper status", "Speed (Mbps)", "MTU",
			"VRF ID", "MAC address", "Public IP", "Private IP", "Color", "NAT type",
		},
	}
	for _, p := range rows {
		s.Rows = append(s.Rows, []string{
			p.RouterName, p.Name, p.Status, p.AdminStatus, p.OperStatus, p.SpeedMbps, p.MTU,
			p.VRFID, p.MACAddress, p.PublicIP, strings.Join(p.PrivateIP, ", "), p.Color, p.NATType,
		})
	}
	return s
}
