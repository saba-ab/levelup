from __future__ import annotations

import datetime as dt
import email.utils
import re
import time
import unittest

from fakeserver import FakeServer, Reply

import levelup
from levelup import LevelUpConnectionError, LevelUpError, LevelUpTimeoutError
from levelup._http import parse_retry_after

API_KEY = "lvl_live_TESTPREFIX_testsecret"
UUID4 = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")


class ClientTestCase(unittest.TestCase):
    server: FakeServer

    @classmethod
    def setUpClass(cls) -> None:
        cls.server = FakeServer().start()

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.stop()

    def setUp(self) -> None:
        self.server.reset()
        self.sleeps: list = []

    def client(self, **overrides) -> levelup.Client:
        kwargs = {"retry_backoff": 0.001, "timeout": 2.0, **overrides}
        c = levelup.Client(API_KEY, base_url=self.server.url, **kwargs)
        c._http._sleep = self.sleeps.append  # record delays instead of sleeping
        return c


class ConstructionTest(unittest.TestCase):
    def test_requires_api_key(self) -> None:
        with self.assertRaises(ValueError):
            levelup.Client("")

    def test_default_base_url_and_trailing_slash(self) -> None:
        self.assertEqual(levelup.Client(API_KEY).base_url, "https://api.levelupos.ge")
        self.assertEqual(levelup.Client(API_KEY, base_url="http://x.test///").base_url, "http://x.test")


class RequestTest(ClientTestCase):
    def test_auth_json_and_prefix(self) -> None:
        self.server.sequence(Reply(202, {"activity_id": "a1", "status": "pending", "duplicate": False}))
        res = self.client().activities.send(
            event_id="evt-1",
            event_type="purchase",
            player_external_id="user-1",
            properties={"amount": 42},
            occurred_at=dt.datetime(2026, 1, 2, 3, 4, 5, tzinfo=dt.timezone.utc),
        )
        self.assertEqual(res["activity_id"], "a1")
        req = self.server.last
        self.assertEqual(req.method, "POST")
        self.assertEqual(req.path, "/api/v1/activities")
        self.assertEqual(req.headers["authorization"], f"Bearer {API_KEY}")
        self.assertEqual(req.headers["content-type"], "application/json")
        self.assertTrue(req.headers["user-agent"].startswith("levelup-python/"))
        self.assertEqual(
            req.body,
            {
                "event_id": "evt-1",
                "event_type": "purchase",
                "player_external_id": "user-1",
                "properties": {"amount": 42},
                "occurred_at": "2026-01-02T03:04:05Z",
            },
        )

    def test_optional_fields_are_omitted(self) -> None:
        self.server.sequence(Reply(202, {"activity_id": "a"}))
        self.client().activities.send("e", "t", "u")
        self.assertEqual(self.server.last.body, {"event_id": "e", "event_type": "t", "player_external_id": "u"})

    def test_path_and_query_encoding(self) -> None:
        self.server.sequence(Reply(200, {"id": "p1"}))
        self.client().players.get_by_external_id("a/b c")
        self.assertEqual(self.server.last.path, "/api/v1/players/by-external-id/a%2Fb%20c")

        self.server.sequence(Reply(200, {"data": [], "next_cursor": ""}))
        self.client().players.list(is_active=False, limit=10)
        self.assertEqual(self.server.last.q("is_active"), "false")
        self.assertEqual(self.server.last.q("limit"), "10")
        self.assertIsNone(self.server.last.q("search"))

    def test_204_returns_none(self) -> None:
        self.server.sequence(Reply(204))
        self.assertIsNone(self.client().players.delete("p1"))
        self.assertEqual(self.server.last.method, "DELETE")

    def test_empty_path_param_rejected_before_sending(self) -> None:
        with self.assertRaises(ValueError):
            self.client().players.get("")
        self.assertEqual(self.server.requests, [])


class ErrorTest(ClientTestCase):
    def test_problem_json_mapping(self) -> None:
        self.server.sequence(
            Reply(
                422,
                {
                    "type": "about:blank",
                    "title": "Unprocessable Entity",
                    "status": 422,
                    "detail": "requested 50, available 10",
                    "code": "insufficient_balance",
                    "errors": {"amount": "too large"},
                    "trace_id": "trace-123",
                },
            )
        )
        with self.assertRaises(LevelUpError) as ctx:
            self.client().wallets.debit("p1", 50)
        err = ctx.exception
        self.assertEqual(err.status, 422)
        self.assertEqual(err.code, "insufficient_balance")
        self.assertEqual(err.detail, "requested 50, available 10")
        self.assertEqual(err.field_errors, {"amount": "too large"})
        self.assertEqual(err.trace_id, "trace-123")
        self.assertIn("insufficient_balance", str(err))

    def test_non_json_error_body(self) -> None:
        self.server.sequence(Reply(404, "<html>nope</html>"))
        with self.assertRaises(LevelUpError) as ctx:
            self.client().players.get("p1")
        self.assertEqual(ctx.exception.code, "not_found")
        self.assertEqual(ctx.exception.body, "<html>nope</html>")

    def test_snake_case_title_used_as_code(self) -> None:
        self.server.sequence(Reply(429, {"type": "about:blank", "title": "rate_limited", "status": 429}))
        with self.assertRaises(LevelUpError) as ctx:
            self.client(retries=0).players.get("p1")
        self.assertEqual(ctx.exception.code, "rate_limited")


class RetryTest(ClientTestCase):
    def test_get_retried_on_503(self) -> None:
        self.server.sequence(Reply(503, {"status": 503}), Reply(503), Reply(200, {"id": "p1"}))
        self.assertEqual(self.client().players.get("p1")["id"], "p1")
        self.assertEqual(len(self.server.requests), 3)
        self.assertEqual(len(self.sleeps), 2)

    def test_gives_up_after_retries(self) -> None:
        self.server.sequence(Reply(502, {"status": 502, "code": "bad_gateway"}))
        with self.assertRaises(LevelUpError) as ctx:
            self.client(retries=2).players.get("p1")
        self.assertEqual(ctx.exception.status, 502)
        self.assertEqual(len(self.server.requests), 3)

    def test_500_not_retried(self) -> None:
        self.server.sequence(Reply(500, {"status": 500}))
        with self.assertRaises(LevelUpError):
            self.client().players.get("p1")
        self.assertEqual(len(self.server.requests), 1)

    def test_post_without_key_not_retried(self) -> None:
        self.server.sequence(Reply(503))
        with self.assertRaises(LevelUpError):
            self.client().activities.send("e", "t", "u")
        self.assertEqual(len(self.server.requests), 1)

    def test_post_with_key_retried_with_same_key(self) -> None:
        self.server.sequence(Reply(429, headers={"Retry-After": "0"}), Reply(202, {"activity_id": "a"}))
        self.client().activities.send("e", "t", "u", idempotency_key="k-1")
        self.assertEqual([r.headers.get("idempotency-key") for r in self.server.requests], ["k-1", "k-1"])

    def test_retry_after_seconds_honoured(self) -> None:
        self.server.sequence(Reply(429, headers={"Retry-After": "7"}), Reply(200, {"id": "p1"}))
        self.client().players.get("p1")
        self.assertEqual(self.sleeps, [7.0])

    def test_retry_after_capped(self) -> None:
        self.server.sequence(Reply(503, headers={"Retry-After": "3600"}), Reply(200, {"id": "p1"}))
        self.client(max_retry_delay=5).players.get("p1")
        self.assertEqual(self.sleeps, [5.0])

    def test_retry_after_really_waits(self) -> None:
        self.server.sequence(Reply(429, headers={"Retry-After": "1"}), Reply(200, {"id": "p1"}))
        c = levelup.Client(API_KEY, base_url=self.server.url, retry_backoff=0)
        started = time.monotonic()
        c.players.get("p1")
        self.assertGreaterEqual(time.monotonic() - started, 0.9)

    def test_network_error_retried_then_raised(self) -> None:
        self.server.sequence(Reply(drop=True))
        with self.assertRaises(LevelUpConnectionError) as ctx:
            self.client(retries=1).players.get("p1")
        self.assertEqual(ctx.exception.status, 0)
        self.assertEqual(ctx.exception.code, "connection_error")
        self.assertEqual(len(self.server.requests), 2)

    def test_timeout(self) -> None:
        self.server.sequence(Reply(200, {}, delay=0.5))
        with self.assertRaises(LevelUpTimeoutError) as ctx:
            self.client(timeout=0.1, retries=0).players.get("p1")
        self.assertEqual(ctx.exception.code, "timeout")

    def test_parse_retry_after(self) -> None:
        self.assertEqual(parse_retry_after("3"), 3.0)
        self.assertIsNone(parse_retry_after(None))
        self.assertIsNone(parse_retry_after("garbage"))
        future = email.utils.formatdate(time.time() + 5, usegmt=True)
        self.assertTrue(3.0 < parse_retry_after(future) <= 5.0)


class IdempotencyTest(ClientTestCase):
    entry = {"id": "e1", "amount": 10, "balance_after": 10}

    def test_generated_uuid4_per_call(self) -> None:
        self.server.sequence(Reply(201, self.entry))
        c = self.client()
        c.wallets.credit("p1", 10)
        c.wallets.credit("p1", 10)
        k1, k2 = (r.headers["idempotency-key"] for r in self.server.requests)
        self.assertRegex(k1, UUID4)
        self.assertNotEqual(k1, k2)
        self.assertEqual(self.server.requests[0].body, {"amount": 10, "kind": "earn"})

    def test_caller_key_for_credit_debit_transfer(self) -> None:
        self.server.sequence(Reply(201, self.entry))
        c = self.client()
        c.wallets.credit("p1", 1, "bonus", idempotency_key="c-1")
        c.wallets.debit("p1", 1, idempotency_key="d-1")
        c.wallets.transfer("p1", "p2", 1, options={"idempotency_key": "t-1"})
        self.assertEqual(
            [(r.path, r.headers["idempotency-key"]) for r in self.server.requests],
            [
                ("/api/v1/players/p1/wallet/credit", "c-1"),
                ("/api/v1/players/p1/wallet/debit", "d-1"),
                ("/api/v1/wallets/transfer", "t-1"),
            ],
        )
        self.assertEqual(self.server.last.body, {"from_player_id": "p1", "to_player_id": "p2", "amount": 1})

    def test_credit_retried_with_same_generated_key(self) -> None:
        self.server.sequence(Reply(503), Reply(201, self.entry))
        self.client().wallets.credit("p1", 5)
        keys = [r.headers["idempotency-key"] for r in self.server.requests]
        self.assertEqual(len(keys), 2)
        self.assertEqual(keys[0], keys[1])


if __name__ == "__main__":
    unittest.main()
