// Package consumer is the experimental in-process facade for the OSIRIS
// JSON consumer capabilities.
//
// Stability: every exported symbol in this package is unstable until
// v1. Names and shapes may change between 0.x releases:
//
//   - content and a validation receipt cross the boundary, never a file
//     path;
//   - a structured Result is produced before any rendering;
//   - filesystem policy (paths, overwrite, permissions, staging) lives
//     in the CLI and future Desktop adapters, not here;
//   - context is the first argument of every capability;
//   - the generic capabilities need no provider object.
//
// This build implements both MVP  0.x releases capabilities end to end:
// GenerateReport renders the vendor-neutral Markdown inventory brief,
// GenerateTopology renders the vendor-neutral draw.io topology, each
// over the safe path from exact validated input through the generic
// graph index. Rendering goes to Request.Output; a structured Result is
// returned alongside.
package consumer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/validation"
)

// Re-exported validation types, so a caller can name a receipt without
// importing an internal package.
type (
	// ValidationReceipt records one canonical validation result. See
	// the OSIRIS JSON toolbox repository for how a receipt is produced;
	// Consumers only consumes it.
	ValidationReceipt = validation.Receipt
	// Diagnostic is one canonical engine finding, carried verbatim.
	Diagnostic = validation.Diagnostic
	// Validator obtains a ValidationReceipt for exact input bytes. The
	// facade never constructs one; an adapter injects it.
	Validator = validation.Validator
)

// A Request is one capability invocation.
type Request struct {
	// Input is the exact document bytes. The facade re-checks that
	// this is a single complete JSON value followed by end of input,
	// within the size bound, before anything else happens.
	Input []byte
	// Output is the caller-supplied artifact sink. GenerateReport and
	// GenerateTopology write the rendered artifact here; it must be
	// non-nil for those calls. The library never opens a file: an
	// adapter that wants an on-disk artifact passes a writer over an
	// atomically finalized temp file (see internal/artifact).
	Output io.Writer
	// XLSXOutput is an optional second artifact sink for the report
	// capability's .xlsx companion workbook, a recognized vendor's own
	// resource-table overflow detail, never a second rendering of the
	// same content (contrast Options.OutputFormat, which picks the
	// primary artifact's own format). nil, the default and unchanged
	// from every release before this field existed, means no companion
	// is ever written, even when the recognized vendor's own model
	// would otherwise want one. Ignored by GenerateTopology. The
	// library never opens a file itself, the same rule Output follows.
	XLSXOutput io.Writer
	// Options tune validation and rendering.
	Options Options
	// Progress receives bounded, structured progress events. It may be
	// nil. It must not block.
	Progress func(Progress)
}

// Options configure a Request. The zero Options are valid: the default
// profile, the default size bound, and no injected validator (a
// pre-obtained receipt must then be supplied).
type Options struct {
	// ValidationProfile is the canonical profile to request. Empty
	// selects "default".
	ValidationProfile string
	// MaxInputBytes bounds the accepted input. Zero or less selects
	// document.DefaultMaxInputBytes.
	MaxInputBytes int64
	// Validator produces the receipt when Receipt is nil. When both are
	// nil the call fails with ErrNoReceipt: the reusable library never
	// runs a validator it was not given.
	Validator Validator
	// Receipt is a pre-obtained canonical validation receipt (the
	// Desktop path). When set, Input must be byte-identical to what the
	// receipt was produced for. Receipt takes precedence over
	// Validator.
	Receipt *ValidationReceipt

	// ArtifactTime is the timestamp a renderer stamps into the
	// artifact. The zero value means "no timestamp": renderers omit the
	// generated line entirely, so output stays deterministic without a
	// clock. An adapter that wants a real time injects it (the CLI
	// passes time.Now().UTC()); a test injects a fixed instant.
	ArtifactTime time.Time

	// MaxInventoryRows bounds each list section of the Markdown report
	// (resources, connections, groups). Zero or less means unbounded:
	// no row is ever omitted. When set and a section is longer, the
	// report keeps the first N rows in sort order and records an
	// explicit truncation notice carrying the full count. It does not
	// affect the topology.
	MaxInventoryRows int

	// OutputFormat selects the report capability's rendered artifact
	// format: "" or "markdown" (the default, unchanged from every
	// release before this field existed) or "pdf". It has no effect on
	// GenerateTopology. An adapter resolves the chosen output format is
	// an option, resolved by the CLI/Desktop adapter, not a new facade
	// verb per format, the facade never infers it from a file extension
	// or any other signal; the CLI adapter owns that convenience on top
	// of this field.
	// Unrecognized value is treated as "markdown", the same permissive
	// zero-value-is-valid rule every other Options field follows; the
	// CLI adapter validates its own flag before ever reaching here.
	OutputFormat string

	// XLSXCompanionName is the file name, display only, e.g.
	// "report.xlsx", never a path the facade resolves or validates,
	// the report's own "see the companion workbook" text shows when a
	// companion is actually written (Request.XLSXOutput is set and the
	// recognized vendor's model decides one is needed for this
	// document). Empty omits that text even when XLSXOutput is set; the
	// CLI adapter owns deciding the real file name and path, the same
	// division of responsibility OutputFormat's extension inference
	// already established.
	XLSXCompanionName string
}

// Progress is one bounded progress event. It carries no percentage and
// no document content.
type Progress struct {
	Stage   Stage
	Message string
}

// Stage names a step of a capability run.
type Stage int

// Progress stages, in the order they occur.
const (
	StageValidating Stage = iota
	StageOpening
	StageIndexing
	StageRendering
	StageFinalizing
)

// String returns a stable lower-case slug for the stage.
func (s Stage) String() string {
	switch s {
	case StageValidating:
		return "validating"
	case StageOpening:
		return "opening"
	case StageIndexing:
		return "indexing"
	case StageRendering:
		return "rendering"
	case StageFinalizing:
		return "finalizing"
	default:
		return fmt.Sprintf("stage(%d)", int(s))
	}
}

// A Result is the structured outcome of a capability run. It is
// populated as far as the run got: on ErrNotImplemented it carries the
// hash, receipt, warnings, and graph anomalies; on ErrValidationFailed
// or ErrUnsupportedOSIRISJSON it still carries the hash and the receipt
// so an adapter can surface the verbatim canonical diagnostics. On a
// malformed-input or no-receipt failure it is the zero value.
type Result struct {
	// InputSHA256 is the lower-case hex SHA-256 of the exact document
	// bytes.
	InputSHA256 string
	// Validation is the canonical validation receipt the run used.
	Validation ValidationReceipt
	// Artifact summarises the finalized artifact. It is the zero value
	// in this build.
	Artifact ArtifactSummary
	// XLSXArtifact summarizes the report capability's .xlsx companion
	// workbook, when one was actually written (Request.XLSXOutput was
	// set and the recognized vendor's own model decided a companion was
	// needed for this document). It is the zero value otherwise,
	// including for a document with no recognized vendor, since the
	// companion-workbook concept is vendor enrichment only, never a
	// generic capability. Always the zero value for GenerateTopology.
	XLSXArtifact ArtifactSummary
	// Warnings are canonical engine warnings, verbatim.
	Warnings []Diagnostic
	// GraphAnomalies are structural inconsistencies found while
	// indexing, after validation. They are defensive notes, not
	// canonical diagnostics, and carry no severity grade.
	GraphAnomalies []GraphAnomaly
}

// ArtifactSummary describes a finalized artifact.
type ArtifactSummary struct {
	SHA256 string
	Bytes  int64
	Path   string
	// PageFormat is the draw.io page size the topology renderer chose:
	// an ISO 216 name ("A5" through "A0") or "custom" when the drawing
	// exceeds landscape A0. It is empty for the Markdown report.
	PageFormat string
	// PageWidth and PageHeight are the emitted landscape page dimension
	// in draw.io user units (PageWidth >= PageHeight always). They are
	// zero for the Markdown report. No topology content is ever scaled
	// or dropped to fit the page.
	PageWidth  int
	PageHeight int
}

// An AllRequest is one GenerateAll invocation: one input, one shared
// safe path, and one optional sink per artifact. A nil sink skips that
// artifact. Like Request, it carries content and writers, never a
// path; the library never opens a file.
type AllRequest struct {
	// Input is the exact document bytes, with the same contract as
	// Request.Input.
	Input []byte
	// Markdown receives the report as Markdown, vendor-enriched when a
	// vendor is recognized.
	Markdown io.Writer
	// PDF receives the report as the vendor-neutral PDF baseline.
	PDF io.Writer
	// Topology receives the draw.io topology, vendor-enriched when a
	// vendor is recognized.
	Topology io.Writer
	// XLSX is the optional .xlsx companion sink, with the same rule as
	// Request.XLSXOutput. It is used only when Markdown is non-nil.
	XLSX io.Writer
	// Options tune validation and rendering. OutputFormat is ignored.
	Options Options
	// Progress receives bounded, structured progress events. It may be
	// nil. It must not block.
	Progress func(Progress)
}

// An AllResult is the structured outcome of a GenerateAll run. Its
// header fields follow the Result contract, including on failure. Each
// artifact summary is the zero value when its sink was nil or, for
// XLSX, when no companion was written.
type AllResult struct {
	InputSHA256    string
	Validation     ValidationReceipt
	Markdown       ArtifactSummary
	PDF            ArtifactSummary
	Topology       ArtifactSummary
	XLSX           ArtifactSummary
	Warnings       []Diagnostic
	GraphAnomalies []GraphAnomaly
}

// A GraphAnomaly is a structural inconsistency in a document's own
// references, surfaced after validation.
type GraphAnomaly struct {
	Kind      string
	Subject   string
	Reference string
	Detail    string
}

// Error categories. Callers branch on these with errors.Is; ExitCode
// maps them to CLI exit codes.
var (
	// ErrNoReceipt means neither a receipt nor a validator was supplied.
	ErrNoReceipt = errors.New("No validation receipt and no validator supplied")
	// ErrInvalidInput means the input is not a single well-formed
	// OSIRIS JSON document within the size bound, or the injected
	// validator could not run. Maps to exit code 2.
	ErrInvalidInput = errors.New("Invalid input")
	// ErrValidationFailed means the receipt reports canonical
	// validation errors. Maps to exit code 1.
	ErrValidationFailed = errors.New("Document failed canonical validation")
	// ErrUnsupportedOSIRISJSON means the document declares an OSIRIS JSON
	// major version this build does not consume. Maps to exit code 1.
	ErrUnsupportedOSIRISJSON = errors.New("Unsupported OSIRIS JSON major version")
	// ErrReceiptMismatch means the receipt does not describe these
	// bytes, or is incomplete. Maps to exit code 2.
	ErrReceiptMismatch = errors.New("Validation receipt does not match the input")
	// ErrNotImplemented means the requested capability is not part of
	// this build. Both MVP capabilities are implemented; this sentinel
	// is reserved for a capability added to the facade before its
	// renderer exists. Maps to exit code 2.
	ErrNotImplemented = errors.New("Capability is not implemented in this build")
	// ErrNoOutput means a rendering capability was called with a nil
	// Request.Output. Maps to exit code 2.
	ErrNoOutput = errors.New("No artifact output writer supplied")
)

// ExitCode maps err to the CLI exit-code contract: 0 for nil, 1 for a
// canonical validation fault in an otherwise well-formed document, and
// 2 for malformed input, a receipt fault, an unimplemented capability,
// cancellation, or any other operational failure.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, ErrValidationFailed),
		errors.Is(err, ErrUnsupportedOSIRISJSON),
		errors.Is(err, validation.ErrReceiptHasErrors),
		errors.Is(err, document.ErrUnsupportedMajor):
		return 1
	default:
		return 2
	}
}

// emit delivers one progress event when a sink is present.
func emit(sink func(Progress), stage Stage, msg string) {
	if sink != nil {
		sink(Progress{Stage: stage, Message: msg})
	}
}

// readBytes enforces the bounded exact-byte input contract on the
// request payload.
func readBytes(req Request) ([]byte, error) {
	b, err := document.Read(bytes.NewReader(req.Input), req.Options.MaxInputBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	return b, nil
}
