# Installing OSIRIS JSON Consumers

`osirisjson-consumer` is a single Go binary. Until the software is
released to its stable version and pre-built packages are published,
install it with the Go toolchain following the procedure below.

## Prerequisites

- [Go 1.25 or later](https://go.dev/dl/), as pinned in `go.mod` is installed.
- [Node.js](https://nodejs.org/en/download/current) and
  [@osirisjson/cli](https://www.npmjs.com/package/@osirisjson/cli), with
  its `osirisjson` command on your `PATH`. The `report` and `topology`
  commands obtain a canonical validation result for the exact input
  bytes by running `osirisjson validate` before any analysis. Without
  it they fail with exit code `2`.

```bash
npm install -g @osirisjson/cli@0.1.0
osirisjson --version
```

## Install via Go

```bash
go install go.osirisjson.org/consumers/cmd/osirisjson-consumer@latest
```

To pin a specific release, replace `@latest` with its tag, for example
`@v0.1.0`. The binary is placed in `$(go env GOBIN)`, or
`$(go env GOPATH)/bin` when `GOBIN` is unset; make sure that directory
is on your `PATH`.

A binary installed this way reports `dev (commit unknown)` from
`osirisjson-consumer version`, because the release version and commit
are injected only by release builds.

## Build from source

```bash
git clone https://github.com/osirisjson/osiris-consumers.git
cd osiris-consumers
```

To install the command on your `PATH` from the clone (no root
required), use `go install`. As above, the binary is placed in
`$(go env GOBIN)`, or `$(go env GOPATH)/bin` when `GOBIN` is unset:

```bash
go install ./cmd/osirisjson-consumer
```

To build a binary without installing it, use `go build`. This writes
`osirisjson-consumer` into the current directory only; run it as
`./osirisjson-consumer`, or copy it onto your `PATH` yourself, for
example system-wide for all users:

```bash
go build -o osirisjson-consumer ./cmd/osirisjson-consumer
sudo install -m 0755 osirisjson-consumer /usr/local/bin/
```

Both commands accept `-ldflags` to stamp the version and commit the same
way release builds do:

```bash
go install -ldflags "-X main.version=0.1.0 -X main.commit=$(git rev-parse --short HEAD)" \
  ./cmd/osirisjson-consumer
```

## Quick start

```bash
osirisjson-consumer version
osirisjson-consumer help
```

Render the Markdown inventory brief and the draw.io topology:

```bash
osirisjson-consumer report   osirisjson-document.json -o report.md
osirisjson-consumer topology osirisjson-document.json -o topology.drawio
```

Render the report as PDF instead (inferred from the `.pdf` extension, or
set explicitly with `--output-format pdf`):

```bash
osirisjson-consumer report osirisjson-document.json -o report.pdf
```

Or write every artifact at once, validating the document only once:

```bash
osirisjson-consumer report osirisjson-document.json --all --out-dir out/
# out/document.md, out/document.pdf, out/document.drawio,
# and out/document.xlsx when the recognized vendor needs one
```

`INPUT` may be `-` to read standard input; `-o -` or omitting `-o`
writes the artifact to standard output. Existing output files are
refused unless `--force` is given. Exit codes: `0` success, `1`
canonically invalid document, `2` usage, malformed input, output
conflict, or I/O failure.
