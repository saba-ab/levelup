import assert from "node:assert/strict";
import { after, before, beforeEach, describe, it } from "node:test";

import { LevelUp, LevelUpConnectionError, LevelUpError, LevelUpTimeoutError } from "../src/index.js";
import { parseRetryAfter } from "../src/core.js";
import { FakeServer } from "./helpers/fake-server.js";

const API_KEY = "lvl_live_TESTPREFIX_testsecret";
const server = new FakeServer();

function client(overrides: Partial<ConstructorParameters<typeof LevelUp>[0]> = {}): LevelUp {
  return new LevelUp({ apiKey: API_KEY, baseUrl: server.url, retryBackoff: 1, timeout: 2_000, ...overrides });
}

async function capture(p: Promise<unknown>): Promise<LevelUpError> {
  try {
    await p;
  } catch (err) {
    assert.ok(err instanceof LevelUpError, `expected LevelUpError, got ${String(err)}`);
    return err;
  }
  assert.fail("expected the call to throw");
}

before(() => server.start());
after(() => server.stop());
beforeEach(() => server.reset());

describe("client construction", () => {
  it("requires an api key", () => {
    assert.throws(() => new LevelUp({ apiKey: "" }), TypeError);
  });

  it("defaults to the production base URL and strips trailing slashes", () => {
    assert.equal(new LevelUp({ apiKey: API_KEY }).baseUrl, "https://api.levelupos.ge");
    assert.equal(new LevelUp({ apiKey: API_KEY, baseUrl: "http://x.test///" }).baseUrl, "http://x.test");
  });
});

describe("requests", () => {
  it("sends bearer auth, JSON and the /api/v1 prefix", async () => {
    server.sequence({ status: 202, body: { activity_id: "a1", status: "pending", duplicate: false } });
    const res = await client().activities.send({
      event_id: "evt-1",
      event_type: "purchase",
      player_external_id: "user-1",
      properties: { amount: 42 },
      occurred_at: new Date("2026-01-02T03:04:05Z"),
    });
    assert.equal(res.activity_id, "a1");
    const req = server.requests[0]!;
    assert.equal(req.method, "POST");
    assert.equal(req.path, "/api/v1/activities");
    assert.equal(req.headers["authorization"], `Bearer ${API_KEY}`);
    assert.equal(req.headers["content-type"], "application/json");
    assert.match(String(req.headers["user-agent"]), /^levelup-typescript\//);
    assert.deepEqual(req.body, {
      event_id: "evt-1",
      event_type: "purchase",
      player_external_id: "user-1",
      properties: { amount: 42 },
      occurred_at: "2026-01-02T03:04:05.000Z",
    });
  });

  it("encodes path segments and query params, skipping undefined", async () => {
    server.sequence({ status: 200, body: { id: "p1", external_id: "a/b c" } });
    await client().players.getByExternalId("a/b c");
    assert.equal(server.requests[0]!.path, "/api/v1/players/by-external-id/a%2Fb%20c");

    server.sequence({ status: 200, body: { data: [], next_cursor: "" } });
    await client().players.list({ is_active: false, search: undefined, limit: 10 });
    const q = server.requests[1]!.query;
    assert.equal(q.get("is_active"), "false");
    assert.equal(q.get("limit"), "10");
    assert.equal(q.has("search"), false);
  });

  it("resolves void on 204", async () => {
    server.sequence({ status: 204 });
    const out = await client().players.delete("p1");
    assert.equal(out, undefined);
    assert.equal(server.requests[0]!.method, "DELETE");
  });
});

describe("errors", () => {
  it("maps problem+json onto LevelUpError", async () => {
    server.sequence({
      status: 422,
      body: {
        type: "about:blank",
        title: "Unprocessable Entity",
        status: 422,
        detail: "requested 50, available 10",
        code: "insufficient_balance",
        errors: { amount: "too large" },
        trace_id: "trace-123",
      },
    });
    const err = await capture(client().wallets.debit("p1", { amount: 50, kind: "spend" }));
    assert.equal(err.status, 422);
    assert.equal(err.code, "insufficient_balance");
    assert.equal(err.detail, "requested 50, available 10");
    assert.deepEqual(err.fieldErrors, { amount: "too large" });
    assert.equal(err.traceId, "trace-123");
    assert.match(err.message, /insufficient_balance/);
  });

  it("falls back to a code when the body has none or is not JSON", async () => {
    server.sequence({ status: 404, body: "<html>nope</html>" });
    const err = await capture(client().players.get("p1"));
    assert.equal(err.status, 404);
    assert.equal(err.code, "not_found");
    assert.equal(err.body, "<html>nope</html>");
  });

  it("uses a snake_case title as code (rate limiter responses)", async () => {
    server.sequence({ status: 429, body: { type: "about:blank", title: "rate_limited", status: 429 } });
    const err = await capture(client({ retries: 0 }).players.get("p1"));
    assert.equal(err.code, "rate_limited");
  });
});

describe("retries", () => {
  it("retries GET on 503 then succeeds", async () => {
    server.sequence({ status: 503, body: { status: 503 } }, { status: 503 }, { status: 200, body: { id: "p1" } });
    const p = await client().players.get("p1");
    assert.equal(p.id, "p1");
    assert.equal(server.requests.length, 3);
  });

  it("gives up after `retries` and throws the last error", async () => {
    server.sequence({ status: 502, body: { status: 502, code: "bad_gateway" } });
    const err = await capture(client({ retries: 2 }).players.get("p1"));
    assert.equal(err.status, 502);
    assert.equal(server.requests.length, 3);
  });

  it("does not retry non-retryable statuses", async () => {
    server.sequence({ status: 500, body: { status: 500 } });
    await capture(client().players.get("p1"));
    assert.equal(server.requests.length, 1);
  });

  it("does not retry a POST without an Idempotency-Key", async () => {
    server.sequence({ status: 503 });
    await capture(client().activities.send({ event_id: "e", event_type: "t", player_external_id: "u" }));
    assert.equal(server.requests.length, 1);
  });

  it("retries a POST carrying an Idempotency-Key with the same key", async () => {
    server.sequence({ status: 429, headers: { "Retry-After": "0" } }, { status: 202, body: { activity_id: "a" } });
    await client().activities.send({ event_id: "e", event_type: "t", player_external_id: "u" }, { idempotencyKey: "k-1" });
    assert.equal(server.requests.length, 2);
    assert.equal(server.requests[0]!.headers["idempotency-key"], "k-1");
    assert.equal(server.requests[1]!.headers["idempotency-key"], "k-1");
  });

  it("honours Retry-After", async () => {
    server.sequence({ status: 429, headers: { "Retry-After": "1" } }, { status: 200, body: { id: "p1" } });
    const started = Date.now();
    await client({ retryBackoff: 0 }).players.get("p1");
    assert.ok(Date.now() - started >= 900, "should wait ~1s as instructed by Retry-After");
  });

  it("retries network errors for GET and surfaces LevelUpConnectionError", async () => {
    server.sequence({ drop: true });
    const err = await capture(client({ retries: 1 }).players.get("p1"));
    assert.ok(err instanceof LevelUpConnectionError);
    assert.equal(err.status, 0);
    assert.equal(err.code, "connection_error");
    assert.equal(server.requests.length, 2);
  });

  it("times out with LevelUpTimeoutError", async () => {
    server.sequence({ delay: 300, status: 200, body: {} });
    const err = await capture(client({ timeout: 50, retries: 0 }).players.get("p1"));
    assert.ok(err instanceof LevelUpTimeoutError);
    assert.equal(err.code, "timeout");
  });

  it("parses Retry-After seconds and HTTP dates", () => {
    assert.equal(parseRetryAfter("3"), 3000);
    assert.equal(parseRetryAfter(null), null);
    assert.equal(parseRetryAfter("garbage"), null);
    const inFuture = new Date(Date.now() + 5_000).toUTCString();
    const ms = parseRetryAfter(inFuture)!;
    assert.ok(ms > 3_000 && ms <= 5_000);
  });
});

describe("idempotency keys on money movements", () => {
  const entry = { id: "e1", amount: 10, balance_after: 10 };

  it("generates a UUIDv4 when none is given, a fresh one per call", async () => {
    server.sequence({ status: 201, body: entry });
    const c = client();
    await c.wallets.credit("p1", { amount: 10, kind: "earn" });
    await c.wallets.credit("p1", { amount: 10, kind: "earn" });
    const k1 = String(server.requests[0]!.headers["idempotency-key"]);
    const k2 = String(server.requests[1]!.headers["idempotency-key"]);
    assert.match(k1, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
    assert.notEqual(k1, k2);
  });

  it("uses the caller's key for credit, debit and transfer", async () => {
    server.sequence({ status: 201, body: entry });
    const c = client();
    await c.wallets.credit("p1", { amount: 1, kind: "bonus" }, { idempotencyKey: "c-1" });
    await c.wallets.debit("p1", { amount: 1, kind: "spend" }, { idempotencyKey: "d-1" });
    await c.wallets.transfer({ from_player_id: "p1", to_player_id: "p2", amount: 1 }, { idempotencyKey: "t-1" });
    assert.deepEqual(
      server.requests.map((r) => [r.path, r.headers["idempotency-key"]]),
      [
        ["/api/v1/players/p1/wallet/credit", "c-1"],
        ["/api/v1/players/p1/wallet/debit", "d-1"],
        ["/api/v1/wallets/transfer", "t-1"],
      ],
    );
  });

  it("retries a credit on 503 with the same generated key", async () => {
    server.sequence({ status: 503 }, { status: 201, body: entry });
    await client().wallets.credit("p1", { amount: 5, kind: "earn" });
    assert.equal(server.requests.length, 2);
    assert.equal(server.requests[0]!.headers["idempotency-key"], server.requests[1]!.headers["idempotency-key"]);
  });
});
