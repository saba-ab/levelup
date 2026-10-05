# ADR-0001: Modular monolith over microservices or a flat layout

**Status:** Accepted · 2026-09-01

## Context
Go services at ~7k RPS fail in two directions: flat `handlers/services/models` layouts
that become untraceable dependency graphs, or premature microservices that pay
distributed-systems tax with no measured driver (PRD §1).

## Decision
One deployable binary set; modules under `internal/modules/<name>` with
compiler-private `internal/`, public surface limited to `contracts/` + `module.go`.
Boundaries are mechanically enforced (compiler, depguard, arch tests — PRD §9).
Extraction is designed for, never performed speculatively: an ADR naming a measured
driver (resource profile, availability class, team ownership) is required before any
split. "Scales better" is rejected as a driver (PRD §14).

## Consequences
Cheap module addition now; extraction cost bounded to one adapter + one config value
(G3). The cost is ceremony at the boundary: ports, adapters, events instead of joins.
