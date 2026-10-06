# LevelUp TypeScript SDK

Official server-side SDK for the LevelUp API (`/api/v1`). Use it from your backend (checkout, game servers,
CRMs) to report activities, manage players and move points.

- Zero runtime dependencies. Uses the built-in `fetch`, so it needs Node.js 18 or later.
- Ships ESM and CommonJS builds, with full TypeScript types for every request and response body.
- Retries safely, paginates with cursors, and verifies webhook signatures.

> API keys are secrets. Never ship them to a browser or a mobile app.

## Install

```bash
npm install @levelup/sdk
```

## Quickstart

```ts
import { LevelUp } from "@levelup/sdk"; // or: const { LevelUp } = require("@levelup/sdk");

const levelup = new LevelUp({
  apiKey: process.env.LEVELUP_API_KEY!, // lvl_live_…
  // baseUrl: "https://api.levelupos.ge",  (default; the SDK appends /api/v1)
  // timeout: 30_000,                       (ms, per attempt)
  // retries: 2,
});

// 1. Report an activity. The rules engine decides what it earns.
const res = await levelup.activities.send({
  event_id: "order-1042",            // your unique id; re-sending it is deduplicated
  event_type: "purchase",
  player_external_id: "user-7",
  properties: { amount: 59.9, currency: "GEL" },
  occurred_at: new Date(),
});
console.log(res.status, res.duplicate);

// 2. Credit points directly.
const player = await levelup.players.getByExternalId("user-7");
const entry = await levelup.wallets.credit(
  player.id,
  { amount: 100, kind: "bonus", description: "Welcome bonus" },
  { idempotencyKey: `welcome-${player.id}` }, // optional; one is generated if omitted
);
console.log(entry.balance_after);
```

### 3. Verify a webhook

LevelUp signs every delivery with this header:

```
LevelUp-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256(secret, "<t>.<raw body>")>
```

Each delivery also carries two more headers:

- `LevelUp-Event` is the event type, for example `badges.awarded`.
- `LevelUp-Delivery` is the delivery id. It stays the same when a delivery is retried, so use it to drop duplicates.

`verifyWebhook` does these checks:

1. It recomputes the HMAC over the raw bytes.
2. It compares the signatures in constant time and accepts any of several `v1` values, which allows secret rotation.
3. It rejects timestamps more than `toleranceSeconds` (default 300) from now.
4. It returns the parsed JSON body.

```ts
import express from "express";
import { verifyWebhook, WebhookVerificationError } from "@levelup/sdk";

app.post("/webhooks/levelup", express.raw({ type: "application/json" }), (req, res) => {
  try {
    const event = verifyWebhook(req.body /* raw Buffer */, req.get("LevelUp-Signature"), process.env.LEVELUP_WEBHOOK_SECRET!);
    const deliveryId = req.get("LevelUp-Delivery");   // dedupe on this
    const eventType = req.get("LevelUp-Event");
    // event: { event, event_id, occurred_at, tenant_id, data }
    res.sendStatus(204);
  } catch (err) {
    if (err instanceof WebhookVerificationError) return res.status(400).send(err.code);
    throw err;
  }
});
```

Always pass the raw body, either the Buffer or the unmodified string. A body that was parsed and re-serialised
will not verify. The `signWebhook(payload, secret, timestamp?)` helper produces a valid header for your own tests.

## Error handling

Every failure throws a `LevelUpError`. HTTP errors are RFC 9457 `application/problem+json`. The error has
these fields:

| field         | meaning                                                                     |
|---------------|-----------------------------------------------------------------------------|
| `status`      | HTTP status, or `0` if no response was received                             |
| `code`        | stable snake_case code, e.g. `insufficient_balance`, `player_not_found`     |
| `detail`      | human-readable explanation. Don't branch on it                              |
| `fieldErrors` | per-field validation messages (422)                                         |
| `traceId`     | server trace id. Include it in support requests                             |

```ts
import { LevelUpError, LevelUpConnectionError } from "@levelup/sdk";

try {
  await levelup.wallets.debit(playerId, { amount: 500, kind: "redeem" });
} catch (err) {
  if (err instanceof LevelUpError && err.code === "insufficient_balance") {
    // tell the user
  } else if (err instanceof LevelUpConnectionError) {
    // network failure or timeout (err.code === "timeout"); status is 0
  } else {
    throw err;
  }
}
```

### Retries and idempotency

The SDK retries **only** requests that are safe to repeat:

- Retries happen on `429`, `502`, `503` and `504`, and on network errors or timeouts.
- They apply only to `GET` requests and to requests that carry an `Idempotency-Key`.
- The delay is exponential backoff with jitter, starting at `retryBackoff` (500 ms by default).
- When the server sends `Retry-After`, the SDK waits that long instead. Every delay is capped at `maxRetryDelay`.
- `retries` sets the number of retries after the first attempt. The default is 2.

`wallets.credit`, `wallets.debit` and `wallets.transfer` **always** send an `Idempotency-Key`. They use yours
when you pass one, and otherwise generate a UUIDv4. Pass a key derived from your own records, such as an order
id. Then re-running your job cannot move points twice: the server replays the original response.

Any other call accepts `{ idempotencyKey }` too, for example `badges.award`, `rewards.claim`,
`progression.grantXp`, or `activities.send` with the `event_id`. Passing a key makes that call retryable.

The last argument of every method is `RequestOptions`. It accepts `idempotencyKey`, `timeout`, `retries`,
`signal` and `headers`.

## Pagination

List endpoints use cursors and return a `Page<T>`:

```ts
const page = await levelup.players.list({ is_active: true, limit: 100 });
page.data;          // Player[]
page.nextCursor;    // string | null
const next = await page.nextPage(); // Page | null

// Every item across all pages, fetched lazily:
for await (const player of levelup.players.iterate({ is_active: true })) {
  console.log(player.external_id);
}

// Page by page:
for await (const p of page.pages()) console.log(p.data.length);
```

The same pattern applies everywhere: `activities.iterate`, `wallets.iterateTransactions`, `badges.iterate`, and
so on. Every `Page` is also async-iterable over its items.

## API surface

| resource       | methods |
|----------------|---------|
| `activities`   | `send`, `sendBatch` (≤100), `get`, `list`, `iterate` |
| `players`      | `create`, `get`, `getByExternalId`, `update` (partial), `list`, `iterate`, `activate`, `deactivate`, `delete` |
| `wallets`      | `get`, `credit`, `debit`, `transfer`, `transactions`, `iterateTransactions` |
| `progression`  | `grantXp`, `getProgress` |
| `badges`       | `list`, `iterate`, `award`, `revoke`, `listForPlayer` |
| `missions`     | `list`, `iterate`, `progress`, `start`, `complete`, `listForPlayer` |
| `streaks`      | `list`, `iterate`, `record`, `listForPlayer` |
| `rewards`      | `list`, `iterate`, `claim`, `getClaim`, `redeem`, `cancelClaim`, `listClaims` |
| `leaderboards` | `list`, `iterate`, `entries`, `playerStanding` |
| `rules`        | `simulate` |

Request and response fields use the API's own snake_case names. For an endpoint without a dedicated method,
use `levelup.request({ method, path, query, body, options })`. The `path` is relative to `/api/v1`.

## Development

```bash
npm install              # only devDependency: typescript
npm test                 # tsc -p . (src + tests -> build/), then node --test build/test/*.test.js
npm run build            # dist/esm + dist/cjs
```

The tests run against a local fake HTTP server and never touch the real API. The package deliberately has no
`@types/node` dependency. The few Node built-ins it uses are declared in `types/node-shim.d.ts`, which is used
only at compile time.
