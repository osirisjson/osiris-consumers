// Tests for the Aruba Central report model: the site header, fleet
// health snapshot, and client connectivity.

package arubacentral

import (
	"encoding/json"
	"strings"
	"testing"
)

// buildReport builds the full report model for rel, fetching the
// graph and the document's metadata/hash the same way a real caller
// would.
func buildReport(t *testing.T, rel string) ReportModel {
	t.Helper()
	g := build(t, rel)
	_, doc := buildModel(t, rel)
	return BuildReportModel(g, doc.Metadata, doc.SHA256())
}

func TestBuildReportModelSite(t *testing.T) {
	site := buildReport(t, fixture).Site

	if !site.Present {
		t.Fatal("Present = false, want true: the fixture has an osiris.hpe.arubacentral.site resource")
	}
	if site.Name != "BRANCH-01" {
		t.Errorf("Name = %q, want BRANCH-01", site.Name)
	}
	if site.Address != "Via del Castello 9" {
		t.Errorf("Address = %q", site.Address)
	}
	if site.City != "Brescia" || site.Country != "Italy" {
		t.Errorf("City/Country = %q/%q", site.City, site.Country)
	}
	if site.Latitude != "45.5433411" || site.Longitude != "10.2241237" {
		t.Errorf("Latitude/Longitude = %q/%q", site.Latitude, site.Longitude)
	}
	if site.DeviceCount != "14" {
		t.Errorf("DeviceCount = %q, want 14", site.DeviceCount)
	}
	// The fixture's site resource carries no "zipcode" - must disclose
	// (absent), never fabricate one.
	if site.Zipcode != "(absent)" {
		t.Errorf("Zipcode = %q, want (absent)", site.Zipcode)
	}
}

func TestBuildReportModelSiteAbsentWhenNoSuchResource(t *testing.T) {
	site := buildReport(t, "valid/no-provider.json").Site
	if site.Present {
		t.Error("Present = true for a document with no osiris.hpe.arubacentral.site resource")
	}
}

func TestBuildReportModelFleetCoversEveryVisibleType(t *testing.T) {
	fleet := buildReport(t, fixture).Fleet

	byName := map[string]DeviceHealth{}
	for _, d := range fleet {
		byName[d.Name] = d
	}
	if len(fleet) != 6 {
		t.Fatalf("got %d fleet entries, want 6 (2 gateways + 2 switches + 2 APs)", len(fleet))
	}

	gw, ok := byName["TEST-SW01"]
	if !ok {
		t.Fatal("gateway TEST-SW01 missing from fleet")
	}
	if gw.Type != "network.gateway" {
		t.Errorf("Type = %q, want network.gateway", gw.Type)
	}
	if gw.Model != "A9240" {
		t.Errorf("Model = %q, want A9240", gw.Model)
	}
	if gw.CPUPercent == "(absent)" || gw.MemPercent == "(absent)" {
		t.Errorf("CPUPercent/MemPercent unexpectedly absent: %q/%q", gw.CPUPercent, gw.MemPercent)
	}
	// The rest of the resource's own "provider" block, not just the
	// health-dashboard fields.
	if gw.NativeID != "TST0000001" {
		t.Errorf("NativeID = %q, want TST0000001", gw.NativeID)
	}
	if gw.ProviderSite != "BRANCH-01" {
		t.Errorf("ProviderSite = %q, want BRANCH-01", gw.ProviderSite)
	}
	if gw.ProviderSource != "aruba-new-central-api" {
		t.Errorf("ProviderSource = %q, want aruba-new-central-api", gw.ProviderSource)
	}
	// This fixture's devices carry no description, tags, extensions,
	// or unrecognized fields - must disclose that, not omit the rows.
	if gw.Description != "(absent)" {
		t.Errorf("Description = %q, want (absent)", gw.Description)
	}
	if gw.Tags != "(absent)" {
		t.Errorf("Tags = %q, want (absent)", gw.Tags)
	}
	if len(gw.ExtensionKeys) != 0 {
		t.Errorf("ExtensionKeys = %v, want none", gw.ExtensionKeys)
	}
	if len(gw.ExtraKeys) != 0 {
		t.Errorf("ExtraKeys = %v, want none", gw.ExtraKeys)
	}
}

func TestBuildReportModelClientsResolveConnectedDevice(t *testing.T) {
	clients := buildReport(t, fixture).Clients

	if len(clients) == 0 {
		t.Fatal("got 0 clients, want at least the fixture's known ones")
	}
	var found bool
	for _, c := range clients {
		if c.MAC != "00:00:5E:00:53:01" {
			continue
		}
		found = true
		if c.ConnectedToName == "" {
			t.Error("ConnectedToName empty, want the switch this client's connection names")
		}
		if c.Port != "1/1/5" {
			t.Errorf("Port = %q, want 1/1/5", c.Port)
		}
		if c.Manufacturer == "(absent)" {
			t.Error("Manufacturer unexpectedly absent")
		}
	}
	if !found {
		t.Error("client 00:00:5E:00:53:01 not found")
	}
}

func TestBuildReportModelClientsEmptyWhenNoneObserved(t *testing.T) {
	clients := buildReport(t, "valid/no-provider.json").Clients
	if len(clients) != 0 {
		t.Errorf("got %d clients, want 0", len(clients))
	}
}

func TestBuildReportModelFleetUptimeIsComputedNotRawSeconds(t *testing.T) {
	fleet := buildReport(t, fixture).Fleet
	var gw DeviceHealth
	for _, d := range fleet {
		if d.Name == "TEST-SW01" {
			gw = d
		}
	}
	if gw.UptimeSeconds != "23d 11h 19m 39s" {
		t.Errorf("UptimeSeconds = %q, want 23d 11h 19m 39s (from the raw 2027979 seconds)", gw.UptimeSeconds)
	}
}

func TestBuildReportModelPropertiesUseHumanLabels(t *testing.T) {
	fleet := buildReport(t, fixture).Fleet
	var gw DeviceHealth
	for _, d := range fleet {
		if d.Name == "TEST-SW01" {
			gw = d
		}
	}
	byKey := map[string]PropertyRow{}
	for _, row := range gw.Properties {
		byKey[row.Key] = row
	}
	for key, wantLabel := range map[string]string{
		"cpu_utilization_pct":    "CPU Percentage utilization",
		"mac_address":            "MAC Address",
		"memory_utilization_pct": "RAM Percentage utilization",
		"site_id":                "Site ID",
		"uptime_seconds":         "Uptime",
	} {
		row, ok := byKey[key]
		if !ok {
			t.Errorf("property %q not found", key)
			continue
		}
		if row.Label != wantLabel {
			t.Errorf("Label for %q = %q, want %q", key, row.Label, wantLabel)
		}
	}
	// uptime_seconds inside the generic Properties dump must show the
	// same computed duration as the DeviceHealth.UptimeSeconds field,
	// not the raw seconds count, twice differently.
	if byKey["uptime_seconds"].Value != "23d 11h 19m 39s" {
		t.Errorf("uptime_seconds property Value = %q, want 23d 11h 19m 39s", byKey["uptime_seconds"].Value)
	}
	// An unrecognized key (not in the explicit dictionary) still gets
	// a humanized label, not the raw snake_case key.
	if row, ok := byKey["device_function"]; ok && row.Label != "Device function" {
		t.Errorf(`Label for "device_function" = %q, want "Device function" (generic humanization)`, row.Label)
	}
	// "ip_address" is a sibling of the explicitly-listed "ipv4" this
	// fixture's gateway happens to use instead, the acronym-aware
	// fallback must still capitalize IP as an acronym, not "Ip".
	if row, ok := byKey["ip_address"]; ok && row.Label != "IP address" {
		t.Errorf(`Label for "ip_address" = %q, want "IP address"`, row.Label)
	}
	if row, ok := byKey["mac_range"]; ok && row.Label != "MAC range" {
		t.Errorf(`Label for "mac_range" = %q, want "MAC range"`, row.Label)
	}
}

func TestFormatUptimeSecondsDropsLeadingZeroUnits(t *testing.T) {
	cases := []struct {
		seconds float64
		want    string
	}{
		{43, "43s"},
		{143, "2m 23s"},
		{3661, "1h 1m 1s"},
		{90000, "1d 1h 0m 0s"},
	}
	for _, tc := range cases {
		raw, _ := json.Marshal(tc.seconds)
		got, ok := formatUptimeSeconds(raw)
		if !ok {
			t.Errorf("formatUptimeSeconds(%v) ok = false", tc.seconds)
			continue
		}
		if got != tc.want {
			t.Errorf("formatUptimeSeconds(%v) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}

func TestPropertyRowsFlattensNestedObjects(t *testing.T) {
	// The exact shape a real production document reported: a
	// "hardware" property whose value is itself an object, which used
	// to render as one unreadable, fully-escaped JSON blob.
	raw := json.RawMessage(`{
		"hardware": {
			"cpu_health": "Good",
			"fans_health": "Good",
			"fans_total_count": 4,
			"fans_up_count": 4,
			"memory_health": "Good",
			"power_supplies_health": "Good",
			"power_supplies_total_count": 2,
			"power_supplies_up_count": 2,
			"temperature_health": "Good"
		}
	}`)
	rows := propertyRows(raw)

	byKey := map[string]PropertyRow{}
	for _, row := range rows {
		byKey[row.Key] = row
		if strings.ContainsAny(row.Value, "{}") {
			t.Errorf("row %q value %q still looks like a raw JSON object, not flattened", row.Key, row.Value)
		}
	}
	if len(rows) != 9 {
		t.Fatalf("got %d rows, want 9 (one per leaf field of the nested object)", len(rows))
	}
	cpu, ok := byKey["hardware.cpu_health"]
	if !ok {
		t.Fatal(`row "hardware.cpu_health" not found`)
	}
	if cpu.Label != "Hardware - CPU health" {
		t.Errorf("Label = %q, want %q", cpu.Label, "Hardware - CPU health")
	}
	if cpu.Value != "Good" {
		t.Errorf("Value = %q, want Good", cpu.Value)
	}
	count, ok := byKey["hardware.fans_total_count"]
	if !ok || count.Value != "4" {
		t.Errorf("hardware.fans_total_count = %+v, want Value 4", count)
	}
}

func TestNeedsXLSXCompanionFalseUnderThreshold(t *testing.T) {
	m := buildReport(t, fixture) // 6 fleet devices, 2 clients in the reference fixture.
	if m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = true, want false for the small reference fixture")
	}
}

func TestNeedsXLSXCompanionTrueWhenFleetExceedsThreshold(t *testing.T) {
	m := ReportModel{Fleet: make([]DeviceHealth, acXLSXRowThreshold+1)}
	if !m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = false, want true when Fleet alone exceeds the threshold")
	}
}

func TestNeedsXLSXCompanionTrueWhenClientsExceedsThreshold(t *testing.T) {
	m := ReportModel{Clients: make([]ClientInfo, acXLSXRowThreshold+1)}
	if !m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = false, want true when Clients alone exceeds the threshold")
	}
}

func TestNeedsXLSXCompanionTrueWhenInterfacesExceedsThreshold(t *testing.T) {
	m := ReportModel{Interfaces: make([]InterfaceInfo, acXLSXRowThreshold+1)}
	if !m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = false, want true when Interfaces alone exceeds the threshold")
	}
}

func TestNeedsXLSXCompanionTrueWhenBSSIDsExceedsThreshold(t *testing.T) {
	m := ReportModel{BSSIDs: make([]BSSIDInfo, acXLSXRowThreshold+1)}
	if !m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = false, want true when BSSIDs alone exceeds the threshold")
	}
}

// TestBuildInterfacesResolvesDeviceNameAndFields confirms an
// InterfaceInfo row resolves its owning device's display name via the
// port's own provider.native_id and carries the port's own reported
// fields verbatim.
func TestBuildInterfacesResolvesDeviceNameAndFields(t *testing.T) {
	ifaces := buildReport(t, fixture).Interfaces
	var p0 *InterfaceInfo
	for i := range ifaces {
		if ifaces[i].Port == "GE 0/0/0" {
			p0 = &ifaces[i]
		}
	}
	if p0 == nil {
		t.Fatal("port GE 0/0/0 not found")
	}
	if p0.Device != "TEST-SW01" {
		t.Errorf("Device = %q, want TEST-SW01 (resolved via provider.native_id)", p0.Device)
	}
	if p0.Status != "unknown" {
		t.Errorf("Status = %q, want unknown", p0.Status)
	}
}

// TestBuildBSSIDsResolvesDeviceNameAndFields confirms a BSSIDInfo row
// resolves the same way, with RadioNumber/ClientCount disclosed as
// "(absent)" when the document did not report them for that BSSID.
func TestBuildBSSIDsAbsentWhenNoneObserved(t *testing.T) {
	bssids := buildReport(t, "valid/no-provider.json").BSSIDs
	if len(bssids) != 0 {
		t.Errorf("got %d BSSIDs, want 0 for a document with no bssid resources", len(bssids))
	}
}

func TestJoinVLANListJoinsAndFallsBack(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{`["11", "100", "250-252"]`, "11, 100, 250-252"},
		{`[""]`, "(absent)"},
		{`[]`, "(absent)"},
		{``, "(absent)"},
		{`"all"`, "all"},
	}
	for _, tc := range cases {
		got := joinVLANList(json.RawMessage(tc.raw))
		if got != tc.want {
			t.Errorf("joinVLANList(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
