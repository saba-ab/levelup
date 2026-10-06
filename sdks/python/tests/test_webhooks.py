from __future__ import annotations

import hashlib
import hmac
import unittest

import fakeserver  # noqa: F401  (puts src/ on sys.path)

from levelup import WebhookVerificationError, sign_webhook, verify_webhook

SECRET = "whsec_test_secret"
BODY = '{"event":"badges.awarded","event_id":"evt_1","occurred_at":"2026-10-05T12:00:00Z","tenant_id":"t1","data":{"player_id":"p1","badge":"ünïcode"}}'
NOW = 1_760_000_000


def header(body, t: int = NOW, secret: str = SECRET) -> str:
    raw = body.encode() if isinstance(body, str) else body
    mac = hmac.new(secret.encode(), f"{t}.".encode() + raw, hashlib.sha256).hexdigest()
    return f"t={t},v1={mac}"


class VerifyWebhookTest(unittest.TestCase):
    def code(self, *args, **kwargs) -> str:
        with self.assertRaises(WebhookVerificationError) as ctx:
            verify_webhook(*args, **kwargs)
        return ctx.exception.code

    def test_valid_string_body(self) -> None:
        event = verify_webhook(BODY, header(BODY), SECRET, now=NOW)
        self.assertEqual(event["event"], "badges.awarded")
        self.assertEqual(event["tenant_id"], "t1")

    def test_valid_bytes_body(self) -> None:
        raw = BODY.encode()
        event = verify_webhook(raw, header(raw), SECRET, now=NOW + 10)
        self.assertEqual(event["data"]["badge"], "ünïcode")

    def test_sign_round_trip(self) -> None:
        self.assertEqual(sign_webhook(BODY, SECRET, NOW), header(BODY))

    def test_multiple_v1_and_unknown_schemes(self) -> None:
        good = header(BODY).split("v1=")[1]
        h = f"t={NOW},v0=abc,v1={'0' * 64},v1={good.upper()}"
        self.assertTrue(verify_webhook(BODY, h, SECRET, now=NOW))

    def test_tampered_body(self) -> None:
        self.assertEqual(self.code(BODY.replace("p1", "p2"), header(BODY), SECRET, now=NOW), "signature_mismatch")

    def test_wrong_secret(self) -> None:
        self.assertEqual(self.code(BODY, header(BODY, secret="other"), SECRET, now=NOW), "signature_mismatch")

    def test_stale_and_future_timestamps(self) -> None:
        self.assertEqual(self.code(BODY, header(BODY, NOW - 301), SECRET, now=NOW), "timestamp_out_of_tolerance")
        self.assertEqual(self.code(BODY, header(BODY, NOW + 301), SECRET, now=NOW), "timestamp_out_of_tolerance")
        self.assertTrue(verify_webhook(BODY, header(BODY, NOW - 300), SECRET, now=NOW))
        self.assertEqual(self.code(BODY, header(BODY, NOW - 61), SECRET, 60, now=NOW), "timestamp_out_of_tolerance")

    def test_timestamp_is_signed(self) -> None:
        sig = header(BODY, NOW - 1000).split(",")[1]
        self.assertEqual(self.code(BODY, f"t={NOW},{sig}", SECRET, now=NOW), "signature_mismatch")

    def test_missing_and_malformed_headers(self) -> None:
        self.assertEqual(self.code(BODY, None, SECRET), "missing_signature")
        self.assertEqual(self.code(BODY, "", SECRET), "missing_signature")
        self.assertEqual(self.code(BODY, "garbage", SECRET), "malformed_signature")
        self.assertEqual(self.code(BODY, f"t={NOW}", SECRET), "malformed_signature")
        self.assertEqual(self.code(BODY, "v1=" + "a" * 64, SECRET), "malformed_signature")
        self.assertEqual(self.code(BODY, f"t=abc,v1={'a' * 64}", SECRET), "malformed_signature")

    def test_signed_non_json_body(self) -> None:
        self.assertEqual(self.code("not json", header("not json"), SECRET, now=NOW), "invalid_payload")

    def test_secret_required(self) -> None:
        with self.assertRaises(ValueError):
            verify_webhook(BODY, header(BODY), "")


if __name__ == "__main__":
    unittest.main()
