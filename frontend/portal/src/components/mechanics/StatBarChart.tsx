import { Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';

export interface StatBarSeries {
  /** Key of the numeric value in each datum. */
  key: string;
  label: string;
  /** CSS color, e.g. "hsl(var(--primary))". */
  color: string;
}

interface StatBarChartProps {
  data: Array<Record<string, string | number>>;
  /** Key of the category / x-axis label in each datum. */
  xKey: string;
  series: StatBarSeries[];
  height?: number;
  /** Format of the x-axis ticks (the tooltip label uses the raw value). */
  tickFormatter?: (value: string) => string;
}

/** Bar chart in the portal's chart style (see Overview), one bar per series. */
export function StatBarChart({ data, xKey, series, height = 260, tickFormatter }: StatBarChartProps) {
  return (
    <div style={{ height }}>
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={data}>
          <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
          <XAxis
            dataKey={xKey}
            stroke="hsl(var(--muted-foreground))"
            fontSize={12}
            tickFormatter={tickFormatter}
            minTickGap={12}
          />
          <YAxis allowDecimals={false} stroke="hsl(var(--muted-foreground))" fontSize={12} width={56} />
          <Tooltip
            cursor={{ fill: 'hsl(var(--secondary))', opacity: 0.4 }}
            contentStyle={{
              backgroundColor: 'hsl(var(--card))',
              border: '1px solid hsl(var(--border))',
              borderRadius: '8px',
            }}
            labelStyle={{ color: 'hsl(var(--foreground))' }}
            formatter={(value: number | string, name: string) => [
              typeof value === 'number' ? value.toLocaleString() : value,
              name,
            ]}
          />
          {series.length > 1 && <Legend wrapperStyle={{ fontSize: 12 }} />}
          {series.map((s) => (
            <Bar key={s.key} dataKey={s.key} name={s.label} fill={s.color} radius={[4, 4, 0, 0]} />
          ))}
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
