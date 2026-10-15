// Package validation defines the canonical-validation handoff for
// OSIRIS JSON Consumers.
//
// Consumers never validates OSIRIS JSON itself. Validation is defined
// by the canonical OSIRIS JSON toolbox and MUST NOT be
// re-implemented here.
// This package holds only the receipt that records a canonical
// validation result and the consistency check that gates every
// capability on it.
//
// A Receipt is trusted provenance, not cryptographic proof: it asserts
// that a named engine ran a named profile over a specific input hash
// reports the result. OpenValidated enforces an internal consistency
// contract over that assertion (the input hash matches and there are no
// errors); it is not an attestation and is not signed in v0.1.0.
//
// The concrete mechanism that produces a Receipt lives outside this
// package. The v0.1.0 mechanism is a subprocess to a pinned
// @osirisjson/cli, implemented in the toolboxcli subpackage and wired
// in by the CLI adapter, never by the reusable library.
package validation

import "context"

// Pinned identifiers for the v0.1.0 supported validation mechanism.
// They are recorded in every Receipt this build's CLI adapter produces
// and in release build manifests, so a consumer diffing two runs can
// see exactly which engine and specification revision were in force.
const (
	// PinnedEngineName is the canonical validation tool Consumers
	// shells out to for v0.1.0.
	PinnedEngineName = "@osirisjson/cli"
	// PinnedEngineVersion is the exact @osirisjson/cli version the
	// v0.1.0 CLI adapter expects on PATH.
	PinnedEngineVersion = "0.1.0"
	// PinnedSpecVersion is the OSIRIS JSON specification line the
	// pinned engine validates against.
	PinnedSpecVersion = "1.0"
	// PinnedExampleRevision identifies the OSIRIS JSON example set the
	// pins were verified against: the osirisjson/osiris repository at
	// release tag v1.0.0. The interoperability suite consumes it as a
	// pinned submodule at testdata/, created for the
	// v0.1.0 release; until then the suite reads a local checkout named
	// by OSIRIS_EXAMPLES_DIR and skips when it is absent.
	PinnedExampleRevision = "osirisjson/osiris@v1.0.0"
)

// DefaultProfile is the validation profile the MVP report and topology
// paths request. Advisory text may additionally consult "strict"; the
// generic capabilities do not.
const DefaultProfile = "default"

// A Receipt records one canonical validation result for one exact input.
//
// The zero Receipt is not usable: OpenValidated rejects it because its
// InputSHA256 cannot match any real input.
type Receipt struct {
	// InputSHA256 is the lower-case hex SHA-256 of the exact bytes the
	// engine validated.
	InputSHA256 string
	// EngineName and EngineVersion identify the canonical engine that
	// produced this result.
	EngineName    string
	EngineVersion string
	// Profile is the validation profile that was applied
	// ("basic", "default", or "strict").
	Profile string
	// SpecificationVersion is the OSIRIS JSON specification line the
	// engine validated against (for example "1.0").
	SpecificationVersion string
	// ErrorCount and WarningCount are the engine's own totals.
	ErrorCount   int
	WarningCount int
	// Diagnostics are the engine diagnostics verbatim, in engine order.
	// Consumers surfaces these codes as-is and invents no parallel
	// code for a schema or reference fault.
	Diagnostics []Diagnostic
}

// A Diagnostic is one engine finding, carried through unchanged.
type Diagnostic struct {
	// Code is the canonical diagnostic code (for example "V-REF-002").
	Code string
	// Stage is the pipeline stage that produced the finding
	// (1 structural, 2 semantic, 3 domain).
	Stage int
	// Severity is "error", "warning", or "info".
	Severity string
	// Message is the human-readable finding text.
	Message string
	// Path is the JSON Pointer into the document the finding refers to.
	Path string
	// Related lists secondary locations the engine attached to the
	// finding (for example the first occurrence of a duplicate id).
	Related []Related
}

// A Related is a secondary location on a Diagnostic.
type Related struct {
	Path    string
	Message string
}

// A Validator obtains a Receipt for exact input bytes. Implementations
// live outside the reusable library; the library depends only on this
// interface.
type Validator interface {
	// Validate runs the canonical engine over input under profile and
	// returns the resulting Receipt. The context bounds the run. It
	// returns a non-nil error only when the engine could not be run or
	// its output could not be understood; an input with validation
	// errors is a successful call that returns a Receipt with
	// ErrorCount > 0.
	Validate(ctx context.Context, input []byte, profile string) (Receipt, error)
}
