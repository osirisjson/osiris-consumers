// Command osirisjson-consumer is the CLI adapter for the OSIRIS JSON
// consumer capabilities.
//
// It reads a canonically validated OSIRIS JSON 1.x document and
// produces deterministic, vendor-neutral output.
// In this initial release it implements only version and help verbs.
// The report and topology verbs are added once the capability
// packages exist.
//
// Usage:
//
//	osirisjson-consumer <command> [arguments]
//
// The commands are:
//
//	version
//		Print the build version, the commit it was built from, and
//		the pinned OSIRIS JSON engine and specification revisions.
//	help
//		Print this usage text.
//
// Diagnostics go to standard error. The exit code is 0 on success and 2
// on a usage error. Canonical validation error handling (exit code 1)
// arrives with the report and topology verbs.
package main

import (
	"fmt"
	"io"
	"os"
)

// Build metadata. These are overridden at build time with, for example:
//
//	go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse --short HEAD)" ./cmd/osirisjson-consumer
//
// The zero values below are what an unstamped local build reports.
var (
	version       = "dev"
	commit        = "unknown"
	engineVersion = "unpinned"
	specRevision  = "unpinned"
)

const usage = `osirisjson-consumer - consume a validated OSIRIS JSON 1.x document

Usage:
	osirisjson-consumer <command> [arguments]

Commands:
	version   print build, commit, and pinned engine/spec revisions
	help      print this usage text

Diagnostics are written to standard error. Exit codes: 0 success,
2 usage or operational failure. The report and topology capabilities,
and exit code 1 for canonical validation errors, are not implemented in
this build.
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
	case "version":
		fmt.Fprintf(stdout, "osirisjson-consumer %s (commit %s)\n", version, commit)
		fmt.Fprintf(stdout, "osiris-json engine: %s\n", engineVersion)
		fmt.Fprintf(stdout, "osiris-json spec:   %s\n", specRevision)
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
