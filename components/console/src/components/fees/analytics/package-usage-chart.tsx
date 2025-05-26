'use client'

import React from 'react'
import { Chart as ChartJS, ArcElement, Tooltip, Legend } from 'chart.js'
import { Pie } from 'react-chartjs-2'
import { PackageAnalytics } from '../types/fee-types'
import { Skeleton } from '@/components/ui/skeleton'

ChartJS.register(ArcElement, Tooltip, Legend)

interface PackageUsageChartProps {
  data: PackageAnalytics[]
  loading?: boolean
}

export function PackageUsageChart({
  data,
  loading = false
}: PackageUsageChartProps) {
  if (loading) {
    return <Skeleton className="h-[300px] w-full" />
  }

  const chartData = {
    labels: data.map((pkg) => pkg.packageName),
    datasets: [
      {
        data: data.map((pkg) => pkg.revenue),
        backgroundColor: [
          'rgba(99, 102, 241, 0.8)',
          'rgba(34, 197, 94, 0.8)',
          'rgba(251, 146, 60, 0.8)',
          'rgba(147, 51, 234, 0.8)',
          'rgba(250, 204, 21, 0.8)'
        ],
        borderColor: [
          'rgba(99, 102, 241, 1)',
          'rgba(34, 197, 94, 1)',
          'rgba(251, 146, 60, 1)',
          'rgba(147, 51, 234, 1)',
          'rgba(250, 204, 21, 1)'
        ],
        borderWidth: 1
      }
    ]
  }

  const options = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: 'right' as const,
        labels: {
          padding: 20,
          usePointStyle: true,
          font: {
            size: 12
          }
        }
      },
      tooltip: {
        callbacks: {
          label: function (context: any) {
            const label = context.label || ''
            const value = '$' + context.parsed.toFixed(2)
            const percentage = data[context.dataIndex].percentage
            return `${label}: ${value} (${percentage}%)`
          }
        }
      }
    }
  }

  return (
    <div className="h-[300px]">
      <Pie data={chartData} options={options} />
    </div>
  )
}
