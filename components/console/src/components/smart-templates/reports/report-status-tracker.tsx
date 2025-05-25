'use client'

import { useState, useEffect } from 'react'
import { formatDistanceToNow } from 'date-fns'
import {
  Clock,
  CheckCircle,
  AlertCircle,
  XCircle,
  RefreshCw,
  Download,
  Eye,
  Loader2
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { useToast } from '@/hooks/use-toast'

interface Report {
  id: string
  templateName: string
  status: 'queued' | 'processing' | 'completed' | 'failed' | 'expired'
  format: string
  fileName: string
  fileSize?: number
  generatedAt?: string
  startedAt?: string
  completedAt?: string
  expiresAt?: string
  downloadCount: number
  queuePosition?: number
  estimatedCompletion?: string
  error?: string
  processingTime?: string
}

interface ReportStatusTrackerProps {
  report: Report
  onRefresh?: () => void
  onDownload?: () => void
  onView?: () => void
  showDetails?: boolean
  className?: string
}

const statusConfig = {
  queued: {
    icon: Clock,
    color: 'text-gray-500',
    bgColor: 'bg-gray-100',
    borderColor: 'border-gray-200',
    label: 'Queued',
    description: 'Waiting in queue for processing'
  },
  processing: {
    icon: RefreshCw,
    color: 'text-blue-500',
    bgColor: 'bg-blue-100',
    borderColor: 'border-blue-200',
    label: 'Processing',
    description: 'Report is being generated'
  },
  completed: {
    icon: CheckCircle,
    color: 'text-green-500',
    bgColor: 'bg-green-100',
    borderColor: 'border-green-200',
    label: 'Completed',
    description: 'Report generated successfully'
  },
  failed: {
    icon: AlertCircle,
    color: 'text-red-500',
    bgColor: 'bg-red-100',
    borderColor: 'border-red-200',
    label: 'Failed',
    description: 'Report generation failed'
  },
  expired: {
    icon: XCircle,
    color: 'text-orange-500',
    bgColor: 'bg-orange-100',
    borderColor: 'border-orange-200',
    label: 'Expired',
    description: 'Report has expired and is no longer available'
  }
}

export function ReportStatusTracker({
  report,
  onRefresh,
  onDownload,
  onView,
  showDetails = true,
  className
}: ReportStatusTrackerProps) {
  const { toast } = useToast()
  const [progress, setProgress] = useState(0)
  const [timeElapsed, setTimeElapsed] = useState('')

  const statusInfo = statusConfig[report.status]
  const StatusIcon = statusInfo.icon

  // Simulate progress for processing reports
  useEffect(() => {
    if (report.status === 'processing' && report.startedAt) {
      const startTime = new Date(report.startedAt).getTime()
      const now = Date.now()
      const elapsed = now - startTime

      // Estimate progress based on time elapsed (max 90% until completed)
      const estimatedDuration = 30000 // 30 seconds estimated
      const calculatedProgress = Math.min(
        (elapsed / estimatedDuration) * 90,
        90
      )
      setProgress(calculatedProgress)

      const interval = setInterval(() => {
        const currentElapsed = Date.now() - startTime
        const newProgress = Math.min(
          (currentElapsed / estimatedDuration) * 90,
          90
        )
        setProgress(newProgress)
        setTimeElapsed(
          formatDistanceToNow(new Date(report.startedAt!), { addSuffix: false })
        )
      }, 1000)

      return () => clearInterval(interval)
    }
  }, [report.status, report.startedAt])

  const handleRefresh = () => {
    onRefresh?.()
    toast({
      title: 'Status Updated',
      description: 'Report status has been refreshed'
    })
  }

  const handleDownload = () => {
    if (report.status === 'completed') {
      onDownload?.()
      toast({
        title: 'Download Started',
        description: `Downloading ${report.fileName}`
      })
    }
  }

  const formatFileSize = (bytes?: number) => {
    if (!bytes) return 'Unknown size'
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
  }

  return (
    <Card className={`${statusInfo.borderColor} ${className}`}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex items-start gap-3">
            <div className={`rounded-full p-2 ${statusInfo.bgColor}`}>
              <StatusIcon
                className={`h-4 w-4 ${statusInfo.color} ${
                  report.status === 'processing' ? 'animate-spin' : ''
                }`}
              />
            </div>
            <div>
              <CardTitle className="text-lg">{report.fileName}</CardTitle>
              <CardDescription>Template: {report.templateName}</CardDescription>
              <div className="mt-1 flex items-center gap-2">
                <Badge
                  className={`${statusInfo.bgColor} ${statusInfo.color} border-0`}
                >
                  {statusInfo.label}
                </Badge>
                <Badge variant="outline" className="text-xs">
                  {report.format.toUpperCase()}
                </Badge>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-2">
            {report.status === 'processing' && (
              <Button variant="ghost" size="sm" onClick={handleRefresh}>
                <RefreshCw className="h-4 w-4" />
              </Button>
            )}
            {report.status === 'completed' && (
              <>
                <Button variant="ghost" size="sm" onClick={onView}>
                  <Eye className="h-4 w-4" />
                </Button>
                <Button size="sm" onClick={handleDownload}>
                  <Download className="mr-2 h-4 w-4" />
                  Download
                </Button>
              </>
            )}
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Processing Progress */}
        {report.status === 'processing' && (
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span>Processing...</span>
              <span>{Math.round(progress)}%</span>
            </div>
            <Progress value={progress} className="w-full" />
            {timeElapsed && (
              <p className="text-xs text-gray-500">
                Processing for {timeElapsed}
              </p>
            )}
          </div>
        )}

        {/* Queue Information */}
        {report.status === 'queued' && report.queuePosition && (
          <div className="space-y-2">
            <div className="flex items-center gap-2 text-sm">
              <Clock className="h-4 w-4 text-gray-400" />
              <span>Position {report.queuePosition} in queue</span>
            </div>
            {report.estimatedCompletion && (
              <p className="text-xs text-gray-500">
                Estimated completion:{' '}
                {formatDistanceToNow(new Date(report.estimatedCompletion), {
                  addSuffix: true
                })}
              </p>
            )}
          </div>
        )}

        {/* Error Information */}
        {report.status === 'failed' && report.error && (
          <div className="rounded-lg border border-red-200 bg-red-50 p-3">
            <p className="text-sm font-medium text-red-800">Error Details:</p>
            <p className="mt-1 text-sm text-red-700">{report.error}</p>
          </div>
        )}

        {/* Completion Details */}
        {report.status === 'completed' && showDetails && (
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <span className="text-gray-500">File Size:</span>
              <div className="font-medium">
                {formatFileSize(report.fileSize)}
              </div>
            </div>
            <div>
              <span className="text-gray-500">Processing Time:</span>
              <div className="font-medium">
                {report.processingTime || 'Unknown'}
              </div>
            </div>
            <div>
              <span className="text-gray-500">Generated:</span>
              <div className="font-medium">
                {report.generatedAt
                  ? formatDistanceToNow(new Date(report.generatedAt), {
                      addSuffix: true
                    })
                  : 'Unknown'}
              </div>
            </div>
            <div>
              <span className="text-gray-500">Downloads:</span>
              <div className="font-medium">{report.downloadCount}</div>
            </div>
          </div>
        )}

        {/* Expiration Warning */}
        {report.status === 'completed' && report.expiresAt && (
          <div className="rounded-lg border border-yellow-200 bg-yellow-50 p-3">
            <p className="text-sm text-yellow-800">
              <strong>Expires:</strong>{' '}
              {formatDistanceToNow(new Date(report.expiresAt), {
                addSuffix: true
              })}
            </p>
          </div>
        )}

        {/* Status Description */}
        <p className="text-xs text-gray-600">{statusInfo.description}</p>
      </CardContent>
    </Card>
  )
}
