# Three-layer boundary enforcement — demonstration (R7)

Each layer was probed with a deliberate violation on 2026-09-01; every probe
was rejected by exactly the layer designed to catch it, and the tree was
restored. Re-run the demonstration any time — the three probes and expected
failures are below.

## Layer 1 — compiler (free, absolute)

Probe: `wallet/internal/app` imports `user/internal/domain`.

```
internal/modules/wallet/internal/app/violation1.go:3:8:
    use of internal package myapp/internal/modules/user/internal/domain not allowed
```

Go's `internal/` rule. Not "should not" — *cannot compile* (PRD §4 P1).

## Layer 2 — depguard (`.golangci.yml`)

Probe: `wallet/internal/app` imports `platform/rabbit`.

```
violation2.go:3:8: import 'myapp/internal/platform/rabbit' is not allowed
    from list 'no-direct-broker': modules never touch the broker:
    events go through Deps.Outbox (P3) (depguard)
```

Also enforced by depguard: platform purity (no modules/config/app from
platform), casbin confinement to `platform/authz`, `shared/` purity,
modules never importing `config`/`app`.

## Layer 3 — arch test (`test/arch`)

Probe: `wallet/internal/app` imports `notification` (the module ROOT — which
*compiles*, and which depguard's prefix rules cannot precisely forbid).

```
--- FAIL: TestModuleBoundaries
    wallet/internal/app imports myapp/internal/modules/notification —
    cross-module import outside contracts/adapters (PRD §9)
```

Layer 3 also asserts: no `AutoMigrate` under `internal/`, no gorm/validate
tags in domain packages, no cross-schema `REFERENCES` in migrations (P5), no
cache eviction inside `InTx` closures (R44), transport never imports
bus/outbox, `jobs.Enqueue` callers allowlisted (R46), no `TableName()`
overrides in modules (they defeat the §7.1 TablePrefix containment), and it
generates `docs/modules.md` (R21).

## Why three layers

The compiler cannot see package-root imports; lint cannot express "only your
own subtree plus other modules' contracts"; the arch test is precise but
runs last. Together: zero boundary violations reachable on `main` (G2).

## Note

The probes above were run against the reference modules (`wallet`, `user`, `notification`) that lived in the tree until 2026-09-06, when the repository became a template with no business modules. The three layers are unchanged and apply to any two modules of your own; `docs/examples.md` keeps the shapes the probes targeted.

