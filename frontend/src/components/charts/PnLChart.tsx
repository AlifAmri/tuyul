import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';

interface PnLChartProps {
  data: Array<{
    timestamp: string;
    profit: number;
  }>;
  height?: number;
}

export function PnLChart({ data, height = 300 }: PnLChartProps) {
  const formatCurrency = (value: number) => {
    return `Rp ${(value / 1000).toFixed(0)}K`;
  };

  const formatDate = (timestamp: string) => {
    const date = new Date(timestamp);
    return date.toLocaleDateString('id-ID', { month: 'short', day: 'numeric' });
  };

  return (
    <ResponsiveContainer width="100%" height={height}>
      <LineChart data={data} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
        <XAxis
          dataKey="timestamp"
          tickFormatter={formatDate}
          stroke="#9CA3AF"
          style={{ fontSize: '12px' }}
        />
        <YAxis
          tickFormatter={formatCurrency}
          stroke="#9CA3AF"
          style={{ fontSize: '12px' }}
        />
        <Tooltip
          contentStyle={{
            backgroundColor: '#1F2937',
            border: '1px solid #374151',
            borderRadius: '8px',
            color: '#fff',
          }}
          labelStyle={{ color: '#9CA3AF' }}
          formatter={(value: number) => [`Rp ${value.toLocaleString()}`, 'Profit']}
          labelFormatter={formatDate}
        />
        <Legend
          wrapperStyle={{ fontSize: '12px', color: '#9CA3AF' }}
        />
        <Line
          type="monotone"
          dataKey="profit"
          stroke="#10b981"
          strokeWidth={2}
          dot={{ fill: '#10b981', r: 4 }}
          activeDot={{ r: 6 }}
          name="Profit (IDR)"
        />
      </LineChart>
    </ResponsiveContainer>
  );
}

