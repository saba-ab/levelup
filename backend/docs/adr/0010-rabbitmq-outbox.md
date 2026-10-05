# ADR-0010: RabbitMQ behind a transactional outbox; events and jobs kept apart

**Status:** Accepted · 2026-09-01

## Context
Cross-module writes must not create dual-write bugs (PRD §4 P3); at 7k RPS Kafka's
ordering/replay machinery is unjustified (PRD §3).

## Decision
Postgres outbox is the only publisher to RabbitMQ; the dispatcher awaits publisher
confirms before marking rows published. Events (`bus.Bus`, topic exchange, many
consumers) and jobs (`jobs.Queue`, direct exchange, one consumer, priority) are
separate abstractions (PRD §7.9). Every queue is quorum; the single documented
exception is the classic priority queue. `bus.Bus` stays an interface so a
log-structured broker remains possible (R28).

## Consequences
A broker outage degrades (outbox accumulates, `outbox_unpublished_age_seconds` climbs)
instead of losing facts. Jobs enqueued directly are fire-and-forget by definition;
state-change-triggered jobs route through the outbox (R46).
