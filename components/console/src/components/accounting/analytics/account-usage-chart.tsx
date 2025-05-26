'use client'

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  Title,
  Tooltip,
  Legend,
  ArcElement
} from 'chart.js'
import { Bar, Pie } from 'react-chartjs-2'

ChartJS.register(
  CategoryScale,
  LinearScale,
  BarElement,
  Title,
  Tooltip,
  Legend,
  ArcElement
)

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { TrendingUp, Activity, Users } from 'lucide-react'

interface AccountUsageData {
  keyValue: string
  name: string
  usageCount: number
  percentage: number
  color?: string
}

interface AccountUsageChartProps {
  data: AccountUsageData[]
  showDetails?: boolean
  variant?: 'bar' | 'pie' | 'both'
}

// Color palette for account types
const ACCOUNT_TYPE_COLORS = [
  '#0088FE', // Blue
  '#00C49F', // Green
  '#FFBB28', // Yellow
  '#FF8042', // Orange
  '#8884D8', // Purple
  '#82CA9D', // Light Green
  '#FFC658', // Light Orange
  '#FF7C7C', // Light Red
  '#8DD1E1', // Light Blue
  '#D084D0' // Light Purple
]

const AccountUsageStats = ({ data }: { data: AccountUsageData[] }) => {
  const totalUsage = data.reduce((sum, item) => sum + item.usageCount, 0)
  const topAccount = data[0]
  const averageUsage = totalUsage / data.length

  return (
    <div className="mb-6 grid grid-cols-1 gap-4 md:grid-cols-3">
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center space-x-2">
            <Activity className="h-4 w-4 text-blue-600" />
            <div className="space-y-1">
              <p className="text-sm font-medium leading-none">
                Total Transactions
              </p>
              <p className="text-2xl font-bold">
                {totalUsage.toLocaleString()}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center space-x-2">
            <TrendingUp className="h-4 w-4 text-green-600" />
            <div className="space-y-1">
              <p className="text-sm font-medium leading-none">Most Used Type</p>
              <p className="text-2xl font-bold">{topAccount.keyValue}</p>
              <p className="text-xs text-muted-foreground">
                {topAccount.percentage}% of total
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center space-x-2">
            <Users className="h-4 w-4 text-purple-600" />
            <div className="space-y-1">
              <p className="text-sm font-medium leading-none">Average Usage</p>
              <p className="text-2xl font-bold">
                {Math.round(averageUsage).toLocaleString()}
              </p>
              <p className="text-xs text-muted-foreground">per account type</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

const UsageTable = ({ data }: { data: AccountUsageData[] }) => (
  <div className="space-y-4">
    <div className="grid grid-cols-4 gap-4 border-b pb-2 text-sm font-medium text-muted-foreground">
      <div>Account Type</div>
      <div>Key Value</div>
      <div className="text-right">Usage Count</div>
      <div className="text-right">Percentage</div>
    </div>
    {data.map((item, index) => (
      <div
        key={item.keyValue}
        className="grid grid-cols-4 items-center gap-4 border-b border-gray-100 py-2 text-sm"
      >
        <div className="flex items-center space-x-2">
          <div
            className="h-3 w-3 rounded-full"
            style={{
              backgroundColor:
                ACCOUNT_TYPE_COLORS[index % ACCOUNT_TYPE_COLORS.length]
            }}
          />
          <span className="font-medium">{item.name}</span>
        </div>
        <div>
          <Badge variant="outline">{item.keyValue}</Badge>
        </div>
        <div className="text-right font-medium">
          {item.usageCount.toLocaleString()}
        </div>
        <div className="text-right">
          <Badge variant="secondary">{item.percentage}%</Badge>
        </div>
      </div>
    ))}
  </div>
)

export const AccountUsageChart = ({
  data,
  showDetails = false,
  variant = 'both'
}: AccountUsageChartProps) => {
  const barChartData = {
    labels: data.map((item) => item.keyValue),
    datasets: [
      {
        label: 'Usage Count',
        data: data.map((item) => item.usageCount),
        backgroundColor: ACCOUNT_TYPE_COLORS.slice(0, data.length),
        borderColor: ACCOUNT_TYPE_COLORS.slice(0, data.length).map(
          (color) => color + '80'
        ),
        borderWidth: 1
      }
    ]
  }

  const pieChartData = {
    labels: data.map((item) => `${item.name} (${item.keyValue})`),
    datasets: [
      {
        data: data.map((item) => item.usageCount),
        backgroundColor: ACCOUNT_TYPE_COLORS.slice(0, data.length),
        borderColor: ACCOUNT_TYPE_COLORS.slice(0, data.length).map(
          (color) => color + '80'
        ),
        borderWidth: 2
      }
    ]
  }

  const chartOptions = {
    responsive: true,
    plugins: {
      legend: {
        position: 'top' as const
      },
      title: {
        display: false
      }
    },
    scales: {
      y: {
        beginAtZero: true
      }
    }
  }

  const pieOptions = {
    responsive: true,
    plugins: {
      legend: {
        position: 'right' as const
      },
      title: {
        display: false
      }
    }
  }

  if (variant === 'bar') {
    return (
      <div className="space-y-4">
        {showDetails && <AccountUsageStats data={data} />}
        <div className="h-[300px]">
          <Bar data={barChartData} options={chartOptions} />
        </div>
      </div>
    )
  }

  if (variant === 'pie') {
    return (
      <div className="space-y-4">
        {showDetails && <AccountUsageStats data={data} />}
        <div className="h-[300px]">
          <Pie data={pieChartData} options={pieOptions} />
        </div>
      </div>
    )
  }

  // Both variant (default)
  return (
    <div className="space-y-6">
      {showDetails && <AccountUsageStats data={data} />}

      <Tabs defaultValue="chart" className="w-full">
        <TabsList className="grid w-full grid-cols-3">
          <TabsTrigger value="chart">Bar Chart</TabsTrigger>
          <TabsTrigger value="pie">Pie Chart</TabsTrigger>
          <TabsTrigger value="table">Data Table</TabsTrigger>
        </TabsList>

        <TabsContent value="chart" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Usage by Account Type</CardTitle>
              <CardDescription>
                Transaction volume distribution across different account types
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-[400px]">
                <Bar data={barChartData} options={chartOptions} />
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="pie" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Usage Distribution</CardTitle>
              <CardDescription>
                Percentage breakdown of transaction volume by account type
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-[400px]">
                <Pie data={pieChartData} options={pieOptions} />
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="table" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Account Type Usage Details</CardTitle>
              <CardDescription>
                Detailed breakdown of usage statistics for each account type
              </CardDescription>
            </CardHeader>
            <CardContent>
              <UsageTable data={data} />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}

// Standalone component with detailed stats
export const DetailedAccountUsageChart = ({
  data
}: {
  data: AccountUsageData[]
}) => {
  return <AccountUsageChart data={data} showDetails={true} variant="both" />
}

// Simple bar chart variant
export const SimpleAccountUsageChart = ({
  data
}: {
  data: AccountUsageData[]
}) => {
  return <AccountUsageChart data={data} showDetails={false} variant="bar" />
}

// Simple pie chart variant
export const PieAccountUsageChart = ({
  data
}: {
  data: AccountUsageData[]
}) => {
  return <AccountUsageChart data={data} showDetails={false} variant="pie" />
}
