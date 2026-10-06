import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";

export interface RecordedRequest {
  method: string;
  path: string;
  query: URLSearchParams;
  headers: Record<string, string | string[] | undefined>;
  body: unknown;
  rawBody: string;
}

export interface FakeResponse {
  status?: number;
  body?: unknown;
  headers?: Record<string, string>;
  /** Kill the socket without answering (simulates a network error). */
  drop?: boolean;
  /** Delay before answering, in ms. */
  delay?: number;
}

export type Handler = (req: RecordedRequest) => FakeResponse | Promise<FakeResponse>;

/** A local HTTP server standing in for the LevelUp API. Never talks to the real API. */
export class FakeServer {
  readonly requests: RecordedRequest[] = [];
  #handler: Handler = () => ({ status: 404, body: { status: 404, code: "no_handler" } });
  #server: Server;
  #url = "";

  constructor() {
    this.#server = createServer((req, res) => void this.#handle(req, res));
  }

  get url(): string {
    return this.#url;
  }

  on(handler: Handler): void {
    this.#handler = handler;
  }

  /** Answers with the given responses in order (the last one repeats). */
  sequence(...responses: FakeResponse[]): void {
    let i = 0;
    this.#handler = () => responses[Math.min(i++, responses.length - 1)]!;
  }

  reset(): void {
    this.requests.length = 0;
  }

  start(): Promise<void> {
    return new Promise((resolve) => {
      this.#server.listen(0, "127.0.0.1", () => {
        const addr = this.#server.address();
        if (addr && typeof addr === "object") {
          this.#url = `http://127.0.0.1:${addr.port}`;
        }
        resolve();
      });
    });
  }

  stop(): Promise<void> {
    return new Promise((resolve) => {
      this.#server.closeAllConnections();
      this.#server.close(() => resolve());
    });
  }

  async #handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const chunks: Uint8Array[] = [];
    for await (const chunk of req) {
      chunks.push(chunk);
    }
    const rawBody = new TextDecoder().decode(concat(chunks));
    const url = new URL(req.url ?? "/", "http://localhost");
    const recorded: RecordedRequest = {
      method: req.method ?? "GET",
      path: url.pathname,
      query: url.searchParams,
      headers: req.headers,
      rawBody,
      body: rawBody ? JSON.parse(rawBody) : undefined,
    };
    this.requests.push(recorded);
    const out = await this.#handler(recorded);
    if (out.delay) {
      await new Promise((r) => setTimeout(r, out.delay));
    }
    if (out.drop) {
      res.socket?.destroy();
      return;
    }
    const status = out.status ?? 200;
    res.statusCode = status;
    for (const [k, v] of Object.entries(out.headers ?? {})) {
      res.setHeader(k, v);
    }
    if (out.body === undefined || status === 204) {
      res.end();
      return;
    }
    const isProblem = status >= 400;
    res.setHeader("Content-Type", isProblem ? "application/problem+json" : "application/json");
    res.end(typeof out.body === "string" ? out.body : JSON.stringify(out.body));
  }
}

function concat(chunks: Uint8Array[]): Uint8Array {
  const total = chunks.reduce((n, c) => n + c.length, 0);
  const out = new Uint8Array(total);
  let off = 0;
  for (const c of chunks) {
    out.set(c, off);
    off += c.length;
  }
  return out;
}
