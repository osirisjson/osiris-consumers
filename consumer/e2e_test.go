// End-to-end test through the real pinned @osirisjson/cli. Skipped when
// the engine is not on PATH, so a Node-less environment still passes.

package consumer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"go.osirisjson.org/consumers/internal/validation/toolboxcli"
)

func requireEngine(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("osirisjson"); err != nil {
		t.Skipf("osirisjson not on PATH: %v", err)
	}
}

func TestEndToEndCleanDocument(t *testing.T) {
	requireEngine(t)
	var buf bytes.Buffer
	req := Request{
		Input:   fixtureBytes(t, "valid/mixed-provider.json"),
		Output:  &buf,
		Options: Options{Validator: toolboxcli.Validator{}},
	}
	res, err := GenerateReport(context.Background(), req)
	if err != nil {
		t.Fatalf("err = %v, want nil after a clean run", err)
	}
	if !strings.HasPrefix(buf.String(), "# OSIRIS JSON inventory brief") {
		t.Errorf("no Markdown brief on stdout")
	}
	if res.Validation.EngineName != "@osirisjson/cli" || res.Validation.ErrorCount != 0 {
		t.Errorf("receipt = %+v", res.Validation)
	}
	if res.InputSHA256 == "" || res.InputSHA256 != res.Validation.InputSHA256 {
		t.Errorf("input hash mismatch: result %q receipt %q", res.InputSHA256, res.Validation.InputSHA256)
	}
	if res.Artifact.Bytes != int64(buf.Len()) || len(res.Artifact.SHA256) != 64 {
		t.Errorf("artifact summary = %+v", res.Artifact)
	}
}

func TestEndToEndValidationErrorExitsOne(t *testing.T) {
	requireEngine(t)
	req := Request{
		Input:   fixtureBytes(t, "adversarial/dangling-connection.json"),
		Output:  io.Discard,
		Options: Options{Validator: toolboxcli.Validator{}},
	}
	_, err := GenerateTopology(context.Background(), req)
	if !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("err = %v, want ErrValidationFailed", err)
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1", ExitCode(err))
	}
}

func TestEndToEndProducerFixtures(t *testing.T) {
	requireEngine(t)
	for _, f := range []string{
		"producer/network/cisco/nxos/cisco-nxos.json",
	} {
		t.Run(f, func(t *testing.T) {
			var report, topo bytes.Buffer
			res, err := GenerateReport(context.Background(), Request{
				Input:   fixtureBytes(t, f),
				Output:  &report,
				Options: Options{Validator: toolboxcli.Validator{}},
			})
			if err != nil {
				t.Fatalf("report err = %v, want nil", err)
			}
			if res.Validation.ErrorCount != 0 || len(res.GraphAnomalies) != 0 {
				t.Errorf("clean producer snapshot: errors=%d anomalies=%+v",
					res.Validation.ErrorCount, res.GraphAnomalies)
			}
			// Whichever renderer the facade auto-selected for this
			// document (generic, or a vendor enrichment for its own
			// homogeneous-provider fixture) must have produced a real
			// report, not an empty or malformed one.
			if !strings.HasPrefix(report.String(), "# OSIRIS JSON inventory brief") &&
				!strings.HasPrefix(report.String(), "# OSIRIS JSON Cisco NX-OS report") {
				t.Errorf("no brief rendered")
			}
			if _, err := GenerateTopology(context.Background(), Request{
				Input:   fixtureBytes(t, f),
				Output:  &topo,
				Options: Options{Validator: toolboxcli.Validator{}},
			}); err != nil {
				t.Fatalf("topology err = %v, want nil", err)
			}
			if !strings.Contains(topo.String(), "</mxfile>") {
				t.Errorf("draw.io output not complete")
			}
		})
	}
}

func TestEndToEndProviderNamespaceMisuseSurfacesCode(t *testing.T) {
	requireEngine(t)
	res, err := GenerateReport(context.Background(), Request{
		Input:   fixtureBytes(t, "adversarial/provider-namespace-misuse.json"),
		Output:  io.Discard,
		Options: Options{Validator: toolboxcli.Validator{}},
	})
	if !errors.Is(err, ErrValidationFailed) || ExitCode(err) != 1 {
		t.Fatalf("err = %v (exit %d), want ErrValidationFailed exit 1", err, ExitCode(err))
	}
	var found bool
	for _, d := range res.Validation.Diagnostics {
		if d.Code == "V-RES-001" {
			found = true
		}
	}
	if !found {
		t.Errorf("V-RES-001 not surfaced on the failed Result: %+v", res.Validation.Diagnostics)
	}
}

func TestEndToEndMalformedJSONExitsTwo(t *testing.T) {
	requireEngine(t)
	req := Request{
		Input:   fixtureBytes(t, "adversarial/malformed-json.json"),
		Output:  io.Discard,
		Options: Options{Validator: toolboxcli.Validator{}},
	}
	_, err := GenerateReport(context.Background(), req)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if ExitCode(err) != 2 {
		t.Errorf("ExitCode = %d, want 2", ExitCode(err))
	}
}
