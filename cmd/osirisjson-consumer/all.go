// This file implements "report --all": one canonical validation and one
// parse of INPUT through consumer.GenerateAll, then every artifact
// (Markdown report, PDF report, draw.io topology, and the recognized
// vendor's .xlsx companion when this document needs one) finalized
// atomically and privately into one output directory through
// internal/artifact.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.osirisjson.org/consumers/consumer"
	"go.osirisjson.org/consumers/internal/artifact"
	"go.osirisjson.org/consumers/internal/validation"
	"go.osirisjson.org/consumers/internal/validation/toolboxcli"
)

// stdinStem is the artifact base name used when INPUT is standard
// input, since there is no file name to derive one from.
const stdinStem = "osirisjson"

// allOptions are the flags "report --all" honors.
type allOptions struct {
	outDir  string
	profile string
	force   bool
	format  string
	quiet   bool
	verbose bool
	maxRows int
}

// allArtifact is one file of the --all set: its kind, its final name
// in the output directory, and the in-memory buffer it is rendered to
// before finalization.
type allArtifact struct {
	kind string
	name string
	buf  *bytes.Buffer
}

// allStem returns the artifact base name for inputArg: the input file's
// own name without its extension, or stdinStem for standard input.
func allStem(inputArg string) string {
	if inputArg == "-" {
		return stdinStem
	}
	base := filepath.Base(inputArg)
	if stem := strings.TrimSuffix(base, filepath.Ext(base)); stem != "" {
		return stem
	}
	return base
}

// runAll renders every artifact for inputArg in one pass and finalizes
// them into opts.outDir. Every destination is checked before any work,
// so an existing file without --force leaves the directory untouched;
// every artifact is rendered in memory before the first one is
// finalized, so a validation or render failure writes nothing.
func runAll(name, inputArg string, opts allOptions, stdout, stderr io.Writer) int {
	stem := allStem(inputArg)
	arts := []allArtifact{
		{kind: "markdown", name: stem + ".md", buf: &bytes.Buffer{}},
		{kind: "xlsx", name: stem + ".xlsx", buf: &bytes.Buffer{}},
		{kind: "pdf", name: stem + ".pdf", buf: &bytes.Buffer{}},
		{kind: "drawio", name: stem + ".drawio", buf: &bytes.Buffer{}},
	}

	fi, err := os.Stat(opts.outDir)
	if err != nil || !fi.IsDir() {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: --out-dir %q is not an existing directory\n", name, opts.outDir)
		return 2
	}
	for _, a := range arts {
		if err := checkDestination(opts.outDir, a.name, opts.force); err != nil {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: %v\n", name, err)
			return 2
		}
	}

	input, err := readInput(inputArg)
	if err != nil {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: %v\n", name, err)
		return 2
	}

	req := consumer.AllRequest{
		Input:    input,
		Markdown: arts[0].buf,
		XLSX:     arts[1].buf,
		PDF:      arts[2].buf,
		Topology: arts[3].buf,
		Options: consumer.Options{
			ValidationProfile: opts.profile,
			Validator:         toolboxcli.Validator{},
			ArtifactTime:      time.Now().UTC(),
			MaxInventoryRows:  opts.maxRows,
			XLSXCompanionName: arts[1].name,
		},
	}
	if opts.verbose {
		req.Progress = func(p consumer.Progress) {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: [%s] %s\n", name, p.Stage, p.Message)
		}
	}

	ctx := context.Background()
	res, err := consumer.GenerateAll(ctx, req)
	if err != nil {
		reportError(stderr, name, consumer.Result{Validation: res.Validation}, err)
		return consumer.ExitCode(err)
	}

	sum := sha256.Sum256(input)
	inputSHA := hex.EncodeToString(sum[:])
	var written []artifact.Manifest
	var kinds []string
	for _, a := range arts {
		if a.buf.Len() == 0 {
			continue
		}
		m, err := artifact.Write(ctx, artifact.Request{
			Dir:             opts.outDir,
			Name:            a.name,
			Force:           opts.force,
			InputSHA256:     inputSHA,
			Capability:      name,
			ConsumerVersion: version,
			EngineName:      validation.PinnedEngineName,
			EngineVersion:   validation.PinnedEngineVersion,
			Profile:         opts.profile,
		}, func(w io.Writer) error {
			_, e := w.Write(a.buf.Bytes())
			return e
		})
		if err != nil {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: writing %s: %v\n", name, a.name, err)
			for _, done := range written {
				fmt.Fprintf(stderr, "osirisjson-consumer %s: already finalized %s\n", name, done.Path)
			}
			return 2
		}
		written = append(written, m)
		kinds = append(kinds, a.kind)
	}

	switch {
	case opts.format == "json":
		if err := writeAllEnvelope(stdout, name, res, kinds, written); err != nil {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: writing result envelope: %v\n", name, err)
			return 2
		}
	case !opts.quiet:
		for _, m := range written {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: wrote %s (%d bytes, sha256 %s)\n",
				name, m.Path, m.Bytes, m.ArtifactSHA256)
		}
	}
	return 0
}

// checkDestination refuses a destination internal/artifact would refuse
// later (a symlink, a non-regular file, or an existing file without
// force), so the --all set is never left half-written by a conflict
// discovered only after the first file was finalized.
func checkDestination(dir, name string, force bool) error {
	fi, err := os.Lstat(filepath.Join(dir, name))
	switch {
	case err != nil && os.IsNotExist(err):
		return nil
	case err != nil:
		return fmt.Errorf("inspecting %s: %w", name, err)
	case fi.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%w: %s", artifact.ErrSymlinkDestination, name)
	case !fi.Mode().IsRegular():
		return fmt.Errorf("%w: %s", artifact.ErrNotRegularFile, name)
	case !force:
		return fmt.Errorf("%w: %s (use --force to overwrite)", artifact.ErrOutputExists, name)
	}
	return nil
}

// allEnvelope is the --format json result for --all. Like envelope it
// carries no document values: only counts, hashes, engine identity, and
// diagnostic metadata.
type allEnvelope struct {
	Capability  string             `json:"capability"`
	InputSHA256 string             `json:"input_sha256"`
	Validation  envelopeValidation `json:"validation"`
	Artifacts   []allEnvelopeItem  `json:"artifacts"`
	Warnings    []envelopeDiag     `json:"warnings,omitempty"`
	Anomalies   []envelopeAnomaly  `json:"anomalies,omitempty"`
}

type allEnvelopeItem struct {
	Kind string `json:"kind"`
	envelopeArtifact
}

func writeAllEnvelope(w io.Writer, name string, res consumer.AllResult, kinds []string, written []artifact.Manifest) error {
	e := allEnvelope{
		Capability:  name,
		InputSHA256: res.InputSHA256,
		Validation: envelopeValidation{
			EngineName:    res.Validation.EngineName,
			EngineVersion: res.Validation.EngineVersion,
			Profile:       res.Validation.Profile,
			SpecVersion:   res.Validation.SpecificationVersion,
			ErrorCount:    res.Validation.ErrorCount,
			WarningCount:  res.Validation.WarningCount,
		},
	}
	for i, m := range written {
		item := allEnvelopeItem{
			Kind: kinds[i],
			envelopeArtifact: envelopeArtifact{
				Path:   m.Path,
				Bytes:  m.Bytes,
				SHA256: m.ArtifactSHA256,
			},
		}
		if kinds[i] == "drawio" {
			item.PageFormat = res.Topology.PageFormat
			item.PageWidth = res.Topology.PageWidth
			item.PageHeight = res.Topology.PageHeight
			item.ExceedsISOA0 = res.Topology.PageFormat == "custom"
		}
		e.Artifacts = append(e.Artifacts, item)
	}
	for _, d := range res.Warnings {
		e.Warnings = append(e.Warnings, envelopeDiag{
			Code: d.Code, Severity: d.Severity, Path: d.Path, Message: d.Message,
		})
	}
	for _, a := range res.GraphAnomalies {
		e.Anomalies = append(e.Anomalies, envelopeAnomaly{
			Kind: a.Kind, Subject: a.Subject, Reference: a.Reference,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}
