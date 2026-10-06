import { HttpClient, type ClientOptions, type RequestSpec } from "./core.js";
import { Activities } from "./resources/activities.js";
import { Badges, Leaderboards, Missions, Progression, Rewards, Rules, Streaks } from "./resources/engagement.js";
import { Players } from "./resources/players.js";
import { Wallets } from "./resources/wallets.js";

/**
 * LevelUp API client for server-side integrations.
 *
 * ```ts
 * const levelup = new LevelUp({ apiKey: process.env.LEVELUP_API_KEY! });
 * await levelup.activities.send({ event_id: "order-1042", event_type: "purchase", player_external_id: "user-7" });
 * ```
 *
 * Never ship an API key to a browser or mobile app.
 */
export class LevelUp {
  readonly activities: Activities;
  readonly players: Players;
  readonly wallets: Wallets;
  readonly progression: Progression;
  readonly badges: Badges;
  readonly missions: Missions;
  readonly streaks: Streaks;
  readonly rewards: Rewards;
  readonly leaderboards: Leaderboards;
  readonly rules: Rules;

  readonly #http: HttpClient;

  constructor(options: ClientOptions) {
    this.#http = new HttpClient(options);
    this.activities = new Activities(this.#http);
    this.players = new Players(this.#http);
    this.wallets = new Wallets(this.#http);
    this.progression = new Progression(this.#http);
    this.badges = new Badges(this.#http);
    this.missions = new Missions(this.#http);
    this.streaks = new Streaks(this.#http);
    this.rewards = new Rewards(this.#http);
    this.leaderboards = new Leaderboards(this.#http);
    this.rules = new Rules(this.#http);
  }

  get baseUrl(): string {
    return this.#http.baseUrl;
  }

  /**
   * Escape hatch for endpoints without a dedicated method. `path` is relative to `/api/v1`.
   * Same auth, error handling and retry policy as the resource methods.
   */
  request<T = unknown>(spec: RequestSpec): Promise<T> {
    return this.#http.request<T>(spec);
  }
}
