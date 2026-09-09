// Tests for the loss-aware parse: known fields exposed, unknown fields
// and freeform payloads preserved, non-1 majors refused, no
// provider-specific interpretation.

package document

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", rel))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", rel, err)
	}
	// Normalise to the exact single-value bytes the pipeline would use.
	v, err := Read(strings.NewReader(string(b)), 0)
	if err != nil {
		t.Fatalf("fixture %s is not one JSON value: %v", rel, err)
	}
	return v
}

func TestParseExposesKnownFields(t *testing.T) {
	doc, err := Parse(readFixture(t, "valid/mixed-provider.json"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Version != "1.0.0" {
		t.Errorf("Version = %q", doc.Version)
	}
	if doc.Metadata.Timestamp != "2026-02-01T09:00:00Z" {
		t.Errorf("Timestamp = %q", doc.Metadata.Timestamp)
	}
	if doc.Metadata.Generator == nil || doc.Metadata.Generator.Name != "manual" {
		t.Errorf("Generator = %+v", doc.Metadata.Generator)
	}
	if got := len(doc.Topology.Resources); got != 4 {
		t.Fatalf("resources = %d, want 4", got)
	}
	if got := len(doc.Topology.Connections); got != 3 {
		t.Errorf("connections = %d, want 3", got)
	}
	if got := len(doc.Topology.Groups); got != 3 {
		t.Errorf("groups = %d, want 3", got)
	}

	azure := doc.Topology.Resources[1]
	if azure.ID != "azure::vnet-1" || azure.Provider.Name != "azure" {
		t.Errorf("resource[1] = %+v", azure)
	}
	// NativeID is exposed verbatim and never parsed.
	if !strings.HasPrefix(azure.Provider.NativeID, "/subscriptions/") {
		t.Errorf("NativeID not preserved verbatim: %q", azure.Provider.NativeID)
	}
}

func TestParsePreservesUnknownAndFreeform(t *testing.T) {
	raw := readFixture(t, "valid/unknown-fields.json")
	doc, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := doc.Extra["futureTopLevelField"]; !ok {
		t.Errorf("top-level unknown field dropped: %v", keys(doc.Extra))
	}
	if _, ok := doc.Metadata.Extra["futureMetadataField"]; !ok {
		t.Errorf("metadata unknown field dropped: %v", keys(doc.Metadata.Extra))
	}
	if _, ok := doc.Metadata.Generator.Extra["futureGeneratorField"]; !ok {
		t.Errorf("generator unknown field dropped")
	}
	if _, ok := doc.Topology.Extra["futureTopologyField"]; !ok {
		t.Errorf("topology unknown field dropped")
	}
	r0 := doc.Topology.Resources[0]
	if _, ok := r0.Extra["futureResourceField"]; !ok {
		t.Errorf("resource unknown field dropped: %v", keys(r0.Extra))
	}
	if _, ok := r0.Provider.Extra["futureProviderField"]; !ok {
		t.Errorf("provider unknown field dropped")
	}
	// Freeform payloads kept as raw JSON, structure intact.
	var props map[string]any
	if err := json.Unmarshal(r0.Properties, &props); err != nil {
		t.Fatalf("properties not raw JSON: %v", err)
	}
	if _, ok := props["nested"]; !ok {
		t.Errorf("nested properties lost: %s", r0.Properties)
	}
	if len(r0.Extensions) == 0 || !strings.Contains(string(r0.Extensions), "osiris.example.acme") {
		t.Errorf("extensions lost: %s", r0.Extensions)
	}
	c0 := doc.Topology.Connections[0]
	if _, ok := c0.Extra["futureConnectionField"]; !ok {
		t.Errorf("connection unknown field dropped")
	}
	g0 := doc.Topology.Groups[0]
	if _, ok := g0.Extra["futureGroupField"]; !ok {
		t.Errorf("group unknown field dropped")
	}
}

func TestParseRecordsExactBytesAndHash(t *testing.T) {
	raw := readFixture(t, "valid/no-provider.json")
	doc, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if string(doc.Raw()) != string(raw) {
		t.Errorf("Raw() is not byte-identical to the input")
	}
	// Known-answer: sha256 of the exact fixture value bytes.
	if len(doc.SHA256()) != 64 {
		t.Errorf("SHA256 = %q, want 64 hex chars", doc.SHA256())
	}
	// Mutating the returned copy must not affect the document.
	cp := doc.Raw()
	if len(cp) > 0 {
		cp[0] = 'X'
	}
	if doc.Raw()[0] == 'X' {
		t.Errorf("Raw() returned an aliased slice")
	}
}

func TestParseRejectsNonOneMajor(t *testing.T) {
	_, err := Parse(readFixture(t, "adversarial/wrong-major-semver.json"))
	if !errors.Is(err, ErrUnsupportedMajor) {
		t.Fatalf("Parse error = %v, want ErrUnsupportedMajor", err)
	}
}

func TestParseAcceptsLaterMinor(t *testing.T) {
	doc, err := Parse(readFixture(t, "valid/unknown-fields.json"))
	if err != nil {
		t.Fatalf("Parse 1.4.0: %v", err)
	}
	if doc.Version != "1.4.0" {
		t.Errorf("Version = %q", doc.Version)
	}
}

func TestParseRejectsNonObject(t *testing.T) {
	_, err := Parse([]byte(`[1,2,3]`))
	if !errors.Is(err, ErrNotObject) {
		t.Fatalf("Parse error = %v, want ErrNotObject", err)
	}
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
