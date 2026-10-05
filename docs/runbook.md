# Runbook

Operational reference for the blueprint's failure modes. Every threshold and
exception here is deliberate; change them by ADR, not by incident hotfix.

## Alerts that page (R40)

| Metric | Threshold | What it means / first move |
|---|---|---|
| `outbox_unpublished_age_seconds` | > 60s | The single best health signal in the system. Dispatcher stuck, Rabbit unreachable, or confirms failing. Check dispatcher logs, then `rabbitmq_connection_blocked`. Facts are safe in Postgres; this is a delivery delay, not loss. |
| `rabbitmq_queue_depth{queue}` | sustained growth | Consumer death shows here before user complaints. Check worker replicas and their logs. |
| `rabbitmq_dlq_total{queue}` | any positive **rate** (alert on the derivative) | Poison messages parking. Inspect the DLQ payloads, fix the consumer, then `task rabbit:replay QUEUE=<source queue>`. |
| `outbox_dead_letters_total{source}` | any positive **rate** | A row landed in `outbox_svc.dead_letters` (R23). `source="dispatcher"`: the broker refused the row `OUTBOX_MAX_PUBLISH_ATTEMPTS` times — almost always a full queue with `reject-publish` (R48); fix whatever let it fill, then `task outbox:deadletter:replay`. `source="consumer"`: a mirror of a broker DLQ park; read the row, then follow the DLQ procedure above. |
| `pgxpool_acquire_wait_seconds` | p99 > 50ms | Pool exhaustion: presents as latency everywhere with a cause nowhere. Re-check pool math (below) before scaling anything. |
| `rabbitmq_connection_blocked` | == 1 for > 30s | Broker disk/memory alarm: publishers are blocked WITHOUT a closed connection (R48). Free broker resources; the outbox drains on its own afterwards. |

`/metrics` and pprof live on the admin port only — never expose it publicly.

## Deliberate exceptions and asymmetries

- **`job.priority` is the only classic queue** (PRD §7.9): quorum queues do
  not support `x-max-priority`. The durability trade is confined to
  fire-and-forget work; nothing that must survive a node loss may use
  priorities. `Topology.Verify()` allowlists exactly this queue.
- **Rate limiting fails OPEN, locks fail CLOSED** (R43). A dead Redis must
  not 429 the API; a dead Redis must also never let two schedulers run one
  job. Do not "fix" one direction to match the other.
- **The outbox table and `authz_svc` are the only cross-module storage
  exceptions**: modules INSERT into `outbox_svc.outbox_events` inside their
  own transactions (that is the design), and `authz_svc` is platform-owned.
- **A `FLUSHALL` on redis-core logs everyone out** (refresh tokens live
  there). Accepted trade (PRD §7.8.2); this is why redis-core runs
  `noeviction` with persistence and redis-cache is a separate instance.

## Consumer checklist (review, on every new subscriber/job)

- Manual ack only — `autoAck: true` is the most common way messages vanish.
- Handler idempotent by construction (upsert on `event_id` or natural key);
  the Redis dedupe wrapper is belt, not braces.
- Retries bounded: the platform ladder gives 3 tiers then DLQ; never
  `nack(requeue=true)` in a loop. Every park is mirrored into
  `outbox_svc.dead_letters` with the handler's error: query it before
  opening the management UI.
- Scheduled jobs RECONCILING, never incremental (R47): "sweep since
  last-successful-run", so a missed tick self-heals.
- State-change-triggered work goes through the outbox as topic
  `job.<name>` (R46); direct `jobs.Enqueue` is for work you can lose.

## Replay procedures (R49)

- **Republish outbox events** (consumer bug, projection rebuild):
  `task outbox:replay FROM=2026-09-01T00:00:00Z TOPIC=user.registered.v1`
  Published rows are retained 7 days; consumers dedupe on `event_id`, so
  replay over-delivery is safe.
- **Drain a DLQ after fixing the consumer**:
  `task rabbit:replay QUEUE=evt.<group>.<topic>` (for example `evt.billing.invoice.created.v1`)
  The shovel resets the retry ladder (fresh `x-attempt`).
- **Inspect and replay dispatcher dead letters** (R23):
  `task outbox:deadletter:list`, then `task outbox:deadletter:replay ID=<id>`
  or `task outbox:deadletter:replay ALL=true TOPIC=<topic>`. Rows re-enter
  `outbox_events` with a fresh attempt count. A nacked row backs off (1s
  doubling to 60s) and is parked after the attempt cap, so one full queue
  can no longer hold every other topic hostage. Consumer rows in the same
  table are a mirror only: replay those from the broker as above.

## Pool math (the most common Go outage at this scale)

`pods × DB_MAX_CONNS` must stay under the PgBouncer pool, which must stay
under Postgres `max_connections`. Defaults: 8 conns/pod, PgBouncer pool 40.
Behind PgBouncer transaction pooling, `PrepareStmt: false` and pgx simple
protocol are mandatory — "prepared statement already exists" errors mean
someone reverted them.

## Deployment shape notes

- `cmd/scheduler` at `replicas: 1` — and the per-tick Redis lock still
  matters, because replicas:1 is a lie during a rolling deploy.
- RabbitMQ: 3 nodes minimum, one per AZ, `pause_minority`, quorum queues
  everywhere (`Topology.Verify` refuses to boot otherwise, R45). Two nodes
  is worse than one: a two-node Raft cluster cannot form a majority.
- Migrations run direct-to-Postgres (`DB_MIGRATE_DSN`), never through
  PgBouncer: goose relies on `search_path`, a session setting.
- Eventual consistency windows: user.registered → wallet/notification
  reactions are asynchronous (normally < 1s, bounded by dispatcher poll +
  queue depth). Where product needs read-your-write, keep it in one module.
