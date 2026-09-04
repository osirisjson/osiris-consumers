# Contributing to OSIRIS JSON Consumers

Contributions of any size are welcome: bug reports, focused fixes,
tests, documentation, and reviews. This repository is the consumer side
of the OSIRIS JSON ecosystem: it reads a canonically validated OSIRIS
JSON 1.x document and produces vendor-neutral output. The specification
and schema live in the authoritative OSIRIS JSON repository
(https://github.com/osirisjson/osiris); spec and schema changes belong
there, not here.

## Principles

- Quality over volume. A few well-scoped changes beat many low-impact
  ones.
- Explain why, not only what. Link the issue or discussion.
- Keep scope focused. Do not mix a refactor and a feature in one PR.
- Generic first. A change must not make a valid OSIRIS JSON 1.x
  document with no recognized provider a degraded case.

## Branch model and PR flow

```
main     stable, tagged releases only (SemVer)
dev      integration line, always buildable
feature/ short-lived, one shippable increment per branch
```

- Branch from `dev`: `git checkout dev && git pull && git checkout -b feature/<area>-<what-it-delivers>`.
  Name the increment, not a version (`feature/report-inventory-model`,
  not `feature/report-0-2-0`).
- Open a PR against `dev`. Keep the diff small and reviewable.
- Feature PRs are squash-merged: the PR title becomes the commit
  subject and MUST be a valid Conventional Commit; the PR body becomes
  the commit body.
- `main` only ever receives a release branch. Do not target `main`.

## Commit messages

Conventional Commits, one line for the subject:

```
type(scope): description
```

`type` is one of `feat`, `fix`, `chore`, `docs`, `test`, `refactor`.
A breaking change is marked with `!` before the colon
(`feat(cli)!: ...`) or a `BREAKING CHANGE:` footer. Update
`CHANGELOG.md` in the same commit as the change it documents.

## Required checks

Run this order locally before opening a PR:

```
gofmt -l .        # must print nothing
go vet ./...
go build ./...
go test ./...
```

Add `go test -race ./...` for any change to concurrent code or the
`consumer/` facade.

CI runs on every PR and must be green before merge:

- `gofmt` clean-diff, `go vet ./...`, `go build ./...`,
  `go test ./...`, `go test -race ./...`;
- `go mod tidy` clean-diff;
- a repository-hygiene scan (no tracked binaries, generated output,
  local settings, or obvious secrets);
- cross-compile of the CLI for Linux, Windows, and macOS.

Once the capabilities exist, CI also runs the official OSIRIS JSON v1.0
examples (from a pinned external revision), adversarial fixtures, and
golden determinism.

## Style

- Follow the [Go doc comment style](https://go.dev/doc/comment). Every
  `.go` file starts with a header comment explaining what it is and
  does.
- Keep a consistent line width across `CHANGELOG.md` and source
  comments (excluding links, code blocks, and tables), per
  [RFC 7994](https://datatracker.ietf.org/doc/html/rfc7994#section-4.3).
- No em-dashes. Use a regular dash. No Unicode arrows; use ASCII `->`
  or `<-`. Keep output plain ASCII-friendly.
- No `panic` in library code. Wrap errors with
  `fmt.Errorf("context: %w", err)`.
- Below the CLI boundary: no `os.Exit`, no `log.*`, no `fmt.Print*` for
  diagnostics, no stderr writes.
- Standard library first. Do not add a dependency without maintainer
  approval; the only place one may be justified is the
  canonical-validation boundary.

## Tests and fixtures

- Test files sit beside the code (`_test.go`). Table-driven tests are
  preferred. Golden artifacts live under `testdata/golden/`,
  adversarial fixtures under `testdata/adversarial/`.
- Never commit real infrastructure data. Fixtures use documentation and
  reserved ranges only: RFC 5737 IPv4 (`192.0.2.0/24`,
  `198.51.100.0/24`, `203.0.113.0/24`), RFC 3849 IPv6 (`2001:DB8::/32`),
  RFC 9542 documentation MAC ranges or locally administered
  (`02:...`) MACs, RFC 2606 names (`example.com`), and clearly invented
  lab prefixes (`LAB-`, `TEST-`, `FAKE-`). Credentials in fixtures are
  obvious placeholders (`admin`, `password`, `changeme`).
- The official OSIRIS JSON examples are consumed from a pinned
  external revision, not copied into this repository.

## AI-assisted contributions

Using AI as a helper is fine. Mass-producing issues or PRs without
understanding the project is not.

If you use AI: you are responsible for correctness and scope; read and
understand the code you are changing; validate outputs with the checks
above; do not open speculative issues you have not reproduced in a real
production environment; avoid drive-by reformatting.

Maintainers may close without extended discussion: duplicates, reports
missing reproducible context, and PRs that are clearly low-effort,
auto-generated, or not aligned with the project direction.

## Code of Conduct

Participation is governed by [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

## Licensing

By contributing, you agree that your contributions are licensed under
the project's license (Apache-2.0, see `LICENSE`).
