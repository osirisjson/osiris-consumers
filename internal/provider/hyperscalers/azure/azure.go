// Package azure provides Microsoft Azure enrichment over the generic
// OSIRIS JSON graph: draw.io Azure iconography and the trigger that
// decides when a document qualifies for it.
//
// Enrichment here never changes resource identity, never removes a
// node or edge, and never turns an absent field into evidence of
// absence. It layers icons over the generic topology model, which
// stays fully reachable underneath it.
package azure

import "go.osirisjson.org/consumers/internal/graph"

// ProviderName is the OSIRIS JSON provider.name value this package
// enriches.
const ProviderName = "azure"

// Applies reports whether g qualifies for Azure enrichment: every
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
