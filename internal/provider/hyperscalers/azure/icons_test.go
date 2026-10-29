package azure

import (
	"strings"
	"testing"
)

func TestIconForARMType(t *testing.T) {
	tests := []struct {
		armType string
		want    string
		ok      bool
	}{
		{"Microsoft.Compute/virtualMachines", "compute/Virtual_Machine.svg", true},
		{"microsoft.compute/virtualmachines", "compute/Virtual_Machine.svg", true}, // case-insensitive
		{"Microsoft.Resources/resourceGroups", "general/Resource_Groups.svg", true},
		{"Microsoft.Network/virtualNetworks/subnets", "networking/Subnet.svg", true},
		{"Microsoft.SqlVirtualMachine/SqlVirtualMachines", "databases/Azure_SQL_VM.svg", true},
		{"Microsoft.Compute/virtualMachineScaleSets", "compute/VM_Scale_Sets.svg", true},
		{"Microsoft.ManagedIdentity/userAssignedIdentities", "identity/Entra_Managed_Identities.svg", true},
		{"Microsoft.Something/unknownResource", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := iconForARMType(tt.armType)
		if ok != tt.ok || got != tt.want {
			t.Errorf("iconForARMType(%q) = (%q, %v), want (%q, %v)", tt.armType, got, ok, tt.want, tt.ok)
		}
	}
}

func TestIconForGroupType(t *testing.T) {
	tests := []struct {
		groupType string
		want      string
		ok        bool
	}{
		{"logical.managementgroup", "general/Management_Groups.svg", true},
		{"logical.subscription", "general/Subscriptions.svg", true},
		{"logical.resourcegroup", "", false}, // deliberately unmapped: redundant with the container's own title
		{"container.region", "", false},      // deliberately unmapped: generic, not Azure-specific, and not rendered as a container at all
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := iconForGroupType(tt.groupType)
		if ok != tt.ok || got != tt.want {
			t.Errorf("iconForGroupType(%q) = (%q, %v), want (%q, %v)", tt.groupType, got, ok, tt.want, tt.ok)
		}
	}
}

func TestResourceIconSize(t *testing.T) {
	tests := []struct {
		path string
		w, h int
	}{
		{"compute/Virtual_Machine.svg", 40, 40},
		{"networking/Virtual_Networks.svg", 40, 22}, // real SVG is wide/short; a flat 40x40 distorts it
		{"networking/Subnet.svg", 40, 22},
		{"devops/API_Connections.svg", 40, 27},                    // real SVG is wide/short (h/w 0.67)
		{"devops/Application_Insights.svg", 28, 40},               // real SVG is narrow/tall (h/w 1.43)
		{"management_governance/Automation_Accounts.svg", 40, 40}, // real SVG is square (h/w 1.0)
	}
	for _, tt := range tests {
		w, h := resourceIconSize(tt.path)
		if w != tt.w || h != tt.h {
			t.Errorf("resourceIconSize(%q) = (%d,%d), want (%d,%d)", tt.path, w, h, tt.w, tt.h)
		}
	}
}

func TestContainerIconSize(t *testing.T) {
	tests := []struct {
		path string
		w, h int
	}{
		{"general/Management_Groups.svg", 18, 18},
		{"general/Subscriptions.svg", 13, 18}, // real SVG is narrow/tall; a flat 18x18 distorts it
	}
	for _, tt := range tests {
		w, h := containerIconSize(tt.path)
		if w != tt.w || h != tt.h {
			t.Errorf("containerIconSize(%q) = (%d,%d), want (%d,%d)", tt.path, w, h, tt.w, tt.h)
		}
	}
}

func TestHiddenResourceTypes(t *testing.T) {
	for _, want := range []string{
		"storage.volume", "container.resourcegroup", "compute.vm.snapshot",
		"osiris.azure.monitor.actiongroup", "osiris.azure.monitor.metricalert", "osiris.azure.loganalytics",
		"osiris.azure.logic.workflow", "osiris.azure.streamanalytics", "application.queue", "osiris.azure.monitor.autoscale",
	} {
		if !hiddenResourceTypes[want] {
			t.Errorf("expected %q to be hidden from this page", want)
		}
	}
	if hiddenResourceTypes["compute.vm"] {
		t.Errorf("compute.vm must not be hidden")
	}
}

func TestStencilStyleForOSIRISType(t *testing.T) {
	style, ok := stencilStyleForOSIRISType("osiris.azure.aks.nodepool")
	if !ok {
		t.Fatal("expected osiris.azure.aks.nodepool to have a stencil style")
	}
	if !strings.Contains(style, "shape=mxgraph.kubernetes.icon2;kubernetesLabel=1;prIcon=node") {
		t.Errorf("stencil style %q missing the exact confirmed shape string", style)
	}
	if strings.Contains(style, "image=") {
		t.Errorf("a stencil shape must never also carry an image= reference: %q", style)
	}
	if _, ok := stencilStyleForOSIRISType("compute.vm"); ok {
		t.Error("compute.vm must not have a stencil style")
	}
}

func TestIconForOSIRISType(t *testing.T) {
	tests := []struct {
		osirisType string
		want       string
		ok         bool
	}{
		{"osiris.azure.keyvault", "security/Key_Vaults.svg", true},
		{"compute.cluster", "compute/Kubernetes_Services.svg", true},
		{"network.endpoint", "networking/Private_Endpoint.svg", true},
		{"osiris.azure.routetable", "networking/Route_Tables.svg", true},
		{"osiris.azure.automation.account", "management_governance/Automation_Accounts.svg", true},
		{"osiris.azure.logic.apiconnection", "devops/API_Connections.svg", true},
		{"osiris.azure.applicationinsights", "devops/Application_Insights.svg", true},
		{"osiris.azure.appserviceplan", "app_services/App_Service_Plans.svg", true},
		{"osiris.azure.logic.workflow", "general/Workflow.svg", true},
		{"osiris.azure.asg", "security/Application_Security_Groups.svg", true},
		{"osiris.azure.routeserver", "networking/Virtual_Router.svg", true},
		{"osiris.azure.applicationgateway", "networking/Application_Gateways.svg", true},
		{"compute.vm", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := iconForOSIRISType(tt.osirisType)
		if ok != tt.ok || got != tt.want {
			t.Errorf("iconForOSIRISType(%q) = (%q, %v), want (%q, %v)", tt.osirisType, got, ok, tt.want, tt.ok)
		}
	}
}

func TestIconStyleReferencesPathNeverEmbedsBytes(t *testing.T) {
	style := iconStyle("compute/Virtual_Machine.svg")
	want := "image=img/lib/azure2/compute/Virtual_Machine.svg;"
	if !strings.Contains(style, want) {
		t.Errorf("iconStyle output %q does not reference the expected path %q", style, want)
	}
	if strings.Contains(style, "base64") || strings.Contains(style, "data:image") {
		t.Errorf("iconStyle output %q must never embed icon bytes (ICON_LICENSING policy)", style)
	}
}
