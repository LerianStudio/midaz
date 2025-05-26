'use client'

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
import { Line, Bar } from 'react-chartjs-2'

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

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  TrendingUp,
  TrendingDown,
  Shield,
  AlertTriangle,
  CheckCircle,
  Clock,
  Target,
  AlertCircle
} from 'lucide-react'

interface ComplianceTrend {
  date: string
  score: number
  violations: number
}

interface ComplianceTrendChartProps {
  data: ComplianceTrend[]
  showDetails?: boolean
  variant?: 'line' | 'area' | 'composed'
}

const ComplianceMetrics = ({ data }: { data: ComplianceTrend[] }) => {
  const latestData = data[data.length - 1]
  const previousData = data[data.length - 2]
  const scoreChange =
    latestData.score - (previousData?.score || latestData.score)
  const violationChange =
    latestData.violations - (previousData?.violations || latestData.violations)

  const averageScore =
    data.reduce((sum, item) => sum + item.score, 0) / data.length
  const totalViolations = data.reduce((sum, item) => sum + item.violations, 0)
  const trend =
    scoreChange > 0 ? 'improving' : scoreChange < 0 ? 'declining' : 'stable'

  const getScoreStatus = (score: number) => {
    if (score >= 95)
      return {
        label: 'Excellent',
        color: 'bg-green-100 text-green-800',
        icon: CheckCircle
      }
    if (score >= 90)
      return { label: 'Good', color: 'bg-blue-100 text-blue-800', icon: Target }
    if (score >= 80)
      return {
        label: 'Fair',
        color: 'bg-yellow-100 text-yellow-800',
        icon: Clock
      }
    return {
      label: 'Needs Attention',
      color: 'bg-red-100 text-red-800',
      icon: AlertCircle
    }
  }

  const scoreStatus = getScoreStatus(latestData.score)
  const ScoreIcon = scoreStatus.icon

  return (
    <div className="mb-6 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center space-x-2">
            <Shield className="h-4 w-4 text-green-600" />
            <div className="space-y-1">
              <p className="text-sm font-medium leading-none">Current Score</p>
              <p className="text-2xl font-bold">{latestData.score}%</p>
              <div className="flex items-center space-x-1">
                {scoreChange !== 0 && (
                  <>
                    {scoreChange > 0 ? (
                      <TrendingUp className="h-3 w-3 text-green-600" />
                    ) : (
                      <TrendingDown className="h-3 w-3 text-red-600" />
                    )}
                    <span
                      className={`text-xs ${scoreChange > 0 ? 'text-green-600' : 'text-red-600'}`}
                    >
                      {scoreChange > 0 ? '+' : ''}
                      {scoreChange.toFixed(1)}%
                    </span>
                  </>
                )}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center space-x-2">
            <AlertTriangle className="h-4 w-4 text-orange-600" />
            <div className="space-y-1">
              <p className="text-sm font-medium leading-none">
                Active Violations
              </p>
              <p className="text-2xl font-bold">{latestData.violations}</p>
              <div className="flex items-center space-x-1">
                {violationChange !== 0 && (
                  <>
                    {violationChange < 0 ? (
                      <TrendingDown className="h-3 w-3 text-green-600" />
                    ) : (
                      <TrendingUp className="h-3 w-3 text-red-600" />
                    )}
                    <span
                      className={`text-xs ${violationChange < 0 ? 'text-green-600' : 'text-red-600'}`}
                    >
                      {violationChange > 0 ? '+' : ''}
                      {violationChange}
                    </span>
                  </>
                )}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center space-x-2">
            <ScoreIcon className="h-4 w-4 text-blue-600" />
            <div className="space-y-1">
              <p className="text-sm font-medium leading-none">Status</p>
              <Badge className={scoreStatus.color}>{scoreStatus.label}</Badge>
              <p className="text-xs text-muted-foreground">
                Avg: {averageScore.toFixed(1)}%
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center space-x-2">
            {trend === 'improving' ? (
              <TrendingUp className="h-4 w-4 text-green-600" />
            ) : trend === 'declining' ? (
              <TrendingDown className="h-4 w-4 text-red-600" />
            ) : (
              <Target className="h-4 w-4 text-gray-600" />
            )}
            <div className="space-y-1">
              <p className="text-sm font-medium leading-none">Trend</p>
              <p
                className={`text-sm font-medium ${
                  trend === 'improving'
                    ? 'text-green-600'
                    : trend === 'declining'
                      ? 'text-red-600'
                      : 'text-gray-600'
                }`}
              >
                {trend.charAt(0).toUpperCase() + trend.slice(1)}
              </p>
              <p className="text-xs text-muted-foreground">
                Total violations: {totalViolations}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

const ComplianceInsights = ({ data }: { data: ComplianceTrend[] }) => {
  const latestScore = data[data.length - 1].score
  const earliestScore = data[0].score
  const overallImprovement = latestScore - earliestScore

  const maxScore = Math.max(...data.map((d) => d.score))
  const minScore = Math.min(...data.map((d) => d.score))
  const maxViolations = Math.max(...data.map((d) => d.violations))

  const insights = [
    {
      type:
        overallImprovement > 0
          ? 'positive'
          : overallImprovement < 0
            ? 'negative'
            : 'neutral',
      title: 'Overall Trend',
      description:
        overallImprovement > 0
          ? `Compliance score improved by ${overallImprovement.toFixed(1)}% over the period`
          : overallImprovement < 0
            ? `Compliance score declined by ${Math.abs(overallImprovement).toFixed(1)}% over the period`
            : 'Compliance score remained stable over the period'
    },
    {
      type: maxScore >= 95 ? 'positive' : 'neutral',
      title: 'Peak Performance',
      description: `Highest compliance score reached: ${maxScore}%`
    },
    {
      type: minScore < 90 ? 'negative' : 'positive',
      title: 'Risk Assessment',
      description:
        minScore < 90
          ? `Lowest score of ${minScore}% indicates periods of elevated risk`
          : `Consistently maintained scores above 90% indicating low risk`
    },
    {
      type:
        maxViolations > 5
          ? 'negative'
          : maxViolations > 2
            ? 'warning'
            : 'positive',
      title: 'Violation Analysis',
      description:
        maxViolations > 5
          ? `Peak violations of ${maxViolations} require immediate attention`
          : maxViolations > 2
            ? `Maximum ${maxViolations} violations indicate room for improvement`
            : `Low violation count (max ${maxViolations}) shows good compliance management`
    }
  ]

  const getInsightIcon = (type: string) => {
    switch (type) {
      case 'positive':
        return <CheckCircle className="h-4 w-4 text-green-600" />
      case 'negative':
        return <AlertCircle className="h-4 w-4 text-red-600" />
      case 'warning':
        return <AlertTriangle className="h-4 w-4 text-yellow-600" />
      default:
        return <Target className="h-4 w-4 text-blue-600" />
    }
  }

  const getInsightColor = (type: string) => {
    switch (type) {
      case 'positive':
        return 'border-green-200 bg-green-50'
      case 'negative':
        return 'border-red-200 bg-red-50'
      case 'warning':
        return 'border-yellow-200 bg-yellow-50'
      default:
        return 'border-blue-200 bg-blue-50'
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Compliance Insights</CardTitle>
        <CardDescription>
          Analysis and recommendations based on compliance trends
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {insights.map((insight, index) => (
            <div
              key={index}
              className={`rounded-lg border p-4 ${getInsightColor(insight.type)}`}
            >
              <div className="flex items-start space-x-3">
                {getInsightIcon(insight.type)}
                <div>
                  <h4 className="text-sm font-medium">{insight.title}</h4>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {insight.description}
                  </p>
                </div>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

export const ComplianceTrendChart = ({
  data,
  showDetails = true,
  variant = 'composed'
}: ComplianceTrendChartProps) => {
  // Format data for charts
  const labels = data.map((item) =>
    new Date(item.date).toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric'
    })
  )

  const scoreData = {
    labels,
    datasets: [
      {
        label: 'Compliance Score (%)',
        data: data.map((item) => item.score),
        borderColor: '#00C49F',
        backgroundColor: 'rgba(0, 196, 159, 0.1)',
        borderWidth: 3,
        fill: true,
        tension: 0.4
      }
    ]
  }

  const violationsData = {
    labels,
    datasets: [
      {
        label: 'Violations',
        data: data.map((item) => item.violations),
        backgroundColor: '#FF8042',
        borderColor: '#FF8042',
        borderWidth: 1
      }
    ]
  }

  const scoreOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: 'top' as const
      }
    },
    scales: {
      y: {
        min: 80,
        max: 100
      }
    }
  }

  const violationsOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: 'top' as const
      }
    },
    scales: {
      y: {
        beginAtZero: true
      }
    }
  }

  if (variant === 'line') {
    return (
      <div className="space-y-6">
        {showDetails && <ComplianceMetrics data={data} />}
        <Card>
          <CardHeader>
            <CardTitle>Compliance Score Trend</CardTitle>
            <CardDescription>
              Compliance score progression over time
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="h-[300px]">
              <Line data={scoreData} options={scoreOptions} />
            </div>
          </CardContent>
        </Card>
      </div>
    )
  }

  // Default view with tabs
  return (
    <div className="space-y-6">
      {showDetails && <ComplianceMetrics data={data} />}

      <Tabs defaultValue="overview" className="w-full">
        <TabsList className="grid w-full grid-cols-3">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="detailed">Detailed Analysis</TabsTrigger>
          <TabsTrigger value="insights">Insights</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Compliance Score Trend</CardTitle>
              <CardDescription>
                Compliance score progression over time
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-[400px]">
                <Line data={scoreData} options={scoreOptions} />
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="detailed" className="space-y-4">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Compliance Score Trend</CardTitle>
                <CardDescription>
                  Detailed compliance score progression
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-[300px]">
                  <Line data={scoreData} options={scoreOptions} />
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Violation Trends</CardTitle>
                <CardDescription>
                  Compliance violations over time
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-[300px]">
                  <Bar data={violationsData} options={violationsOptions} />
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Compliance Data Table */}
          <Card>
            <CardHeader>
              <CardTitle>Compliance History</CardTitle>
              <CardDescription>
                Detailed compliance data for each period
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="grid grid-cols-3 gap-4 border-b pb-2 text-sm font-medium text-muted-foreground">
                  <div>Date</div>
                  <div className="text-center">Compliance Score</div>
                  <div className="text-center">Violations</div>
                </div>
                {data
                  .slice()
                  .reverse()
                  .map((item, index) => (
                    <div
                      key={index}
                      className="grid grid-cols-3 items-center gap-4 border-b border-gray-100 py-2 text-sm"
                    >
                      <div className="font-medium">
                        {new Date(item.date).toLocaleDateString('en-US', {
                          month: 'long',
                          day: 'numeric',
                          year: 'numeric'
                        })}
                      </div>
                      <div className="text-center">
                        <Badge
                          variant={
                            item.score >= 95
                              ? 'default'
                              : item.score >= 90
                                ? 'secondary'
                                : 'destructive'
                          }
                        >
                          {item.score}%
                        </Badge>
                      </div>
                      <div className="text-center">
                        <Badge
                          variant={
                            item.violations === 0
                              ? 'default'
                              : item.violations <= 2
                                ? 'secondary'
                                : 'destructive'
                          }
                        >
                          {item.violations}
                        </Badge>
                      </div>
                    </div>
                  ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="insights" className="space-y-4">
          <ComplianceInsights data={data} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// Standalone variants
export const SimpleComplianceTrendChart = ({
  data
}: {
  data: ComplianceTrend[]
}) => {
  return <ComplianceTrendChart data={data} showDetails={false} variant="line" />
}

export const DetailedComplianceTrendChart = ({
  data
}: {
  data: ComplianceTrend[]
}) => {
  return (
    <ComplianceTrendChart data={data} showDetails={true} variant="composed" />
  )
}
