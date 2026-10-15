// Package toolboxcli implements the v0.1.0 canonical-validation
// mechanism: a subprocess to a pinned @osirisjson/cli.
//
// It shells out to "osirisjson validate --format json", feeds the
// exact input bytes on stdin, and turns the structured JSON result into
// a validation.Receipt. Node and the pinned @osirisjson/cli are a
// documented prerequisite of the CLI adapter; the reusable Consumers
// library never imports this package.
//
// The CLI's own exit codes are mapped through unchanged: 0 is a clean
// document, 1 is valid JSON with OSIRIS JSON validation errors
// (a Receipt with ErrorCount > 0), and 2 is malformed JSON or an
// operational failure (a non-nil error from Validate).
package toolboxcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"go.osirisjson.org/consumers/internal/validation"
)

// DefaultBinary is the command looked up on PATH when Validator.Binary
// is empty.
const DefaultBinary = "osirisjson"

// ErrMalformedInput means the engine rejected the bytes as not being a
// single well-formed JSON value (CLI exit code 2, "invalid_json"). The
// CLI adapter maps it to its own exit code 2.
var ErrMalformedInput = errors.New("toolboxcli: input is not a single well-formed JSON value")

// ErrEngineUnavailable means the engine could not be executed at all
// (not on PATH, not runnable).
var ErrEngineUnavailable = errors.New("toolboxcli: canonical validation engine is not available")

// ErrEngineOutput means the engine ran but produced output this adapter
// could not understand.
var ErrEngineOutput = errors.New("toolboxcli: could not parse engine output")

// A Validator runs the pinned @osirisjson/cli as a subprocess. The zero
// value is usable and resolves the engine as DefaultBinary on PATH.
type Validator struct {
	// Binary overrides the engine command. Empty selects DefaultBinary.
	Binary string
	// RequirePinnedVersion, when true, makes Validate fail if the
	// engine's reported version is not validation.PinnedEngineVersion.
	// When false a mismatch is recorded on the Receipt's engine
	// version verbatim and the run proceeds.
	RequirePinnedVersion bool
}

// cliResult mirrors the JSON document "osirisjson validate --format
// json" writes for a single input.
type cliResult struct {
	Status  string `json:"status"`
	Valid   bool   `json:"valid"`
	Profile string `json:"profile"`
	Counts  struct {
		Error   int `json:"error"`
		Warning int `json:"warning"`
		Info    int `json:"info"`
	} `json:"counts"`
	Diagnostics []struct {
		Code     string `json:"code"`
		Stage    int    `json:"stage"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
		Path     string `json:"path"`
		Related  []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"related"`
	} `json:"diagnostics"`
	OperationalError *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"operationalError"`
}

// Validate implements validation.Validator by running the engine over
// input under profile.
func (v Validator) Validate(ctx context.Context, input []byte, profile string) (validation.Receipt, error) {
	bin := v.Binary
	if bin == "" {
		bin = DefaultBinary
	}
	if profile == "" {
		profile = validation.DefaultProfile
	}
	if err := ctx.Err(); err != nil {
		return validation.Receipt{}, err
	}

	engineVersion, err := v.engineVersion(ctx, bin)
	if err != nil {
		return validation.Receipt{}, err
	}
	if v.RequirePinnedVersion && engineVersion != validation.PinnedEngineVersion {
		return validation.Receipt{}, fmt.Errorf(
			"toolboxcli: engine version %q is not the pinned %q",
			engineVersion, validation.PinnedEngineVersion)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "validate", "--profile", profile, "--format", "json", "--color", "never", "-")
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return validation.Receipt{}, ctx.Err()
	}
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return validation.Receipt{}, fmt.Errorf("%w: %v", ErrEngineUnavailable, runErr)
	}

	var res cliResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &res); err != nil {
		return validation.Receipt{}, fmt.Errorf("%w: %v (stderr: %s)",
			ErrEngineOutput, err, strings.TrimSpace(stderr.String()))
	}

	if res.OperationalError != nil {
		return validation.Receipt{}, fmt.Errorf("%w: %s", ErrMalformedInput, res.OperationalError.Message)
	}

	sum := sha256.Sum256(input)
	receipt := validation.Receipt{
		InputSHA256:          hex.EncodeToString(sum[:]),
		EngineName:           validation.PinnedEngineName,
		EngineVersion:        engineVersion,
		Profile:              orString(res.Profile, profile),
		SpecificationVersion: validation.PinnedSpecVersion,
		ErrorCount:           res.Counts.Error,
		WarningCount:         res.Counts.Warning,
	}
	for _, d := range res.Diagnostics {
		diag := validation.Diagnostic{
			Code:     d.Code,
			Stage:    d.Stage,
			Severity: d.Severity,
			Message:  d.Message,
			Path:     d.Path,
		}
		for _, rel := range d.Related {
			diag.Related = append(diag.Related, validation.Related{Path: rel.Path, Message: rel.Message})
		}
		receipt.Diagnostics = append(receipt.Diagnostics, diag)
	}
	return receipt, nil
}

// engineVersion returns the engine's reported "--version" string.
func (v Validator) engineVersion(ctx context.Context, bin string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "--version")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%w: %v", ErrEngineUnavailable, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func orString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
