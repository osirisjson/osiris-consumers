# What does this PR change?

## Why is it needed?
<!-- Link the issue/discussion this follows, or explain why none exists. -->

## How was it tested?
Explain briefly how and what you tested.

## Change type
- [ ] Editorial (docs/comments only, no behavior change)
- [ ] Additive (new flag, model field, or capability, no breaking change)
- [ ] Breaking (changes the shape or semantics of existing output or the
      experimental `consumer/` facade)

## Checklist
- [ ] I linked the relevant issue/discussion (or explained why none exists)
- [ ] I kept scope focused and avoided unrelated changes
- [ ] I added/updated tests (if applicable)
- [ ] I ran `gofmt -l .`, `go vet ./...`, `go build ./...`, and
      `go test ./...` (add `-race` for concurrent code or the facade)
- [ ] I updated `CHANGELOG.md` in the same commit as the change it
      documents (if behavior changed)
- [ ] Commit messages / PR title follow Conventional Commits (`feat`,
      `fix`, `docs`, `chore`, `test`, `refactor`; `!` for breaking)
- [ ] No real production data (IPs, hostnames, credentials, serial
      numbers) in code, tests, or docs

## Breaking changes
<!-- If none, write "None". Otherwise describe what callers must change. -->
