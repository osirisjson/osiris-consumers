// End-to-end tests for the experimental facade: the safe path from
// exact validated input through the generic graph to a rendered
// artifact, the exit-code contract, and the failure paths that must
// never reach a renderer.

package consumer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/validation"
)

func fixtureBytes(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", rel))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", rel, err)
	}
	return b
}

// fakeValidator stands in for the canonical engine. By default it
// returns a clean receipt whose hash matches the exact bytes it was
// given, so tests can drive the post-validation path without Node.
type fakeValidator struct {
	receipt   validation.Receipt
	err       error
	called    *bool
	fixedHash string
}

func (f fakeValidator) Validate(_ context.Context, input []byte, profile string) (validation.Receipt, error) {
	if f.called != nil {
		*f.called = true
	}
	if f.err != nil {
		return validation.Receipt{}, f.err
	}
	r := f.receipt
	r.EngineName = validation.PinnedEngineName
	r.EngineVersion = validation.PinnedEngineVersion
	r.Profile = profile
	if f.fixedHash != "" {
		r.InputSHA256 = f.fixedHash
	} else if r.InputSHA256 == "" {
		s := sha256.Sum256(input)
		r.InputSHA256 = hex.EncodeToString(s[:])
	}
	return r, nil
}

func cleanReq(t *testing.T, rel string) Request {
	return Request{
		Input:   fixtureBytes(t, rel),
		Output:  io.Discard,
		Options: Options{Validator: fakeValidator{}},
	}
}

func TestGenerateReportRendersMarkdown(t *testing.T) {
	var buf bytes.Buffer
	req := cleanReq(t, "valid/no-provider.json")
	req.Output = &buf
	res, err := GenerateReport(context.Background(), req)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !strings.HasPrefix(buf.String(), "# OSIRIS JSON inventory brief\n") {
		t.Errorf("markdown = %q...", buf.String()[:min(80, buf.Len())])
	}
	if len(res.InputSHA256) != 64 {
		t.Errorf("InputSHA256 = %q", res.InputSHA256)
	}
	if res.Validation.EngineName != validation.PinnedEngineName {
		t.Errorf("receipt not carried into Result: %+v", res.Validation)
	}
	if res.Artifact.Bytes != int64(buf.Len()) || len(res.Artifact.SHA256) != 64 {
		t.Errorf("artifact summary = %+v (buf %d bytes)", res.Artifact, buf.Len())
	}
	if len(res.GraphAnomalies) != 0 {
		t.Errorf("unexpected anomalies: %+v", res.GraphAnomalies)
	}
}

func TestGenerateTopologyMixedProviderRendersCompleteGraph(t *testing.T) {
	var buf bytes.Buffer
	req := cleanReq(t, "valid/mixed-provider.json")
	req.Output = &buf
	res, err := GenerateTopology(context.Background(), req)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !strings.Contains(buf.String(), "<mxfile") || !strings.Contains(buf.String(), "</mxfile>") {
		t.Errorf("draw.io output not well framed: %q", buf.String())
	}
	if len(res.GraphAnomalies) != 0 {
		t.Errorf("mixed-provider fixture should be anomaly-free: %+v", res.GraphAnomalies)
	}
}

func TestUnknownFieldsAndLaterMinorAccepted(t *testing.T) {
	_, err := GenerateReport(context.Background(), cleanReq(t, "valid/unknown-fields.json"))
	if err != nil {
		t.Fatalf("err = %v, want nil (forward-compatible doc must not be rejected)", err)
	}
}

func TestNilOutputRejectedBeforeAnyWork(t *testing.T) {
	called := false
	req := Request{
		Input:   fixtureBytes(t, "valid/no-provider.json"),
		Options: Options{Validator: fakeValidator{called: &called}},
	}
	_, err := GenerateReport(context.Background(), req)
	if !errors.Is(err, ErrNoOutput) {
		t.Fatalf("err = %v, want ErrNoOutput", err)
	}
	if ExitCode(err) != 2 {
		t.Errorf("ExitCode = %d, want 2", ExitCode(err))
	}
	if called {
		t.Errorf("validator ran despite a nil Output")
	}
}

func TestValidationErrorsAreRefusedWithExitOne(t *testing.T) {
	req := Request{
		Input:  fixtureBytes(t, "valid/no-provider.json"),
		Output: io.Discard,
		Options: Options{Validator: fakeValidator{
			receipt: validation.Receipt{ErrorCount: 2},
		}},
	}
	res, err := GenerateReport(context.Background(), req)
	if !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("err = %v, want ErrValidationFailed", err)
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1", ExitCode(err))
	}
	// The receipt travels with the error so an adapter can print the
	// verbatim canonical diagnostics.
	if res.InputSHA256 == "" || res.Validation.ErrorCount != 2 {
		t.Errorf("failed-validation Result should still carry the receipt: %+v", res)
	}
}

func TestUnsupportedMajorResultCarriesReceipt(t *testing.T) {
	req := Request{
		Input:  fixtureBytes(t, "adversarial/wrong-major-semver.json"),
		Output: io.Discard,
		Options: Options{Validator: fakeValidator{
			receipt: validation.Receipt{
				Diagnostics: []Diagnostic{{Code: "V-DOC-004", Severity: "error", Path: "/version"}},
			},
		}},
	}
	res, err := GenerateReport(context.Background(), req)
	if !errors.Is(err, ErrUnsupportedOSIRIS) {
		t.Fatalf("err = %v, want ErrUnsupportedOSIRIS", err)
	}
	if res.InputSHA256 == "" || len(res.Validation.Diagnostics) != 1 {
		t.Errorf("unsupported-major Result should still carry the receipt: %+v", res)
	}
}

func TestMalformedInputRejectedBeforeValidation(t *testing.T) {
	called := false
	req := Request{
		Input:   fixtureBytes(t, "adversarial/malformed-json.json"),
		Output:  io.Discard,
		Options: Options{Validator: fakeValidator{called: &called}},
	}
	_, err := GenerateReport(context.Background(), req)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if ExitCode(err) != 2 {
		t.Errorf("ExitCode = %d, want 2", ExitCode(err))
	}
	if called {
		t.Errorf("validator was called on malformed input")
	}
}

func TestTrailingJSONRejectedBeforeValidation(t *testing.T) {
	called := false
	req := Request{
		Input:   fixtureBytes(t, "adversarial/trailing-json.json"),
		Output:  io.Discard,
		Options: Options{Validator: fakeValidator{called: &called}},
	}
	_, err := GenerateTopology(context.Background(), req)
	if !errors.Is(err, ErrInvalidInput) || ExitCode(err) != 2 {
		t.Fatalf("err = %v (exit %d), want ErrInvalidInput exit 2", err, ExitCode(err))
	}
	if called {
		t.Errorf("validator was called on trailing-data input")
	}
}

func TestOversizeInputRejected(t *testing.T) {
	req := Request{
		Input:   fixtureBytes(t, "valid/mixed-provider.json"),
		Output:  io.Discard,
		Options: Options{Validator: fakeValidator{}, MaxInputBytes: 64},
	}
	_, err := GenerateReport(context.Background(), req)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput for oversize", err)
	}
}

func TestNoReceiptAndNoValidator(t *testing.T) {
	req := Request{Input: fixtureBytes(t, "valid/no-provider.json"), Output: io.Discard}
	_, err := GenerateReport(context.Background(), req)
	if !errors.Is(err, ErrNoReceipt) {
		t.Fatalf("err = %v, want ErrNoReceipt", err)
	}
}

func TestReceiptHashMismatchRejected(t *testing.T) {
	req := Request{
		Input:  fixtureBytes(t, "valid/no-provider.json"),
		Output: io.Discard,
		Options: Options{Validator: fakeValidator{
			fixedHash: strings.Repeat("00", 32),
		}},
	}
	_, err := GenerateReport(context.Background(), req)
	if !errors.Is(err, ErrReceiptMismatch) {
		t.Fatalf("err = %v, want ErrReceiptMismatch", err)
	}
	if ExitCode(err) != 2 {
		t.Errorf("ExitCode = %d, want 2", ExitCode(err))
	}
}

func TestPreSuppliedReceiptPath(t *testing.T) {
	// Normalise to the exact single-value bytes and hash those, exactly
	// as a Desktop caller would after its own validation.
	raw := fixtureBytes(t, "valid/no-provider.json")
	view, err := OpenValidated(mustReadValue(t, raw), receiptFor(mustReadValue(t, raw)))
	if err != nil {
		t.Fatalf("OpenValidated: %v", err)
	}
	if view.Receipt().ErrorCount != 0 {
		t.Fatalf("receipt not clean")
	}

	var buf bytes.Buffer
	req := Request{
		Input:   raw,
		Output:  &buf,
		Options: Options{Receipt: ptr(receiptFor(mustReadValue(t, raw)))},
	}
	_, err = GenerateTopology(context.Background(), req)
	if err != nil {
		t.Fatalf("err = %v, want nil on the pre-supplied receipt path", err)
	}
	if !strings.Contains(buf.String(), "<mxfile") {
		t.Errorf("no draw.io output on the pre-supplied receipt path")
	}
}

func TestUnsupportedMajorRejectedWithExitOne(t *testing.T) {
	req := Request{
		Input:   fixtureBytes(t, "adversarial/wrong-major-semver.json"),
		Output:  io.Discard,
		Options: Options{Validator: fakeValidator{}},
	}
	_, err := GenerateReport(context.Background(), req)
	if !errors.Is(err, ErrUnsupportedOSIRIS) {
		t.Fatalf("err = %v, want ErrUnsupportedOSIRIS", err)
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1", ExitCode(err))
	}
}

func TestGraphAnomaliesSurfacedNotFatal(t *testing.T) {
	// A clean receipt over a document whose internal references are
	// broken (as if validation had been run at a laxer profile). The
	// artifact still renders; the anomalies ride along on the Result.
	var buf bytes.Buffer
	req := Request{
		Input:   fixtureBytes(t, "adversarial/dangling-connection.json"),
		Output:  &buf,
		Options: Options{Validator: fakeValidator{}},
	}
	res, err := GenerateReport(context.Background(), req)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(res.GraphAnomalies) == 0 {
		t.Fatalf("dangling reference not surfaced")
	}
	if res.GraphAnomalies[0].Kind != "dangling-connection-target" {
		t.Errorf("anomaly = %+v", res.GraphAnomalies[0])
	}
	if !strings.Contains(buf.String(), "dangling-connection-target") {
		t.Errorf("anomaly not rendered into the brief")
	}
}

func TestProgressEventsAreEmittedInOrder(t *testing.T) {
	var stages []Stage
	req := cleanReq(t, "valid/no-provider.json")
	req.Progress = func(p Progress) { stages = append(stages, p.Stage) }

	if _, err := GenerateReport(context.Background(), req); err != nil {
		t.Fatalf("err = %v", err)
	}

	want := []Stage{StageValidating, StageValidating, StageOpening, StageIndexing, StageRendering, StageFinalizing}
	if len(stages) != len(want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	for i := range want {
		if stages[i] != want[i] {
			t.Fatalf("stage[%d] = %v, want %v (%v)", i, stages[i], want[i], stages)
		}
	}
}

func TestCancellationStopsThePipeline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GenerateReport(ctx, cleanReq(t, "valid/mixed-provider.json"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestArtifactTimeAbsentKeepsOutputClockFree(t *testing.T) {
	var withTime, without bytes.Buffer

	req := cleanReq(t, "valid/mixed-provider.json")
	req.Output = &without
	if _, err := GenerateReport(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(without.String(), "Generated ") {
		t.Errorf("zero ArtifactTime should not stamp a generated line")
	}

	req2 := cleanReq(t, "valid/mixed-provider.json")
	req2.Output = &withTime
	req2.Options.ArtifactTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := GenerateReport(context.Background(), req2); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(withTime.String(), "Generated 2026-01-02T03:04:05Z") {
		t.Errorf("injected ArtifactTime not stamped: %q", firstLine(withTime.String(), 3))
	}
}

// helpers

func firstLine(s string, n int) string {
	parts := strings.SplitN(s, "\n", n+1)
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, "\n")
}

func mustReadValue(t *testing.T, b []byte) []byte {
	t.Helper()
	v, err := readBytes(Request{Input: b})
	if err != nil {
		t.Fatalf("readBytes: %v", err)
	}
	return v
}

func receiptFor(exact []byte) ValidationReceipt {
	s := sha256.Sum256(exact)
	return ValidationReceipt{
		InputSHA256:          hex.EncodeToString(s[:]),
		EngineName:           validation.PinnedEngineName,
		EngineVersion:        validation.PinnedEngineVersion,
		Profile:              validation.DefaultProfile,
		SpecificationVersion: validation.PinnedSpecVersion,
	}
}

func ptr[T any](v T) *T { return &v }
