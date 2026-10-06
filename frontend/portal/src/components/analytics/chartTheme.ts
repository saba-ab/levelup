/** Series colors, matching the Overview page palette. */
export const SERIES_COLORS = ['hsl(var(--primary))', '#10B981', '#F59E0B', '#EC4899', '#6366F1', '#64748B'];

export const axisProps = {
  stroke: 'hsl(var(--muted-foreground))',
  fontSize: 12,
} as const;

export const gridProps = {
  strokeDasharray: '3 3',
  stroke: 'hsl(var(--border))',
} as const;

export const tooltipProps = {
  contentStyle: {
    backgroundColor: 'hsl(var(--card))',
    border: '1px solid hsl(var(--border))',
    borderRadius: '8px',
  },
  labelStyle: { color: 'hsl(var(--foreground))' },
} as const;

export function formatNumber(n: number): string {
  return n.toLocaleString();
}

export function formatPct(n: number): string {
  return `${n.toLocaleString(undefined, { maximumFractionDigits: 1 })}%`;
}

