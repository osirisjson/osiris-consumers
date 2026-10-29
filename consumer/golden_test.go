// Golden-file tests: the report and topology artifacts for a fixed set
// of in-repo fixtures, rendered with a fixed artifact time and a
// pre-supplied clean receipt (no engine needed), must match the
// committed bytes under testdata/golden/. Run with update to
// regenerate after an intentional change, then review the diff.

package consumer

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden artifacts under testdata/golden/")

// goldenTime is the fixed artifact time stamped into every golden.
var goldenTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// goldenFixtures is the representative subset: no provider, a mixed
// multi-provider document, adversarial injection strings, a document
// with unresolved references, and one neutralised snapshot per
// active OSIRIS JSON producer.
var goldenFixtures = []string{
	"valid/no-provider.json",
	"valid/mixed-provider.json",
	"valid/unknown-fields.json",
	"rendering/injection-strings.json",
	"adversarial/dangling-connection.json",
	"producer/network/cisco/nxos/cisco-nxos.json",
}

func TestGoldenArtifacts(t *testing.T) {
	for _, fx := range goldenFixtures {
		fx := fx
		stem := goldenStem(fx)
		raw := fixtureBytes(t, fx)
		exact := mustReadValue(t, raw)
		receipt := receiptFor(exact)

		for _, c := range []struct {
			name string
			ext  string
			fn   capabilityFn
		}{
			{"report", "report.md", GenerateReport},
			{"topology", "topology.drawio", GenerateTopology},
		} {
			t.Run(stem+"/"+c.name, func(t *testing.T) {
				var buf bytes.Buffer
				_, err := c.fn(context.Background(), Request{
					Input:  raw,
					Output: &buf,
					Options: Options{
						Receipt:      ptr(receipt),
						ArtifactTime: goldenTime,
					},
				})
				if err != nil {
					t.Fatalf("%s: %v", c.name, err)
				}
				path := filepath.Join("..", "testdata", "golden", stem+"."+c.ext)
				if *updateGolden {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("reading golden (run: go test ./consumer -run TestGoldenArtifacts -update): %v", err)
				}
				if !bytes.Equal(want, buf.Bytes()) {
					t.Errorf("%s artifact for %s differs from the golden; re-run with -update and review the diff", c.name, fx)
				}
			})
		}
	}
}

// TestGoldenDeterminismAcrossProcesses is the two-fresh-processes check
// in one process: render each fixture twice and require byte identity.
func TestGoldenDeterminismRepeatable(t *testing.T) {
	for _, fx := range goldenFixtures {
		raw := fixtureBytes(t, fx)
		receipt := receiptFor(mustReadValue(t, raw))
		req := func(w *bytes.Buffer) Request {
			return Request{Input: raw, Output: w, Options: Options{Receipt: ptr(receipt), ArtifactTime: goldenTime}}
		}
		var a, b bytes.Buffer
		if _, err := GenerateReport(context.Background(), req(&a)); err != nil {
			t.Fatal(err)
		}
		if _, err := GenerateReport(context.Background(), req(&b)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Errorf("%s: report not reproducible", fx)
		}
		var c, d bytes.Buffer
		if _, err := GenerateTopology(context.Background(), req(&c)); err != nil {
			t.Fatal(err)
		}
		if _, err := GenerateTopology(context.Background(), req(&d)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(c.Bytes(), d.Bytes()) {
			t.Errorf("%s: topology not reproducible", fx)
		}
	}
}

type capabilityFn func(context.Context, Request) (Result, error)

func goldenStem(fixture string) string {
	base := filepath.Base(fixture)
	return base[:len(base)-len(filepath.Ext(base))]
}
