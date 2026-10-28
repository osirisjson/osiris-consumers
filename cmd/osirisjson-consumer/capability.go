// This file implements the report and topology verbs: flag parsing,
// input acquisition, the call into the in-process facade, verbatim
// canonical-diagnostic reporting, atomic private file output through
// internal/artifact, and the optional JSON result envelope. All
// filesystem policy lives here, never below the facade.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.osirisjson.org/consumers/consumer"
	"go.osirisjson.org/consumers/internal/artifact"
	"go.osirisjson.org/consumers/internal/validation"
	"go.osirisjson.org/consumers/internal/validation/toolboxcli"
)

// capabilityFunc is GenerateReport or GenerateTopology.
type capabilityFunc func(context.Context, consumer.Request) (consumer.Result, error)

// runCapability handles "report" and "topology".
func runCapability(name string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "-", `artifact destination file, or "-" for standard output`)
	profile := fs.String("validation-profile", validation.DefaultProfile, "canonical validation profile")
	force := fs.Bool("force", false, "overwrite an existing regular file at -o")
	format := fs.String("format", "text", `success envelope format: "text" or "json"`)
	quiet := fs.Bool("quiet", false, "suppress the success summary")
	verbose := fs.Bool("verbose", false, "print bounded progress events")
	var maxRows int
	var outputFormat string
	var xlsxCompanion string
	var all bool
	var outDir string
	if name == "report" {
		fs.BoolVar(&all, "all", false,
			"validate and parse INPUT once and write every artifact (.md, .pdf, .drawio, and .xlsx when needed) into --out-dir")
		fs.StringVar(&outDir, "out-dir", "", `--all output directory (default ".")`)
		fs.IntVar(&maxRows, "max-rows", 0, "bound each report table to N rows (0 = unbounded)")
		fs.StringVar(&outputFormat, "output-format", "",
			`artifact format: "markdown" or "pdf" (default "markdown"; inferred from -o FILE's .pdf extension when omitted)`)
		fs.StringVar(&xlsxCompanion, "xlsx-companion", "",
			`write the recognized vendor's .xlsx companion workbook to this file, when one is needed for this document; omitted means none is ever written`)
	}

	// Grammar: "<verb> INPUT [flags]". INPUT is the first argument; the
	// rest are flags, which may therefore follow it. Any leftover
	// positional after flag parsing is a usage error.
	if len(args) == 0 {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: expected exactly one INPUT argument\n", name)
		return 2
	}
	inputArg := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: unexpected extra arguments after INPUT: %v\n", name, fs.Args())
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: --format must be \"text\" or \"json\"\n", name)
		return 2
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["out-dir"] && !all {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: --out-dir requires --all\n", name)
		return 2
	}
	if all {
		// --all names every file itself, so the single-artifact
		// destination and format flags have nothing to apply to.
		for _, f := range []string{"o", "output-format", "xlsx-companion"} {
			if set[f] {
				fmt.Fprintf(stderr, "osirisjson-consumer %s: -%s cannot be combined with --all (use --out-dir)\n", name, f)
				return 2
			}
		}
		if outDir == "" {
			outDir = "."
		}
		return runAll(name, inputArg, allOptions{
			outDir:  outDir,
			profile: *profile,
			force:   *force,
			format:  *format,
			quiet:   *quiet,
			verbose: *verbose,
			maxRows: maxRows,
		}, stdout, stderr)
	}
	toStdout := *out == "-" || *out == ""
	if *format == "json" && toStdout {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: --format json requires -o FILE; it never mixes with the artifact stream\n", name)
		return 2
	}
	if name == "report" {
		switch outputFormat {
		case "", "markdown", "pdf":
		default:
			fmt.Fprintf(stderr, "osirisjson-consumer %s: --output-format must be \"markdown\" or \"pdf\"\n", name)
			return 2
		}
		// No explicit --output-format: infer pdf from -o FILE's own
		// extension as a convenience, never when writing to standard
		// output (there is no filename to infer from there) and never
		// overriding an explicit flag.
		if outputFormat == "" && !toStdout && strings.EqualFold(filepath.Ext(*out), ".pdf") {
			outputFormat = "pdf"
		}
		if xlsxCompanion == "-" {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: --xlsx-companion must be a file path, not \"-\"\n", name)
			return 2
		}
		if xlsxCompanion != "" && !toStdout && xlsxCompanion == *out {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: --xlsx-companion must name a different file than -o\n", name)
			return 2
		}
	}

	input, err := readInput(inputArg)
	if err != nil {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: %v\n", name, err)
		return 2
	}

	fn := consumer.GenerateReport
	if name == "topology" {
		fn = consumer.GenerateTopology
	}

	req := consumer.Request{
		Input: input,
		Options: consumer.Options{
			ValidationProfile: *profile,
			Validator:         toolboxcli.Validator{},
			ArtifactTime:      time.Now().UTC(),
			MaxInventoryRows:  maxRows,
			OutputFormat:      outputFormat,
		},
	}
	// xlsxBuf is written to in-memory first, never staged through
	// internal/artifact directly: whether the recognized vendor's model
	// actually needs a companion for this document is only known once
	// fn runs, and internal/artifact.Write always finalizes whatever
	// its callback returned successfully from, staging it eagerly
	// would leave a spurious empty .xlsx file on disk for a document
	// that never needed one. Finalized separately below, only when
	// xlsxBuf actually holds something.
	var xlsxBuf *bytes.Buffer
	if xlsxCompanion != "" {
		req.Options.XLSXCompanionName = filepath.Base(xlsxCompanion)
		xlsxBuf = &bytes.Buffer{}
		req.XLSXOutput = xlsxBuf
	}
	if *verbose {
		req.Progress = func(p consumer.Progress) {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: [%s] %s\n", name, p.Stage, p.Message)
		}
	}

	ctx := context.Background()

	var (
		res      consumer.Result
		runErr   error
		manifest artifact.Manifest
		destPath string
	)
	if toStdout {
		req.Output = stdout
		res, runErr = fn(ctx, req)
	} else {
		dir := filepath.Dir(*out)
		base := filepath.Base(*out)
		destPath = *out
		sum := sha256.Sum256(input)
		manifest, runErr = artifact.Write(ctx, artifact.Request{
			Dir:             dir,
			Name:            base,
			Force:           *force,
			InputSHA256:     hex.EncodeToString(sum[:]),
			Capability:      name,
			ConsumerVersion: version,
			EngineName:      validation.PinnedEngineName,
			EngineVersion:   validation.PinnedEngineVersion,
			Profile:         *profile,
		}, func(w io.Writer) error {
			req.Output = w
			var e error
			res, e = fn(ctx, req)
			return e
		})
	}

	if runErr != nil {
		reportError(stderr, name, res, runErr)
		return consumer.ExitCode(runErr)
	}

	var xlsxManifest artifact.Manifest
	if xlsxBuf != nil && xlsxBuf.Len() > 0 {
		sum := sha256.Sum256(input)
		xlsxManifest, err = artifact.Write(ctx, artifact.Request{
			Dir:             filepath.Dir(xlsxCompanion),
			Name:            filepath.Base(xlsxCompanion),
			Force:           *force,
			InputSHA256:     hex.EncodeToString(sum[:]),
			Capability:      name,
			ConsumerVersion: version,
			EngineName:      validation.PinnedEngineName,
			EngineVersion:   validation.PinnedEngineVersion,
			Profile:         *profile,
		}, func(w io.Writer) error {
			_, e := w.Write(xlsxBuf.Bytes())
			return e
		})
		if err != nil {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: writing xlsx companion: %v\n", name, err)
			return 2
		}
	}

	switch {
	case *format == "json":
		if err := writeEnvelope(stdout, name, res, manifest, destPath, xlsxManifest); err != nil {
			fmt.Fprintf(stderr, "osirisjson-consumer %s: writing result envelope: %v\n", name, err)
			return 2
		}
	case !*quiet && !toStdout:
		fmt.Fprintf(stderr, "osirisjson-consumer %s: wrote %s (%d bytes, sha256 %s)\n",
			name, destPath, manifest.Bytes, manifest.ArtifactSHA256)
	case !*quiet && toStdout && *verbose:
		fmt.Fprintf(stderr, "osirisjson-consumer %s: wrote %d bytes to standard output\n", name, res.Artifact.Bytes)
	}
	// The companion is its own file-system side effect regardless of
	// whether the primary artifact went to stdout or was reported at
	// all above (stdout without --verbose stays silent for the primary
	// by design, but a new file silently appearing on filesystem
	// without notice at all would be worse than one extra line) --quiet
	// still suppresses it, and --format json already carries it in the
	// envelope instead.
	if *format != "json" && !*quiet && xlsxManifest.Path != "" {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: wrote %s (%d bytes, sha256 %s)\n",
			name, xlsxManifest.Path, xlsxManifest.Bytes, xlsxManifest.ArtifactSHA256)
	}
	return 0
}

// readInput returns the bytes of arg, which is a file path or "-" for
// standard input.
func readInput(arg string) ([]byte, error) {
	if arg == "-" {
		return io.ReadAll(os.Stdin)
	}
	b, err := os.ReadFile(arg)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}
	return b, nil
}

// reportError prints a human-readable failure to stderr. For a
// canonical-validation failure it prints every engine diagnostic
// verbatim, in engine order.
func reportError(stderr io.Writer, name string, res consumer.Result, err error) {
	if errors.Is(err, consumer.ErrValidationFailed) || errors.Is(err, consumer.ErrUnsupportedOSIRIS) {
		fmt.Fprintf(stderr, "osirisjson-consumer %s: document failed canonical validation\n", name)
		for _, d := range res.Validation.Diagnostics {
			line := d.Code
			if d.Severity != "" {
				line += " " + d.Severity
			}
			if d.Path != "" {
				line += " at " + d.Path
			}
			fmt.Fprintf(stderr, "  %s: %s\n", line, d.Message)
			for _, rel := range d.Related {
				fmt.Fprintf(stderr, "    related %s: %s\n", rel.Path, rel.Message)
			}
		}
		return
	}
	fmt.Fprintf(stderr, "osirisjson-consumer %s: %v\n", name, err)
}

// envelope is the --format json result. It carries no document values:
// only counts, hashes, engine identity, and diagnostic metadata.
type envelope struct {
	Capability   string             `json:"capability"`
	InputSHA256  string             `json:"input_sha256"`
	Validation   envelopeValidation `json:"validation"`
	Artifact     envelopeArtifact   `json:"artifact"`
	XLSXArtifact *envelopeArtifact  `json:"xlsx_artifact,omitempty"`
	Warnings     []envelopeDiag     `json:"warnings,omitempty"`
	Anomalies    []envelopeAnomaly  `json:"anomalies,omitempty"`
}

type envelopeValidation struct {
	EngineName    string `json:"engine_name"`
	EngineVersion string `json:"engine_version"`
	Profile       string `json:"profile"`
	SpecVersion   string `json:"spec_version"`
	ErrorCount    int    `json:"error_count"`
	WarningCount  int    `json:"warning_count"`
}

type envelopeArtifact struct {
	Path         string `json:"path,omitempty"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
	PageFormat   string `json:"page_format,omitempty"`
	PageWidth    int    `json:"page_width,omitempty"`
	PageHeight   int    `json:"page_height,omitempty"`
	ExceedsISOA0 bool   `json:"exceeds_iso_a0,omitempty"`
}

type envelopeDiag struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

type envelopeAnomaly struct {
	Kind      string `json:"kind"`
	Subject   string `json:"subject,omitempty"`
	Reference string `json:"reference,omitempty"`
}

func writeEnvelope(w io.Writer, name string, res consumer.Result, m artifact.Manifest, destPath string, xlsxM artifact.Manifest) error {
	e := envelope{
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
		Artifact: envelopeArtifact{
			Path:         destPath,
			Bytes:        m.Bytes,
			SHA256:       m.ArtifactSHA256,
			PageFormat:   res.Artifact.PageFormat,
			PageWidth:    res.Artifact.PageWidth,
			PageHeight:   res.Artifact.PageHeight,
			ExceedsISOA0: res.Artifact.PageFormat == "custom",
		},
	}
	if xlsxM.Path != "" {
		e.XLSXArtifact = &envelopeArtifact{
			Path:   xlsxM.Path,
			Bytes:  xlsxM.Bytes,
			SHA256: xlsxM.ArtifactSHA256,
		}
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
