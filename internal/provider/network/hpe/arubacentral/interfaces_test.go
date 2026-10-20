// Tests for the interface-detail page model: data-driven port
// collection (one cell per reported network.interface, never a
// hardware-catalog count) and the properties display.

package arubacentral

import (
	"strings"
	"testing"
)

const fixture = "producer/network/hpe/arubacentral/hpe-arubacentral-2026-08-06T16-47-25Z-BRANCH-01.json"

func TestBuildInterfacePageGatewayIsDataDriven(t *testing.T) {
	g := build(t, fixture)
	page := BuildInterfacePage(g, "network.gateway")

	if page.ResourceType != "network.gateway" {
		t.Errorf("ResourceType = %q, want network.gateway", page.ResourceType)
	}
	if len(page.Devices) != 2 {
		t.Fatalf("got %d gateway devices, want 2", len(page.Devices))
	}

	var dev *Device
	for i := range page.Devices {
		if page.Devices[i].ResourceID == "hpe.arubacentral::TST0000001" {
			dev = &page.Devices[i]
		}
	}
	if dev == nil {
		t.Fatal("gateway TST0000001 not found")
	}
	if dev.Name != "TEST-SW01" {
		t.Errorf("device Name = %q, want TEST-SW01", dev.Name)
	}
	// The document reports exactly 4 ports for this gateway (GE
	// 0/0/0..3) not a hardware-model port count.
	if len(dev.Ports) != 4 {
		t.Fatalf("got %d ports, want 4 (data-driven from the document)", len(dev.Ports))
	}

	var p0 *Port
	for i := range dev.Ports {
		if dev.Ports[i].ResourceID == "hpe.arubacentral::TST0000001/port/GE 0/0/0" {
			p0 = &dev.Ports[i]
		}
	}
	if p0 == nil {
		t.Fatal("port GE 0/0/0 not found")
	}
	if p0.Name != "GE 0/0/0" {
		t.Errorf("port Name = %q, want %q", p0.Name, "GE 0/0/0")
	}
	if p0.Status != "unknown" {
		t.Errorf("port Status = %q, want the document's verbatim %q", p0.Status, "unknown")
	}
	for _, want := range []string{
		`"connector_type": "SFP"`,
		`"duplex": "Auto"`,
		`"mac_address": "00:00:5E:00:53:00"`,
		`"vlan": "1"`,
	} {
		if !strings.Contains(p0.PropertiesJSON, want) {
			t.Errorf("PropertiesJSON missing %q; got:\n%s", want, p0.PropertiesJSON)
		}
	}
}

func TestBuildInterfacePageSwitchPortVocabularyDiffersFromGateway(t *testing.T) {
	g := build(t, fixture)
	page := BuildInterfacePage(g, "network.switch")

	if len(page.Devices) != 2 {
		t.Fatalf("got %d switch devices, want 2", len(page.Devices))
	}
	var dev *Device
	for i := range page.Devices {
		if page.Devices[i].ResourceID == "hpe.arubacentral::TST0000002" {
			dev = &page.Devices[i]
		}
	}
	if dev == nil {
		t.Fatal("switch TST0000002 not found")
	}
	// Only the two VSX inter-switch link ports are modeled as
	// resources for this switch in the fixture, not its full
	// physical port count.
	if len(dev.Ports) != 2 {
		t.Fatalf("got %d ports, want 2 (data-driven from the document)", len(dev.Ports))
	}
	// A switch port's properties use a different vocabulary than a
	// gateway port's (e.g. "connector" not "connector_type", plus
	// fields a gateway port never carries), the model must not
	// assume a fixed field set.
	for _, want := range []string{
		`"connector": "SFP"`,
		`"lag": "LAG256"`,
		`"native_vlan": 1`,
	} {
		if !strings.Contains(dev.Ports[0].PropertiesJSON, want) {
			t.Errorf("switch port PropertiesJSON missing %q; got:\n%s", want, dev.Ports[0].PropertiesJSON)
		}
	}
}

func TestBuildInterfacePageAbsentTypeYieldsNoDevices(t *testing.T) {
	g := build(t, "valid/no-provider.json")
	page := BuildInterfacePage(g, "no.such.type")
	if len(page.Devices) != 0 {
		t.Fatalf("got %d devices for an absent type, want 0", len(page.Devices))
	}
}
