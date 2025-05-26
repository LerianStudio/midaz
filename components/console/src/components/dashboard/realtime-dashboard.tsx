'use client'

import { useEffect, useMemo, useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useWebSocket } from '@/providers/websocket-provider'
import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  LineChart,
  Line,
  AreaChart,
  Area,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts'
import {
  TrendingUp,
  TrendingDown,
  Activity,
  DollarSign,
  Users,
  CreditCard,
  AlertCircle,
  CheckCircle,
  Clock,
} from 'lucide-react'

interface DashboardMetrics {
  totalBalance: number
  totalTransactions: number
  activeAccounts: number
  pendingTransactions: number
  dailyVolume: { date: string; volume: number }[]
  accountDistribution: { type: string; count: number; percentage: number }[]
  recentActivity: ActivityItem[]
}

interface ActivityItem {
  id: string
  type: 'transaction' | 'account' | 'alert'
  title: string
  description: string
  timestamp: string
  status: 'success' | 'pending' | 'failed'
  amount?: number
}

const COLORS = ['#0088FE', '#00C49F', '#FFBB28', '#FF8042', '#8884D8']

export function RealtimeDashboard() {
  const { subscribe, unsubscribe } = useWebSocket()
  const [liveMetrics, setLiveMetrics] = useState<Partial<DashboardMetrics>>({})
  const [activityFeed, setActivityFeed] = useState<ActivityItem[]>([])
  
  // Fetch initial dashboard data
  const { data: dashboardData, isLoading } = useQuery({
    queryKey: ['dashboard-metrics'],
    queryFn: async () => {
      const response = await fetch('/api/dashboard/metrics')
      if (!response.ok) throw new Error('Failed to fetch dashboard metrics')
      return response.json() as Promise<DashboardMetrics>
    },
    refetchInterval: 60000, // Refetch every minute
  })
  
  // Merge live updates with fetched data
  const metrics = useMemo(() => ({
    ...dashboardData,
    ...liveMetrics,
  }), [dashboardData, liveMetrics])
  
  // Subscribe to real-time updates
  useEffect(() => {
    const handleMetricUpdate = (update: Partial<DashboardMetrics>) => {
      setLiveMetrics(prev => ({ ...prev, ...update }))
    }
    
    const handleNewActivity = (activity: ActivityItem) => {
      setActivityFeed(prev => [activity, ...prev].slice(0, 10))
    }
    
    subscribe('metrics:update', handleMetricUpdate)
    subscribe('activity:new', handleNewActivity)
    
    return () => {
      unsubscribe('metrics:update', handleMetricUpdate)
      unsubscribe('activity:new', handleNewActivity)
    }
  }, [subscribe, unsubscribe])
  
  if (isLoading) {
    return <DashboardSkeleton />
  }
  
  return (
    <div className="space-y-6 p-6">
      {/* Metrics Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          title="Total Balance"
          value={metrics.totalBalance}
          icon={DollarSign}
          trend={12.5}
          format="currency"
        />
        <MetricCard
          title="Transactions"
          value={metrics.totalTransactions}
          icon={CreditCard}
          trend={8.2}
          format="number"
        />
        <MetricCard
          title="Active Accounts"
          value={metrics.activeAccounts}
          icon={Users}
          trend={-2.4}
          format="number"
        />
        <MetricCard
          title="Pending"
          value={metrics.pendingTransactions}
          icon={Clock}
          format="number"
          variant="warning"
        />
      </div>
      
      {/* Charts Row */}
      <div className="grid gap-4 lg:grid-cols-2">
        {/* Daily Volume Chart */}
        <Card>
          <CardHeader>
            <CardTitle>Daily Transaction Volume</CardTitle>
          </CardHeader>
          <CardContent>
            <ResponsiveContainer width="100%" height={300}>
              <AreaChart data={metrics.dailyVolume}>
                <defs>
                  <linearGradient id="colorVolume" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#8884d8" stopOpacity={0.8}/>
                    <stop offset="95%" stopColor="#8884d8" stopOpacity={0}/>
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                <XAxis 
                  dataKey="date" 
                  className="text-xs"
                  tick={{ fill: 'currentColor' }}
                />
                <YAxis 
                  className="text-xs"
                  tick={{ fill: 'currentColor' }}
                />
                <Tooltip 
                  contentStyle={{ 
                    backgroundColor: 'hsl(var(--background))',
                    border: '1px solid hsl(var(--border))',
                    borderRadius: '6px',
                  }}
                />
                <Area
                  type="monotone"
                  dataKey="volume"
                  stroke="#8884d8"
                  fillOpacity={1}
                  fill="url(#colorVolume)"
                />
              </AreaChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>
        
        {/* Account Distribution */}
        <Card>
          <CardHeader>
            <CardTitle>Account Distribution</CardTitle>
          </CardHeader>
          <CardContent>
            <ResponsiveContainer width="100%" height={300}>
              <PieChart>
                <Pie
                  data={metrics.accountDistribution}
                  cx="50%"
                  cy="50%"
                  labelLine={false}
                  label={({ percentage }) => `${percentage.toFixed(0)}%`}
                  outerRadius={80}
                  fill="#8884d8"
                  dataKey="count"
                >
                  {metrics.accountDistribution?.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip />
                <Legend />
              </PieChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>
      </div>
      
      {/* Activity Feed */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Live Activity Feed</CardTitle>
          <Activity className="h-4 w-4 text-muted-foreground animate-pulse" />
        </CardHeader>
        <CardContent>
          <AnimatePresence mode="popLayout">
            {activityFeed.length === 0 ? (
              <p className="text-center text-muted-foreground py-8">
                No recent activity
              </p>
            ) : (
              <div className="space-y-2">
                {activityFeed.map((activity) => (
                  <motion.div
                    key={activity.id}
                    initial={{ opacity: 0, y: -20 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, x: -100 }}
                    transition={{ duration: 0.3 }}
                  >
                    <ActivityFeedItem activity={activity} />
                  </motion.div>
                ))}
              </div>
            )}
          </AnimatePresence>
        </CardContent>
      </Card>
    </div>
  )
}

// Metric Card Component
function MetricCard({
  title,
  value,
  icon: Icon,
  trend,
  format = 'number',
  variant = 'default',
}: {
  title: string
  value?: number
  icon: React.ComponentType<{ className?: string }>
  trend?: number
  format?: 'number' | 'currency'
  variant?: 'default' | 'warning' | 'danger'
}) {
  const formattedValue = useMemo(() => {
    if (value === undefined) return '—'
    
    if (format === 'currency') {
      return new Intl.NumberFormat('en-US', {
        style: 'currency',
        currency: 'USD',
        minimumFractionDigits: 0,
        maximumFractionDigits: 0,
      }).format(value)
    }
    
    return new Intl.NumberFormat('en-US').format(value)
  }, [value, format])
  
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <Icon className={`h-4 w-4 ${
          variant === 'warning' ? 'text-yellow-500' :
          variant === 'danger' ? 'text-red-500' :
          'text-muted-foreground'
        }`} />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold">{formattedValue}</div>
        {trend !== undefined && (
          <p className="text-xs text-muted-foreground flex items-center mt-1">
            {trend > 0 ? (
              <>
                <TrendingUp className="h-3 w-3 mr-1 text-green-500" />
                <span className="text-green-500">+{trend}%</span>
              </>
            ) : (
              <>
                <TrendingDown className="h-3 w-3 mr-1 text-red-500" />
                <span className="text-red-500">{trend}%</span>
              </>
            )}
            <span className="ml-1">from last month</span>
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// Activity Feed Item Component
function ActivityFeedItem({ activity }: { activity: ActivityItem }) {
  const statusIcon = {
    success: <CheckCircle className="h-4 w-4 text-green-500" />,
    pending: <Clock className="h-4 w-4 text-yellow-500" />,
    failed: <AlertCircle className="h-4 w-4 text-red-500" />,
  }
  
  return (
    <div className="flex items-start space-x-3 p-3 rounded-lg hover:bg-muted/50 transition-colors">
      <div className="mt-0.5">{statusIcon[activity.status]}</div>
      <div className="flex-1 space-y-1">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium">{activity.title}</p>
          {activity.amount && (
            <span className="text-sm font-medium">
              ${activity.amount.toLocaleString()}
            </span>
          )}
        </div>
        <p className="text-xs text-muted-foreground">{activity.description}</p>
        <p className="text-xs text-muted-foreground">
          {new Date(activity.timestamp).toLocaleTimeString()}
        </p>
      </div>
      <Badge variant={
        activity.status === 'success' ? 'default' :
        activity.status === 'pending' ? 'secondary' :
        'destructive'
      }>
        {activity.type}
      </Badge>
    </div>
  )
}

// Dashboard Skeleton
function DashboardSkeleton() {
  return (
    <div className="space-y-6 p-6">
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i}>
            <CardHeader className="space-y-0 pb-2">
              <Skeleton className="h-4 w-24" />
            </CardHeader>
            <CardContent>
              <Skeleton className="h-8 w-32" />
              <Skeleton className="h-3 w-20 mt-2" />
            </CardContent>
          </Card>
        ))}
      </div>
      
      <div className="grid gap-4 lg:grid-cols-2">
        {Array.from({ length: 2 }).map((_, i) => (
          <Card key={i}>
            <CardHeader>
              <Skeleton className="h-5 w-32" />
            </CardHeader>
            <CardContent>
              <Skeleton className="h-[300px] w-full" />
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )
}