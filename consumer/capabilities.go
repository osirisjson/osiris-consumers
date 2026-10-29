// This file wires the capability entry points. Each runs the same safe
// path (bounded input, canonical receipt, receipt check, loss-aware
// parse, linear generic index), then builds a format-neutral model and
// renders it through a checked, deterministic renderer. GenerateAll
// runs that safe path once and renders every artifact from it.

package consumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/validation"
)

// sha256Hex is the lower-case hex SHA-256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// GenerateReport produces the vendor-neutral Markdown brief and
// inventory for a validated OSIRIS JSON document and writes it to
// req.Output.
//
// It runs input, validation, receipt verification, loss-aware parse,
// and generic indexing, builds the format-neutral report model, and
// renders it as deterministic Markdown. The returned Result carries the
// input hash, the receipt, engine warnings, structural anomalies, and
// the artifact summary (hash and byte count).
func GenerateReport(ctx context.Context, req Request) (Result, error) {
	return generate(ctx, req, "report")
}

// GenerateTopology produces the vendor-neutral draw.io topology for a
// validated OSIRIS JSON document and writes it to req.Output.
//
// It runs the same safe path as GenerateReport, builds the
// format-neutral topology model (one node per resource, one edge per
// connection, groups represented, generic fallback for unknown types),
// and renders it as deterministic draw.io XML.
func GenerateTopology(ctx context.Context, req Request) (Result, error) {
	return generate(ctx, req, "topology")
}

// generate is the shared safe path. capability is "report" or
// "topology".
func generate(ctx context.Context, req Request, capability string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if req.Output == nil {
		return Result{}, fmt.Errorf("%s: %w", capability, ErrNoOutput)
	}

	view, g, res, err := prepare(ctx, req.Input, req.Options, req.Progress)
	if err != nil {
		return res, err
	}

	emit(req.Progress, StageRendering, "rendering the "+capability+" artifact")
	var summary, xlsxSummary ArtifactSummary
	switch capability {
	case "report":
		summary, xlsxSummary, err = renderReport(ctx, req.Output, req.XLSXOutput, view.doc, g, res.Validation, req.Options)
	case "topology":
		summary, err = renderTopology(ctx, req.Output, view.doc, g, req.Options)
	default:
		return res, fmt.Errorf("%s: %w", capability, ErrNotImplemented)
	}
	if err != nil {
		return res, fmt.Errorf("%s: %w", capability, err)
	}
	res.Artifact = summary
	res.XLSXArtifact = xlsxSummary

	if summary.PageFormat == "custom" {
		emit(req.Progress, StageRendering, fmt.Sprintf(
			"topology exceeds ISO A0 page size; rendered on a custom %dx%d page; no nodes, edges, or groups omitted",
			summary.PageWidth, summary.PageHeight))
	}

	emit(req.Progress, StageFinalizing, "artifact rendered")
	return res, nil
}

// GenerateAll produces every artifact the report and topology
// capabilities can render for one document, from a single pass over
// the shared safe path: the input is read, validated (the injected
// Validator runs at most once), receipt-checked, parsed, and indexed
// exactly once, and every requested artifact is rendered from that same
// validated view and graph.
//
// Each sink in req is optional; a nil sink skips that artifact. At
// least one of Markdown, PDF, or Topology must be non-nil, otherwise
// GenerateAll fails with ErrNoOutput before any work. The PDF is always
// the vendor-neutral baseline, the same artifact GenerateReport renders
// with Options.OutputFormat "pdf". The .xlsx companion follows the same
// rule as Request.XLSXOutput: it is written only when a recognized
// vendor's model decides this document needs one, and it requires the
// Markdown sink, whose text is what invites the reader to open it.
// Options.OutputFormat is ignored.
//
// Artifacts render in a fixed order (Markdown with its companion, then
// PDF, then topology) and the first render error stops the run; an
// adapter that stages sinks in memory therefore finalizes nothing on
// failure.
func GenerateAll(ctx context.Context, req AllRequest) (AllResult, error) {
	if err := ctx.Err(); err != nil {
		return AllResult{}, err
	}
	if req.Markdown == nil && req.PDF == nil && req.Topology == nil {
		return AllResult{}, fmt.Errorf("all: %w", ErrNoOutput)
	}

	view, g, res, err := prepare(ctx, req.Input, req.Options, req.Progress)
	out := AllResult{
		InputSHA256:    res.InputSHA256,
		Validation:     res.Validation,
		Warnings:       res.Warnings,
		GraphAnomalies: res.GraphAnomalies,
	}
	if err != nil {
		return out, err
	}

	if req.Markdown != nil {
		emit(req.Progress, StageRendering, "rendering the report artifact (markdown)")
		opts := req.Options
		opts.OutputFormat = "markdown"
		out.Markdown, out.XLSX, err = renderReport(ctx, req.Markdown, req.XLSX, view.doc, g, res.Validation, opts)
		if err != nil {
			return out, fmt.Errorf("report: %w", err)
		}
	}
	if req.PDF != nil {
		emit(req.Progress, StageRendering, "rendering the report artifact (pdf)")
		opts := req.Options
		opts.OutputFormat = "pdf"
		out.PDF, _, err = renderReport(ctx, req.PDF, nil, view.doc, g, res.Validation, opts)
		if err != nil {
			return out, fmt.Errorf("report: %w", err)
		}
	}
	if req.Topology != nil {
		emit(req.Progress, StageRendering, "rendering the topology artifact")
		out.Topology, err = renderTopology(ctx, req.Topology, view.doc, g, req.Options)
		if err != nil {
			return out, fmt.Errorf("topology: %w", err)
		}
		if out.Topology.PageFormat == "custom" {
			emit(req.Progress, StageRendering, fmt.Sprintf(
				"topology exceeds ISO A0 page size; rendered on a custom %dx%d page; no nodes, edges, or groups omitted",
				out.Topology.PageWidth, out.Topology.PageHeight))
		}
	}

	emit(req.Progress, StageFinalizing, "artifacts rendered")
	return out, nil
}

// prepare runs the shared safe path every capability starts from:
// bounded input, canonical receipt, receipt check, loss-aware parse,
// and the linear generic index. It returns the validated view, its
// graph, and the Result header (hash, receipt, warnings, anomalies).
// On failure the returned Result is populated as far as the run got,
// per the Result contract.
func prepare(ctx context.Context, input []byte, opts Options, progress func(Progress)) (*Validated, *graph.Graph, Result, error) {
	profile := opts.ValidationProfile
	if profile == "" {
		profile = validation.DefaultProfile
	}

	emit(progress, StageValidating, "reading input")
	input, err := readBytes(Request{Input: input, Options: opts})
	if err != nil {
		return nil, nil, Result{}, err
	}

	receipt, err := obtainReceipt(ctx, opts, progress, input, profile)
	if err != nil {
		return nil, nil, Result{}, err
	}

	emit(progress, StageOpening, "verifying validation receipt")
	view, err := OpenValidated(input, receipt)
	if err != nil {
		// Return the receipt with the error so an adapter can surface
		// the verbatim canonical diagnostics OSIRIS JSON (V-* codes)
		// and the engine identity even though no capability will run.
		return nil, nil, Result{
			InputSHA256: sha256Hex(input),
			Validation:  receipt,
			Warnings:    warningsOf(receipt),
		}, err
	}

	emit(progress, StageIndexing, "indexing the generic graph")
	g, err := graph.Build(ctx, view.doc)
	if err != nil {
		return nil, nil, Result{}, err
	}

	return view, g, Result{
		InputSHA256:    view.InputSHA256(),
		Validation:     receipt,
		Warnings:       warningsOf(receipt),
		GraphAnomalies: exportAnomalies(g.Anomalies),
	}, nil
}

// obtainReceipt returns the pre-supplied receipt, or runs the injected
// validator, or fails with ErrNoReceipt. The reusable library never
// runs a validator it was not given.
func obtainReceipt(ctx context.Context, opts Options, progress func(Progress), input []byte, profile string) (ValidationReceipt, error) {
	if opts.Receipt != nil {
		return *opts.Receipt, nil
	}
	if opts.Validator == nil {
		return ValidationReceipt{}, ErrNoReceipt
	}
	emit(progress, StageValidating, "running canonical validation")
	receipt, err := opts.Validator.Validate(ctx, input, profile)
	if err != nil {
		if ctx.Err() != nil {
			return ValidationReceipt{}, ctx.Err()
		}
		return ValidationReceipt{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	return receipt, nil
}

// warningsOf returns the warning-severity diagnostics from a receipt.
func warningsOf(receipt ValidationReceipt) []Diagnostic {
	var out []Diagnostic
	for _, d := range receipt.Diagnostics {
		if d.Severity == "warning" {
			out = append(out, d)
		}
	}
	return out
}

// exportAnomalies converts internal graph anomalies to the public shape.
func exportAnomalies(in []graph.Anomaly) []GraphAnomaly {
	if len(in) == 0 {
		return nil
	}
	out := make([]GraphAnomaly, len(in))
	for i, a := range in {
		out[i] = GraphAnomaly{
			Kind:      a.Kind.String(),
			Subject:   a.Subject,
			Reference: a.Reference,
			Detail:    a.Detail,
		}
	}
	return out
}
