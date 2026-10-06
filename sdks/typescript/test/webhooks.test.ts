import assert from "node:assert/strict";
import { createHmac } from "node:crypto";
import { describe, it } from "node:test";

import { WebhookVerificationError, signWebhook, verifyWebhook } from "../src/index.js";

const SECRET = "whsec_test_secret";
const BODY = '{"event":"badges.awarded","event_id":"evt_1","occurred_at":"2026-10-05T12:00:00Z","tenant_id":"t1","data":{"player_id":"p1","badge":"ünïcode"}}';
const NOW = 1_760_000_000;

function header(body: string | Uint8Array, t = NOW, secret = SECRET): string {
  const mac = createHmac("sha256", secret).update(`${t}.`).update(body).digest("hex");
  return `t=${t},v1=${mac}`;
}

function code(fn: () => unknown): string {
  try {
    fn();
  } catch (err) {
    assert.ok(err instanceof WebhookVerificationError, `unexpected ${String(err)}`);
    return err.code;
  }
  assert.fail("expected verification to fail");
}

describe("verifyWebhook", () => {
  it("accepts a valid signature over a string body and returns the parsed event", () => {
    const event = verifyWebhook(BODY, header(BODY), SECRET, 300, { now: NOW });
    assert.equal(event.event, "badges.awarded");
    assert.equal(event.tenant_id, "t1");
  });

  it("accepts raw bytes", () => {
    const bytes = new TextEncoder().encode(BODY);
    const event = verifyWebhook<{ data: { badge: string } }>(bytes, header(bytes), SECRET, 300, { now: NOW + 10 });
    assert.equal(event.data.badge, "ünïcode");
  });

  it("matches the documented HMAC scheme (signWebhook round-trip)", () => {
    assert.equal(signWebhook(BODY, SECRET, NOW), header(BODY));
  });

  it("accepts any of several v1 signatures (secret rotation) and ignores unknown schemes", () => {
    const good = header(BODY).split("v1=")[1];
    const h = `t=${NOW},v0=abc,v1=${"0".repeat(64)},v1=${good}`;
    assert.ok(verifyWebhook(BODY, h, SECRET, 300, { now: NOW }));
  });

  it("rejects a tampered body", () => {
    assert.equal(code(() => verifyWebhook(BODY.replace("p1", "p2"), header(BODY), SECRET, 300, { now: NOW })), "signature_mismatch");
  });

  it("rejects the wrong secret", () => {
    assert.equal(code(() => verifyWebhook(BODY, header(BODY, NOW, "other"), SECRET, 300, { now: NOW })), "signature_mismatch");
  });

  it("rejects stale and future timestamps", () => {
    assert.equal(code(() => verifyWebhook(BODY, header(BODY, NOW - 301), SECRET, 300, { now: NOW })), "timestamp_out_of_tolerance");
    assert.equal(code(() => verifyWebhook(BODY, header(BODY, NOW + 301), SECRET, 300, { now: NOW })), "timestamp_out_of_tolerance");
    assert.ok(verifyWebhook(BODY, header(BODY, NOW - 300), SECRET, 300, { now: NOW }));
  });

  it("rejects a re-signed timestamp (timestamp is covered by the MAC)", () => {
    const sig = header(BODY, NOW - 1000).split(",")[1];
    assert.equal(code(() => verifyWebhook(BODY, `t=${NOW},${sig}`, SECRET, 300, { now: NOW })), "signature_mismatch");
  });

  it("rejects missing and malformed headers", () => {
    assert.equal(code(() => verifyWebhook(BODY, undefined, SECRET)), "missing_signature");
    assert.equal(code(() => verifyWebhook(BODY, "", SECRET)), "missing_signature");
    assert.equal(code(() => verifyWebhook(BODY, "garbage", SECRET)), "malformed_signature");
    assert.equal(code(() => verifyWebhook(BODY, `t=${NOW}`, SECRET)), "malformed_signature");
    assert.equal(code(() => verifyWebhook(BODY, "v1=" + "a".repeat(64), SECRET)), "malformed_signature");
    assert.equal(code(() => verifyWebhook(BODY, `t=abc,v1=${"a".repeat(64)}`, SECRET)), "malformed_signature");
  });

  it("rejects a signed body that is not JSON", () => {
    assert.equal(code(() => verifyWebhook("not json", header("not json"), SECRET, 300, { now: NOW })), "invalid_payload");
  });

  it("requires a secret", () => {
    assert.throws(() => verifyWebhook(BODY, header(BODY), ""), TypeError);
  });
});
