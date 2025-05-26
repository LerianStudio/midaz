'use client'

import React from 'react'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'react-chartjs-2'
import { TimeSeriesPoint } from '../types/fee-types'
import { Skeleton } from '@/components/ui/skeleton'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

interface FeeRevenueChartProps {
  data: TimeSeriesPoint[]
  loading?: boolean
}

export function FeeRevenueChart({
  data,
  loading = false
}: FeeRevenueChartProps) {
  if (loading) {
    return <Skeleton className="h-[300px] w-full" />
  }

  const chartData = {
    labels: data.map((point) => {
      const date = new Date(point.date)
      return date.toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric'
      })
    }),
    datasets: [
      {
        label: 'Revenue',
        data: data.map((point) => point.revenue),
        borderColor: 'rgb(99, 102, 241)',
        backgroundColor: 'rgba(99, 102, 241, 0.1)',
        fill: true,
        tension: 0.3
      },
      {
        label: 'Transactions',
        data: data.map((point) => point.transactionCount * 10), // Scale for visibility
        borderColor: 'rgb(34, 197, 94)',
        backgroundColor: 'rgba(34, 197, 94, 0.1)',
        fill: false,
        tension: 0.3,
        yAxisID: 'y1'
      }
    ]
  }

  const options = {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      mode: 'index' as const,
      intersect: false
    },
    plugins: {
      legend: {
        display: true,
        position: 'top' as const
      },
      tooltip: {
        callbacks: {
          label: function (context: any) {
            let label = context.dataset.label || ''
            if (label) {
              label += ': '
            }
            if (context.dataset.label === 'Revenue') {
              label += '$' + context.parsed.y.toFixed(2)
            } else {
              label += Math.round(context.parsed.y / 10) + ' transactions'
            }
            return label
          }
        }
      }
    },
    scales: {
      x: {
        grid: {
          display: false
        }
      },
      y: {
        type: 'linear' as const,
        display: true,
        position: 'left' as const,
        title: {
          display: true,
          text: 'Revenue ($)'
        }
      },
      y1: {
        type: 'linear' as const,
        display: true,
        position: 'right' as const,
        title: {
          display: true,
          text: 'Transactions'
        },
        grid: {
          drawOnChartArea: false
        }
      }
    }
  }

  return (
    <div className="h-[300px]">
      <Line data={chartData} options={options} />
    </div>
  )
}
