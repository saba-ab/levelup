import { createHmac, timingSafeEqual } from "node:crypto";

/** `t=<unix seconds>,v1=<hex HMAC-SHA256(secret, "<t>.<raw body>")>` */
export const SIGNATURE_HEADER = "LevelUp-Signature";
/** The event type of the delivery, e.g. "badges.awarded". */
export const EVENT_HEADER = "LevelUp-Event";
/** Unique id of the delivery. Stable across redeliveries: use it to deduplicate. */
export const DELIVERY_HEADER = "LevelUp-Delivery";

export const DEFAULT_TOLERANCE_SECONDS = 300;

/** The JSON body of every webhook delivery. `data` is the event's own payload. */
export interface WebhookEvent<T = Record<string, unknown>> {
  /** Event type, same as the LevelUp-Event header, e.g. "badges.awarded". */
  event: string;
  /** Id of the underlying fact. Several endpoints may receive the same event_id. */
  event_id: string;
  occurred_at: string;
  tenant_id: string;
  data: T;
}

export type WebhookVerificationErrorCode =
  | "missing_signature"
  | "malformed_signature"
  | "timestamp_out_of_tolerance"
  | "signature_mismatch"
  | "invalid_payload";

export class WebhookVerificationError extends Error {
  readonly code: WebhookVerificationErrorCode;

  constructor(code: WebhookVerificationErrorCode, message: string) {
    super(message);
    this.name = "WebhookVerificationError";
    this.code = code;
  }
}

export interface VerifyWebhookOptions {
  /** Current time in unix seconds (for tests). Defaults to the system clock. */
  now?: number;
}

const encoder = new TextEncoder();
const decoder = new TextDecoder("utf-8", { fatal: true });

/**
 * Verifies a webhook delivery and returns its parsed JSON body.
 *
 * Pass the **raw** request body exactly as received (a Buffer/Uint8Array or the unmodified
 * string), never a re-serialised object: any byte difference breaks the signature.
 *
 * Throws WebhookVerificationError when the header is missing or malformed, the timestamp is
 * further than `toleranceSeconds` from now (replay protection), or no `v1` signature matches.
 * Several `v1` entries are accepted (secret rotation); comparison is constant-time.
 */
export function verifyWebhook<T = WebhookEvent>(
  payload: string | Uint8Array,
  signatureHeader: string | null | undefined,
  secret: string,
  toleranceSeconds: number = DEFAULT_TOLERANCE_SECONDS,
  options: VerifyWebhookOptions = {},
): T {
  if (!secret) {
    throw new TypeError("verifyWebhook: `secret` is required");
  }
  if (!signatureHeader) {
    throw new WebhookVerificationError("missing_signature", `missing ${SIGNATURE_HEADER} header`);
  }
  const { timestamp, signatures } = parseSignatureHeader(signatureHeader);

  const now = options.now ?? Math.floor(Date.now() / 1000);
  if (Math.abs(now - timestamp) > toleranceSeconds) {
    throw new WebhookVerificationError("timestamp_out_of_tolerance", "webhook timestamp is outside the tolerance window");
  }

  const body = typeof payload === "string" ? encoder.encode(payload) : payload;
  const expected = encoder.encode(sign(body, secret, timestamp));
  let matched = false;
  for (const candidate of signatures) {
    const bytes = encoder.encode(candidate);
    if (bytes.length === expected.length && timingSafeEqual(bytes, expected)) {
      matched = true;
    }
  }
  if (!matched) {
    throw new WebhookVerificationError("signature_mismatch", "no signature matches the payload");
  }

  try {
    return JSON.parse(typeof payload === "string" ? payload : decoder.decode(payload)) as T;
  } catch {
    throw new WebhookVerificationError("invalid_payload", "webhook body is not valid JSON");
  }
}

/**
 * Builds a `LevelUp-Signature` header value. Useful to test your webhook endpoint
 * with deliveries signed exactly like LevelUp signs them.
 */
export function signWebhook(payload: string | Uint8Array, secret: string, timestamp: number = Math.floor(Date.now() / 1000)): string {
  const body = typeof payload === "string" ? encoder.encode(payload) : payload;
  return `t=${timestamp},v1=${sign(body, secret, timestamp)}`;
}

function sign(body: Uint8Array, secret: string, timestamp: number): string {
  return createHmac("sha256", secret).update(`${timestamp}.`).update(body).digest("hex");
}

function parseSignatureHeader(header: string): { timestamp: number; signatures: string[] } {
  let timestamp: number | null = null;
  const signatures: string[] = [];
  for (const part of header.split(",")) {
    const idx = part.indexOf("=");
    if (idx <= 0) {
      continue;
    }
    const key = part.slice(0, idx).trim();
    const value = part.slice(idx + 1).trim();
    if (key === "t" && /^\d+$/.test(value)) {
      timestamp = Number(value);
    } else if (key === "v1" && /^[0-9a-fA-F]{64}$/.test(value)) {
      signatures.push(value.toLowerCase());
    }
  }
  if (timestamp === null || signatures.length === 0) {
    throw new WebhookVerificationError("malformed_signature", `malformed ${SIGNATURE_HEADER} header`);
  }
  return { timestamp, signatures };
}
