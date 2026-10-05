# ADR-0007: go-stub as the modgen template engine

**Status:** Accepted · 2026-09-01

## Context
`task new-module NAME=x` must render a stub tree with placeholder substitution in file
names and content, gofmt output, and refuse to clobber (PRD §6.3, R12).

## Decision
`github.com/binafy/go-stub`, confined to `tools/go.mod` so it never enters the service
module graph. Mandatory options: `WithStrict()` (missing keys fail naming the key),
`WithDelimiters("<<", ">>")` (default `{{}}` collides with text/template),
`WithFormat()` (gofmt-clean output), no `WithForce`.

## Consequences
Single-maintainer dependency, accepted: dev-time only, zero-dependency, and the stdlib
fallback (WalkDir + text/template + go/format + case helpers) is ~80 lines if it goes
unmaintained. If the upstream API diverges from the PRD sketch, the fallback is
implemented in `tools/` under this same contract.

**Amendment 2026-09-01:** two deviations from the sketch, both forced by reality: (1) modgen lives in `tools/modgen` (not `cmd/modgen`) — a root-module cmd would pull go-stub into the root go.mod, which R12 forbids; (2) `WithFormat()` is documented as all-files and chokes on the .sql stub, so modgen runs go/format over the generated .go files itself. Strict mode, `<< >>` delimiters and no-overwrite stand as specified.
