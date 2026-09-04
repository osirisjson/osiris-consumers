# Security policy

This policy covers the `go.osirisjson.org/consumers` Go module: the
`osirisjson-consumer` CLI, the experimental `consumer/` facade, and the
`internal/` packages behind them.

## Supported versions

| Version | Supported |
|---------|-----------|
| latest tagged release | Yes |
| older | No |

There is no tagged release yet. Once releases exist, only the most
recent tag receives security fixes; older versions are upgraded rather
than patched.

## Reporting a vulnerability

**Please do not open a public issue for a security vulnerability.**

Use a private channel:

1. **GitHub private security advisory (preferred).** Open a
   [private security advisory](https://github.com/osirisjson/osiris-consumers/security/advisories/new)
   on the repository. This keeps the report confidential until a fix is
   published.
2. **Email.** Send a report to
   [tiazanella@osirisjson.org](mailto:tiazanella@osirisjson.org).
   Encrypt sensitive reports with the maintainer's GPG public key
   (fetch the full key from a keyserver):

   `gpg --keyserver hkps://keys.openpgp.org --recv-keys 29B75E5B135F6B87`

### What to include

- A clear description of the vulnerability and its potential impact.
- The affected component (`cmd/osirisjson-consumer`, `consumer/`, or an
  `internal/` package) and the version or commit.
- Step-by-step reproduction or a minimal proof of concept. For an
  input-handling issue, attach the smallest OSIRIS JSON document that
  triggers it, with only synthetic data.
- Any relevant output, stack trace, or manifest.
- Your suggested severity (Critical / High / Medium / Low /
  Informational).

### Response timeline

| Event | Target |
|---|---|
| Acknowledgement | Within 3 business days |
| Initial assessment | Within 7 business days |
| Fix or mitigation | By severity; critical issues are prioritized immediately |
| Public disclosure | Coordinated with the reporter, typically after a fix is released |

If you have not been acknowledged within 3 business days, follow up by
email.

## Disclosure policy

Coordinated (responsible) disclosure:

1. The reporter submits a private report.
2. Maintainers confirm the issue and prepare a fix.
3. A patched release is published.
4. A security advisory (CVE if applicable) is published alongside it.
5. The reporter is credited unless they request anonymity.

## Threat model

Consumers reads a single OSIRIS JSON document from a file or stdin and
writes one artifact. It makes no network calls and no vendor API calls.
The security-relevant surface is:

- **Untrusted input.** Every OSIRIS JSON input document is treated as
  sensitive and every string in it as untrusted. The document can carry
  topology, naming, account, address, and security-policy data.
- **Output injection.** An untrusted resource name or description must
  not become active content in a Markdown viewer or malformed XML in a
  draw.io file. Rendering uses destination-aware escaping, preserves
  valid UTF-8, and emits no raw HTML into Markdown.
- **File writes.** Output is created private (`0600`), refuses to
  clobber an existing file without `--force`, refuses a symlink
  destination even with `--force`, rejects path traversal, checks
  flush/sync/close, and finalizes atomically. A failed write leaves no
  finalized artifact.
- **Data at rest and in logs.** No copy of an input document is
  persisted outside the requested output by default. No raw document
  value is logged. Credential-like diagnostics never echo the candidate
  value.
- **Validation provenance.** No capability runs without a canonical
  validation receipt for the exact input bytes.

## Contributor practices

- Never commit credentials, tokens, or certificates. Test fixtures use
  only the synthetic ranges documented for this project.
- Keep dependencies minimal. The core is standard-library first; a
  non-stdlib dependency is only justified at the validation boundary
  and only when approved and recorded as an ADR. Pin versions in
  `go.mod` and review `go.sum` changes.
- Run `go vet ./...`, `go test ./...`, and `go test -race ./...` before
  opening a PR. CI additionally runs `govulncheck` and a
  repository-hygiene scan.
- All changes land by reviewed pull request. Changes to escaping, file
  finalization, or the validation boundary require maintainer review.
- Release binaries are built from a protected tag with `-trimpath` and
  a `-ldflags`-injected version, never from a branch push.

## Out of scope

- Issues in Go itself or in third-party dependencies; report those
  upstream (a dependency advisory that affects a release is in scope
  for a version bump).
- Social engineering of maintainers.
- Theoretical vulnerabilities without a working proof of concept.
- Resource exhaustion from an input larger than the configured size
  bound when the caller has raised or disabled that bound; the default
  bound is enforced and lowering risk is the caller's choice.

## Contacts

| Role | Contact |
|---|---|
| Lead maintainer (email) | Tia Zanella `@skhell` - [tiazanella@osirisjson.org](mailto:tiazanella@osirisjson.org) |
| Lead maintainer (Matrix) | [@skhell:matrix.org](https://matrix.to/#/@skhell:matrix.org) |
| General community | [community@osirisjson.org](mailto:community@osirisjson.org) |
