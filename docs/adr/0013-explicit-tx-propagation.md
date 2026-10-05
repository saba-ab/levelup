# ADR-0013: Transactions propagate as explicit *gorm.DB arguments

**Status:** Accepted · 2026-09-01 · resolves PRD §13 Q3

## Context
Context-smuggled transactions are quiet but hide where transactions begin; explicit
arguments are verbose but greppable. Mixing the two produces silent
non-transactional writes.

## Decision
Uniformly explicit: `postgres.InTx(ctx, db, func(tx *gorm.DB) error)`; every
write-path repository method takes `tx *gorm.DB` immediately after `ctx`;
`outbox.Store.Publish(ctx, tx, topic, payload)` joins the caller's transaction.
Read-path methods use the repository's own handle. The PRD's own examples
(§7.8.1, §8) pass tx explicitly; `*gorm.DB` (not `pgx.Tx`) because repositories are
GORM and pessimistic locking uses GORM clauses (§7.1).

## Consequences
Transaction boundaries are visible at every call site and greppable
(`InTx(` / `tx *gorm.DB`). The arch test for R44 (no cache invalidation inside a tx)
keys off the `InTx` closure, which explicit propagation makes statically findable.
