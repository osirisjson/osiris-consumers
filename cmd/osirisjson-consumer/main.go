// Command osirisjson-consumer is the CLI adapter for the OSIRIS JSON
// consumer capabilities.
//
// It reads a canonically validated OSIRIS JSON 1.x document and
// produces deterministic, vendor-neutral output: a report, as Markdown
// inventory brief (the default) or a PDF, optionally with a recognized
// vendor's .xlsx companion workbook, or a draw.io topology.
package main

import (
	"fmt"
	"io"
	"os"

	"go.osirisjson.org/consumers/internal/validation"
)

// Build metadata.
// The zero values below are what an unstamped local build reports.
var (
	version = "dev"
	commit  = "unknown"
)

const usage = `osirisjson-consumer - read a validated OSIRIS JSON document

Usage:
	osirisjson-consumer <command> [arguments]

Commands:
	report INPUT [flags]     render the vendor-neutral inventory brief (Markdown or PDF)
	topology INPUT [flags]   render the vendor-neutral draw.io topology
	version                  print version release
	help                     print this help guideline

Common flags:
	-o FILE|-                artifact destination; "-" or omitted means standard output
	--validation-profile P   canonical validation profile (default "default")
	--force                  overwrite an existing regular file at -o (never a symlink)
	--format text|json       result envelope format on success (default "text");
	                         "json" requires -o FILE and never mixes with the artifact
	--quiet                  suppress the success summary on standard error
	--verbose                print bounded progress events to standard error

Flags for report command only:
	--max-rows N             bound each report table to N rows with an explicit notice
	--output-format markdown|pdf
	                         artifact format (default "markdown"); inferred as pdf when
	                         -o names a file ending in ".pdf" and the flag is omitted.
	                         A recognized vendor's own enrichment applies to markdown
	                         only in this release; pdf always renders the generic,
	                         vendor-neutral baseline for now.
	--xlsx-companion FILE    write the recognized vendor's .xlsx companion workbook
	                         to FILE, only when this document actually needs one;
	                         omitted (the default), none is ever written. FILE must
	                         differ from -o and cannot be "-".
	--all                    validate and parse INPUT once and write every artifact
	                         into --out-dir: STEM.md, STEM.pdf, STEM.drawio, and
	                         STEM.xlsx when this document needs one. STEM is INPUT's
	                         file name without its extension ("osirisjson" for "-").
	                         Cannot be combined with -o, --output-format, or
	                         --xlsx-companion. With --format json the result
	                         envelope goes to standard output.
	--out-dir DIR            --all output directory (default "."). Every
	                         destination is checked before any run: an existing
	                         file presence is refused to proceed without --force flag.

INPUT is a file path or "-" for standard input. Diagnostics and progress
go to standard error.
Exit codes: 0 success, 1 canonically invalid document, 2 usage
malformed input, output conflict, I/O failure.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one invocation and returns the process exit code. It
// takes its output sinks explicitly so it can be exercised in tests
// without touching the real standard streams.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "osirisjson-consumer: no command given")
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "report", "topology":
		return runCapability(args[0], args[1:], stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "osirisjson-consumer %s (commit %s)\n", version, commit)
		fmt.Fprintf(stdout, "osiris-json engine: %s@%s\n", validation.PinnedEngineName, validation.PinnedEngineVersion)
		fmt.Fprintf(stdout, "osiris-json spec:   %s\n", validation.PinnedSpecVersion)
		fmt.Fprintf(stdout, "osiris-json examples: %s\n", validation.PinnedExampleRevision)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "osirisjson-consumer: unknown command %q\n", args[0])
		fmt.Fprint(stderr, usage)
		return 2
	}
}
