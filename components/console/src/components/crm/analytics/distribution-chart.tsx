'use client'

import React from 'react'
import {
  PieChart,
  Pie,
  Cell,
  ResponsiveContainer,
  Legend,
  Tooltip
} from 'recharts'
import { Skeleton } from '@/components/ui/skeleton'
import { useTheme } from 'next-themes'

interface DistributionChartProps {
  data: Array<{
    type: string
    count: number
    percentage: number
  }>
  loading?: boolean
  title?: string
}

export const DistributionChart: React.FC<DistributionChartProps> = ({
  data,
  loading = false,
  title
}) => {
  const { theme } = useTheme()
  const isDark = theme === 'dark'

  if (loading) {
    return <Skeleton className="h-[300px] w-full" />
  }

  const COLORS = [
    '#3b82f6', // blue
    '#10b981', // emerald
    '#f59e0b', // amber
    '#ef4444', // red
    '#8b5cf6', // violet
    '#ec4899', // pink
    '#06b6d4', // cyan
    '#84cc16' // lime
  ]

  const renderCustomizedLabel = ({
    cx,
    cy,
    midAngle,
    innerRadius,
    outerRadius,
    percentage
  }: any) => {
    const RADIAN = Math.PI / 180
    const radius = innerRadius + (outerRadius - innerRadius) * 0.5
    const x = cx + radius * Math.cos(-midAngle * RADIAN)
    const y = cy + radius * Math.sin(-midAngle * RADIAN)

    if (percentage < 5) return null

    return (
      <text
        x={x}
        y={y}
        fill="white"
        textAnchor={x > cx ? 'start' : 'end'}
        dominantBaseline="central"
        fontSize={12}
        fontWeight="bold"
      >
        {`${percentage}%`}
      </text>
    )
  }

  return (
    <div className="space-y-4">
      {title && (
        <h4 className="text-sm font-medium text-muted-foreground">{title}</h4>
      )}
      <ResponsiveContainer width="100%" height={300}>
        <PieChart>
          <Pie
            data={data}
            cx="50%"
            cy="50%"
            labelLine={false}
            label={renderCustomizedLabel}
            outerRadius={100}
            fill="#8884d8"
            dataKey="percentage"
          >
            {data.map((entry, index) => (
              <Cell
                key={`cell-${index}`}
                fill={COLORS[index % COLORS.length]}
              />
            ))}
          </Pie>
          <Tooltip
            contentStyle={{
              backgroundColor: isDark ? '#1f2937' : '#ffffff',
              border: `1px solid ${isDark ? '#374151' : '#e5e7eb'}`,
              borderRadius: '8px'
            }}
            formatter={(value: number, name: string, props: any) => [
              `${props.payload.count} (${value}%)`,
              props.payload.type
            ]}
          />
          <Legend
            verticalAlign="bottom"
            height={36}
            formatter={(value: string, entry: any) => `${entry.payload.type}`}
          />
        </PieChart>
      </ResponsiveContainer>
    </div>
  )
}
