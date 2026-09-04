# OSIRIS JSON Consumers

Consumers for OSIRIS JSON: a CLI tool and Go packages that read a
canonically validated OSIRIS JSON 1.x document and turn it into
deterministic, vendor-neutral output a human can use:

- Infrastructure report
- Diff view and diff report comparing infrastructure snapshots
- Autogenerate topology in draw.io

## What OSIRIS JSON is

OSIRIS JSON is an open specification for an infrastructure resource
interchange schema. A producer connects to vendor infrastructure and
emits a portable OSIRIS JSON document. That document is validated
against the canonical schema and rules. OSIRIS JSON Consumers takes the
validated OSIRIS JSON document and produces something useful from it.

The specification, schema, and validation rules live in the
authoritative OSIRIS JSON repository:
https://github.com/osirisjson/osiris

This repository consumes that contract. It MUST NOT define or
re-implement it.

## What this repository does (MVP scope)

The `v0.1.0` MVP is exactly two capabilities, both vendor-neutral:

1. `report` - a Markdown brief and inventory that represents every
   resource, connection, group, provider, and unknown type in the
   document without semantic loss.
2. `topology` - a draw.io topology that represents the complete graph
   with generic fallback nodes and literal edges.

Generic processing is the default and the only path in the MVP. A valid
OSIRIS JSON 1.x document with no recognized provider still produces full
output. Mixed-provider documents are processed resource by resource.

## What this repository does not do

- It does not validate OSIRIS JSON. Validation is canonical and
  external (the OSIRIS JSON toolbox). A capability only runs against a
  document that already has a successful validation result for the
  exact input bytes.
- It does not merge, diff, or re-emit OSIRIS JSON documents in the MVP.
- It does not produce compliance verdicts, maturity grades, or
  architecture-quality scores. Provider enrichment, when it exists, uses
  evidence-state language only (observed/absent/unknown/not-applicable).
- It does not ship provider-specific behavior in the MVP. Provider
  enrichment (Azure first) is optional and post-MVP, and never a gate
  to producing useful output.

## Support matrix

| Item | MVP target |
|---|---|
| OSIRIS JSON version | 1.x (`1.0.x` and later `1.x.y`); unknown fields, types, and extensions are preserved, not rejected |
| Input | one complete JSON document, read with a configurable size bound; a trailing second value is rejected |
| Capabilities | `report` (Markdown), `topology` (draw.io) |
| Providers | generic only; no provider selector; enrichment is post-MVP |
| Output | to a file or stdout, created private and finalized atomically; existing output refused without `--force` |
| Platforms | Linux, Windows, macOS |
| Determinism | stable input plus stable options plus stable tool versions yields byte-identical output |

## Build, test, run

Requires the Go toolchain version pinned in `go.mod`.

```
go vet ./...
go build ./...
go test ./...
```

Build the CLI:

```
go build -o osirisjson-consumer ./cmd/osirisjson-consumer ./osirisjson-consumer version ./osirisjson-consumer help
```

Planned CLI once capabilities land:

```
osirisjson-consumer report   INPUT [-o FILE|-] [--validation-profile default] [--force]
osirisjson-consumer topology INPUT [-o FILE|-] [--force]
osirisjson-consumer version
osirisjson-consumer help
```

Exit codes: `0` success, `1` canonical validation errors, `2` usage or
operational failure.

## Security and sensitive data

Treat every OSIRIS JSON input document as sensitive and every string in
it as untrusted. Documents can carry topology, naming, account,
address, and security-policy data. The consumer persists no copy of an
input document outside the requested output by default, never logs a
raw document value, and never echoes a credential-like string.

## Contributing

See `CONTRIBUTING.md`. Contributions target the `dev` branch as small,
reviewable increments. `main` carries tagged releases only.
