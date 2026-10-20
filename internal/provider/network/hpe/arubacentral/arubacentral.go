// Package arubacentral provides HPE Aruba Networking Central enrichment
// over the generic OSIRIS JSON graph: resource icons, a per-type
// physical-interface topology, and the trigger that decides when a
// document qualifies for it.
//
// Enrichment here never changes resource identity, never removes a
// node or edge, and never turns an absent field into evidence of
// absence. It adds icons, page structure, and interface detail read
// directly from the document; the generic report and topology models
// stay fully reachable underneath it.
package arubacentral

import "go.osirisjson.org/consumers/internal/graph"

// ProviderName is the OSIRIS JSON provider.name value this package
// enriches.
const ProviderName = "hpe.arubacentral"

// Applies reports whether g qualifies for Aruba Central enrichment:
// every resource in the document declares ProviderName.
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
