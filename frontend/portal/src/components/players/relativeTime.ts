/** "just now", "5 min ago", "3 h ago", "4 d ago", or a date beyond 30 days. */
export function formatRelativeTime(iso: string, now = Date.now()): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return '-';
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 60) return 'just now';
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86_400) return `${Math.floor(s / 3600)} h ago`;
  if (s < 30 * 86_400) return `${Math.floor(s / 86_400)} d ago`;
  return new Date(t).toLocaleDateString();
}
