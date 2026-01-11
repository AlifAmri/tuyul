import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Cell } from 'recharts';

interface PumpScoreChartProps {
  data: Array<{
    pair: string;
    score: number;
  }>;
  height?: number;
}

export function PumpScoreChart({ data, height = 300 }: PumpScoreChartProps) {
  const getBarColor = (score: number) => {
    if (score >= 80) return '#ef4444'; // red-500 - Very hot
    if (score >= 60) return '#f97316'; // orange-500 - Hot
    if (score >= 40) return '#eab308'; // yellow-500 - Warm
    return '#10b981'; // green-500 - Normal
  };

  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={data} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
        <XAxis
          dataKey="pair"
          stroke="#9CA3AF"
          style={{ fontSize: '12px' }}
        />
        <YAxis
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
          formatter={(value: number) => [value.toFixed(2), 'Pump Score']}
        />
        <Bar dataKey="score" radius={[8, 8, 0, 0]}>
          {data.map((entry, index) => (
            <Cell key={`cell-${index}`} fill={getBarColor(entry.score)} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  );
}

