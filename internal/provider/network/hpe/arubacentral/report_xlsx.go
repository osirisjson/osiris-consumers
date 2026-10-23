// This file is the .xlsx companion renderer for the Aruba Central
// report model: one sheet each for the fleet, clients, interfaces, and
// BSSIDs, the four categories NeedsXLSXCompanion (report_model.go)
// judges against the 50-row threshold. It renders unconditionally when
// asked (deciding *whether* to render it is the caller's job, generally
// gated on NeedsXLSXCompanion); every sheet always gets a full sheet,
// regardless of that category's own size, so the companion file is
// consistent regardless of which category happened to trigger it.
// Interfaces (network.interface) and BSSIDs
// (osiris.hpe.arubacentral.bssid) are xlsx-only:
// neither has a Markdown section, since a document with many access
// points can carry tens of thousands of BSSID resources alone. Device
// properties deliberately has no sheet here either: it stays a
// Markdown-only section, always shown in full, regardless of fleet size
// unlike the other categories, it doesn't reduce cleanly to one
// fixed-column table, so there is no compacting behavior for it either
// (see sectionDeviceProperties).
//
// No third-party dependency: internal/render/xlsx is a minimal,
// standard-library-only .xlsx writer with no styling or autofilter
// deliberately out of scope.
package arubacentral

import (
	"context"
	"io"

	"go.osirisjson.org/consumers/internal/render/xlsx"
)

// XLSXRenderer renders a ReportModel's fleet and client categories as
// a multi-sheet .xlsx workbook.
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
		fleetSheet(m.Fleet),
		clientsSheet(m.Clients),
		interfacesSheet(m.Interfaces),
		bssidsSheet(m.BSSIDs),
	}
}

func fleetSheet(rows []DeviceHealth) xlsx.Sheet {
	s := xlsx.Sheet{
		Name: "Fleet",
		Header: []string{
			"Name", "Type", "Model", "Firmware", "Status", "State",
			"CPU %", "Memory %", "Power (W)", "Temp (C)", "Uptime",
		},
	}
	for _, d := range rows {
		typ := acFleetSectionTitle[d.Type]
		if typ == "" {
			typ = d.Type
		}
		s.Rows = append(s.Rows, []string{
			d.Name, typ, d.Model, d.FirmwareVersion, d.Status, d.State,
			d.CPUPercent, d.MemPercent, d.PowerWatts, d.TempC, d.UptimeSeconds,
		})
	}
	return s
}

func clientsSheet(rows []ClientInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name: "Clients",
		Header: []string{
			"Name", "MAC", "Manufacturer", "OS", "Category", "Connection type", "Function",
			"VLAN ID", "VLAN name", "Connected to", "Port", "Connected at",
		},
	}
	for _, c := range rows {
		s.Rows = append(s.Rows, []string{
			c.Name, c.MAC, c.Manufacturer, c.OS, c.Category, c.ConnectionType, c.Function,
			c.VLANID, c.VLANName, c.ConnectedToName, c.Port, c.ConnectedAt,
		})
	}
	return s
}

func interfacesSheet(rows []InterfaceInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name: "Interfaces",
		Header: []string{
			"Device", "Port", "Description", "Status", "Admin status", "Oper status",
			"Connector", "Duplex", "Speed (Mbps)", "MTU", "Native VLAN", "Allowed VLANs",
			"PoE status", "LAG", "Module",
		},
	}
	for _, p := range rows {
		s.Rows = append(s.Rows, []string{
			p.Device, p.Port, p.Description, p.Status, p.AdminStatus, p.OperStatus,
			p.Connector, p.Duplex, p.SpeedMbps, p.MTU, p.NativeVLAN, p.AllowedVLANs,
			p.PoEStatus, p.LAG, p.Module,
		})
	}
	return s
}

func bssidsSheet(rows []BSSIDInfo) xlsx.Sheet {
	s := xlsx.Sheet{
		Name:   "BSSIDs",
		Header: []string{"Access point", "BSSID", "MAC", "WLAN name", "Radio number", "Client count"},
	}
	for _, b := range rows {
		s.Rows = append(s.Rows, []string{
			b.Device, b.BSSID, b.MACAddress, b.WLANName, b.RadioNumber, b.ClientCount,
		})
	}
	return s
}
