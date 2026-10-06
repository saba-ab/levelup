import assert from "node:assert/strict";
import { after, before, beforeEach, describe, it } from "node:test";

import { LevelUp, type Player } from "../src/index.js";
import { FakeServer, type RecordedRequest } from "./helpers/fake-server.js";

const server = new FakeServer();
let levelup: LevelUp;

before(async () => {
  await server.start();
  levelup = new LevelUp({ apiKey: "lvl_live_TESTPREFIX_secret", baseUrl: server.url, retryBackoff: 1 });
});
after(() => server.stop());
beforeEach(() => {
  server.reset();
  server.on(() => ({ status: 200, body: { data: [], next_cursor: "" } }));
});

function last(): RecordedRequest {
  return server.requests[server.requests.length - 1]!;
}

function route(): [string, string] {
  const r = last();
  const qs = r.query.toString();
  return [r.method, r.path.replace("/api/v1", "") + (qs ? `?${qs}` : "")];
}

describe("pagination", () => {
  const players = (ids: string[]): Player[] => ids.map((id) => ({ id }) as Player);

  it("follows next_cursor and keeps the filters", async () => {
    server.on((req) => {
      const cursor = req.query.get("cursor");
      if (!cursor) return { body: { data: players(["a", "b"]), next_cursor: "c1" } };
      if (cursor === "c1") return { body: { data: players(["c"]), next_cursor: "c2" } };
      return { body: { data: players(["d"]), next_cursor: "" } };
    });

    const ids: string[] = [];
    for await (const p of levelup.players.iterate({ is_active: true, limit: 2 })) {
      ids.push(p.id);
    }
    assert.deepEqual(ids, ["a", "b", "c", "d"]);
    assert.equal(server.requests.length, 3);
    for (const r of server.requests) {
      assert.equal(r.query.get("is_active"), "true");
      assert.equal(r.query.get("limit"), "2");
    }
    assert.deepEqual(
      server.requests.map((r) => r.query.get("cursor")),
      [null, "c1", "c2"],
    );
  });

  it("exposes page-level navigation", async () => {
    server.on((req) =>
      req.query.get("cursor") ? { body: { data: players(["z"]), next_cursor: "" } } : { body: { data: players(["y"]), next_cursor: "n" } },
    );
    const first = await levelup.players.list();
    assert.equal(first.hasNextPage(), true);
    assert.equal(first.nextCursor, "n");
    const second = await first.nextPage();
    assert.deepEqual(second?.data.map((p) => p.id), ["z"]);
    assert.equal(second?.hasNextPage(), false);
    assert.equal(await second?.nextPage(), null);

    let pageCount = 0;
    for await (const _ of first.pages()) pageCount++;
    assert.equal(pageCount, 2);
  });

  it("stops when the server echoes the same cursor", async () => {
    server.on(() => ({ body: { data: players(["x"]), next_cursor: "same" } }));
    const ids: string[] = [];
    for await (const p of levelup.players.iterate({ cursor: "same" })) ids.push(p.id);
    assert.deepEqual(ids, ["x"]);
  });

  it("keeps extra envelope fields on leaderboard entries", async () => {
    server.on(() => ({ body: { data: [{ rank: 1, score: 9, player_id: "p" }], next_cursor: "", period_start: "2026-01-01T00:00:00Z" } }));
    const page = await levelup.leaderboards.entries("lb1", { period: "current", limit: 5 });
    assert.equal(page.body.period_start, "2026-01-01T00:00:00Z");
    assert.equal(page.data[0]?.rank, 1);
    assert.deepEqual(route(), ["GET", "/leaderboards/lb1/entries?period=current&limit=5"]);
  });
});

describe("resource routes", () => {
  const cases: Array<[string, () => Promise<unknown>, [string, string], unknown?]> = [
    ["activities.sendBatch", () => levelup.activities.sendBatch([{ event_id: "e", event_type: "t", player_external_id: "u" }]), ["POST", "/activities/batch"], { items: [{ event_id: "e", event_type: "t", player_external_id: "u" }] }],
    ["activities.get", () => levelup.activities.get("a1"), ["GET", "/activities/a1"]],
    ["activities.list", () => levelup.activities.list({ event_type: "purchase", status: "decided" }), ["GET", "/activities?event_type=purchase&status=decided"]],
    ["players.create", () => levelup.players.create({ external_id: "u1", display_name: "U" }), ["POST", "/players"], { external_id: "u1", display_name: "U" }],
    ["players.get", () => levelup.players.get("p1"), ["GET", "/players/p1"]],
    ["players.update", () => levelup.players.update("p1", { email: "a@b.c" }), ["PATCH", "/players/p1"], { email: "a@b.c" }],
    ["players.activate", () => levelup.players.activate("p1"), ["POST", "/players/p1/activate"]],
    ["players.deactivate", () => levelup.players.deactivate("p1"), ["POST", "/players/p1/deactivate"]],
    ["wallets.get", () => levelup.wallets.get("p1"), ["GET", "/players/p1/wallet"]],
    ["wallets.transactions", () => levelup.wallets.transactions("p1", { direction: "debit" }), ["GET", "/players/p1/wallet/transactions?direction=debit"]],
    ["progression.grantXp", () => levelup.progression.grantXp("p1", { amount: 50 }), ["POST", "/players/p1/xp"], { amount: 50 }],
    ["progression.getProgress", () => levelup.progression.getProgress("p1"), ["GET", "/players/p1/progress"]],
    ["badges.list", () => levelup.badges.list({ tier: "gold", active: true }), ["GET", "/badges?tier=gold&active=true"]],
    ["badges.award", () => levelup.badges.award("b1", { player_id: "p1" }), ["POST", "/badges/b1/award"], { player_id: "p1" }],
    ["badges.revoke", () => levelup.badges.revoke("b1", "p1"), ["DELETE", "/badges/b1/players/p1"]],
    ["badges.listForPlayer", () => levelup.badges.listForPlayer("p1"), ["GET", "/players/p1/badges"]],
    ["missions.list", () => levelup.missions.list({ status: "active" }), ["GET", "/missions?status=active"]],
    ["missions.progress", () => levelup.missions.progress("m1", { player_id: "p1", increment: 2 }), ["POST", "/missions/m1/progress"], { player_id: "p1", increment: 2 }],
    ["missions.start", () => levelup.missions.start("m1", { player_id: "p1" }), ["POST", "/missions/m1/start"], { player_id: "p1" }],
    ["missions.complete", () => levelup.missions.complete("m1", { player_id: "p1" }), ["POST", "/missions/m1/complete"], { player_id: "p1" }],
    ["missions.listForPlayer", () => levelup.missions.listForPlayer("p1", { status: "completed" }), ["GET", "/players/p1/missions?status=completed"]],
    ["streaks.list", () => levelup.streaks.list({ period: "daily" }), ["GET", "/streaks?period=daily"]],
    ["streaks.record", () => levelup.streaks.record("s1", { player_id: "p1" }), ["POST", "/streaks/s1/record"], { player_id: "p1" }],
    ["streaks.listForPlayer", () => levelup.streaks.listForPlayer("p1"), ["GET", "/players/p1/streaks"]],
    ["rewards.list", () => levelup.rewards.list({ is_active: true }), ["GET", "/rewards?is_active=true"]],
    ["rewards.claim", () => levelup.rewards.claim("r1", { player_id: "p1" }), ["POST", "/rewards/r1/claim"], { player_id: "p1" }],
    ["rewards.getClaim", () => levelup.rewards.getClaim("c1"), ["GET", "/rewards/claims/c1"]],
    ["rewards.redeem", () => levelup.rewards.redeem("c1"), ["POST", "/rewards/claims/c1/redeem"]],
    ["rewards.cancelClaim", () => levelup.rewards.cancelClaim("c1"), ["POST", "/rewards/claims/c1/cancel"]],
    ["rewards.listClaims", () => levelup.rewards.listClaims("p1"), ["GET", "/players/p1/reward-claims"]],
    ["leaderboards.list", () => levelup.leaderboards.list({ type: "points" }), ["GET", "/leaderboards?type=points"]],
    ["leaderboards.playerStanding", () => levelup.leaderboards.playerStanding("lb1", "p1", { around: 2 }), ["GET", "/leaderboards/lb1/players/p1?around=2"]],
    ["rules.simulate", () => levelup.rules.simulate({ event_type: "purchase", player: { level: 3 } }), ["POST", "/rules/simulate"], { event_type: "purchase", player: { level: 3 } }],
  ];

  for (const [name, call, expected, body] of cases) {
    it(name, async () => {
      await call();
      assert.deepEqual(route(), expected);
      if (body !== undefined) {
        assert.deepEqual(last().body, body);
      }
    });
  }

  it("only money movements get an automatic Idempotency-Key", async () => {
    await levelup.badges.award("b1", { player_id: "p1" });
    assert.equal(last().headers["idempotency-key"], undefined);
    await levelup.rewards.claim("r1", { player_id: "p1" }, { idempotencyKey: "order-9" });
    assert.equal(last().headers["idempotency-key"], "order-9");
  });

  it("rejects empty path parameters before sending", async () => {
    await assert.rejects(() => levelup.players.get(""), TypeError);
    assert.equal(server.requests.length, 0);
  });
});
