import { seg, type HttpClient, type RequestOptions } from "../core.js";
import type { Page } from "../pagination.js";
import type {
  AwardBadgeParams,
  AwardBadgeResponse,
  Badge,
  ClaimRewardParams,
  GrantXpParams,
  GrantXpResponse,
  Leaderboard,
  LeaderboardEntriesPage,
  LeaderboardEntriesParams,
  LeaderboardEntry,
  ListBadgesParams,
  ListLeaderboardsParams,
  ListMissionsParams,
  ListParams,
  ListPlayerMissionsParams,
  ListRewardsParams,
  ListStreaksParams,
  Mission,
  MissionAttempt,
  MissionPlayerParams,
  MissionProgressParams,
  MissionProgressResponse,
  PlayerBadge,
  PlayerProgress,
  PlayerStanding,
  PlayerStandingParams,
  PlayerStreak,
  RecordStreakParams,
  RecordStreakResponse,
  Reward,
  RewardClaim,
  SimulateRulesParams,
  SimulateRulesResponse,
  Streak,
} from "../types.js";

export class Progression {
  constructor(private readonly http: HttpClient) {}

  /** Grants XP. Pass `idempotencyKey` to make the grant safe to retry (a replay answers `replayed: true`). */
  async grantXp(playerId: string, params: GrantXpParams, options?: RequestOptions): Promise<GrantXpResponse> {
    return this.http.request({ method: "POST", path: `/players/${seg(playerId)}/xp`, body: params, options });
  }

  async getProgress(playerId: string, options?: RequestOptions): Promise<PlayerProgress> {
    return this.http.request({ method: "GET", path: `/players/${seg(playerId)}/progress`, options });
  }
}

export class Badges {
  constructor(private readonly http: HttpClient) {}

  async list(params?: ListBadgesParams, options?: RequestOptions): Promise<Page<Badge>> {
    return this.http.page("/badges", params, options);
  }

  async *iterate(params?: ListBadgesParams, options?: RequestOptions): AsyncGenerator<Badge, void, undefined> {
    yield* await this.list(params, options);
  }

  /** Awards a badge. 409 `badge_already_earned` for a non-stackable badge the player holds. */
  async award(badgeId: string, params: AwardBadgeParams, options?: RequestOptions): Promise<AwardBadgeResponse> {
    return this.http.request({ method: "POST", path: `/badges/${seg(badgeId)}/award`, body: params, options });
  }

  async revoke(badgeId: string, playerId: string, options?: RequestOptions): Promise<void> {
    await this.http.request<void>({ method: "DELETE", path: `/badges/${seg(badgeId)}/players/${seg(playerId)}`, options });
  }

  /** Badges a player holds. */
  async listForPlayer(playerId: string, params?: ListParams, options?: RequestOptions): Promise<Page<PlayerBadge>> {
    return this.http.page(`/players/${seg(playerId)}/badges`, params, options);
  }
}

export class Missions {
  constructor(private readonly http: HttpClient) {}

  async list(params?: ListMissionsParams, options?: RequestOptions): Promise<Page<Mission>> {
    return this.http.page("/missions", params, options);
  }

  async *iterate(params?: ListMissionsParams, options?: RequestOptions): AsyncGenerator<Mission, void, undefined> {
    yield* await this.list(params, options);
  }

  /** Adds progress to the player's current attempt (started implicitly when needed). */
  async progress(missionId: string, params: MissionProgressParams, options?: RequestOptions): Promise<MissionProgressResponse> {
    return this.http.request({ method: "POST", path: `/missions/${seg(missionId)}/progress`, body: params, options });
  }

  async start(missionId: string, params: MissionPlayerParams, options?: RequestOptions): Promise<MissionAttempt> {
    return this.http.request({ method: "POST", path: `/missions/${seg(missionId)}/start`, body: params, options });
  }

  async complete(missionId: string, params: MissionPlayerParams, options?: RequestOptions): Promise<MissionAttempt> {
    return this.http.request({ method: "POST", path: `/missions/${seg(missionId)}/complete`, body: params, options });
  }

  /** A player's mission attempts. */
  async listForPlayer(playerId: string, params?: ListPlayerMissionsParams, options?: RequestOptions): Promise<Page<MissionAttempt>> {
    return this.http.page(`/players/${seg(playerId)}/missions`, params, options);
  }
}

export class Streaks {
  constructor(private readonly http: HttpClient) {}

  async list(params?: ListStreaksParams, options?: RequestOptions): Promise<Page<Streak>> {
    return this.http.page("/streaks", params, options);
  }

  async *iterate(params?: ListStreaksParams, options?: RequestOptions): AsyncGenerator<Streak, void, undefined> {
    yield* await this.list(params, options);
  }

  /** Records qualifying activity for the player in the streak's current period. */
  async record(streakId: string, params: RecordStreakParams, options?: RequestOptions): Promise<RecordStreakResponse> {
    return this.http.request({ method: "POST", path: `/streaks/${seg(streakId)}/record`, body: params, options });
  }

  /** A player's streaks. */
  async listForPlayer(playerId: string, options?: RequestOptions): Promise<Page<PlayerStreak>> {
    return this.http.page(`/players/${seg(playerId)}/streaks`, undefined, options);
  }
}

export class Rewards {
  constructor(private readonly http: HttpClient) {}

  async list(params?: ListRewardsParams, options?: RequestOptions): Promise<Page<Reward>> {
    return this.http.page("/rewards", params, options);
  }

  async *iterate(params?: ListRewardsParams, options?: RequestOptions): AsyncGenerator<Reward, void, undefined> {
    yield* await this.list(params, options);
  }

  /** Claims a reward for a player (spends `points_cost`). Pass `idempotencyKey` to make it safe to retry. */
  async claim(rewardId: string, params: ClaimRewardParams, options?: RequestOptions): Promise<RewardClaim> {
    return this.http.request({ method: "POST", path: `/rewards/${seg(rewardId)}/claim`, body: params, options });
  }

  async getClaim(claimId: string, options?: RequestOptions): Promise<RewardClaim> {
    return this.http.request({ method: "GET", path: `/rewards/claims/${seg(claimId)}`, options });
  }

  /** Marks a claim as redeemed (e.g. the voucher was used at checkout). */
  async redeem(claimId: string, options?: RequestOptions): Promise<RewardClaim> {
    return this.http.request({ method: "POST", path: `/rewards/claims/${seg(claimId)}/redeem`, options });
  }

  /** Cancels a pending claim. */
  async cancelClaim(claimId: string, options?: RequestOptions): Promise<RewardClaim> {
    return this.http.request({ method: "POST", path: `/rewards/claims/${seg(claimId)}/cancel`, options });
  }

  /** A player's reward claims. */
  async listClaims(playerId: string, params?: ListParams, options?: RequestOptions): Promise<Page<RewardClaim>> {
    return this.http.page(`/players/${seg(playerId)}/reward-claims`, params, options);
  }
}

export class Leaderboards {
  constructor(private readonly http: HttpClient) {}

  async list(params?: ListLeaderboardsParams, options?: RequestOptions): Promise<Page<Leaderboard>> {
    return this.http.page("/leaderboards", params, options);
  }

  async *iterate(params?: ListLeaderboardsParams, options?: RequestOptions): AsyncGenerator<Leaderboard, void, undefined> {
    yield* await this.list(params, options);
  }

  /** Ranked entries of a period. `page.body.period_start` / `period_end` describe the period. */
  async entries(
    leaderboardId: string,
    params?: LeaderboardEntriesParams,
    options?: RequestOptions,
  ): Promise<Page<LeaderboardEntry, LeaderboardEntriesPage>> {
    return this.http.page<LeaderboardEntry, LeaderboardEntriesPage>(`/leaderboards/${seg(leaderboardId)}/entries`, params, options);
  }

  /** A player's rank, optionally with `around` neighbours on each side. */
  async playerStanding(
    leaderboardId: string,
    playerId: string,
    params?: PlayerStandingParams,
    options?: RequestOptions,
  ): Promise<PlayerStanding> {
    return this.http.request({
      method: "GET",
      path: `/leaderboards/${seg(leaderboardId)}/players/${seg(playerId)}`,
      query: params,
      options,
    });
  }
}

export class Rules {
  constructor(private readonly http: HttpClient) {}

  /** Dry-runs the published rules against an event. Nothing is recorded or awarded. */
  async simulate(params: SimulateRulesParams, options?: RequestOptions): Promise<SimulateRulesResponse> {
    return this.http.request({ method: "POST", path: "/rules/simulate", body: params, options });
  }
}
