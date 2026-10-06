/** Inclusive UTC day range, "YYYY-MM-DD". */
export interface DayRange {
  from: string;
  to: string;
}

export const MAX_RANGE_DAYS = 366;
export const DEFAULT_RANGE_DAYS = 30;

const DAY_MS = 86_400_000;

export function todayUTC(): string {
  return new Date().toISOString().slice(0, 10);
}

export function addDays(day: string, n: number): string {
  return new Date(Date.parse(`${day}T00:00:00Z`) + n * DAY_MS).toISOString().slice(0, 10);
}

/** Inclusive day count. */
export function rangeDays({ from, to }: DayRange): number {
  return Math.round((Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) / DAY_MS) + 1;
}

/** The last `days` days ending today (UTC). */
export function lastDays(days: number): DayRange {
  const to = todayUTC();
  return { from: addDays(to, -(days - 1)), to };
}

/** A problem with the range, or null when the API will accept it. */
export function rangeProblem(r: DayRange): string | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(r.from) || !/^\d{4}-\d{2}-\d{2}$/.test(r.to)) return 'Choose both dates.';
  if (r.from > r.to) return '"From" must not be after "to".';
  if (rangeDays(r) > MAX_RANGE_DAYS) return `The range may span at most ${MAX_RANGE_DAYS} days.`;
  return null;
}

/** "2026-03-14" -> "Mar 14" (UTC). */
export function shortDay(day: string): string {
  return new Date(`${day}T00:00:00Z`).toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' });
}
