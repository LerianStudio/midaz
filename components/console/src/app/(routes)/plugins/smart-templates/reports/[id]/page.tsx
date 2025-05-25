'use client'

import { useState, useEffect } from 'react'
import { useParams, useRouter } from 'next/navigation'
import {
  ArrowLeft,
  Download,
  Eye,
  RefreshCw,
  FileText,
  Clock,
  CheckCircle,
  AlertCircle,
  X,
  Calendar,
  User,
  Settings,
  Share2
} from 'lucide-react'
import Link from 'next/link'
import { format, formatDistanceToNow } from 'date-fns'

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
import { Progress } from '@/components/ui/progress'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/hooks/use-toast'

const mockReport = {
  id: '01956b69-9102-75b7-8860-3e75c11d231e',
  templateId: '01956b69-9102-75b7-8860-3e75c11d231c',
  templateName: 'Monthly Account Statement',
  status: 'completed',
  format: 'pdf',
  fileName: 'monthly-statement-dec-2024.pdf',
  fileSize: 245760,
  processingTime: '2.5s',
  parameters: {
    account_id: '01956b69-9102-75b7-8860-3e75c11d231f',
    account_alias: 'john-checking-001',
    month: '2024-12',
    include_metadata: true,
    locale: 'en-US',
    timezone: 'America/New_York'
  },
  generatedBy: 'john.doe@company.com',
  generatedAt: '2025-01-01T10:15:00Z',
  downloadCount: 3,
  lastDownloaded: '2025-01-01T14:30:00Z',
  expiresAt: '2025-02-01T00:00:00Z',
  tags: ['monthly', 'statement', 'december'],
  logs: [
    {
      timestamp: '2025-01-01T10:15:00Z',
      level: 'info',
      message: 'Report generation started',
      details: 'Template loaded successfully'
    },
    {
      timestamp: '2025-01-01T10:15:01Z',
      level: 'info',
      message: 'Data source connected',
      details: 'Connected to midaz_onboarding database'
    },
    {
      timestamp: '2025-01-01T10:15:02Z',
      level: 'info',
      message: 'Template rendered',
      details: 'Template processed with 45 transactions'
    },
    {
      timestamp: '2025-01-01T10:15:03Z',
      level: 'info',
      message: 'PDF generation completed',
      details: 'Output file size: 240 KB'
    },
    {
      timestamp: '2025-01-01T10:15:03Z',
      level: 'success',
      message: 'Report generation completed',
      details: 'Total processing time: 2.5 seconds'
    }
  ]
}

const statusConfig = {
  queued: {
    icon: Clock,
    color: 'text-gray-500',
    bgColor: 'bg-gray-100',
    label: 'Queued'
  },
  processing: {
    icon: RefreshCw,
    color: 'text-blue-500',
    bgColor: 'bg-blue-100',
    label: 'Processing'
  },
  completed: {
    icon: CheckCircle,
    color: 'text-green-500',
    bgColor: 'bg-green-100',
    label: 'Completed'
  },
  failed: {
    icon: AlertCircle,
    color: 'text-red-500',
    bgColor: 'bg-red-100',
    label: 'Failed'
  },
  expired: {
    icon: X,
    color: 'text-orange-500',
    bgColor: 'bg-orange-100',
    label: 'Expired'
  }
}

export default function ReportDetailsPage() {
  const params = useParams()
  const router = useRouter()
  const { toast } = useToast()
  const reportId = params.id as string

  const [isLoading, setIsLoading] = useState(true)
  const [downloadProgress, setDownloadProgress] = useState(0)
  const [isDownloading, setIsDownloading] = useState(false)

  useEffect(() => {
    // Simulate loading
    const timer = setTimeout(() => setIsLoading(false), 1000)
    return () => clearTimeout(timer)
  }, [])

  const handleDownload = async () => {
    setIsDownloading(true)
    setDownloadProgress(0)

    // Simulate download progress
    const interval = setInterval(() => {
      setDownloadProgress((prev) => {
        if (prev >= 100) {
          clearInterval(interval)
          setIsDownloading(false)
          toast({
            title: 'Download Complete',
            description: `${mockReport.fileName} has been downloaded successfully.`
          })
          return 100
        }
        return prev + 10
      })
    }, 200)
  }

  const handleRegenerateReport = () => {
    toast({
      title: 'Report Regeneration Started',
      description: 'A new version of this report is being generated.'
    })
  }

  const handleShareReport = () => {
    navigator.clipboard.writeText(window.location.href)
    toast({
      title: 'Link Copied',
      description: 'Report link has been copied to clipboard.'
    })
  }

  const statusInfo =
    statusConfig[mockReport.status as keyof typeof statusConfig]
  const StatusIcon = statusInfo.icon

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
        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
          {[...Array(3)].map((_, i) => (
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
            <Link href="/plugins/smart-templates/reports">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <PageHeader.InfoTitle
              title="Report Details"
              subtitle={mockReport.fileName}
            />
            <div className="mt-2 flex items-center gap-2">
              <Badge variant="secondary">
                {mockReport.format.toUpperCase()}
              </Badge>
              <Badge
                variant={
                  mockReport.status === 'completed' ? 'default' : 'secondary'
                }
                className={`${statusInfo.color} ${statusInfo.bgColor} border-0`}
              >
                <StatusIcon className="mr-1 h-3 w-3" />
                {statusInfo.label}
              </Badge>
            </div>
          </div>
        </div>
        <PageHeader.InfoTooltip subtitle="View report details, download files, and monitor generation status" />
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={handleShareReport} size="sm">
            <Share2 className="mr-2 h-4 w-4" />
            Share
          </Button>
          <Button variant="outline" onClick={handleRegenerateReport} size="sm">
            <RefreshCw className="mr-2 h-4 w-4" />
            Regenerate
          </Button>
          <Button
            onClick={handleDownload}
            disabled={isDownloading || mockReport.status !== 'completed'}
          >
            {isDownloading ? (
              <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Download className="mr-2 h-4 w-4" />
            )}
            Download
          </Button>
        </div>
      </PageHeader.Root>

      {/* Download Progress */}
      {isDownloading && (
        <Card>
          <CardContent className="pt-6">
            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>Downloading {mockReport.fileName}</span>
                <span>{downloadProgress}%</span>
              </div>
              <Progress value={downloadProgress} className="w-full" />
            </div>
          </CardContent>
        </Card>
      )}

      {/* Key Information */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">File Size</CardTitle>
            <FileText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {(mockReport.fileSize / 1024).toFixed(0)} KB
            </div>
            <p className="text-xs text-muted-foreground">
              Generated{' '}
              {formatDistanceToNow(new Date(mockReport.generatedAt), {
                addSuffix: true
              })}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Processing Time
            </CardTitle>
            <Clock className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mockReport.processingTime}
            </div>
            <p className="text-xs text-muted-foreground">
              Template: {mockReport.templateName}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Downloads</CardTitle>
            <Download className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{mockReport.downloadCount}</div>
            <p className="text-xs text-muted-foreground">
              Last:{' '}
              {formatDistanceToNow(new Date(mockReport.lastDownloaded), {
                addSuffix: true
              })}
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Report Details Tabs */}
      <Tabs defaultValue="overview" className="w-full">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="parameters">Parameters</TabsTrigger>
          <TabsTrigger value="logs">Generation Logs</TabsTrigger>
          <TabsTrigger value="preview">Preview</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <FileText className="h-5 w-5" />
                  Report Information
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div>
                  <label className="text-sm font-medium text-gray-600">
                    Template
                  </label>
                  <div className="mt-1">
                    <Link
                      href={`/plugins/smart-templates/templates/${mockReport.templateId}`}
                      className="text-blue-600 hover:underline"
                    >
                      {mockReport.templateName}
                    </Link>
                  </div>
                </div>

                <div>
                  <label className="text-sm font-medium text-gray-600">
                    File Name
                  </label>
                  <p className="mt-1 font-mono text-sm">
                    {mockReport.fileName}
                  </p>
                </div>

                <div>
                  <label className="text-sm font-medium text-gray-600">
                    Format
                  </label>
                  <div className="mt-1">
                    <Badge variant="outline">
                      {mockReport.format.toUpperCase()}
                    </Badge>
                  </div>
                </div>

                <div>
                  <label className="text-sm font-medium text-gray-600">
                    File Size
                  </label>
                  <p className="mt-1 text-sm">
                    {(mockReport.fileSize / 1024).toFixed(2)} KB
                  </p>
                </div>

                <div>
                  <label className="text-sm font-medium text-gray-600">
                    Tags
                  </label>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {mockReport.tags.map((tag) => (
                      <Badge key={tag} variant="secondary" className="text-xs">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Clock className="h-5 w-5" />
                  Timeline
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div>
                  <label className="text-sm font-medium text-gray-600">
                    Generated By
                  </label>
                  <div className="mt-1 flex items-center gap-2">
                    <User className="h-4 w-4 text-gray-400" />
                    <span className="text-sm">{mockReport.generatedBy}</span>
                  </div>
                </div>

                <div>
                  <label className="text-sm font-medium text-gray-600">
                    Generated At
                  </label>
                  <div className="mt-1 flex items-center gap-2">
                    <Calendar className="h-4 w-4 text-gray-400" />
                    <span className="text-sm">
                      {format(new Date(mockReport.generatedAt), "PPP 'at' pp")}
                    </span>
                  </div>
                </div>

                <div>
                  <label className="text-sm font-medium text-gray-600">
                    Processing Time
                  </label>
                  <p className="mt-1 text-sm">{mockReport.processingTime}</p>
                </div>

                <div>
                  <label className="text-sm font-medium text-gray-600">
                    Expires At
                  </label>
                  <div className="mt-1">
                    <span className="text-sm">
                      {format(new Date(mockReport.expiresAt), 'PPP')}
                    </span>
                    <span className="ml-2 text-xs text-gray-500">
                      (
                      {formatDistanceToNow(new Date(mockReport.expiresAt), {
                        addSuffix: true
                      })}
                      )
                    </span>
                  </div>
                </div>

                <div>
                  <label className="text-sm font-medium text-gray-600">
                    Downloads
                  </label>
                  <p className="mt-1 text-sm">
                    {mockReport.downloadCount} times
                    {mockReport.lastDownloaded && (
                      <span className="ml-2 text-gray-500">
                        (last:{' '}
                        {formatDistanceToNow(
                          new Date(mockReport.lastDownloaded),
                          { addSuffix: true }
                        )}
                        )
                      </span>
                    )}
                  </p>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="parameters" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Settings className="h-5 w-5" />
                Generation Parameters
              </CardTitle>
              <CardDescription>
                Parameters used to generate this report
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {Object.entries(mockReport.parameters).map(([key, value]) => (
                  <div
                    key={key}
                    className="flex items-start justify-between border-b pb-2"
                  >
                    <div className="text-sm font-medium">
                      {key.replace(/_/g, ' ')}
                    </div>
                    <div className="max-w-xs text-right font-mono text-sm text-gray-600">
                      {typeof value === 'boolean'
                        ? value
                          ? 'true'
                          : 'false'
                        : String(value)}
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="logs" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileText className="h-5 w-5" />
                Generation Logs
              </CardTitle>
              <CardDescription>
                Detailed logs from the report generation process
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                {mockReport.logs.map((log, index) => (
                  <div
                    key={index}
                    className="flex items-start gap-3 rounded-lg border p-3"
                  >
                    <div
                      className={`mt-2 h-2 w-2 rounded-full ${
                        log.level === 'success'
                          ? 'bg-green-500'
                          : log.level === 'error'
                            ? 'bg-red-500'
                            : log.level === 'warning'
                              ? 'bg-yellow-500'
                              : 'bg-blue-500'
                      }`}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between">
                        <p className="text-sm font-medium">{log.message}</p>
                        <span className="text-xs text-gray-500">
                          {format(new Date(log.timestamp), 'HH:mm:ss')}
                        </span>
                      </div>
                      {log.details && (
                        <p className="mt-1 text-xs text-gray-600">
                          {log.details}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="preview" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Eye className="h-5 w-5" />
                Report Preview
              </CardTitle>
              <CardDescription>
                Preview of the generated report (first page)
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="rounded-lg border bg-gray-50 p-8 text-center">
                <FileText className="mx-auto mb-4 h-16 w-16 text-gray-400" />
                <p className="mb-2 text-gray-600">
                  {mockReport.format.toUpperCase()} Preview
                </p>
                <p className="mb-4 text-sm text-gray-500">
                  {mockReport.fileName}
                </p>
                <div className="space-y-2">
                  <Button variant="outline" size="sm">
                    <Eye className="mr-2 h-4 w-4" />
                    Open in New Tab
                  </Button>
                  <p className="text-xs text-gray-500">
                    Full preview available after download
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
