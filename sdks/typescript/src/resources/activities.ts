import { seg, type HttpClient, type RequestOptions } from "../core.js";
import type { Page } from "../pagination.js";
import type {
  Activity,
  ListActivitiesParams,
  SendActivityBatchResponse,
  SendActivityParams,
  SendActivityResponse,
} from "../types.js";

export class Activities {
  constructor(private readonly http: HttpClient) {}

  /**
   * Reports one activity. Answers 202 when accepted for processing, or 200 with
   * `duplicate: true` when `event_id` was already received.
   *
   * POST /activities isn't retried unless you pass `idempotencyKey` (using the
   * `event_id` as key is a good choice).
   */
  async send(params: SendActivityParams, options?: RequestOptions): Promise<SendActivityResponse> {
    return this.http.request({ method: "POST", path: "/activities", body: params, options });
  }

  /** Reports up to 100 activities in one call. Each item succeeds or fails on its own; see `results`. */
  async sendBatch(items: SendActivityParams[], options?: RequestOptions): Promise<SendActivityBatchResponse> {
    return this.http.request({ method: "POST", path: "/activities/batch", body: { items }, options });
  }

  async get(id: string, options?: RequestOptions): Promise<Activity> {
    return this.http.request({ method: "GET", path: `/activities/${seg(id)}`, options });
  }

  async list(params?: ListActivitiesParams, options?: RequestOptions): Promise<Page<Activity>> {
    return this.http.page("/activities", params, options);
  }

  /** Iterates every matching activity across all pages. */
  async *iterate(params?: ListActivitiesParams, options?: RequestOptions): AsyncGenerator<Activity, void, undefined> {
    yield* await this.list(params, options);
  }
}
