// This file bridges the generic capability models to their renderers.
// It keeps the model-then-render ordering explicit (a structured model
// is always built before a byte is written) and computes the artifact
// hash and size as the bytes stream to the caller's writer.

package consumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/provider/hyperscalers/azure"
	"go.osirisjson.org/consumers/internal/provider/network/cisco/apic"
	"go.osirisjson.org/consumers/internal/provider/network/cisco/nxos"
	"go.osirisjson.org/consumers/internal/provider/network/cisco/vmanage"
	"go.osirisjson.org/consumers/internal/provider/network/hpe/arubacentral"
	"go.osirisjson.org/consumers/internal/render"
	"go.osirisjson.org/consumers/internal/render/drawio"
	"go.osirisjson.org/consumers/internal/render/markdown"
	"go.osirisjson.org/consumers/internal/render/pdf"
	"go.osirisjson.org/consumers/internal/report"
	"go.osirisjson.org/consumers/internal/topology"
	"go.osirisjson.org/consumers/internal/validation"
)

// renderReport builds the report model and renders it to w.
//
// An OSIRIS JSON document that declares provider.name like
// "hpe.arubacentral" then (arubacentral.Applies), "cisco.apic"
// (apic.Applies), "cisco.nxos" (nxos.Applies), "cisco.vmanage"
// (vmanage.Applies), or "azure" (azure.Applies), all whole-document
// checks, never a first-resource guess, gets that vendor's
// report instead of the generic one. This is auto-detected from the
// document's own data, never a --provider flag or caller option, per
// "Generic processing is the default; there is no provider selector
// flag". Anything else, including a mixed-provider document, stays
// fully generic.
//
// opts.OutputFormat "pdf" always renders the generic report model
// through pdf.NewReportRenderer, bypassing the internal/provider
// branches: their renderers emit Markdown only, and PDF enrichment for
// a recognized vendor is a deferred follow-up increment.
//
// xlsxW is the optional .xlsx companion sink (Request.XLSXOutput). It
// only ever produces a second artifact through one of the
// internal/provider branches, each of which decides independently (via
// its own ReportModel.NeedsXLSXCompanion) whether the specific document
// actually needs one, xlsxW being non-nil is necessary, not
// sufficient. The generic fallback has no companion-workbook concept
// at all (it was never a generic capability, only ever vendor
// enrichment), so its own xlsxSummary return is always the zero value.
func renderReport(ctx context.Context, w, xlsxW io.Writer, doc *document.Document, g *graph.Graph, receipt validation.Receipt, opts Options) (ArtifactSummary, ArtifactSummary, error) {
	if opts.OutputFormat == "pdf" {
		model, err := report.Build(ctx, doc, g, receipt, report.Options{MaxRows: opts.MaxInventoryRows})
		if err != nil {
			return ArtifactSummary{}, ArtifactSummary{}, err
		}
		sum, err := renderThrough(ctx, w, pdf.NewReportRenderer(model, opts.ArtifactTime))
		return sum, ArtifactSummary{}, err
	}
	if arubacentral.Applies(g) {
		model := arubacentral.BuildReportModel(g, doc.Metadata, doc.SHA256())
		return renderVendorReport(ctx, w, xlsxW, model.NeedsXLSXCompanion(), opts,
			func(xlsxName string) render.Renderer {
				return arubacentral.NewReportRenderer(model, opts.ArtifactTime, xlsxName)
			},
			func() render.Renderer { return arubacentral.NewXLSXRenderer(model) })
	}
	if apic.Applies(g) {
		model := apic.BuildReportModel(g, doc.Metadata, doc.SHA256())
		return renderVendorReport(ctx, w, xlsxW, model.NeedsXLSXCompanion(), opts,
			func(xlsxName string) render.Renderer {
				return apic.NewReportRenderer(model, opts.ArtifactTime, xlsxName)
			},
			func() render.Renderer { return apic.NewXLSXRenderer(model) })
	}
	if nxos.Applies(g) {
		model := nxos.BuildReportModel(g, doc.Metadata, doc.SHA256())
		return renderVendorReport(ctx, w, xlsxW, model.NeedsXLSXCompanion(), opts,
			func(xlsxName string) render.Renderer {
				return nxos.NewReportRenderer(model, opts.ArtifactTime, xlsxName)
			},
			func() render.Renderer { return nxos.NewXLSXRenderer(model) })
	}
	if vmanage.Applies(g) {
		model := vmanage.BuildReportModel(g, doc.Metadata, doc.SHA256())
		return renderVendorReport(ctx, w, xlsxW, model.NeedsXLSXCompanion(), opts,
			func(xlsxName string) render.Renderer {
				return vmanage.NewReportRenderer(model, opts.ArtifactTime, xlsxName)
			},
			func() render.Renderer { return vmanage.NewXLSXRenderer(model) })
	}
	if azure.Applies(g) {
		model := azure.BuildReportModel(g, doc.Metadata, doc.SHA256())
		return renderVendorReport(ctx, w, xlsxW, model.NeedsXLSXCompanion(), opts,
			func(xlsxName string) render.Renderer {
				return azure.NewReportRenderer(model, opts.ArtifactTime, xlsxName)
			},
			func() render.Renderer { return azure.NewXLSXRenderer(model) })
	}
	model, err := report.Build(ctx, doc, g, receipt, report.Options{MaxRows: opts.MaxInventoryRows})
	if err != nil {
		return ArtifactSummary{}, ArtifactSummary{}, err
	}
	sum, err := renderThrough(ctx, w, markdown.NewReportRenderer(model, opts.ArtifactTime))
	return sum, ArtifactSummary{}, err
}

// renderVendorReport runs one vendor's "render the primary report,
// then optionally its .xlsx companion" sequence, the same two-step
// shared by internal/provider, since every internal/provider/*/report_
// {model,render,xlsx}.go exposes the identical NeedsXLSXCompanion /
// NewReportRenderer / NewXLSXRenderer shape (duck-typed, like the
// Applies(g) dispatch above). newReport takes the resolved companion
// name, already "" when no companion will actually be written, so a
// vendor's own Markdown invite text never references a file this call
// does not also produce.
func renderVendorReport(ctx context.Context, w, xlsxW io.Writer, needsXLSX bool, opts Options,
	newReport func(xlsxName string) render.Renderer, newXLSX func() render.Renderer,
) (ArtifactSummary, ArtifactSummary, error) {
	xlsxName := ""
	if needsXLSX && xlsxW != nil {
		xlsxName = opts.XLSXCompanionName
	}
	sum, err := renderThrough(ctx, w, newReport(xlsxName))
	if err != nil {
		return ArtifactSummary{}, ArtifactSummary{}, err
	}
	if xlsxName == "" {
		return sum, ArtifactSummary{}, nil
	}
	xlsxSum, err := renderThrough(ctx, xlsxW, newXLSX())
	if err != nil {
		return ArtifactSummary{}, ArtifactSummary{}, err
	}
	return sum, xlsxSum, nil
}

// renderTopology builds the topology model and renders it as draw.io
// XML to w. The generic topology.Build call runs unconditionally,
// whether or not a vendor enrichment ends up choosing the renderer,
// the enrichment invariant (generic node/edge/resource counts
// identical with and without enrichment; see
// TestArubaCentralEnrichmentPreservesGenericCounts,
// TestApicEnrichmentPreservesGenericCounts, and
// TestNxosEnrichmentPreservesGenericCounts) holds because every path
// starts from the exact same model, never a document re-parse or a
// second, enrichment-specific graph pass.
// As an example the internal/provider Cisco vManage only dispatches
// to its own confirmed-panel renderer when every router in the document
// resolves to a confirmed model (vmanage.ConfirmedTopologyApplies, a
// stricter check than vmanage.Applies); a document with an unconfirmed
// router model falls all the way through to the plain generic renderer
// below, per "generic first, provider optional": no enrichment is not a
// failure mode. Cisco APIC, Cisco NX-OS, and Cisco vManage's own
// topology renderers draw their layouts straight from the graph rather
// than the generic topology.Topology model (their layouts do not map
// onto the generic node/edge shape), but they still consume the same
// *graph.Graph the generic model was built from, and never mutate it.
func renderTopology(ctx context.Context, w io.Writer, doc *document.Document, g *graph.Graph, opts Options) (ArtifactSummary, error) {
	model, err := topology.Build(ctx, doc, g)
	if err != nil {
		return ArtifactSummary{}, err
	}

	if arubacentral.Applies(g) {
		sum, err := renderThrough(ctx, w, arubacentral.NewTopologyRenderer(model, g, doc.Metadata, opts.ArtifactTime))
		if err != nil {
			return ArtifactSummary{}, err
		}
		page := arubacentral.ChooseTopologyPage(model)
		sum.PageFormat = page.Format
		sum.PageWidth = page.Width
		sum.PageHeight = page.Height
		return sum, nil
	}

	if apic.Applies(g) {
		sum, err := renderThrough(ctx, w, apic.NewTopologyRenderer(g, doc.Metadata, opts.ArtifactTime))
		if err != nil {
			return ArtifactSummary{}, err
		}
		page := apic.ChooseTopologyPage(g)
		sum.PageFormat = page.Format
		sum.PageWidth = page.Width
		sum.PageHeight = page.Height
		return sum, nil
	}

	if nxos.Applies(g) {
		sum, err := renderThrough(ctx, w, nxos.NewTopologyRenderer(g, doc.Metadata, opts.ArtifactTime))
		if err != nil {
			return ArtifactSummary{}, err
		}
		page := nxos.ChooseTopologyPage(g)
		sum.PageFormat = page.Format
		sum.PageWidth = page.Width
		sum.PageHeight = page.Height
		return sum, nil
	}

	// vmanage.ConfirmedTopologyApplies, not vmanage.Applies: a vManage
	// document with an unconfirmed router model falls all the way
	// through to the plain generic renderer below, never a partial
	// confirmed-panel page for some of its routers.
	if vmanage.ConfirmedTopologyApplies(g) {
		sum, err := renderThrough(ctx, w, vmanage.NewTopologyRenderer(g, doc.Metadata, opts.ArtifactTime))
		if err != nil {
			return ArtifactSummary{}, err
		}
		page, _ := vmanage.ChooseTopologyPage(g)
		sum.PageFormat = page.Format
		sum.PageWidth = page.Width
		sum.PageHeight = page.Height
		return sum, nil
	}

	// Microsoft Azure (azure.Applies, whole-document):
	// unlike the vendors above, this reuses the exact generic
	// topology.Topology model and its group-forest swimlane layout
	// only each node's style changes, to a draw.io azure2 icon when its
	// ARM native type is recognized (internal/provider/hyperscalers/
	// azure/icons.go). The generic report stays the renderer for an
	// Azure document until the report side of this enrichment lands.
	if azure.Applies(g) {
		sum, err := renderThrough(ctx, w, azure.NewTopologyRenderer(model, g, doc.Metadata, opts.ArtifactTime))
		if err != nil {
			return ArtifactSummary{}, err
		}
		page := azure.ChooseTopologyPage(model, g)
		sum.PageFormat = page.Format
		sum.PageWidth = page.Width
		sum.PageHeight = page.Height
		return sum, nil
	}

	sum, err := renderThrough(ctx, w, drawio.NewTopologyRenderer(model, opts.ArtifactTime))
	if err != nil {
		return ArtifactSummary{}, err
	}
	page := drawio.ChoosePage(model)
	sum.PageFormat = page.Format
	sum.PageWidth = page.Width
	sum.PageHeight = page.Height
	return sum, nil
}

// renderThrough runs r against w while hashing and counting the bytes.
// It returns the artifact summary on success. A renderer write error or
// a cancelled context propagates unchanged; no partial summary is
// returned.
func renderThrough(ctx context.Context, w io.Writer, r render.Renderer) (ArtifactSummary, error) {
	h := sha256.New()
	cw := &countingHasher{w: w, h: h}
	if err := r.Render(ctx, cw); err != nil {
		return ArtifactSummary{}, err
	}
	if cw.err != nil {
		return ArtifactSummary{}, cw.err
	}
	return ArtifactSummary{
		SHA256: hex.EncodeToString(h.Sum(nil)),
		Bytes:  cw.n,
	}, nil
}

// countingHasher forwards writes to w and a hash, recording the byte
// count and the first write error.
type countingHasher struct {
	w   io.Writer
	h   hash.Hash
	n   int64
	err error
}

func (c *countingHasher) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	_, _ = c.h.Write(p[:n])
	if err != nil {
		c.err = err
	}
	return n, err
}
