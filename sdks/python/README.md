# LevelUp Python SDK

Official server-side SDK for the LevelUp API (`/api/v1`). Use it from your backend to report activities,
manage players and move points.

- Standard library only. The SDK uses `urllib`, has no dependencies, and supports Python 3.9 and later.
- Responses are typed with `TypedDict`s (`levelup.types`).
- The SDK retries safely, paginates with cursors, and verifies webhook signatures.

> API keys are secrets. Never ship them to a browser or a mobile app.

## Install

```bash
pip install levelup            # or, from this repo: pip install ./sdks/python
```

## Quickstart

```python
import os
import levelup

client = levelup.Client(
    api_key=os.environ["LEVELUP_API_KEY"],    # lvl_live_…
    # base_url="https://api.levelupos.ge",    (default; the SDK appends /api/v1)
    # timeout=30.0,                           (seconds, per attempt)
    # retries=2,
)

# 1. Report an activity. The rules engine decides what it earns.
res = client.activities.send(
    event_id="order-1042",                 # your unique id; re-sending it is deduplicated
    event_type="purchase",
    player_external_id="user-7",
    properties={"amount": 59.9, "currency": "GEL"},
)
print(res["status"], res.get("duplicate"))

# 2. Credit points directly.
player = client.players.get_by_external_id("user-7")
entry = client.wallets.credit(
    player["id"], 100, kind="bonus", description="Welcome bonus",
    idempotency_key=f"welcome-{player['id']}",   # optional; one is generated if omitted
)
print(entry["balance_after"])
```

### 3. Verify a webhook

LevelUp signs every delivery with this header:

```
LevelUp-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256(secret, "<t>.<raw body>")>
```

Each delivery also carries two more headers:

- `LevelUp-Event` is the event type, for example `badges.awarded`.
- `LevelUp-Delivery` is the delivery id. It stays the same when a delivery is retried, so use it to drop duplicates.

`verify_webhook` does these checks:

1. It recomputes the HMAC over the raw bytes.
2. It compares the signatures in constant time and accepts any of several `v1` values, which allows secret rotation.
3. It rejects timestamps more than `tolerance_seconds` (default 300) from now.
4. It returns the parsed JSON body.

```python
import os
from flask import Flask, request, abort
import levelup

app = Flask(__name__)

@app.post("/webhooks/levelup")
def levelup_webhook():
    try:
        event = levelup.verify_webhook(
            request.get_data(),                         # raw bytes, NOT request.json
            request.headers.get(levelup.SIGNATURE_HEADER),
            os.environ["LEVELUP_WEBHOOK_SECRET"],
        )
    except levelup.WebhookVerificationError as err:
        abort(400, err.code)
    delivery_id = request.headers.get(levelup.DELIVERY_HEADER)   # dedupe on this
    event_type = request.headers.get(levelup.EVENT_HEADER)
    # event: {"event", "event_id", "occurred_at", "tenant_id", "data"}
    return "", 204
```

`levelup.sign_webhook(payload, secret, timestamp=None)` produces a valid header for testing your endpoint.

## Error handling

Every failure raises `levelup.LevelUpError`. HTTP errors are RFC 9457 `application/problem+json`. The error has
these attributes:

| attribute      | meaning                                                                 |
|----------------|-------------------------------------------------------------------------|
| `status`       | HTTP status, or `0` if no response was received                         |
| `code`         | stable snake_case code, e.g. `insufficient_balance`, `player_not_found` |
| `detail`       | human-readable explanation. Don't branch on it                          |
| `field_errors` | per-field validation messages (422)                                     |
| `trace_id`     | server trace id. Include it in support requests                         |

```python
try:
    client.wallets.debit(player_id, 500, kind="redeem")
except levelup.LevelUpConnectionError as err:   # network failure / timeout (err.code == "timeout"), status 0
    ...
except levelup.LevelUpError as err:
    if err.code == "insufficient_balance":
        ...
    else:
        raise
```

### Retries and idempotency

The SDK retries **only** requests that are safe to repeat:

- Retries happen on `429`, `502`, `503` and `504`, and on network errors or timeouts.
- They apply only to `GET` requests and to requests that carry an `Idempotency-Key`.
- The delay is exponential backoff with jitter, starting at `retry_backoff` (0.5 s by default).
- When the server sends `Retry-After`, the SDK waits that long instead. Every delay is capped at `max_retry_delay`.
- `retries` sets the number of retries after the first attempt. The default is 2.

`wallets.credit`, `wallets.debit` and `wallets.transfer` **always** send an `Idempotency-Key`. They use yours
when you pass one, and otherwise generate a UUIDv4. Pass a key derived from your own records, such as an order
id. Then re-running your job cannot move points twice: the server replays the original response.

Several other mutating methods take `idempotency_key=` too, which makes them retryable:

- `activities.send` and `activities.send_batch`
- `players.create`
- `progression.grant_xp`
- `badges.award`
- `missions.progress`
- `streaks.record`
- `rewards.claim`

Every method also accepts `options={"timeout": ..., "retries": ..., "headers": {...}, "idempotency_key": ...}`.

`timeout` is the `urllib` socket timeout, which applies per blocking operation rather than to the whole request.

## Pagination

List endpoints use cursors and return a `levelup.Page`:

```python
page = client.players.list(is_active=True, limit=100)
page.data           # list of players on this page
page.next_cursor    # str | None
nxt = page.next_page()   # Page | None

# Every item across all pages, fetched lazily:
for player in client.players.iterate(is_active=True):
    print(player["external_id"])

# Iterating a Page also walks all following pages; page by page:
for p in page.iter_pages():
    print(len(p.data))
```

The same pattern applies everywhere: `activities.iterate`, `wallets.iter_transactions`, `badges.iterate`, and
so on.

## API surface

| resource       | methods |
|----------------|---------|
| `activities`   | `send`, `send_batch` (≤100), `get`, `list`, `iterate` |
| `players`      | `create`, `get`, `get_by_external_id`, `update` (partial), `list`, `iterate`, `activate`, `deactivate`, `delete` |
| `wallets`      | `get`, `credit`, `debit`, `transfer`, `transactions`, `iter_transactions` |
| `progression`  | `grant_xp`, `get_progress` |
| `badges`       | `list`, `iterate`, `award`, `revoke`, `list_for_player` |
| `missions`     | `list`, `iterate`, `progress`, `start`, `complete`, `list_for_player` |
| `streaks`      | `list`, `iterate`, `record`, `list_for_player` |
| `rewards`      | `list`, `iterate`, `claim`, `get_claim`, `redeem`, `cancel_claim`, `list_claims` |
| `leaderboards` | `list`, `iterate`, `entries`, `player_standing` |
| `rules`        | `simulate` |

Responses are plain dicts with the API's snake_case keys. `datetime` arguments are serialised as RFC 3339 UTC.
For an endpoint without a dedicated method, use `client.request(method, path, query=..., body=...)`. The `path`
is relative to `/api/v1`.

## Development

```bash
python3 -m unittest discover sdks/python/tests
```

The tests run against a local `http.server` fake and never touch the real API.
