// Tests for "report --all flag": the full artifact set in one run, the
// pre-flight destination check that leaves the directory untouched, the
// JSON envelope on standard output, and the flag combinations --all
// refuses. The rendering cases need the pinned engine on PATH and skip
// without it.

package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportAllWritesEveryArtifact(t *testing.T) {
	requireEngine(t)
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "--all", "--out-dir", dir}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("--all must not write to stdout without --format json: %q", out.String())
	}
	tests := []struct {
		name   string
		prefix string
	}{
		{"mixed-provider.md", "# OSIRIS JSON inventory brief"},
		{"mixed-provider.pdf", "%PDF-1.7"},
		{"mixed-provider.drawio", "<mxfile "},
	}
	for _, tt := range tests {
		p := filepath.Join(dir, tt.name)
		fi, err := os.Stat(p)
		if err != nil {
			t.Errorf("%s not written: %v", tt.name, err)
			continue
		}
		if fi.Mode().Perm() != fs.FileMode(0o600) {
			t.Errorf("%s mode = %v, want 0600", tt.name, fi.Mode().Perm())
		}
		b, _ := os.ReadFile(p)
		if !bytes.HasPrefix(b, []byte(tt.prefix)) {
			t.Errorf("%s does not start with %q", tt.name, tt.prefix)
		}
		if !strings.Contains(errb.String(), "wrote "+p) {
			t.Errorf("no success line for %s on stderr: %q", tt.name, errb.String())
		}
	}
	// No recognized vendor: the companion workbook is never written.
	if _, err := os.Stat(filepath.Join(dir, "mixed-provider.xlsx")); err == nil {
		t.Error("an .xlsx companion was written for a document with no recognized vendor")
	}
}

// TestReportAllRefusesExistingFileBeforeAnyWrite confirms one
// conflicting destination leaves the whole directory untouched, and
// --force then replaces it.
func TestReportAllRefusesExistingFileBeforeAnyWrite(t *testing.T) {
	requireEngine(t)
	dir := t.TempDir()
	existing := filepath.Join(dir, "mixed-provider.drawio")
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"report", repoFixture(t, "valid/mixed-provider.json"), "--all", "--out-dir", dir}

	var out, errb bytes.Buffer
	if code := run(args, &out, &errb); code != 2 {
		t.Fatalf("exit = %d, want 2; stderr = %s", code, errb.String())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("directory changed on refusal: %d entries", len(entries))
	}
	if b, _ := os.ReadFile(existing); string(b) != "keep" {
		t.Error("existing file was modified without --force")
	}

	errb.Reset()
	if code := run(append(args, "--force"), &out, &errb); code != 0 {
		t.Fatalf("--force exit = %d, stderr = %s", code, errb.String())
	}
	if b, _ := os.ReadFile(existing); !bytes.HasPrefix(b, []byte("<mxfile ")) {
		t.Error("--force did not replace the existing file")
	}
}

func TestReportAllJSONEnvelopeOnStdout(t *testing.T) {
	requireEngine(t)
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := run([]string{"report", repoFixture(t, "valid/mixed-provider.json"), "--all", "--out-dir", dir, "--format", "json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	var env allEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not the JSON envelope: %v", err)
	}
	var kinds []string
	for _, a := range env.Artifacts {
		kinds = append(kinds, a.Kind)
		if a.Path == "" || a.SHA256 == "" || a.Bytes == 0 {
			t.Errorf("incomplete artifact entry: %+v", a)
		}
	}
	if got := strings.Join(kinds, ","); got != "markdown,pdf,drawio" {
		t.Errorf("artifact kinds = %s, want markdown,pdf,drawio", got)
	}
	if env.InputSHA256 == "" || env.Validation.EngineName == "" {
		t.Errorf("envelope header not populated: %+v", env)
	}
}

func TestReportAllRejectsConflictingFlags(t *testing.T) {
	input := repoFixture(t, "valid/mixed-provider.json")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"with -o", []string{"--all", "-o", "x.md"}, "-o cannot be combined with --all"},
		{"with --output-format", []string{"--all", "--output-format", "pdf"}, "-output-format cannot be combined with --all"},
		{"with --xlsx-companion", []string{"--all", "--xlsx-companion", "x.xlsx"}, "-xlsx-companion cannot be combined with --all"},
		{"--out-dir alone", []string{"--out-dir", "."}, "--out-dir requires --all"},
		{"missing out-dir", []string{"--all", "--out-dir", filepath.Join(t.TempDir(), "absent")}, "is not an existing directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := run(append([]string{"report", input}, tt.args...), &out, &errb)
			if code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if !strings.Contains(errb.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", errb.String(), tt.want)
			}
		})
	}
}

func TestTopologyHasNoAllFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"topology", repoFixture(t, "valid/mixed-provider.json"), "--all"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestAllStem(t *testing.T) {
	tests := []struct{ in, want string }{
		{"-", stdinStem},
		{"/tmp/sw.json", "sw"},
		{"dir/fabric.v2.json", "fabric.v2"},
		{"noext", "noext"},
		{".json", ".json"},
	}
	for _, tt := range tests {
		if got := allStem(tt.in); got != tt.want {
			t.Errorf("allStem(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
