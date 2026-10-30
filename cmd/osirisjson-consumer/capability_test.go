// Tests for the report and topology verbs: stdout and file output, the
// atomic private file, the JSON envelope, --force, and the exit-code
// contract. These need the pinned engine on PATH and skip without it.

package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requireEngine(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("osirisjson"); err != nil {
		t.Skipf("osirisjson not on PATH: %v", err)
	}
}

// repoFixture returns the absolute path to an in-repo testdata file.
func repoFixture(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "testdata", rel))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s: %v", rel, err)
	}
	return p
}

func TestReportToStdout(t *testing.T) {
	requireEngine(t)
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json")}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if !strings.HasPrefix(out.String(), "# OSIRIS JSON inventory brief") {
		t.Errorf("stdout is not the brief: %q", out.String()[:min(80, out.Len())])
	}
}

func TestTopologyToFileIsPrivateAndAtomic(t *testing.T) {
	requireEngine(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "topology.drawio")
	var out, errb bytes.Buffer
	code := run([]string{"topology", repoFixture(t, "valid/mixed-provider.json"), "-o", dest}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("artifact not created: %v", err)
	}
	if fi.Mode().Perm() != fs.FileMode(0o600) {
		t.Errorf("artifact mode = %v, want 0600", fi.Mode().Perm())
	}
	b, _ := os.ReadFile(dest)
	if !bytes.HasPrefix(b, []byte("<mxfile ")) {
		t.Errorf("artifact is not draw.io XML")
	}
	if out.Len() != 0 {
		t.Errorf("file output must not also write the artifact to stdout: %q", out.String())
	}
	if !strings.Contains(errb.String(), "wrote "+dest) {
		t.Errorf("no success summary on stderr: %q", errb.String())
	}
}

func TestReportForceRequiredToOverwrite(t *testing.T) {
	requireEngine(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "report.md")
	if err := os.WriteFile(dest, []byte("existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := repoFixture(t, "valid/no-provider.json")

	var o1, e1 bytes.Buffer
	if code := run([]string{"report", in, "-o", dest}, &o1, &e1); code != 2 {
		t.Fatalf("overwrite without --force: exit = %d, want 2", code)
	}
	if got, _ := os.ReadFile(dest); string(got) != "existing\n" {
		t.Errorf("file was modified without --force")
	}

	var o2, e2 bytes.Buffer
	if code := run([]string{"report", in, "-o", dest, "--force"}, &o2, &e2); code != 0 {
		t.Fatalf("overwrite with --force: exit = %d, stderr = %s", code, e2.String())
	}
	if got, _ := os.ReadFile(dest); !strings.HasPrefix(string(got), "# OSIRIS JSON inventory brief") {
		t.Errorf("--force did not overwrite with the new artifact")
	}
}

func TestJSONEnvelope(t *testing.T) {
	requireEngine(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "report.md")
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "-o", dest, "--format", "json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	var env struct {
		Capability  string `json:"capability"`
		InputSHA256 string `json:"input_sha256"`
		Validation  struct {
			EngineName string `json:"engine_name"`
			Profile    string `json:"profile"`
			ErrorCount int    `json:"error_count"`
		} `json:"validation"`
		Artifact struct {
			Path   string `json:"path"`
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"artifact"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("envelope is not valid JSON: %v\n%s", err, out.String())
	}
	if env.Capability != "report" || len(env.InputSHA256) != 64 {
		t.Errorf("envelope core fields wrong: %+v", env)
	}
	if env.Validation.EngineName == "" || env.Validation.Profile != "default" {
		t.Errorf("envelope validation block wrong: %+v", env.Validation)
	}
	if env.Artifact.Path != dest || env.Artifact.Bytes == 0 || len(env.Artifact.SHA256) != 64 {
		t.Errorf("envelope artifact block wrong: %+v", env.Artifact)
	}
	// No document values in the envelope.
	if strings.Contains(out.String(), "aws::vpc-1") || strings.Contains(out.String(), "quantum.entangler") {
		t.Errorf("envelope leaked a document value")
	}
}

func TestJSONEnvelopeRefusedToStdout(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/no-provider.json"), "--format", "json"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (json envelope may not mix with the artifact stream)", code)
	}
	if !strings.Contains(errb.String(), "requires -o FILE") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestValidationErrorExitsOneWithVerbatimCode(t *testing.T) {
	requireEngine(t)
	var out, errb bytes.Buffer
	code := run([]string{"topology", repoFixture(t, "adversarial/dangling-connection.json")}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "failed canonical validation") {
		t.Errorf("stderr = %q", errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("no artifact should be written for an invalid document")
	}
}

func TestMalformedInputExitsTwo(t *testing.T) {
	requireEngine(t)
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "adversarial/malformed-json.json")}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestMissingInputFileExitsTwo(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"report", filepath.Join(t.TempDir(), "nope.json")}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "reading input") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestUnknownFlagExitsTwo(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"report", "--bogus", repoFixture(t, "valid/no-provider.json")}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestReportOutputFormatPDFToStdout(t *testing.T) {
	requireEngine(t)
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "--output-format", "pdf"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if !bytes.HasPrefix(out.Bytes(), []byte("%PDF-1.7")) {
		t.Errorf("stdout is not a PDF: %q", out.String()[:min(20, out.Len())])
	}
}

// TestReportOutputFormatInferredFromPDFExtension confirms -o FILE.pdf
// with no explicit --output-format infers pdf, the same artifact
// --output-format pdf would have produced (checked by content, not raw
// bytes: each run stamps its own real ArtifactTime, so the two files
// legitimately differ byte-for-byte).
func TestReportOutputFormatInferredFromPDFExtension(t *testing.T) {
	requireEngine(t)
	dir := t.TempDir()
	inferred := filepath.Join(dir, "report.pdf")
	explicit := filepath.Join(dir, "report-explicit.pdf")

	var out1, errb1 bytes.Buffer
	if code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "-o", inferred}, &out1, &errb1); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb1.String())
	}
	var out2, errb2 bytes.Buffer
	if code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "-o", explicit, "--output-format", "pdf"}, &out2, &errb2); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb2.String())
	}

	for _, p := range []string{inferred, explicit} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("reading %s: %v", p, err)
		}
		if !bytes.HasPrefix(b, []byte("%PDF-1.7")) {
			t.Errorf("%s is not a PDF", p)
		}
	}
}

func TestReportOutputFormatRejectsUnknownValue(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "--output-format", "xml"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--output-format") {
		t.Errorf("stderr = %q", errb.String())
	}
}

// TestTopologyHasNoOutputFormatFlag confirms --output-format stays
// report-only: topology rejects the flag as unknown rather than
// silently accepting it.
func TestTopologyHasNoOutputFormatFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"topology", repoFixture(t, "valid/mixed-provider.json"), "--output-format", "pdf"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

// TestReportXLSXCompanionNeverWrittenWithoutARecognizedVendor confirms
// the companion-workbook concept stays vendor enrichment only, never a
// generic capability: a document with no recognized vendor
// (mixed-provider.json) asked for a companion gets the primary report
// written successfully and no companion file at all never an empty
// or placeholder .xlsx (the whole point of buffering the companion in
// memory first, rather than staging it eagerly through
// internal/artifact, is to avoid exactly that).
func TestReportXLSXCompanionNeverWrittenWithoutARecognizedVendor(t *testing.T) {
	requireEngine(t)
	dir := t.TempDir()
	mdDest := filepath.Join(dir, "report.md")
	xlsxDest := filepath.Join(dir, "report.xlsx")
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "-o", mdDest, "--xlsx-companion", xlsxDest}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if _, err := os.Stat(mdDest); err != nil {
		t.Fatalf("primary report not written: %v", err)
	}
	if _, err := os.Stat(xlsxDest); !os.IsNotExist(err) {
		t.Errorf("xlsx companion must not exist when no vendor is recognized (stat err = %v)", err)
	}
}

func TestReportXLSXCompanionRejectsSamePathAsPrimary(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "report.md")
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "-o", dest, "--xlsx-companion", dest}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--xlsx-companion") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestReportXLSXCompanionRejectsStdinSentinel(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "--xlsx-companion", "-"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestTopologyHasNoXLSXCompanionFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"topology", repoFixture(t, "valid/mixed-provider.json"), "--xlsx-companion", "whatever.xlsx"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

// TestReportJSONEnvelopeOmitsXLSXArtifactWhenNoCompanion confirms the
// envelope's xlsx_artifact field is genuinely absent (not a present-
// but-empty object) when no companion was written a caller can
// branch on its presence alone.
func TestReportJSONEnvelopeOmitsXLSXArtifactWhenNoCompanion(t *testing.T) {
	requireEngine(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "report.md")
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "-o", dest, "--format", "json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if strings.Contains(out.String(), "xlsx_artifact") {
		t.Errorf("envelope should omit xlsx_artifact entirely when no companion was written: %s", out.String())
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{6047, "5.9 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{3 * 1024 * 1024 * 1024, "3.0 GB"},
	}
	for _, tt := range tests {
		if got := formatSize(tt.n); got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Microsecond, "0.03ms"},
		{312450 * time.Microsecond, "312.45ms"},
		{time.Second, "1.00s"},
		{1234 * time.Millisecond, "1.23s"},
	}
	for _, tt := range tests {
		if got := formatElapsed(tt.d); got != tt.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
