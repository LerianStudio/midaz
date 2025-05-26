'use client'

import { useState } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { Button } from '@/components/ui/button'
import {
  TrendingUp,
  TrendingDown,
  BarChart3,
  PieChart,
  Activity,
  Clock,
  FileText,
  Users,
  Zap,
  Download,
  Calendar,
  Filter,
  RefreshCw,
  AlertTriangle,
  CheckCircle2,
  XCircle
} from 'lucide-react'

interface AnalyticsData {
  period: string
  totalReports: number
  successfulReports: number
  failedReports: number
  avgGenerationTime: number
  totalTemplates: number
  activeTemplates: number
  uniqueUsers: number
  storageUsed: string
  topTemplates: Array<{
    id: string
    name: string
    usage: number
    successRate: number
  }>
  performanceMetrics: Array<{
    date: string
    reports: number
    avgTime: number
    successRate: number
  }>
  errorAnalysis: Array<{
    error: string
    count: number
    percentage: number
  }>
}

const mockAnalyticsData: AnalyticsData = {
  period: 'last-30-days',
  totalReports: 2456,
  successfulReports: 2398,
  failedReports: 58,
  avgGenerationTime: 3.2,
  totalTemplates: 45,
  activeTemplates: 38,
  uniqueUsers: 124,
  storageUsed: '128.5 GB',
  topTemplates: [
    {
      id: 'tpl-1',
      name: 'Financial Performance Report',
      usage: 456,
      successRate: 98.5
    },
    {
      id: 'tpl-2',
      name: 'Customer Analytics Dashboard',
      usage: 298,
      successRate: 99.2
    },
    {
      id: 'tpl-3',
      name: 'Compliance Audit Report',
      usage: 234,
      successRate: 95.8
    },
    { id: 'tpl-4', name: 'Transaction Summary', usage: 189, successRate: 97.1 },
    {
      id: 'tpl-5',
      name: 'Marketing Performance',
      usage: 167,
      successRate: 96.4
    }
  ],
  performanceMetrics: [
    { date: '2024-01-01', reports: 45, avgTime: 3.1, successRate: 97.8 },
    { date: '2024-01-02', reports: 52, avgTime: 3.4, successRate: 98.1 },
    { date: '2024-01-03', reports: 38, avgTime: 2.9, successRate: 98.7 },
    { date: '2024-01-04', reports: 61, avgTime: 3.8, successRate: 95.2 },
    { date: '2024-01-05', reports: 49, avgTime: 3.2, successRate: 97.9 },
    { date: '2024-01-06', reports: 44, avgTime: 3.0, successRate: 98.6 },
    { date: '2024-01-07', reports: 56, avgTime: 3.5, successRate: 96.4 }
  ],
  errorAnalysis: [
    { error: 'Data source timeout', count: 28, percentage: 48.3 },
    { error: 'Template validation failed', count: 15, percentage: 25.9 },
    { error: 'Insufficient memory', count: 8, percentage: 13.8 },
    { error: 'Network connectivity', count: 4, percentage: 6.9 },
    { error: 'Other', count: 3, percentage: 5.1 }
  ]
}

export function AnalyticsDashboard() {
  const [data] = useState<AnalyticsData>(mockAnalyticsData)
  const [selectedPeriod, setSelectedPeriod] = useState(data.period)
  const [refreshing, setRefreshing] = useState(false)

  const successRate = (data.successfulReports / data.totalReports) * 100
  const failureRate = (data.failedReports / data.totalReports) * 100
  const templateUtilization = (data.activeTemplates / data.totalTemplates) * 100

  const handleRefresh = async () => {
    setRefreshing(true)
    // Simulate API call
    setTimeout(() => {
      setRefreshing(false)
    }, 1000)
  }

  const formatNumber = (num: number) => {
    return new Intl.NumberFormat().format(num)
  }

  const renderOverviewTab = () => (
    <div className="space-y-6">
      {/* Key Metrics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Total Reports</p>
                <p className="text-2xl font-bold">
                  {formatNumber(data.totalReports)}
                </p>
                <div className="mt-1 flex items-center space-x-1">
                  <TrendingUp className="h-3 w-3 text-green-500" />
                  <span className="text-xs text-green-600">+12.5%</span>
                </div>
              </div>
              <FileText className="h-8 w-8 text-blue-500" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Success Rate</p>
                <p className="text-2xl font-bold">{successRate.toFixed(1)}%</p>
                <div className="mt-1 flex items-center space-x-1">
                  <TrendingUp className="h-3 w-3 text-green-500" />
                  <span className="text-xs text-green-600">+2.3%</span>
                </div>
              </div>
              <CheckCircle2 className="h-8 w-8 text-green-500" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">
                  Avg Generation Time
                </p>
                <p className="text-2xl font-bold">{data.avgGenerationTime}s</p>
                <div className="mt-1 flex items-center space-x-1">
                  <TrendingDown className="h-3 w-3 text-green-500" />
                  <span className="text-xs text-green-600">-0.8s</span>
                </div>
              </div>
              <Clock className="h-8 w-8 text-orange-500" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Active Users</p>
                <p className="text-2xl font-bold">{data.uniqueUsers}</p>
                <div className="mt-1 flex items-center space-x-1">
                  <TrendingUp className="h-3 w-3 text-green-500" />
                  <span className="text-xs text-green-600">+8 users</span>
                </div>
              </div>
              <Users className="h-8 w-8 text-purple-500" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Success Rate Breakdown */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center space-x-2">
              <PieChart className="h-5 w-5" />
              <span>Report Status Distribution</span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center space-x-2">
                  <div className="h-3 w-3 rounded-full bg-green-500"></div>
                  <span className="text-sm">Successful</span>
                </div>
                <div className="text-right">
                  <span className="font-medium">
                    {formatNumber(data.successfulReports)}
                  </span>
                  <span className="ml-2 text-xs text-muted-foreground">
                    ({successRate.toFixed(1)}%)
                  </span>
                </div>
              </div>
              <Progress value={successRate} className="h-2" />

              <div className="flex items-center justify-between">
                <div className="flex items-center space-x-2">
                  <div className="h-3 w-3 rounded-full bg-red-500"></div>
                  <span className="text-sm">Failed</span>
                </div>
                <div className="text-right">
                  <span className="font-medium">
                    {formatNumber(data.failedReports)}
                  </span>
                  <span className="ml-2 text-xs text-muted-foreground">
                    ({failureRate.toFixed(1)}%)
                  </span>
                </div>
              </div>
              <Progress value={failureRate} className="h-2" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center space-x-2">
              <Activity className="h-5 w-5" />
              <span>Template Utilization</span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">
                  Active Templates
                </span>
                <span className="font-medium">
                  {data.activeTemplates} of {data.totalTemplates}
                </span>
              </div>
              <Progress value={templateUtilization} className="h-2" />
              <p className="text-xs text-muted-foreground">
                {templateUtilization.toFixed(1)}% of templates are actively used
              </p>

              <div className="space-y-2 pt-4">
                <div className="flex items-center justify-between text-sm">
                  <span className="text-muted-foreground">Storage Used</span>
                  <span className="font-medium">{data.storageUsed}</span>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-muted-foreground">
                    Reports This Month
                  </span>
                  <span className="font-medium">
                    {formatNumber(data.totalReports)}
                  </span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Top Templates */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center space-x-2">
            <BarChart3 className="h-5 w-5" />
            <span>Most Used Templates</span>
          </CardTitle>
          <CardDescription>
            Templates ranked by usage and success rate
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {data.topTemplates.map((template, index) => (
              <div key={template.id} className="flex items-center space-x-4">
                <div className="w-8 text-center">
                  <Badge variant={index < 3 ? 'default' : 'secondary'}>
                    #{index + 1}
                  </Badge>
                </div>
                <div className="flex-1">
                  <h4 className="font-medium">{template.name}</h4>
                  <div className="flex items-center space-x-4 text-sm text-muted-foreground">
                    <span>{formatNumber(template.usage)} reports</span>
                    <span>{template.successRate}% success rate</span>
                  </div>
                </div>
                <div className="w-24">
                  <Progress value={template.successRate} className="h-2" />
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  )

  const renderPerformanceTab = () => (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Performance Trends</CardTitle>
          <CardDescription>
            Daily report generation metrics over time
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex h-64 items-center justify-center text-muted-foreground">
            <div className="text-center">
              <BarChart3 className="mx-auto mb-4 h-16 w-16" />
              <p>Performance chart visualization would appear here</p>
              <p className="text-sm">
                Integration with charting library needed
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {data.performanceMetrics.slice(-3).map((metric, index) => (
          <Card key={index}>
            <CardContent className="p-4">
              <div className="space-y-2">
                <p className="text-sm text-muted-foreground">
                  {new Date(metric.date).toLocaleDateString()}
                </p>
                <div className="space-y-1">
                  <div className="flex justify-between">
                    <span className="text-xs">Reports</span>
                    <span className="text-xs font-medium">
                      {metric.reports}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-xs">Avg Time</span>
                    <span className="text-xs font-medium">
                      {metric.avgTime}s
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-xs">Success Rate</span>
                    <span className="text-xs font-medium">
                      {metric.successRate}%
                    </span>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )

  const renderErrorsTab = () => (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center space-x-2">
            <AlertTriangle className="h-5 w-5 text-red-500" />
            <span>Error Analysis</span>
          </CardTitle>
          <CardDescription>
            Breakdown of report generation failures
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {data.errorAnalysis.map((error, index) => (
              <div key={index} className="space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">{error.error}</span>
                  <div className="text-right">
                    <span className="text-sm font-medium">{error.count}</span>
                    <span className="ml-2 text-xs text-muted-foreground">
                      ({error.percentage}%)
                    </span>
                  </div>
                </div>
                <Progress value={error.percentage} className="h-2" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              Resolution Recommendations
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="rounded-lg bg-orange-50 p-3 dark:bg-orange-900/20">
              <h4 className="mb-1 text-sm font-medium">Data Source Timeouts</h4>
              <p className="text-xs text-muted-foreground">
                Consider increasing timeout values or implementing connection
                pooling
              </p>
            </div>
            <div className="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
              <h4 className="mb-1 text-sm font-medium">Template Validation</h4>
              <p className="text-xs text-muted-foreground">
                Review template syntax and required variables
              </p>
            </div>
            <div className="rounded-lg bg-green-50 p-3 dark:bg-green-900/20">
              <h4 className="mb-1 text-sm font-medium">Memory Issues</h4>
              <p className="text-xs text-muted-foreground">
                Optimize template complexity or increase memory allocation
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Error Trends</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">This week</span>
                <div className="flex items-center space-x-1">
                  <XCircle className="h-3 w-3 text-red-500" />
                  <span className="font-medium">12 errors</span>
                </div>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Last week</span>
                <div className="flex items-center space-x-1">
                  <XCircle className="h-3 w-3 text-red-500" />
                  <span className="font-medium">18 errors</span>
                </div>
              </div>
              <div className="flex items-center space-x-1 text-sm text-green-600">
                <TrendingDown className="h-3 w-3" />
                <span>33% reduction</span>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Analytics Dashboard</h1>
          <p className="text-muted-foreground">
            Monitor template performance and usage metrics
          </p>
        </div>
        <div className="flex items-center space-x-2">
          <Select value={selectedPeriod} onValueChange={setSelectedPeriod}>
            <SelectTrigger className="w-[140px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="last-7-days">Last 7 days</SelectItem>
              <SelectItem value="last-30-days">Last 30 days</SelectItem>
              <SelectItem value="last-90-days">Last 90 days</SelectItem>
              <SelectItem value="last-year">Last year</SelectItem>
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            onClick={handleRefresh}
            disabled={refreshing}
            className="flex items-center space-x-2"
          >
            <RefreshCw
              className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`}
            />
            <span>Refresh</span>
          </Button>
          <Button variant="outline" className="flex items-center space-x-2">
            <Download className="h-4 w-4" />
            <span>Export</span>
          </Button>
        </div>
      </div>

      {/* Analytics Tabs */}
      <Tabs defaultValue="overview" className="space-y-6">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
          <TabsTrigger value="errors">Errors</TabsTrigger>
        </TabsList>

        <TabsContent value="overview">{renderOverviewTab()}</TabsContent>

        <TabsContent value="performance">{renderPerformanceTab()}</TabsContent>

        <TabsContent value="errors">{renderErrorsTab()}</TabsContent>
      </Tabs>
    </div>
  )
}
