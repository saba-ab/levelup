import { seg, type HttpClient, type RequestOptions } from "../core.js";
import type { Page } from "../pagination.js";
import type { CreatePlayerParams, ListPlayersParams, Player, UpdatePlayerParams } from "../types.js";

export class Players {
  constructor(private readonly http: HttpClient) {}

  /** Creates a player. Fails with 409 `code: "player_external_id_taken"` when `external_id` is already used. */
  async create(params: CreatePlayerParams, options?: RequestOptions): Promise<Player> {
    return this.http.request({ method: "POST", path: "/players", body: params, options });
  }

  async get(id: string, options?: RequestOptions): Promise<Player> {
    return this.http.request({ method: "GET", path: `/players/${seg(id)}`, options });
  }

  /** Looks a player up by your own id. */
  async getByExternalId(externalId: string, options?: RequestOptions): Promise<Player> {
    return this.http.request({ method: "GET", path: `/players/by-external-id/${seg(externalId)}`, options });
  }

  /** Partial update: only the fields you pass are changed. */
  async update(id: string, params: UpdatePlayerParams, options?: RequestOptions): Promise<Player> {
    return this.http.request({ method: "PATCH", path: `/players/${seg(id)}`, body: params, options });
  }

  async list(params?: ListPlayersParams, options?: RequestOptions): Promise<Page<Player>> {
    return this.http.page("/players", params, options);
  }

  /** Iterates every matching player across all pages: `for await (const p of client.players.iterate()) {}`. */
  async *iterate(params?: ListPlayersParams, options?: RequestOptions): AsyncGenerator<Player, void, undefined> {
    yield* await this.list(params, options);
  }

  async activate(id: string, options?: RequestOptions): Promise<Player> {
    return this.http.request({ method: "POST", path: `/players/${seg(id)}/activate`, options });
  }

  async deactivate(id: string, options?: RequestOptions): Promise<Player> {
    return this.http.request({ method: "POST", path: `/players/${seg(id)}/deactivate`, options });
  }

  async delete(id: string, options?: RequestOptions): Promise<void> {
    await this.http.request<void>({ method: "DELETE", path: `/players/${seg(id)}`, options });
  }
}
