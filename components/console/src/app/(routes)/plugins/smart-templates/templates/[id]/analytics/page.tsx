'use client'

import { useState, useEffect } from 'react'
import { useParams, useRouter } from 'next/navigation'
import {
  ArrowLeft,
  TrendingUp,
  Activity,
  Clock,
  Download,
  Eye,
  Users,
  Calendar,
  BarChart3,
  PieChart
} from 'lucide-react'
import Link from 'next/link'
import { format, subDays, startOfDay } from 'date-fns'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'

const mockTemplate = {
  id: '01956b69-9102-75b7-8860-3e75c11d231c',
  name: 'Monthly Account Statement',
  category: 'financial_reports',
  status: 'active',
  createdAt: '2024-11-15T00:00:00Z',
  lastUsed: '2025-01-01T12:30:00Z'
}

const mockAnalytics = {
  overview: {
    totalGenerations: 1456,
    thisMonth: 234,
    avgProcessingTime: '2.3s',
    successRate: 98.7,
    uniqueUsers: 89,
    popularFormats: {
      pdf: 67,
      html: 23,
      docx: 8,
      csv: 2
    }
  },
  usage: [
    { date: '2025-01-01', generations: 45, users: 12 },
    { date: '2024-12-31', generations: 38, users: 10 },
    { date: '2024-12-30', generations: 52, users: 15 },
    { date: '2024-12-29', generations: 31, users: 8 },
    { date: '2024-12-28', generations: 29, users: 9 },
    { date: '2024-12-27', generations: 41, users: 11 },
    { date: '2024-12-26', generations: 33, users: 7 }
  ],
  performance: {
    averageTime: [
      { period: '00:00', time: 2.1 },
      { period: '04:00', time: 1.8 },
      { period: '08:00', time: 3.2 },
      { period: '12:00', time: 4.1 },
      { period: '16:00', time: 3.8 },
      { period: '20:00', time: 2.5 }
    ],
    errors: [
      { date: '2025-01-01', count: 2 },
      { date: '2024-12-31', count: 1 },
      { date: '2024-12-30', count: 0 },
      { date: '2024-12-29', count: 3 },
      { date: '2024-12-28', count: 1 }
    ]
  },
  topUsers: [
    {
      name: 'john.doe@company.com',
      generations: 145,
      lastUsed: '2025-01-01T12:30:00Z'
    },
    {
      name: 'jane.smith@company.com',
      generations: 98,
      lastUsed: '2024-12-31T16:45:00Z'
    },
    {
      name: 'bob.wilson@company.com',
      generations: 76,
      lastUsed: '2024-12-30T09:15:00Z'
    },
    {
      name: 'alice.brown@company.com',
      generations: 67,
      lastUsed: '2024-12-29T14:20:00Z'
    },
    {
      name: 'charlie.davis@company.com',
      generations: 45,
      lastUsed: '2024-12-28T11:30:00Z'
    }
  ]
}

export default function TemplateAnalyticsPage() {
  const params = useParams()
  const router = useRouter()
  const templateId = params.id as string

  const [isLoading, setIsLoading] = useState(true)
  const [timeRange, setTimeRange] = useState('7d')

  useEffect(() => {
    // Simulate loading
    const timer = setTimeout(() => setIsLoading(false), 1000)
    return () => clearTimeout(timer)
  }, [])

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader.Root>
          <div className="flex items-center gap-3">
            <Skeleton className="h-8 w-8" />
            <div className="space-y-2">
              <Skeleton className="h-6 w-48" />
              <Skeleton className="h-4 w-64" />
            </div>
          </div>
          <Skeleton className="h-8 w-32" />
        </PageHeader.Root>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-32" />
          ))}
        </div>
        <Skeleton className="h-96" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" asChild>
            <Link href={`/plugins/smart-templates/templates/${templateId}`}>
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <PageHeader.InfoTitle
              title="Template Analytics"
              subtitle={mockTemplate.name}
            />
            <div className="mt-2 flex items-center gap-2">
              <Badge variant="secondary">
                {mockTemplate.category.replace('_', ' ')}
              </Badge>
              <Badge
                variant={
                  mockTemplate.status === 'active' ? 'default' : 'secondary'
                }
              >
                {mockTemplate.status}
              </Badge>
            </div>
          </div>
        </div>
        <PageHeader.InfoTooltip subtitle="Detailed usage analytics and performance metrics for this template" />
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm">
            <Download className="mr-2 h-4 w-4" />
            Export Report
          </Button>
        </div>
      </PageHeader.Root>

      {/* Key Metrics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Total Generations
            </CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mockAnalytics.overview.totalGenerations.toLocaleString()}
            </div>
            <p className="text-xs text-muted-foreground">
              +{mockAnalytics.overview.thisMonth} this month
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Avg Processing Time
            </CardTitle>
            <Clock className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mockAnalytics.overview.avgProcessingTime}
            </div>
            <p className="text-xs text-muted-foreground">
              -0.3s from last month
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Success Rate</CardTitle>
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">
              {mockAnalytics.overview.successRate}%
            </div>
            <p className="text-xs text-muted-foreground">
              +0.3% from last month
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Unique Users</CardTitle>
            <Users className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mockAnalytics.overview.uniqueUsers}
            </div>
            <p className="text-xs text-muted-foreground">
              +12 new users this month
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Analytics Tabs */}
      <Tabs defaultValue="usage" className="w-full">
        <TabsList>
          <TabsTrigger value="usage">Usage Trends</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
          <TabsTrigger value="formats">Output Formats</TabsTrigger>
          <TabsTrigger value="users">Top Users</TabsTrigger>
        </TabsList>

        <TabsContent value="usage" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <BarChart3 className="h-5 w-5" />
                Usage Over Time
              </CardTitle>
              <CardDescription>
                Daily report generations and active users
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="grid grid-cols-7 gap-2 text-sm">
                  {mockAnalytics.usage.map((day, index) => (
                    <div key={index} className="text-center">
                      <div className="mb-1 text-xs text-gray-500">
                        {format(new Date(day.date), 'MMM dd')}
                      </div>
                      <div className="rounded bg-blue-100 p-2">
                        <div className="font-medium text-blue-800">
                          {day.generations}
                        </div>
                        <div className="text-xs text-blue-600">reports</div>
                      </div>
                      <div className="mt-1 rounded bg-green-100 p-1">
                        <div className="text-xs text-green-800">
                          {day.users} users
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
                <div className="text-center text-sm text-gray-500">
                  Last 7 days
                </div>
              </div>
            </CardContent>
          </Card>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Peak Usage Hours</CardTitle>
                <CardDescription>
                  Most active times for report generation
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {mockAnalytics.performance.averageTime.map((slot, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between"
                    >
                      <span className="text-sm font-medium">{slot.period}</span>
                      <div className="flex items-center gap-2">
                        <div className="h-2 w-20 rounded-full bg-gray-200">
                          <div
                            className="h-2 rounded-full bg-blue-600"
                            style={{ width: `${(slot.time / 5) * 100}%` }}
                          />
                        </div>
                        <span className="text-sm text-gray-600">
                          {slot.time}s
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-lg">Recent Activity</CardTitle>
                <CardDescription>
                  Latest template generation events
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {mockAnalytics.usage.slice(0, 5).map((day, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between text-sm"
                    >
                      <div>
                        <div className="font-medium">
                          {format(new Date(day.date), 'MMM dd, yyyy')}
                        </div>
                        <div className="text-gray-500">
                          {day.users} users generated {day.generations} reports
                        </div>
                      </div>
                      <Badge variant="outline">{day.generations}</Badge>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="performance" className="space-y-4">
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Clock className="h-5 w-5" />
                  Processing Time Trends
                </CardTitle>
                <CardDescription>
                  Average processing time by hour
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {mockAnalytics.performance.averageTime.map((slot, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between"
                    >
                      <span className="text-sm font-medium">{slot.period}</span>
                      <div className="flex items-center gap-2">
                        <div className="h-3 w-32 rounded-full bg-gray-200">
                          <div
                            className="h-3 rounded-full bg-blue-600"
                            style={{ width: `${(slot.time / 5) * 100}%` }}
                          />
                        </div>
                        <span className="font-mono text-sm">{slot.time}s</span>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <TrendingUp className="h-5 w-5" />
                  Error Tracking
                </CardTitle>
                <CardDescription>Daily error count over time</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {mockAnalytics.performance.errors.map((day, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between"
                    >
                      <span className="text-sm">
                        {format(new Date(day.date), 'MMM dd')}
                      </span>
                      <div className="flex items-center gap-2">
                        <Badge
                          variant={
                            day.count === 0
                              ? 'default'
                              : day.count <= 2
                                ? 'secondary'
                                : 'destructive'
                          }
                        >
                          {day.count} errors
                        </Badge>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="formats" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <PieChart className="h-5 w-5" />
                Output Format Distribution
              </CardTitle>
              <CardDescription>
                Most popular report formats for this template
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {Object.entries(mockAnalytics.overview.popularFormats).map(
                  ([format, percentage]) => (
                    <div
                      key={format}
                      className="flex items-center justify-between"
                    >
                      <div className="flex items-center gap-3">
                        <div
                          className="h-4 w-4 rounded bg-blue-600"
                          style={{ opacity: percentage / 100 }}
                        />
                        <span className="font-medium uppercase">{format}</span>
                      </div>
                      <div className="flex items-center gap-2">
                        <div className="h-2 w-32 rounded-full bg-gray-200">
                          <div
                            className="h-2 rounded-full bg-blue-600"
                            style={{ width: `${percentage}%` }}
                          />
                        </div>
                        <span className="text-sm font-medium">
                          {percentage}%
                        </span>
                      </div>
                    </div>
                  )
                )}
              </div>
              <div className="mt-6 border-t pt-4">
                <div className="text-sm text-gray-600">
                  <strong>Total Downloads:</strong>{' '}
                  {mockAnalytics.overview.totalGenerations.toLocaleString()}{' '}
                  reports
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="users" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Users className="h-5 w-5" />
                Top Users
              </CardTitle>
              <CardDescription>
                Most active users of this template
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {mockAnalytics.topUsers.map((user, index) => (
                  <div
                    key={index}
                    className="flex items-center justify-between rounded-lg border p-3"
                  >
                    <div>
                      <div className="font-medium">{user.name}</div>
                      <div className="text-sm text-gray-500">
                        Last used:{' '}
                        {format(
                          new Date(user.lastUsed),
                          "MMM dd, yyyy 'at' HH:mm"
                        )}
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="font-medium">{user.generations}</div>
                      <div className="text-sm text-gray-500">generations</div>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
