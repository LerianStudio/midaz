'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  Plus,
  Search,
  Filter,
  Download,
  Eye,
  Clock,
  CheckCircle,
  AlertCircle,
  FileText,
  Upload
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'

export default function ImportsPage() {
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')

  // Mock data - will be replaced with real API calls
  const imports = [
    {
      id: '1',
      fileName: 'bank_transactions_december_2024.csv',
      fileSize: 1048576,
      status: 'completed',
      totalRecords: 2500,
      processedRecords: 2500,
      failedRecords: 0,
      startedAt: '2024-12-01T10:00:00Z',
      completedAt: '2024-12-01T10:05:30Z',
      createdAt: '2024-12-01T09:59:45Z'
    },
    {
      id: '2',
      fileName: 'payment_processor_data.json',
      fileSize: 2097152,
      status: 'processing',
      totalRecords: 5000,
      processedRecords: 3200,
      failedRecords: 12,
      startedAt: '2024-12-01T11:30:00Z',
      completedAt: null,
      createdAt: '2024-12-01T11:29:30Z'
    },
    {
      id: '3',
      fileName: 'credit_card_settlements_q4.xlsx',
      fileSize: 524288,
      status: 'failed',
      totalRecords: 1200,
      processedRecords: 450,
      failedRecords: 750,
      startedAt: '2024-12-01T12:00:00Z',
      completedAt: null,
      createdAt: '2024-12-01T11:58:22Z'
    },
    {
      id: '4',
      fileName: 'ach_returns_november.csv',
      fileSize: 786432,
      status: 'pending',
      totalRecords: 850,
      processedRecords: 0,
      failedRecords: 0,
      startedAt: null,
      completedAt: null,
      createdAt: '2024-12-01T13:15:10Z'
    }
  ]

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'completed':
        return <CheckCircle className="h-4 w-4 text-green-600" />
      case 'processing':
        return <Clock className="h-4 w-4 animate-spin text-blue-600" />
      case 'failed':
        return <AlertCircle className="h-4 w-4 text-red-600" />
      case 'pending':
        return <Clock className="h-4 w-4 text-gray-600" />
      default:
        return <FileText className="h-4 w-4 text-gray-600" />
    }
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'completed':
        return (
          <Badge className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400">
            Completed
          </Badge>
        )
      case 'processing':
        return (
          <Badge className="bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400">
            Processing
          </Badge>
        )
      case 'failed':
        return <Badge variant="destructive">Failed</Badge>
      case 'pending':
        return <Badge variant="outline">Pending</Badge>
      default:
        return <Badge variant="outline">{status}</Badge>
    }
  }

  const formatFileSize = (bytes: number) => {
    const sizes = ['Bytes', 'KB', 'MB', 'GB']
    if (bytes === 0) return '0 Bytes'
    const i = Math.floor(Math.log(bytes) / Math.log(1024))
    return Math.round((bytes / Math.pow(1024, i)) * 100) / 100 + ' ' + sizes[i]
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    })
  }

  const calculateProgress = (processed: number, total: number) => {
    return total > 0 ? Math.round((processed / total) * 100) : 0
  }

  const filteredImports = imports.filter((importItem) => {
    const matchesSearch = importItem.fileName
      .toLowerCase()
      .includes(searchQuery.toLowerCase())
    const matchesStatus =
      statusFilter === 'all' || importItem.status === statusFilter
    return matchesSearch && matchesStatus
  })

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">
            Transaction Imports
          </h2>
          <p className="text-muted-foreground">
            Upload and process transaction files for reconciliation
          </p>
        </div>
        <Link href="/plugins/reconciliation/imports/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            Import File
          </Button>
        </Link>
      </div>

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Imports</CardTitle>
            <Upload className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{imports.length}</div>
            <p className="text-xs text-muted-foreground">
              {imports.filter((i) => i.status === 'completed').length} completed
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Processing</CardTitle>
            <Clock className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-600">
              {imports.filter((i) => i.status === 'processing').length}
            </div>
            <p className="text-xs text-muted-foreground">Active imports</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Failed</CardTitle>
            <AlertCircle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-red-600">
              {imports.filter((i) => i.status === 'failed').length}
            </div>
            <p className="text-xs text-muted-foreground">Require attention</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Records Processed
            </CardTitle>
            <FileText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {imports
                .reduce((total, imp) => total + imp.processedRecords, 0)
                .toLocaleString()}
            </div>
            <p className="text-xs text-muted-foreground">Total transactions</p>
          </CardContent>
        </Card>
      </div>

      {/* Filters and Search */}
      <Card>
        <CardHeader>
          <CardTitle>Import History</CardTitle>
          <CardDescription>
            View and manage your transaction file imports
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div className="flex flex-1 gap-2">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search imports..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-10"
                />
              </div>
              <Select value={statusFilter} onValueChange={setStatusFilter}>
                <SelectTrigger className="w-40">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Status</SelectItem>
                  <SelectItem value="completed">Completed</SelectItem>
                  <SelectItem value="processing">Processing</SelectItem>
                  <SelectItem value="failed">Failed</SelectItem>
                  <SelectItem value="pending">Pending</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" className="gap-2">
                <Filter className="h-4 w-4" />
                Filters
              </Button>
              <Button variant="outline" size="sm" className="gap-2">
                <Download className="h-4 w-4" />
                Export
              </Button>
            </div>
          </div>

          {/* Import List */}
          <div className="space-y-4">
            {filteredImports.map((importItem) => (
              <div
                key={importItem.id}
                className="flex items-center gap-4 rounded-lg border p-4 transition-colors hover:bg-muted/50"
              >
                <div className="flex-shrink-0">
                  {getStatusIcon(importItem.status)}
                </div>

                <div className="min-w-0 flex-1 space-y-2">
                  <div className="flex items-center gap-3">
                    <h4 className="truncate font-medium">
                      {importItem.fileName}
                    </h4>
                    {getStatusBadge(importItem.status)}
                  </div>

                  <div className="grid grid-cols-2 gap-4 text-sm text-muted-foreground md:grid-cols-4">
                    <div>
                      <span className="font-medium">Size:</span>{' '}
                      {formatFileSize(importItem.fileSize)}
                    </div>
                    <div>
                      <span className="font-medium">Records:</span>{' '}
                      {importItem.totalRecords.toLocaleString()}
                    </div>
                    <div>
                      <span className="font-medium">Processed:</span>{' '}
                      {importItem.processedRecords.toLocaleString()}
                    </div>
                    <div>
                      <span className="font-medium">Created:</span>{' '}
                      {formatDate(importItem.createdAt)}
                    </div>
                  </div>

                  {importItem.status === 'processing' && (
                    <div className="space-y-2">
                      <div className="flex justify-between text-sm">
                        <span>Progress</span>
                        <span>
                          {calculateProgress(
                            importItem.processedRecords,
                            importItem.totalRecords
                          )}
                          %
                        </span>
                      </div>
                      <Progress
                        value={calculateProgress(
                          importItem.processedRecords,
                          importItem.totalRecords
                        )}
                      />
                    </div>
                  )}

                  {importItem.failedRecords > 0 && (
                    <div className="text-sm text-red-600">
                      <span className="font-medium">Failed records:</span>{' '}
                      {importItem.failedRecords}
                    </div>
                  )}
                </div>

                <div className="flex gap-2">
                  <Link
                    href={`/plugins/reconciliation/imports/${importItem.id}`}
                  >
                    <Button variant="outline" size="sm">
                      <Eye className="h-4 w-4" />
                    </Button>
                  </Link>
                  {importItem.status === 'processing' && (
                    <Link
                      href={`/plugins/reconciliation/imports/${importItem.id}/progress`}
                    >
                      <Button variant="outline" size="sm">
                        <Clock className="h-4 w-4" />
                      </Button>
                    </Link>
                  )}
                </div>
              </div>
            ))}
          </div>

          {filteredImports.length === 0 && (
            <div className="py-8 text-center">
              <FileText className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
              <h3 className="mb-2 text-lg font-medium">No imports found</h3>
              <p className="mb-4 text-muted-foreground">
                {searchQuery || statusFilter !== 'all'
                  ? 'Try adjusting your search or filters'
                  : 'Start by importing your first transaction file'}
              </p>
              {!searchQuery && statusFilter === 'all' && (
                <Link href="/plugins/reconciliation/imports/create">
                  <Button className="gap-2">
                    <Plus className="h-4 w-4" />
                    Import File
                  </Button>
                </Link>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
