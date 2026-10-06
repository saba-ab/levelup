import { seg, withIdempotencyKey, type HttpClient, type RequestOptions } from "../core.js";
import type { Page } from "../pagination.js";
import type {
  CreditParams,
  DebitParams,
  LedgerEntry,
  ListTransactionsParams,
  TransferParams,
  TransferResponse,
  Wallet,
} from "../types.js";

/**
 * Points wallets. `credit`, `debit` and `transfer` ALWAYS send an Idempotency-Key:
 * yours when you pass `idempotencyKey`, otherwise a fresh UUIDv4 per call. Pass your
 * own (e.g. your order id) so that re-running your job cannot move points twice.
 */
export class Wallets {
  constructor(private readonly http: HttpClient) {}

  async get(playerId: string, options?: RequestOptions): Promise<Wallet> {
    return this.http.request({ method: "GET", path: `/players/${seg(playerId)}/wallet`, options });
  }

  async credit(playerId: string, params: CreditParams, options?: RequestOptions): Promise<LedgerEntry> {
    return this.http.request({
      method: "POST",
      path: `/players/${seg(playerId)}/wallet/credit`,
      body: params,
      options: withIdempotencyKey(options),
    });
  }

  /** Fails with 422 `code: "insufficient_balance"` when the balance is too low. */
  async debit(playerId: string, params: DebitParams, options?: RequestOptions): Promise<LedgerEntry> {
    return this.http.request({
      method: "POST",
      path: `/players/${seg(playerId)}/wallet/debit`,
      body: params,
      options: withIdempotencyKey(options),
    });
  }

  async transfer(params: TransferParams, options?: RequestOptions): Promise<TransferResponse> {
    return this.http.request({
      method: "POST",
      path: "/wallets/transfer",
      body: params,
      options: withIdempotencyKey(options),
    });
  }

  /** The player's ledger, newest first. */
  async transactions(playerId: string, params?: ListTransactionsParams, options?: RequestOptions): Promise<Page<LedgerEntry>> {
    return this.http.page(`/players/${seg(playerId)}/wallet/transactions`, params, options);
  }

  async *iterateTransactions(
    playerId: string,
    params?: ListTransactionsParams,
    options?: RequestOptions,
  ): AsyncGenerator<LedgerEntry, void, undefined> {
    yield* await this.transactions(playerId, params, options);
  }
}
