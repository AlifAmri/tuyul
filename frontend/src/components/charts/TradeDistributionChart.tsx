import { PieChart, Pie, Cell, ResponsiveContainer, Legend, Tooltip } from 'recharts';

interface TradeDistributionChartProps {
  wins: number;
  losses: number;
  height?: number;
}

export function TradeDistributionChart({ wins, losses, height = 300 }: TradeDistributionChartProps) {
  const data = [
    { name: 'Wins', value: wins, color: '#10b981' },
    { name: 'Losses', value: losses, color: '#ef4444' },
  ];

  const total = wins + losses;

  return (
    <ResponsiveContainer width="100%" height={height}>
      <PieChart>
        <Pie
          data={data}
          cx="50%"
          cy="50%"
          labelLine={false}
          label={({ name, percent }) => `${name}: ${(percent * 100).toFixed(0)}%`}
          outerRadius={80}
          fill="#8884d8"
          dataKey="value"
        >
          {data.map((entry, index) => (
            <Cell key={`cell-${index}`} fill={entry.color} />
          ))}
        </Pie>
        <Tooltip
          contentStyle={{
            backgroundColor: '#1F2937',
            border: '1px solid #374151',
            borderRadius: '8px',
            color: '#fff',
          }}
          formatter={(value: number) => [
            `${value} trades (${((value / total) * 100).toFixed(1)}%)`,
            '',
          ]}
        />
        <Legend
          wrapperStyle={{ fontSize: '12px', color: '#9CA3AF' }}
          formatter={(value) => <span style={{ color: '#9CA3AF' }}>{value}</span>}
        />
      </PieChart>
    </ResponsiveContainer>
  );
}

