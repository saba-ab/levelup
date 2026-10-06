from __future__ import annotations

import unittest
from urllib.parse import urlencode

from fakeserver import FakeServer, Recorded, Reply

import levelup

EMPTY = {"data": [], "next_cursor": ""}


class ResourceTestCase(unittest.TestCase):
    server: FakeServer

    @classmethod
    def setUpClass(cls) -> None:
        cls.server = FakeServer().start()
        cls.client = levelup.Client("lvl_live_TESTPREFIX_secret", base_url=cls.server.url, retry_backoff=0.001)

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.stop()

    def setUp(self) -> None:
        self.server.reset()
        self.server.handler = lambda req: Reply(200, EMPTY)

    def route(self) -> tuple:
        r = self.server.last
        qs = urlencode([(k, v) for k, vs in r.query.items() for v in vs])
        return (r.method, r.path.replace("/api/v1", "", 1) + (f"?{qs}" if qs else ""))


class PaginationTest(ResourceTestCase):
    @staticmethod
    def players(*ids: str) -> list:
        return [{"id": i} for i in ids]

    def test_iterates_all_pages_and_keeps_filters(self) -> None:
        def handler(req: Recorded) -> Reply:
            cursor = req.q("cursor")
            if not cursor:
                return Reply(200, {"data": self.players("a", "b"), "next_cursor": "c1"})
            if cursor == "c1":
                return Reply(200, {"data": self.players("c"), "next_cursor": "c2"})
            return Reply(200, {"data": self.players("d"), "next_cursor": ""})

        self.server.handler = handler
        ids = [p["id"] for p in self.client.players.iterate(is_active=True, limit=2)]
        self.assertEqual(ids, ["a", "b", "c", "d"])
        self.assertEqual([r.q("cursor") for r in self.server.requests], [None, "c1", "c2"])
        for r in self.server.requests:
            self.assertEqual(r.q("is_active"), "true")
            self.assertEqual(r.q("limit"), "2")

    def test_page_navigation(self) -> None:
        self.server.handler = lambda req: (
            Reply(200, {"data": self.players("z"), "next_cursor": ""})
            if req.q("cursor")
            else Reply(200, {"data": self.players("y"), "next_cursor": "n"})
        )
        first = self.client.players.list()
        self.assertTrue(first.has_next_page())
        self.assertEqual(first.next_cursor, "n")
        second = first.next_page()
        assert second is not None
        self.assertEqual([p["id"] for p in second.data], ["z"])
        self.assertFalse(second.has_next_page())
        self.assertIsNone(second.next_page())
        self.assertEqual(len(list(first.iter_pages())), 2)
        self.assertEqual([p["id"] for p in first], ["y", "z"])

    def test_stops_on_echoed_cursor(self) -> None:
        self.server.handler = lambda req: Reply(200, {"data": self.players("x"), "next_cursor": "same"})
        self.assertEqual([p["id"] for p in self.client.players.iterate(cursor="same")], ["x"])

    def test_leaderboard_entries_keep_envelope(self) -> None:
        self.server.handler = lambda req: Reply(
            200,
            {"data": [{"rank": 1, "score": 9}], "next_cursor": "", "period_start": "2026-01-01T00:00:00Z"},
        )
        page = self.client.leaderboards.entries("lb1", period="current", limit=5)
        self.assertEqual(page.body["period_start"], "2026-01-01T00:00:00Z")
        self.assertEqual(page.data[0]["rank"], 1)
        self.assertEqual(self.route(), ("GET", "/leaderboards/lb1/entries?period=current&limit=5"))


class RoutesTest(ResourceTestCase):
    def check(self, call, route, body=None) -> None:
        call()
        self.assertEqual(self.route(), route)
        if body is not None:
            self.assertEqual(self.server.last.body, body)

    def test_activities(self) -> None:
        c = self.client
        item = {"event_id": "e", "event_type": "t", "player_external_id": "u"}
        self.check(lambda: c.activities.send_batch([item]), ("POST", "/activities/batch"), {"items": [item]})
        self.check(lambda: c.activities.get("a1"), ("GET", "/activities/a1"))
        self.check(
            lambda: c.activities.list(event_type="purchase", status="decided"),
            ("GET", "/activities?event_type=purchase&status=decided"),
        )

    def test_players(self) -> None:
        c = self.client
        self.check(lambda: c.players.create("u1", display_name="U"), ("POST", "/players"), {"external_id": "u1", "display_name": "U"})
        self.check(lambda: c.players.get("p1"), ("GET", "/players/p1"))
        self.check(lambda: c.players.update("p1", email="a@b.c"), ("PATCH", "/players/p1"), {"email": "a@b.c"})
        self.check(lambda: c.players.activate("p1"), ("POST", "/players/p1/activate"))
        self.check(lambda: c.players.deactivate("p1"), ("POST", "/players/p1/deactivate"))

    def test_wallets_and_progression(self) -> None:
        c = self.client
        self.check(lambda: c.wallets.get("p1"), ("GET", "/players/p1/wallet"))
        self.check(lambda: c.wallets.transactions("p1", direction="debit"), ("GET", "/players/p1/wallet/transactions?direction=debit"))
        self.check(lambda: c.progression.grant_xp("p1", 50), ("POST", "/players/p1/xp"), {"amount": 50})
        self.assertNotIn("idempotency-key", self.server.last.headers)
        self.check(lambda: c.progression.get_progress("p1"), ("GET", "/players/p1/progress"))

    def test_badges(self) -> None:
        c = self.client
        self.check(lambda: c.badges.list(tier="gold", active=True), ("GET", "/badges?tier=gold&active=true"))
        self.check(lambda: c.badges.award("b1", "p1", idempotency_key="aw-1"), ("POST", "/badges/b1/award"), {"player_id": "p1"})
        self.assertEqual(self.server.last.headers["idempotency-key"], "aw-1")
        self.check(lambda: c.badges.revoke("b1", "p1"), ("DELETE", "/badges/b1/players/p1"))
        self.check(lambda: c.badges.list_for_player("p1"), ("GET", "/players/p1/badges"))

    def test_missions(self) -> None:
        c = self.client
        self.check(lambda: c.missions.list(status="active"), ("GET", "/missions?status=active"))
        self.check(lambda: c.missions.progress("m1", "p1", 2), ("POST", "/missions/m1/progress"), {"player_id": "p1", "increment": 2})
        self.check(lambda: c.missions.start("m1", "p1"), ("POST", "/missions/m1/start"), {"player_id": "p1"})
        self.check(lambda: c.missions.complete("m1", "p1"), ("POST", "/missions/m1/complete"), {"player_id": "p1"})
        self.check(lambda: c.missions.list_for_player("p1", status="completed"), ("GET", "/players/p1/missions?status=completed"))

    def test_streaks(self) -> None:
        c = self.client
        self.check(lambda: c.streaks.list(period="daily"), ("GET", "/streaks?period=daily"))
        self.check(lambda: c.streaks.record("s1", "p1"), ("POST", "/streaks/s1/record"), {"player_id": "p1"})
        self.check(lambda: c.streaks.list_for_player("p1"), ("GET", "/players/p1/streaks"))

    def test_rewards(self) -> None:
        c = self.client
        self.check(lambda: c.rewards.list(is_active=True), ("GET", "/rewards?is_active=true"))
        self.check(lambda: c.rewards.claim("r1", "p1"), ("POST", "/rewards/r1/claim"), {"player_id": "p1"})
        self.check(lambda: c.rewards.get_claim("c1"), ("GET", "/rewards/claims/c1"))
        self.check(lambda: c.rewards.redeem("c1"), ("POST", "/rewards/claims/c1/redeem"))
        self.check(lambda: c.rewards.cancel_claim("c1"), ("POST", "/rewards/claims/c1/cancel"))
        self.check(lambda: c.rewards.list_claims("p1"), ("GET", "/players/p1/reward-claims"))

    def test_leaderboards_and_rules(self) -> None:
        c = self.client
        self.check(lambda: c.leaderboards.list(type="points"), ("GET", "/leaderboards?type=points"))
        self.check(lambda: c.leaderboards.player_standing("lb1", "p1", around=2), ("GET", "/leaderboards/lb1/players/p1?around=2"))
        self.check(
            lambda: c.rules.simulate("purchase", player={"level": 3}),
            ("POST", "/rules/simulate"),
            {"event_type": "purchase", "player": {"level": 3}},
        )

    def test_escape_hatch(self) -> None:
        self.check(lambda: self.client.request("GET", "/levels", query={"active": True}), ("GET", "/levels?active=true"))


if __name__ == "__main__":
    unittest.main()
