'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import {
  Play,
  Pause,
  RotateCcw,
  Download,
  Eye,
  Trash2,
  MoreHorizontal,
  Search,
  Filter,
  RefreshCw,
  Clock,
  CheckCircle,
  XCircle,
  AlertCircle,
  FileText,
  Calendar,
  TrendingUp,
  Activity,
  Zap
} from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'

interface ReportJob {
  id: string
  name: string
  templateId: string
  templateName: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled'
  progress: number
  createdAt: string
  updatedAt: string
  createdBy: string
  duration?: number
  outputFormat: string
  fileSize?: string
  downloadUrl?: string
  errorMessage?: string
  parameters: Record<string, any>
}

const mockReportJobs: ReportJob[] = [
  {
    id: 'job-1',
    name: 'Monthly Financial Report - December 2024',
    templateId: 'tpl-1',
    templateName: 'Financial Performance Template',
    status: 'running',
    progress: 75,
    createdAt: '2024-01-15T14:30:00Z',
    updatedAt: '2024-01-15T14:35:00Z',
    createdBy: 'John Doe',
    outputFormat: 'PDF',
    parameters: { month: 'December', year: '2024' }
  },
  {
    id: 'job-2',
    name: 'Customer Analytics Report',
    templateId: 'tpl-2',
    templateName: 'Customer Insights Template',
    status: 'completed',
    progress: 100,
    createdAt: '2024-01-15T13:15:00Z',
    updatedAt: '2024-01-15T13:18:00Z',
    createdBy: 'Jane Smith',
    duration: 180,
    outputFormat: 'EXCEL',
    fileSize: '2.4 MB',
    downloadUrl: '/downloads/customer-analytics-20240115.xlsx',
    parameters: { period: 'Q4 2024' }
  },
  {
    id: 'job-3',
    name: 'Compliance Report Q4',
    templateId: 'tpl-3',
    templateName: 'Regulatory Compliance Template',
    status: 'failed',
    progress: 45,
    createdAt: '2024-01-15T12:00:00Z',
    updatedAt: '2024-01-15T12:10:00Z',
    createdBy: 'Bob Wilson',
    outputFormat: 'PDF',
    errorMessage: 'Data source connection timeout',
    parameters: { quarter: 'Q4', year: '2024' }
  },
  {
    id: 'job-4',
    name: 'Weekly Transaction Summary',
    templateId: 'tpl-4',
    templateName: 'Transaction Summary Template',
    status: 'queued',
    progress: 0,
    createdAt: '2024-01-15T14:45:00Z',
    updatedAt: '2024-01-15T14:45:00Z',
    createdBy: 'Alice Johnson',
    outputFormat: 'HTML',
    parameters: { week: '2024-W03' }
  },
  {
    id: 'job-5',
    name: 'Annual Performance Report',
    templateId: 'tpl-1',
    templateName: 'Financial Performance Template',
    status: 'completed',
    progress: 100,
    createdAt: '2024-01-14T16:20:00Z',
    updatedAt: '2024-01-14T16:25:00Z',
    createdBy: 'John Doe',
    duration: 300,
    outputFormat: 'PDF',
    fileSize: '5.7 MB',
    downloadUrl: '/downloads/annual-performance-2024.pdf',
    parameters: { year: '2024' }
  }
]

export function ReportMonitoringDashboard() {
  const [jobs, setJobs] = useState<ReportJob[]>(mockReportJobs)
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [refreshing, setRefreshing] = useState(false)

  const statusColors = {
    queued: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200',
    running:
      'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200',
    completed:
      'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
    failed: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200',
    cancelled: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
  }

  const statusIcons = {
    queued: <Clock className="h-4 w-4" />,
    running: <Play className="h-4 w-4" />,
    completed: <CheckCircle className="h-4 w-4" />,
    failed: <XCircle className="h-4 w-4" />,
    cancelled: <Pause className="h-4 w-4" />
  }

  const filteredJobs = jobs.filter((job) => {
    const matchesSearch =
      job.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      job.templateName.toLowerCase().includes(searchQuery.toLowerCase()) ||
      job.createdBy.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesStatus = statusFilter === 'all' || job.status === statusFilter

    return matchesSearch && matchesStatus
  })

  const handleRefresh = async () => {
    setRefreshing(true)
    // Simulate API call
    setTimeout(() => {
      setRefreshing(false)
    }, 1000)
  }

  const handleRetry = (jobId: string) => {
    setJobs(
      jobs.map((job) =>
        job.id === jobId
          ? {
              ...job,
              status: 'queued' as const,
              progress: 0,
              errorMessage: undefined
            }
          : job
      )
    )
  }

  const handleCancel = (jobId: string) => {
    setJobs(
      jobs.map((job) =>
        job.id === jobId ? { ...job, status: 'cancelled' as const } : job
      )
    )
  }

  const handleDelete = (jobId: string) => {
    setJobs(jobs.filter((job) => job.id !== jobId))
  }

  const getStatusCounts = () => {
    return {
      total: jobs.length,
      running: jobs.filter((j) => j.status === 'running').length,
      completed: jobs.filter((j) => j.status === 'completed').length,
      failed: jobs.filter((j) => j.status === 'failed').length,
      queued: jobs.filter((j) => j.status === 'queued').length
    }
  }

  const statusCounts = getStatusCounts()

  const formatDuration = (seconds: number) => {
    const minutes = Math.floor(seconds / 60)
    const remainingSeconds = seconds % 60
    return `${minutes}m ${remainingSeconds}s`
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Report Generation</h1>
          <p className="text-muted-foreground">
            Monitor and manage report generation jobs
          </p>
        </div>
        <div className="flex items-center space-x-2">
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
        </div>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-5">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <FileText className="h-5 w-5 text-blue-500" />
              <div>
                <p className="text-sm text-muted-foreground">Total Jobs</p>
                <p className="text-2xl font-bold">{statusCounts.total}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <Activity className="h-5 w-5 text-orange-500" />
              <div>
                <p className="text-sm text-muted-foreground">Running</p>
                <p className="text-2xl font-bold">{statusCounts.running}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <CheckCircle className="h-5 w-5 text-green-500" />
              <div>
                <p className="text-sm text-muted-foreground">Completed</p>
                <p className="text-2xl font-bold">{statusCounts.completed}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <XCircle className="h-5 w-5 text-red-500" />
              <div>
                <p className="text-sm text-muted-foreground">Failed</p>
                <p className="text-2xl font-bold">{statusCounts.failed}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <Clock className="h-5 w-5 text-blue-500" />
              <div>
                <p className="text-sm text-muted-foreground">Queued</p>
                <p className="text-2xl font-bold">{statusCounts.queued}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Filters */}
      <div className="flex flex-col gap-4 sm:flex-row">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
          <Input
            placeholder="Search jobs..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10"
          />
        </div>

        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-[140px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Status</SelectItem>
            <SelectItem value="queued">Queued</SelectItem>
            <SelectItem value="running">Running</SelectItem>
            <SelectItem value="completed">Completed</SelectItem>
            <SelectItem value="failed">Failed</SelectItem>
            <SelectItem value="cancelled">Cancelled</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Jobs List */}
      <Card>
        <CardHeader>
          <CardTitle>Generation Jobs</CardTitle>
          <CardDescription>
            Real-time status of report generation jobs
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <ScrollArea className="h-[600px]">
            <div className="space-y-1 p-4">
              {filteredJobs.length === 0 ? (
                <div className="py-8 text-center text-muted-foreground">
                  <FileText className="mx-auto mb-2 h-8 w-8 opacity-50" />
                  <p>No jobs found matching your criteria</p>
                </div>
              ) : (
                filteredJobs.map((job) => (
                  <Card key={job.id} className="p-4">
                    <div className="flex items-start justify-between">
                      <div className="flex-1 space-y-2">
                        <div className="flex items-center space-x-3">
                          <h4 className="font-medium">{job.name}</h4>
                          <Badge
                            className={statusColors[job.status]}
                            variant="secondary"
                          >
                            <div className="flex items-center space-x-1">
                              {statusIcons[job.status]}
                              <span>{job.status.toUpperCase()}</span>
                            </div>
                          </Badge>
                          <Badge variant="outline">{job.outputFormat}</Badge>
                        </div>

                        <div className="text-sm text-muted-foreground">
                          <p>Template: {job.templateName}</p>
                          <p>
                            Created by {job.createdBy} •{' '}
                            {formatDistanceToNow(new Date(job.createdAt), {
                              addSuffix: true
                            })}
                          </p>
                        </div>

                        {job.status === 'running' && (
                          <div className="space-y-1">
                            <div className="flex justify-between text-sm">
                              <span>Progress</span>
                              <span>{job.progress}%</span>
                            </div>
                            <Progress value={job.progress} className="h-2" />
                          </div>
                        )}

                        {job.status === 'completed' && (
                          <div className="flex items-center space-x-4 text-sm text-muted-foreground">
                            <span>
                              Duration:{' '}
                              {job.duration
                                ? formatDuration(job.duration)
                                : 'N/A'}
                            </span>
                            <span>Size: {job.fileSize || 'N/A'}</span>
                          </div>
                        )}

                        {job.status === 'failed' && job.errorMessage && (
                          <div className="flex items-center space-x-2 text-sm text-red-600">
                            <AlertCircle className="h-4 w-4" />
                            <span>{job.errorMessage}</span>
                          </div>
                        )}
                      </div>

                      <div className="flex items-center space-x-2">
                        {job.status === 'completed' && job.downloadUrl && (
                          <Button variant="outline" size="sm">
                            <Download className="mr-2 h-4 w-4" />
                            Download
                          </Button>
                        )}

                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-8 w-8 p-0"
                            >
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            {job.status === 'completed' && (
                              <>
                                <DropdownMenuItem>
                                  <Eye className="mr-2 h-4 w-4" />
                                  View Details
                                </DropdownMenuItem>
                                <DropdownMenuItem>
                                  <Download className="mr-2 h-4 w-4" />
                                  Download
                                </DropdownMenuItem>
                                <DropdownMenuSeparator />
                              </>
                            )}
                            {job.status === 'failed' && (
                              <>
                                <DropdownMenuItem
                                  onClick={() => handleRetry(job.id)}
                                >
                                  <RotateCcw className="mr-2 h-4 w-4" />
                                  Retry
                                </DropdownMenuItem>
                                <DropdownMenuSeparator />
                              </>
                            )}
                            {(job.status === 'queued' ||
                              job.status === 'running') && (
                              <>
                                <DropdownMenuItem
                                  onClick={() => handleCancel(job.id)}
                                >
                                  <Pause className="mr-2 h-4 w-4" />
                                  Cancel
                                </DropdownMenuItem>
                                <DropdownMenuSeparator />
                              </>
                            )}
                            <DropdownMenuItem
                              onClick={() => handleDelete(job.id)}
                              className="text-red-600"
                            >
                              <Trash2 className="mr-2 h-4 w-4" />
                              Delete
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </div>
                  </Card>
                ))
              )}
            </div>
          </ScrollArea>
        </CardContent>
      </Card>
    </div>
  )
}
