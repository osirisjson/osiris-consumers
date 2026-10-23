// Package apic provides Cisco APIC (ACI fabric controller) enrichment
// over the generic OSIRIS JSON graph, and the trigger that decides
// when a document qualifies for it.
//
// APIC is one of three sibling packages under
// internal/provider/network/cisco/ (apic, nxos, iosxe, vmanage):
// these are genuinely different product shapes (fabric controller,
// switch/router OS, SD-WAN orchestrator) sharing only a vendor
// name, not three flavors of one thing, so each gets its own package
// and its own increment, mirroring internal/provider/network/hpe/
// arubacentral's pattern rather than sharing a package with the others.
//
// Enrichment here never changes resource identity, never removes a
// node or edge, and never turns an absent field into evidence of
// absence.
package apic

import "go.osirisjson.org/consumers/internal/graph"

// ProviderName is the OSIRIS JSON provider.name value this package
// enriches.
const ProviderName = "cisco.apic"

// Applies reports whether g qualifies for Cisco APIC enrichment: every
// resource in the document declares ProviderName. This is a
// whole-document check, never a first-resource inference: a
// mixed-provider document, an unrecognized one, or one with no
// resources at all does not qualify and stays on the generic path.
func Applies(g *graph.Graph) bool {
	if len(g.ResourceIDs) == 0 {
		return false
	}
	if len(g.ByProvider) != 1 {
		return false
	}
	_, ok := g.ByProvider[ProviderName]
	return ok
}
