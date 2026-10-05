/**
 * Builds a PATCH body: the keys of `next` whose value differs from
 * `original` (deep-compared via JSON for objects and arrays).
 */
export function changedFields<T extends object>(original: object, next: T): Partial<T> {
  const before = original as Record<string, unknown>;
  const out: Partial<T> = {};
  (Object.keys(next) as (keyof T)[]).forEach((key) => {
    const value = next[key];
    if (value === undefined) return;
    if (JSON.stringify(value) !== JSON.stringify(before[key as string] ?? null)) {
      out[key] = value;
    }
  });
  return out;
}

/** "" -> undefined, "12" -> 12 (for optional numeric inputs). */
export function optionalNumber(raw: string): number | undefined {
  if (raw.trim() === '') return undefined;
  const n = Number(raw);
  return Number.isFinite(n) ? n : undefined;
}

/** <input type="date"> value -> RFC 3339 at UTC midnight, or undefined. */
export function dateToRfc3339(date: string): string | undefined {
  return date ? new Date(`${date}T00:00:00Z`).toISOString() : undefined;
}

/** RFC 3339 -> <input type="date"> value. */
export function rfc3339ToDate(value: string | null | undefined): string {
  return value ? value.slice(0, 10) : '';
}
