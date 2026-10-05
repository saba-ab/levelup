# ADR-0012: No per-key event ordering guarantee in v1

**Status:** Accepted · 2026-09-01 · resolves PRD §13 Q2

## Context
RabbitMQ provides no ordering across a queue's consumers, and redeliveries land behind
newer messages (PRD §7.9). Guaranteeing per-user ordering requires the
consistent-hash exchange plugin or a different broker.

## Decision
No v1 consumer requires ordering, and consumers are written so that stays true:
handlers are idempotent and commutative (wallet creates on `user.registered.v1` via
upsert; notifications are append-only). Consumer dedupe uses redis-core
(`SET NX EX 24h` on `event_id`) as belt-and-braces on top of idempotent handlers —
the handler's own idempotency is the real guarantee across Redis restarts (§13 Q6).

## Consequences
If a future projection provably needs per-key ordering, the trigger in R28 fires:
consistent-hash exchange first, log-structured broker second. Deciding then is cheap
because `bus.Bus` is the seam.
