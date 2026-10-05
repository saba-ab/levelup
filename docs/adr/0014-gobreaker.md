# ADR-0014: sony/gobreaker for the remote-adapter circuit breaker

**Status:** Accepted · 2026-09-01 · stack addition under §4 P6

## Context
R6 requires the remote user adapter to ship with timeout + breaker + batch. The
locked stack (§7) contains no breaker.

## Decision
`github.com/sony/gobreaker/v2`: ~300 lines, zero transitive dependencies, the
de-facto standard state machine (closed → open after consecutive failures → half-open
probe). Hand-rolling breaker concurrency is exactly the kind of subtle code the
blueprint avoids writing twice.

## Consequences
One small dependency confined to `modules/*/adapters/`. Settings live beside the
adapter: open after 5 consecutive failures, half-open after 10s.
