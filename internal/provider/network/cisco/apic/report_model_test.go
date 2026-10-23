// Tests for the Cisco APIC report model.

package apic

import (
	"strings"
	"testing"
)

func TestBuildReportModelFabricSummaryMatchesFixture(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	f := m.Fabric

	cases := []struct {
		name string
		got  int
		want int
	}{
		{"Pods", f.Pods, 2},
		{"Controllers", f.Controllers, 3},
		{"Spines", f.Spines, 4},
		{"Leafs", f.Leafs, 22},
		{"Neighbours", f.Neighbours, 15},
		{"Tenants", f.Tenants, 6},
		{"VRFs", f.VRFs, 3},
		{"EPGs", f.EPGs, 12},
		{"BridgeDomains", f.BridgeDomains, 20},
		{"Subnets", f.Subnets, 14},
		{"L3Outs", f.L3Outs, 7},
		{"Ports", f.Ports, 1056},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("Fabric.%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestBuildReportModelNodesOrdering(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())

	if want := 3 + 4 + 34; len(m.Nodes) != want {
		t.Fatalf("len(Nodes) = %d, want %d (controllers + spines + leafs)", len(m.Nodes), want)
	}
	for i := 0; i < 3; i++ {
		if m.Nodes[i].Band != bandController {
			t.Errorf("Nodes[%d].Band = %v, want bandController", i, m.Nodes[i].Band)
		}
	}
	// Every controller sorts before every spine/leaf, and every pod-1
	// node sorts before every pod-2 node within its own band grouping
	// (see buildNodes: controllers, then each pod's spines then leafs,
	// in ascending pod order).
	seenPods := map[string]bool{}
	lastPod := ""
	for _, n := range m.Nodes[3:] {
		if n.Pod != lastPod && seenPods[n.Pod] {
			t.Fatalf("pod %q reappeared out of order after %q", n.Pod, lastPod)
		}
		seenPods[n.Pod] = true
		lastPod = n.Pod
	}
}

func TestBuildTenantsRollupSumsMatchFabricSummary(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())

	vrfs, epgs := 0, 0
	for _, tn := range m.Tenants {
		vrfs += tn.VRFs
		epgs += tn.EPGs
	}
	if vrfs != m.Fabric.VRFs {
		t.Errorf("sum of Tenants[].VRFs = %d, want Fabric.VRFs = %d", vrfs, m.Fabric.VRFs)
	}
	if epgs != m.Fabric.EPGs {
		t.Errorf("sum of Tenants[].EPGs = %d, want Fabric.EPGs = %d", epgs, m.Fabric.EPGs)
	}
}

func TestBuildFaultsMatchesFixtureTotalAndSeverityOrder(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())

	// 392 faults across the role-bearing switches plus 3 warning-
	// severity faults on the controllers: 327 critical, 13 major, 24
	// minor, 31 warning (see the reference fixture's own
	// extensions.osiris.cisco.faults entries).
	if want := 395; len(m.Faults) != want {
		t.Fatalf("len(Faults) = %d, want %d", len(m.Faults), want)
	}
	counts := map[string]int{}
	for _, f := range m.Faults {
		counts[f.Severity]++
	}
	wantCounts := map[string]int{"critical": 327, "major": 13, "minor": 24, "warning": 31}
	for sev, want := range wantCounts {
		if counts[sev] != want {
			t.Errorf("severity %q count = %d, want %d", sev, counts[sev], want)
		}
	}

	lastRank := -1
	for i, f := range m.Faults {
		if r := severityRank(f.Severity); r < lastRank {
			t.Fatalf("Faults[%d] severity %q sorts before a higher-ranked fault - not critical-first", i, f.Severity)
		} else {
			lastRank = r
		}
	}
}

func TestBuildReportModelEnumerableListsMatchFixtureCounts(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())

	cases := []struct {
		name string
		got  int
		want int
	}{
		{"VRFs", len(m.VRFs), 3},
		{"EPGs", len(m.EPGs), 12},
		{"BridgeDomains", len(m.BridgeDomains), 20},
		{"Subnets", len(m.Subnets), 14},
		{"Ports.Rows", len(m.Ports.Rows), 1056},
		{"Neighbours.Rows", len(m.Neighbours.Rows), 15},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("len(%s) = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestNeedsXLSXCompanionTriggersOnTheReferenceFixture(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	if !m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = false, want true for the reference fixture")
	}
}

func TestNeedsXLSXCompanionFalseUnderThreshold(t *testing.T) {
	m := ReportModel{
		Faults:  make([]FaultRecord, 10),
		Tenants: make([]TenantInfo, 10),
		VRFs:    make([]GroupRollup, 10),
		EPGs:    make([]GroupRollup, 10),
	}
	m.BridgeDomains = make([]BridgeDomainInfo, 10)
	m.Subnets = make([]SubnetInfo, 10)
	m.Ports.Rows = make([]PortInfo, 10)
	m.Neighbours.Rows = make([]NeighbourInfo, 10)
	if m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = true, want false when every category is under the threshold")
	}
}

func TestNeedsXLSXCompanionTrueWhenAnyCategoryExceedsThreshold(t *testing.T) {
	m := ReportModel{Faults: make([]FaultRecord, apicXLSXRowThreshold+1)}
	if !m.NeedsXLSXCompanion() {
		t.Error("NeedsXLSXCompanion() = false, want true when Faults alone exceeds the threshold")
	}
}

func TestBuildGroupMembersMatchesFixtureAndStripsProviderPrefix(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())

	if len(m.VRFMembers) != 26 {
		t.Errorf("len(VRFMembers) = %d, want 26", len(m.VRFMembers))
	}
	if len(m.EPGMembers) != 12 {
		t.Errorf("len(EPGMembers) = %d, want 12", len(m.EPGMembers))
	}
	for _, vm := range m.VRFMembers {
		if strings.Contains(vm.Member, "::") {
			t.Errorf("VRF member %q still carries its provider prefix", vm.Member)
		}
	}
	for _, em := range m.EPGMembers {
		if strings.Contains(em.Member, "::") {
			t.Errorf("EPG member %q still carries its provider prefix", em.Member)
		}
	}
}

func TestStripProviderPrefix(t *testing.T) {
	cases := []struct{ id, want string }{
		{"cisco.apic::uni/tn-infra/BD-ave-ctrl", "uni/tn-infra/BD-ave-ctrl"},
		{"no-prefix-here", "no-prefix-here"},
	}
	for _, tc := range cases {
		if got := stripProviderPrefix(tc.id); got != tc.want {
			t.Errorf("stripProviderPrefix(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestBuildGroupRollupsResolveOwningTenant(t *testing.T) {
	doc, g := loadDoc(t, fixture)
	m := BuildReportModel(g, doc.Metadata, doc.SHA256())
	for _, vrf := range m.VRFs {
		if vrf.Tenant == "(absent)" || vrf.Tenant == "" {
			t.Errorf("VRF %q has no resolved tenant", vrf.Name)
		}
	}
	for _, epg := range m.EPGs {
		if epg.Tenant == "(absent)" || epg.Tenant == "" {
			t.Errorf("EPG %q has no resolved tenant", epg.Name)
		}
	}
}

func TestFaultCountsTotal(t *testing.T) {
	f := FaultCounts{Critical: 1, Major: 2, Minor: 3, Warning: 4}
	if got := f.Total(); got != 10 {
		t.Errorf("Total() = %d, want 10", got)
	}
}

func TestPodLabel(t *testing.T) {
	cases := []struct{ pod, want string }{
		{"pod-1", "1"},
		{"pod-12", "12"},
		{"", "(unknown)"},
	}
	for _, tc := range cases {
		if got := podLabel(tc.pod); got != tc.want {
			t.Errorf("podLabel(%q) = %q, want %q", tc.pod, got, tc.want)
		}
	}
}
