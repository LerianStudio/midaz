'use client'

import React from 'react'
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  BarChart,
  Bar
} from 'recharts'
import { Skeleton } from '@/components/ui/skeleton'
import { useTheme } from 'next-themes'

interface HolderGrowthChartProps {
  data: Array<{
    date: string
    holders: number
    newHolders: number
  }>
  loading?: boolean
  chartType?: 'area' | 'bar'
}

export const HolderGrowthChart: React.FC<HolderGrowthChartProps> = ({
  data,
  loading = false,
  chartType = 'area'
}) => {
  const { theme } = useTheme()
  const isDark = theme === 'dark'

  if (loading) {
    return <Skeleton className="h-[300px] w-full" />
  }

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr)
    return date.toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric'
    })
  }

  const chartColors = {
    primary: isDark ? '#3b82f6' : '#2563eb',
    secondary: isDark ? '#10b981' : '#059669',
    grid: isDark ? '#374151' : '#e5e7eb',
    text: isDark ? '#9ca3af' : '#6b7280'
  }

  if (chartType === 'bar') {
    return (
      <ResponsiveContainer width="100%" height={300}>
        <BarChart data={data}>
          <CartesianGrid strokeDasharray="3 3" stroke={chartColors.grid} />
          <XAxis
            dataKey="date"
            tickFormatter={formatDate}
            stroke={chartColors.text}
            fontSize={12}
          />
          <YAxis stroke={chartColors.text} fontSize={12} />
          <Tooltip
            contentStyle={{
              backgroundColor: isDark ? '#1f2937' : '#ffffff',
              border: `1px solid ${chartColors.grid}`,
              borderRadius: '8px'
            }}
            labelFormatter={(value) => formatDate(value as string)}
          />
          <Bar
            dataKey="newHolders"
            fill={chartColors.secondary}
            name="New Holders"
            radius={[4, 4, 0, 0]}
          />
        </BarChart>
      </ResponsiveContainer>
    )
  }

  return (
    <ResponsiveContainer width="100%" height={300}>
      <AreaChart data={data}>
        <defs>
          <linearGradient id="colorHolders" x1="0" y1="0" x2="0" y2="1">
            <stop
              offset="5%"
              stopColor={chartColors.primary}
              stopOpacity={0.8}
            />
            <stop
              offset="95%"
              stopColor={chartColors.primary}
              stopOpacity={0}
            />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke={chartColors.grid} />
        <XAxis
          dataKey="date"
          tickFormatter={formatDate}
          stroke={chartColors.text}
          fontSize={12}
        />
        <YAxis stroke={chartColors.text} fontSize={12} />
        <Tooltip
          contentStyle={{
            backgroundColor: isDark ? '#1f2937' : '#ffffff',
            border: `1px solid ${chartColors.grid}`,
            borderRadius: '8px'
          }}
          labelFormatter={(value) => formatDate(value as string)}
        />
        <Area
          type="monotone"
          dataKey="holders"
          stroke={chartColors.primary}
          fillOpacity={1}
          fill="url(#colorHolders)"
          strokeWidth={2}
          name="Total Holders"
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}
