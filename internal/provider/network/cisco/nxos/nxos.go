// Package nxos provides Cisco NX-OS (Nexus switch) enrichment over the
// generic OSIRIS JSON graph, and the trigger that decides when a
// document qualifies for it.
//
// NX-OS is one of three sibling packages under
// internal/provider/network/cisco/ (apic, nxos, vmanage): these are
// genuinely different product shapes (fabric controller, switch OS,
// SD-WAN orchestrator) sharing only a vendor name, not three flavors of
// one thing, so each gets its own package and its own increment,
// mirroring internal/provider/network/hpe/arubacentral's pattern rather
// than sharing a package with the other two.
//
// Enrichment here never changes resource identity, never removes a
// node or edge, and never turns an absent field into evidence of
// absence.
package nxos

import "go.osirisjson.org/consumers/internal/graph"

// ProviderName is the OSIRIS JSON provider.name value this package
// enriches.
const ProviderName = "cisco.nxos"

// Applies reports whether g qualifies for Cisco NX-OS enrichment.
//
// Unlike Cisco APIC and HPE Aruba Central (a strict "every resource
// shares the one provider name" check), the Cisco NX-OS producer also
// emits a resource for every CDP/LLDP-discovered neighbour it saw
// devices it observed but does not own or fully inventory, under
// provider.name "unknown" rather than ProviderName (OSIRIS JSON's own
// placeholder for "no attributable provider"; internal/report already
// treats "" and "unknown" as the same case). That is not a
// mixed-vendor document: it is one real vendor's own snapshot plus its
// own best-effort discovery stubs, so Applies accepts a document whose
// resources are entirely ProviderName and "unknown"/"" (with at least
// one real ProviderName resource), and still refuses anything that
// mixes in a third, genuinely different provider or carries no
// resources at all.
func Applies(g *graph.Graph) bool {
	if len(g.ResourceIDs) == 0 {
		return false
	}
	if _, ok := g.ByProvider[ProviderName]; !ok {
		return false
	}
	for name := range g.ByProvider {
		if name != ProviderName && name != "" && name != "unknown" {
			return false
		}
	}
	return true
}

// isExternallyDiscovered reports whether a resource's provider name
// marks it as one of the CDP/LLDP-discovered neighbour stubs described
// above, rather than a resource this NX-OS switch itself owns.
func isExternallyDiscovered(providerName string) bool {
	return providerName == "" || providerName == "unknown"
}
