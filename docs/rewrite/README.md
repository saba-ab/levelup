# LevelUpOS → Go rewrite documentation

These docs describe the Laravel backend in this repository and how it maps onto the Go modular-monolith
blueprint (`~/Projects/golang_advanced_blueprint`).

Start with **[00-target-architecture.md](00-target-architecture.md)**. It is the canonical plan: the module
list, the topic names, the cross-module flows, the R50+ requirements and the open questions. Wherever the
domain docs disagree with it on naming, 00 wins.

| Doc | Contents |
|---|---|
| [00-target-architecture.md](00-target-architecture.md) | Target design: 13 modules, canonical topics and jobs, event chains, decisions, NFRs, requirements, phases |
| [01-platform-cross-cutting.md](01-platform-cross-cutting.md) | Routing (97 routes), auth (Passport), authz matrix, tenancy semantics, wire contract, observers, queues, Filament, config, 287-test inventory |
| [02-identity-tenancy.md](02-identity-tenancy.md) | Auth flows, users, tenants, roles → `identity` module |
| [03-players-programs-events.md](03-players-programs-events.md) | Players, programs, event-type catalogue → `player`, `program`, `eventcatalog` |
| [04-points-levels-badges.md](04-points-levels-badges.md) | Wallet ledger, XP/levels, badges → `points`, `progression`, `badges` |
| [05-missions-streaks-rewards-leaderboards.md](05-missions-streaks-rewards-leaderboards.md) | → `missions`, `streaks`, `rewards` (claim saga), `leaderboards` |
| [06-rules-engine-activity-pipeline.md](06-rules-engine-activity-pipeline.md) | Rule grammar, execution, golden test corpus → `activity`, `rules` |

Each domain doc follows the same structure:

1. Concepts
2. Data model
3. Enums and state machines
4. Business flows
5. HTTP API
6. Authorization
7. Events and observers
8. Admin
9. Tests as an acceptance checklist
10. Bugs to fix rather than port
11. Go blueprint mapping (DDL, contracts, permissions, ports, jobs, migration)
12. Open questions

Caveat: `vendor/` was not installed while these docs were written, so no tests or `artisan route:list` were
run. Behaviour that depends on runtime is marked *inferred*.
