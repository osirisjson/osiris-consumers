// Integration tests for the pinned @osirisjson/cli subprocess adapter.
// They require the "osirisjson" binary on PATH and are skipped when it
// is absent, so they do not block a Node-less build or CI lane.

package toolboxcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"go.osirisjson.org/consumers/internal/validation"
)

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func requireEngine(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(DefaultBinary); err != nil {
		t.Skipf("%s not on PATH: %v", DefaultBinary, err)
	}
}

func fixture(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", rel))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return b
}

func TestValidateCleanDocument(t *testing.T) {
	requireEngine(t)
	in := fixture(t, "valid/mixed-provider.json")

	got, err := Validator{}.Validate(context.Background(), in, "")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0 (%+v)", got.ErrorCount, got.Diagnostics)
	}
	if got.EngineName != validation.PinnedEngineName {
		t.Errorf("EngineName = %q, want %q", got.EngineName, validation.PinnedEngineName)
	}
	if got.EngineVersion == "" {
		t.Errorf("EngineVersion is empty")
	}
	if got.Profile != validation.DefaultProfile {
		t.Errorf("Profile = %q, want %q", got.Profile, validation.DefaultProfile)
	}
	wantHash := sha256Hex(in)
	if got.InputSHA256 != wantHash {
		t.Errorf("InputSHA256 = %q, want %q", got.InputSHA256, wantHash)
	}
}

func TestValidateSurfacesValidationErrorsVerbatim(t *testing.T) {
	requireEngine(t)
	cases := []struct {
		fixture string
		code    string
	}{
		{"adversarial/missing-version.json", "V-DOC-002"},
		{"adversarial/bad-semver.json", "V-DOC-003"},
		{"adversarial/bad-timestamp.json", "V-META-001"},
		{"adversarial/wrong-major-semver.json", "V-DOC-004"},
		{"adversarial/dangling-connection.json", "V-REF-002"},
		{"adversarial/duplicate-resource-id.json", "V-ID-001"},
		{"adversarial/provider-namespace-misuse.json", "V-RES-001"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			got, err := Validator{}.Validate(context.Background(), fixture(t, tc.fixture), "default")
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if got.ErrorCount == 0 {
				t.Fatalf("ErrorCount = 0, want > 0")
			}
			if !hasCode(got.Diagnostics, tc.code) {
				t.Errorf("diagnostics %+v missing code %q", got.Diagnostics, tc.code)
			}
		})
	}
}

func TestValidateProducerFixturesAreClean(t *testing.T) {
	requireEngine(t)
	for _, f := range []string{
		"producer/network/cisco/apic/cisco-apic.json",
		"producer/network/cisco/nxos/cisco-nxos.json",
		"producer/network/cisco/vmanage/cisco-vmanage.json",
		"producer/network/hpe/arubacentral/hpe-arubacentral.json",
	} {
		t.Run(f, func(t *testing.T) {
			got, err := Validator{}.Validate(context.Background(), fixture(t, f), "default")
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if got.ErrorCount != 0 {
				t.Fatalf("ErrorCount = %d, want 0: %+v", got.ErrorCount, got.Diagnostics)
			}
		})
	}
}

func TestValidateMalformedJSONIsAnError(t *testing.T) {
	requireEngine(t)
	_, err := Validator{}.Validate(context.Background(), fixture(t, "adversarial/malformed-json.json"), "default")
	if !errors.Is(err, ErrMalformedInput) {
		t.Fatalf("error = %v, want ErrMalformedInput", err)
	}
}

func TestValidateHonoursCancelledContext(t *testing.T) {
	requireEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Validator{}.Validate(ctx, fixture(t, "valid/no-provider.json"), "default")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestValidateMissingBinaryIsUnavailable(t *testing.T) {
	_, err := Validator{Binary: "osirisjson-does-not-exist-xyz"}.
		Validate(context.Background(), []byte(`{}`), "default")
	if !errors.Is(err, ErrEngineUnavailable) {
		t.Fatalf("error = %v, want ErrEngineUnavailable", err)
	}
}

func hasCode(ds []validation.Diagnostic, code string) bool {
	for _, d := range ds {
		if d.Code == code {
			return true
		}
	}
	return false
}
