/**
 * Minimal ambient declarations for the handful of Node built-ins this package
 * touches. They exist so the SDK type-checks and builds with nothing but
 * `typescript` installed (no @types/node). They are compile-time only: nothing
 * here is referenced from the emitted .d.ts files, so consumers never see them.
 */

declare module "node:crypto" {
  interface Hmac {
    update(data: string | Uint8Array): Hmac;
    digest(encoding: "hex"): string;
  }
  export function createHmac(algorithm: string, key: string | Uint8Array): Hmac;
  export function timingSafeEqual(a: Uint8Array, b: Uint8Array): boolean;
  export function randomUUID(): string;
}

declare module "node:http" {
  export interface IncomingMessage extends AsyncIterable<Uint8Array> {
    method?: string;
    url?: string;
    headers: Record<string, string | string[] | undefined>;
  }
  export interface ServerResponse {
    statusCode: number;
    setHeader(name: string, value: string | number): void;
    end(body?: string | Uint8Array): void;
    destroy(): void;
    socket: { destroy(): void } | null;
  }
  export interface Server {
    listen(port: number, host: string, cb: () => void): Server;
    close(cb?: (err?: Error) => void): Server;
    closeAllConnections(): void;
    address(): { port: number; address: string } | string | null;
  }
  export function createServer(handler: (req: IncomingMessage, res: ServerResponse) => void): Server;
}

declare module "node:test" {
  type Fn = (t?: unknown) => void | Promise<void>;
  export function test(name: string, fn: Fn): Promise<void>;
  export function describe(name: string, fn: () => void | Promise<void>): Promise<void>;
  export function it(name: string, fn: Fn): Promise<void>;
  export function before(fn: Fn): void;
  export function after(fn: Fn): void;
  export function beforeEach(fn: Fn): void;
}

declare module "node:assert/strict" {
  interface Assert {
    (value: unknown, message?: string): asserts value;
    ok(value: unknown, message?: string): asserts value;
    equal(actual: unknown, expected: unknown, message?: string): void;
    notEqual(actual: unknown, expected: unknown, message?: string): void;
    deepEqual(actual: unknown, expected: unknown, message?: string): void;
    match(value: string, re: RegExp, message?: string): void;
    throws(fn: () => unknown, expected?: unknown, message?: string): void;
    rejects(p: Promise<unknown> | (() => Promise<unknown>), expected?: unknown, message?: string): Promise<void>;
    fail(message?: string): never;
  }
  const assert: Assert;
  export default assert;
}
