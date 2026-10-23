// Package vmanage provides Cisco vManage (SD-WAN orchestrator)
// enrichment over the generic OSIRIS JSON graph, and the trigger that
// decides when a document qualifies for it.
//
// vManage is one of the sibling packages under
// internal/provider/network/cisco/ (apic, nxos, vmanage): these are
// genuinely different product shapes (fabric controller, switch OS,
// SD-WAN orchestrator) sharing only a vendor name, not three flavors of
// one thing, so each gets its own package and its own increment, rather
// than sharing a package with the other two.
//
// Unlike Cisco NX-OS (exactly one switch per document) a vManage
// document can describe several routers at one site, vManage manages
// a fleet, and a single snapshot's own "metadata.scope.sites" may name
// one site with more than one edge router for redundancy. Every model
// in this package is built to hold zero, one, or many routers, never
// assuming exactly one.
package vmanage

import "go.osirisjson.org/consumers/internal/graph"

// ProviderName is the OSIRIS JSON provider.name value this package
// enriches.
const ProviderName = "cisco.vmanage"

// Applies reports whether g qualifies for Cisco vManage enrichment: a
// whole-document check (every resource declares ProviderName, and at
// least one resource exists), never a first-resource guess.
// Unlike Cisco NX-OS, this producer's own fixtures (0.1.0 release)
// carry no CDP/LLDP-style discovery stub under a different provider
// name, so there is no "unknown"/"" carve-out here, if that changes in
// a future OSIRIS JSON producer release, add it the same way
// nxos.isExternallyDiscovered does, not by loosening
// this check silently.
func Applies(g *graph.Graph) bool {
	if len(g.ResourceIDs) == 0 {
		return false
	}
	for _, id := range g.ResourceIDs {
		r := g.Resources[id]
		if r == nil || r.Provider.Name != ProviderName {
			return false
		}
	}
	return true
}
