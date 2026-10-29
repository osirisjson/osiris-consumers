// This file maps a resource's ARM native type (provider.type) to a
// draw.io Azure icon, referenced by path from draw.io's own bundled
// "azure2" clipart library rather than embedded (ICON_LICENSING policy
// carried forward from the alpha prototype: Microsoft's Azure
// Architecture Icons forbid redistribution in a third-party icon
// library). A drawio.mxfile that references
// "img/lib/azure2/<category>/<file>.svg" resolves correctly against
// any draw.io Desktop or diagrams.net install; this package never
// copies the SVG bytes into the repository.
//
// Extend this table only against a real or official Microsoft
// example document, never by guessing a category or file name.
//
// Icon geometry is sized from each icon's own real SVG intrinsic
// width/height (fetched from the same jgraph/drawio source, never
// guessed), scaled so it never stretches: draw.io's shape=image with
// imageAspect=0 stretches the icon to fill whatever cell geometry it is
// given, so a cell whose aspect ratio does not match the icon's own
// distorts it. Every other mapped icon here renders close enough to
// square at 40x40 / 18x18 that the user's own reviewed mockup kept it
// square rather than applying a continuous per-icon aspect formula;
// this table matches that reviewed decision exactly rather than a
// computed approximation.
package azure

// azureIconBase is the draw.io bundled-library root every icon path is
// relative to.
const azureIconBase = "img/lib/azure2/"

// armTypeIcon maps an ARM native resource type (resource.provider.type,
// e.g. "Microsoft.Compute/virtualMachines") to its azure2 icon path,
// relative to azureIconBase. Lookup is case-insensitive (iconForARMType
// lowercases both sides) because ARM type casing has drifted across
// Microsoft Azure API versions for the same resource provider.
var armTypeIcon = map[string]string{
	"microsoft.resources/resourcegroups":               "general/Resource_Groups.svg",
	"microsoft.storage/storageaccounts":                "storage/Storage_Accounts.svg",
	"microsoft.compute/virtualmachines":                "compute/Virtual_Machine.svg",
	"microsoft.compute/disks":                          "compute/Disks.svg",
	"microsoft.compute/availabilitysets":               "compute/Availability_Sets.svg",
	"microsoft.hybridcompute/machines":                 "management_governance/Arc_Machines.svg",
	"microsoft.containerregistry/registries":           "containers/Container_Registries.svg",
	"microsoft.network/networkinterfaces":              "networking/Network_Interfaces.svg",
	"microsoft.network/virtualnetworks":                "networking/Virtual_Networks.svg",
	"microsoft.network/virtualnetworks/subnets":        "networking/Subnet.svg",
	"microsoft.network/networksecuritygroups":          "networking/Network_Security_Groups.svg",
	"microsoft.network/publicipaddresses":              "networking/Public_IP_Addresses.svg",
	"microsoft.network/loadbalancers":                  "networking/Load_Balancers.svg",
	"microsoft.network/bastionhosts":                   "networking/Bastions.svg",
	"microsoft.network/trafficmanagerprofiles":         "networking/Traffic_Manager_Profiles.svg",
	"microsoft.network/expressroutecircuits":           "networking/ExpressRoute_Circuits.svg",
	"microsoft.network/virtualnetworkgateways":         "networking/Virtual_Network_Gateways.svg",
	"microsoft.network/connections":                    "networking/Connections.svg",
	"microsoft.network/publicipprefixes":               "networking/Public_IP_Prefixes.svg",
	"microsoft.sqlvirtualmachine/sqlvirtualmachines":   "databases/Azure_SQL_VM.svg",
	"microsoft.compute/virtualmachinescalesets":        "compute/VM_Scale_Sets.svg",
	"microsoft.managedidentity/userassignedidentities": "identity/Entra_Managed_Identities.svg",
	"microsoft.insights/datacollectionendpoints":       "other/Data_Collection_Rules.svg",
	"microsoft.dataprotection/backupvaults":            "other/Backup_Vault.svg",
	"microsoft.recoveryservices/vaults":                "management_governance/Recovery_Services_Vaults.svg",
}

// osirisTypeIcon maps an OSIRIS JSON resource type to an azure2 icon
// path used when no ARM-type-specific entry in armTypeIcon applies: a
// single OSIRIS JSON type can cover more than one ARM shape (a
// "compute.cluster" resource is an AKS cluster in general, but
// a "Microsoft.Compute/virtualMachineScaleSets" one is really a VM
// scale set, armTypeIcon's own specific entry above wins for that one
// ARM type; every other compute.cluster resource falls back to here).
// "osiris.azure.keyvault" for example has no ARM-type qualifier, so
// it is keyed at the OSIRIS JSON type level directly.
var osirisTypeIcon = map[string]string{
	"application.database":                    "databases/SQL_Database.svg",
	"compute.cluster":                         "compute/Kubernetes_Services.svg",
	"network.endpoint":                        "networking/Private_Endpoint.svg",
	"osiris.azure.keyvault":                   "security/Key_Vaults.svg",
	"osiris.azure.routetable":                 "networking/Route_Tables.svg",
	"osiris.azure.automation.account":         "management_governance/Automation_Accounts.svg",
	"osiris.azure.logic.apiconnection":        "devops/API_Connections.svg",
	"osiris.azure.applicationinsights":        "devops/Application_Insights.svg",
	"osiris.azure.appserviceplan":             "app_services/App_Service_Plans.svg",
	"osiris.azure.logic.workflow":             "general/Workflow.svg",
	"osiris.azure.asg":                        "security/Application_Security_Groups.svg",
	"osiris.azure.routeserver":                "networking/Virtual_Router.svg",
	"osiris.azure.applicationgateway":         "networking/Application_Gateways.svg",
	"osiris.azure.dns.privatezone":            "networking/DNS_Private_Resolver.svg",
	"osiris.azure.sqlserver":                  "databases/SQL_Server.svg",
	"osiris.azure.sql.elasticpool":            "databases/SQL_Elastic_Pools.svg",
	"osiris.azure.datafactory":                "storage/Azure_Stack_Edge.svg",
	"osiris.azure.synapse.workspace":          "databases/Azure_Synapse_Analytics.svg",
	"osiris.azure.monitor.datacollectionrule": "management_governance/Monitor.svg",
	"osiris.azure.sqlmi":                      "databases/SQL_Managed_Instance.svg",
	"osiris.azure.communicationservice":       "other/Azure_Communication_Services.svg",
}

// iconForOSIRISType returns the azure2 icon path (relative to
// azureIconBase) for osirisType, and whether one was found.
func iconForOSIRISType(osirisType string) (string, bool) {
	path, ok := osirisTypeIcon[osirisType]
	return path, ok
}

// osirisTypeStencilShape maps an OSIRIS JSON resource type to complete
// draw.io vector stencil shape definition not an azure2 clipart image
// at all. draw.io bundles its own Kubernetes stencil library
// (shape=mxgraph.kubernetes.*) separately from the azure2 icon set; an
// AKS node pool renders as that stencil rather than an azure2 image
// icon. Checked before iconPathFor in nodeStyleFor, since a type listed
// here never falls through to an image icon at all the two mechanisms
// are mutually exclusive per type.
var osirisTypeStencilShape = map[string]string{
	"osiris.azure.aks.nodepool":        "shape=mxgraph.kubernetes.icon2;kubernetesLabel=1;prIcon=node",
	"osiris.azure.emailservice.domain": "shape=mxgraph.azure.message",
}

// stencilStyleForOSIRISType returns the complete draw.io style for a
// vector stencil shape (osirisTypeStencilShape), with the same label/
// alignment conventions iconStyle uses, and whether one was found.
func stencilStyleForOSIRISType(osirisType string) (string, bool) {
	shape, ok := osirisTypeStencilShape[osirisType]
	if !ok {
		return "", false
	}
	return shape + ";verticalLabelPosition=bottom;labelBackgroundColor=default;" +
		"verticalAlign=top;align=center;html=1;fontSize=9;", true
}

// groupTypeIcon maps an OSIRIS JSON group type to its azure2 icon path.
// Groups carry no provider.type (they are an OSIRIS JSON level
// construct, not an ARM resource), so this is keyed on the OSIRIS JSON
// group type instead.
//
// "logical.resourcegroup" is deliberately absent: a resource-group
// swimlane's own title already carries that information, so a header
// icon there is redundant only management-group and subscription
// containers get one.
// "container.region" is also absent: an OSIRIS JSON region grouping is
// generic, not an Azure resource, and region is no longer rendered as
// a container on this page at all.
var groupTypeIcon = map[string]string{
	"logical.managementgroup": "general/Management_Groups.svg",
	"logical.subscription":    "general/Subscriptions.svg",
}

// hiddenResourceTypes are OSIRIS JSON resource types this page never
// draws as a member icon. They stay fully present in the generic
// topology.Topology model underneath only this curated page hides them.
// Hiding unwanted resources help to avoid a spaghetti style topology.
var hiddenResourceTypes = map[string]bool{
	"storage.volume":                   true,
	"container.resourcegroup":          true,
	"compute.vm.snapshot":              true,
	"osiris.azure.monitor.actiongroup": true,
	"osiris.azure.monitor.metricalert": true,
	"osiris.azure.loganalytics":        true,
	"osiris.azure.logic.workflow":      true,
	"osiris.azure.streamanalytics":     true,
	"application.queue":                true,
	"osiris.azure.monitor.autoscale":   true,
}

// iconSize is a target width/height in mxGraph user units, in whichever
// order a caller needs (resourceIconSize / containerIconSize below
// apply it against a different default and override table).
type iconSize struct{ w, h int }

// resourceIconOverride holds the icons among armTypeIcon's values whose
// real SVG intrinsic aspect ratio is far enough from square that the
// default 40x40 resource-icon size visibly distorts them.
// Every other mapped icon stays the 40x40 default.
var resourceIconOverride = map[string]iconSize{
	"networking/Virtual_Networks.svg": {40, 22},
	"networking/Subnet.svg":           {40, 22},
	"devops/API_Connections.svg":      {40, 27},
	"devops/Application_Insights.svg": {28, 40},
}

// containerIconOverride is the same idea as resourceIconOverride, for
// the small header left icon.
// Only Subscriptions.svg is narrow enough to need it.
var containerIconOverride = map[string]iconSize{
	"general/Subscriptions.svg":       {13, 18},
	"networking/Virtual_Networks.svg": {22, 12},
	"networking/Subnet.svg":           {22, 12},
}

const (
	defaultResourceIconSize  = 40
	defaultContainerIconSize = 18
)

// iconForARMType returns the azure2 icon path (relative to
// azureIconBase) for armType, and whether one was found.
func iconForARMType(armType string) (string, bool) {
	path, ok := armTypeIcon[toLowerASCII(armType)]
	return path, ok
}

// iconForGroupType returns the azure2 icon path (relative to
// azureIconBase) for groupType, and whether one was found.
func iconForGroupType(groupType string) (string, bool) {
	path, ok := groupTypeIcon[groupType]
	return path, ok
}

// resourceIconSize returns the width/height a resource-node icon at
// path should render at.
func resourceIconSize(path string) (w, h int) {
	if s, ok := resourceIconOverride[path]; ok {
		return s.w, s.h
	}
	return defaultResourceIconSize, defaultResourceIconSize
}

// containerIconSize returns the width/height a container header icon
// at path should render at.
func containerIconSize(path string) (w, h int) {
	if s, ok := containerIconOverride[path]; ok {
		return s.w, s.h
	}
	return defaultContainerIconSize, defaultContainerIconSize
}

// toLowerASCII lowercases the ASCII letters in s. ARM type strings are
// ASCII (Go/dotted provider namespace segments); this avoids pulling in
// Unicode case-folding for a lookup key that is never displayed.
func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// iconStyle returns the draw.io vertex style for a resource node whose
// icon path (relative to azureIconBase) is iconPath. whiteSpace=wrap
// and labelWidth=65 let a resource's name wrap across a fixed 65-unit
// label width instead of the label's text stretching to the icon's
// own narrow width.
func iconStyle(iconPath string) string {
	return "shape=image;verticalLabelPosition=bottom;labelBackgroundColor=default;" +
		"verticalAlign=top;align=center;html=1;aspect=fixed;imageAspect=0;fontSize=9;" +
		"whiteSpace=wrap;labelWidth=65;" +
		"image=" + azureIconBase + iconPath + ";"
}

// groupIconStyle returns the draw.io vertex style for the small,
// unlabelled icon placed in a swimlane header corner (see topology.go's
// emitContainer) to mark a recognized OSIRIS JSON group type.
func groupIconStyle(iconPath string) string {
	return "shape=image;html=1;aspect=fixed;imageAspect=0;labelPosition=right;" +
		"image=" + azureIconBase + iconPath + ";"
}
