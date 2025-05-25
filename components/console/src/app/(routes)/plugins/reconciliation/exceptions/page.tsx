'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  Plus,
  Search,
  Filter,
  AlertTriangle,
  Clock,
  User,
  CheckCircle,
  XCircle,
  Eye,
  ArrowUpCircle,
  FileText
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'

export default function ExceptionsPage() {
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [priorityFilter, setPriorityFilter] = useState('all')
  const [selectedExceptions, setSelectedExceptions] = useState<string[]>([])

  // Mock data - will be replaced with real API calls
  const exceptions = [
    {
      id: '1',
      processId: 'proc-1',
      externalTransactionId: 'ext-txn-1',
      reason: 'no_match_found',
      category: 'unmatched',
      priority: 'critical',
      status: 'pending',
      assignedTo: 'analyst@company.com',
      amount: 2547.82,
      description: 'Wire transfer with no matching internal transaction',
      age: '3 hours',
      suggestedActions: ['manual_match', 'investigate'],
      escalationLevel: 1,
      createdAt: '2024-12-01T10:30:00Z'
    },
    {
      id: '2',
      processId: 'proc-1',
      externalTransactionId: 'ext-txn-2',
      reason: 'multiple_matches',
      category: 'ambiguous',
      priority: 'high',
      status: 'assigned',
      assignedTo: 'senior.analyst@company.com',
      amount: 1250.0,
      description: 'Payment with multiple potential matches',
      age: '6 hours',
      suggestedActions: ['manual_match'],
      escalationLevel: 0,
      createdAt: '2024-12-01T07:30:00Z'
    },
    {
      id: '3',
      processId: 'proc-2',
      externalTransactionId: 'ext-txn-3',
      reason: 'amount_mismatch',
      category: 'discrepancy',
      priority: 'medium',
      status: 'investigating',
      assignedTo: 'analyst2@company.com',
      amount: 875.5,
      description: 'Transaction amounts do not match within tolerance',
      age: '1 day',
      suggestedActions: ['create_adjustment', 'investigate'],
      escalationLevel: 0,
      createdAt: '2024-11-30T14:15:00Z'
    },
    {
      id: '4',
      processId: 'proc-2',
      externalTransactionId: 'ext-txn-4',
      reason: 'low_confidence',
      category: 'validation',
      priority: 'medium',
      status: 'under_review',
      assignedTo: 'analyst3@company.com',
      amount: 432.18,
      description: 'AI matching confidence below threshold',
      age: '2 days',
      suggestedActions: ['manual_match', 'escalate'],
      escalationLevel: 0,
      createdAt: '2024-11-29T16:45:00Z'
    },
    {
      id: '5',
      processId: 'proc-3',
      externalTransactionId: 'ext-txn-5',
      reason: 'date_mismatch',
      category: 'discrepancy',
      priority: 'low',
      status: 'resolved',
      assignedTo: 'analyst@company.com',
      resolvedBy: 'analyst@company.com',
      amount: 156.75,
      description: 'Transaction dates outside acceptable window',
      age: '3 days',
      suggestedActions: [],
      escalationLevel: 0,
      createdAt: '2024-11-28T11:20:00Z'
    }
  ]

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'resolved':
        return <CheckCircle className="h-4 w-4 text-green-600" />
      case 'investigating':
        return <Clock className="h-4 w-4 text-blue-600" />
      case 'assigned':
        return <User className="h-4 w-4 text-purple-600" />
      case 'escalated':
        return <ArrowUpCircle className="h-4 w-4 text-red-600" />
      case 'pending':
        return <AlertTriangle className="h-4 w-4 text-yellow-600" />
      default:
        return <FileText className="h-4 w-4 text-gray-600" />
    }
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'resolved':
        return (
          <Badge className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400">
            Resolved
          </Badge>
        )
      case 'investigating':
        return (
          <Badge className="bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400">
            Investigating
          </Badge>
        )
      case 'assigned':
        return (
          <Badge className="bg-purple-100 text-purple-800 dark:bg-purple-900/20 dark:text-purple-400">
            Assigned
          </Badge>
        )
      case 'escalated':
        return <Badge variant="destructive">Escalated</Badge>
      case 'pending':
        return (
          <Badge className="bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400">
            Pending
          </Badge>
        )
      default:
        return <Badge variant="outline">{status}</Badge>
    }
  }

  const getPriorityBadge = (priority: string) => {
    switch (priority) {
      case 'critical':
        return <Badge variant="destructive">Critical</Badge>
      case 'high':
        return (
          <Badge className="bg-orange-100 text-orange-800 dark:bg-orange-900/20 dark:text-orange-400">
            High
          </Badge>
        )
      case 'medium':
        return (
          <Badge className="bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400">
            Medium
          </Badge>
        )
      case 'low':
        return <Badge variant="outline">Low</Badge>
      default:
        return <Badge variant="outline">{priority}</Badge>
    }
  }

  const getReasonLabel = (reason: string) => {
    const reasonMap: Record<string, string> = {
      no_match_found: 'No Match Found',
      multiple_matches: 'Multiple Matches',
      amount_mismatch: 'Amount Mismatch',
      date_mismatch: 'Date Mismatch',
      low_confidence: 'Low Confidence',
      duplicate_transaction: 'Duplicate Transaction',
      validation_failed: 'Validation Failed'
    }
    return reasonMap[reason] || reason
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

  const handleSelectException = (exceptionId: string) => {
    setSelectedExceptions((prev) =>
      prev.includes(exceptionId)
        ? prev.filter((id) => id !== exceptionId)
        : [...prev, exceptionId]
    )
  }

  const handleSelectAll = () => {
    setSelectedExceptions(
      selectedExceptions.length === filteredExceptions.length
        ? []
        : filteredExceptions.map((exc) => exc.id)
    )
  }

  const filteredExceptions = exceptions.filter((exception) => {
    const matchesSearch =
      exception.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
      exception.reason.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesStatus =
      statusFilter === 'all' || exception.status === statusFilter
    const matchesPriority =
      priorityFilter === 'all' || exception.priority === priorityFilter
    return matchesSearch && matchesStatus && matchesPriority
  })

  const pendingCount = exceptions.filter((e) => e.status === 'pending').length
  const criticalCount = exceptions.filter(
    (e) => e.priority === 'critical'
  ).length
  const assignedCount = exceptions.filter(
    (e) => e.status === 'assigned' || e.status === 'investigating'
  ).length
  const resolvedCount = exceptions.filter((e) => e.status === 'resolved').length

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">
            Exception Management
          </h2>
          <p className="text-muted-foreground">
            Review and resolve reconciliation exceptions
          </p>
        </div>
        <div className="flex gap-2">
          {selectedExceptions.length > 0 && (
            <div className="flex gap-2">
              <Button variant="outline" size="sm">
                Bulk Assign ({selectedExceptions.length})
              </Button>
              <Button variant="outline" size="sm">
                Bulk Escalate
              </Button>
            </div>
          )}
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Pending</CardTitle>
            <AlertTriangle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-yellow-600">
              {pendingCount}
            </div>
            <p className="text-xs text-muted-foreground">
              Unassigned exceptions
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Critical</CardTitle>
            <XCircle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-red-600">
              {criticalCount}
            </div>
            <p className="text-xs text-muted-foreground">
              High priority issues
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">In Progress</CardTitle>
            <Clock className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-600">
              {assignedCount}
            </div>
            <p className="text-xs text-muted-foreground">Being investigated</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Resolved</CardTitle>
            <CheckCircle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">
              {resolvedCount}
            </div>
            <p className="text-xs text-muted-foreground">Completed today</p>
          </CardContent>
        </Card>
      </div>

      {/* Exception List */}
      <Card>
        <CardHeader>
          <CardTitle>Exception Queue</CardTitle>
          <CardDescription>
            Review and manage reconciliation exceptions
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div className="flex flex-1 gap-2">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search exceptions..."
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
                  <SelectItem value="pending">Pending</SelectItem>
                  <SelectItem value="assigned">Assigned</SelectItem>
                  <SelectItem value="investigating">Investigating</SelectItem>
                  <SelectItem value="resolved">Resolved</SelectItem>
                  <SelectItem value="escalated">Escalated</SelectItem>
                </SelectContent>
              </Select>
              <Select value={priorityFilter} onValueChange={setPriorityFilter}>
                <SelectTrigger className="w-40">
                  <SelectValue placeholder="Priority" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Priority</SelectItem>
                  <SelectItem value="critical">Critical</SelectItem>
                  <SelectItem value="high">High</SelectItem>
                  <SelectItem value="medium">Medium</SelectItem>
                  <SelectItem value="low">Low</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button variant="outline" size="sm" className="gap-2">
              <Filter className="h-4 w-4" />
              Advanced Filters
            </Button>
          </div>

          {/* Bulk Selection Header */}
          {filteredExceptions.length > 0 && (
            <div className="flex items-center gap-4 rounded-lg bg-muted p-3">
              <Checkbox
                checked={
                  selectedExceptions.length === filteredExceptions.length
                }
                onCheckedChange={handleSelectAll}
              />
              <span className="text-sm font-medium">
                {selectedExceptions.length > 0
                  ? `${selectedExceptions.length} selected`
                  : 'Select all'}
              </span>
              {selectedExceptions.length > 0 && (
                <div className="ml-auto flex gap-2">
                  <Button variant="outline" size="sm">
                    Assign
                  </Button>
                  <Button variant="outline" size="sm">
                    Change Priority
                  </Button>
                  <Button variant="outline" size="sm">
                    Escalate
                  </Button>
                </div>
              )}
            </div>
          )}

          {/* Exception List */}
          <div className="space-y-4">
            {filteredExceptions.map((exception) => (
              <div
                key={exception.id}
                className="flex items-center gap-4 rounded-lg border p-4 transition-colors hover:bg-muted/50"
              >
                <Checkbox
                  checked={selectedExceptions.includes(exception.id)}
                  onCheckedChange={() => handleSelectException(exception.id)}
                />

                <div className="flex-shrink-0">
                  {getStatusIcon(exception.status)}
                </div>

                <div className="min-w-0 flex-1 space-y-3">
                  <div className="flex flex-wrap items-center gap-3">
                    <h4 className="truncate font-medium">
                      {exception.description}
                    </h4>
                    {getStatusBadge(exception.status)}
                    {getPriorityBadge(exception.priority)}
                    <Badge variant="outline" className="text-xs">
                      {getReasonLabel(exception.reason)}
                    </Badge>
                    {exception.escalationLevel > 0 && (
                      <Badge variant="destructive" className="text-xs">
                        Escalated L{exception.escalationLevel}
                      </Badge>
                    )}
                  </div>

                  <div className="grid grid-cols-2 gap-4 text-sm text-muted-foreground lg:grid-cols-5">
                    <div>
                      <span className="font-medium">Amount:</span> $
                      {exception.amount.toFixed(2)}
                    </div>
                    <div>
                      <span className="font-medium">Age:</span> {exception.age}
                    </div>
                    <div>
                      <span className="font-medium">Assigned:</span>{' '}
                      {exception.assignedTo || 'Unassigned'}
                    </div>
                    <div>
                      <span className="font-medium">Created:</span>{' '}
                      {formatDate(exception.createdAt)}
                    </div>
                    <div>
                      <span className="font-medium">Process:</span>{' '}
                      {exception.processId}
                    </div>
                  </div>

                  {exception.suggestedActions.length > 0 && (
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">
                        Suggested Actions:
                      </span>
                      <div className="flex flex-wrap gap-1">
                        {exception.suggestedActions.map((action, index) => (
                          <Badge
                            key={index}
                            variant="outline"
                            className="bg-blue-50 text-xs text-blue-700 dark:bg-blue-950/20 dark:text-blue-400"
                          >
                            {action.replace('_', ' ')}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}

                  {exception.status === 'resolved' && exception.resolvedBy && (
                    <div className="rounded bg-green-50 p-2 text-sm text-green-600 dark:bg-green-950/20">
                      <span className="font-medium">✓ Resolved by:</span>{' '}
                      {exception.resolvedBy}
                    </div>
                  )}
                </div>

                <div className="flex gap-2">
                  {exception.status === 'pending' && (
                    <Button variant="outline" size="sm" title="Assign to me">
                      <User className="h-4 w-4" />
                    </Button>
                  )}
                  {exception.status !== 'resolved' && (
                    <Button variant="outline" size="sm" title="Escalate">
                      <ArrowUpCircle className="h-4 w-4" />
                    </Button>
                  )}
                  <Link
                    href={`/plugins/reconciliation/exceptions/${exception.id}`}
                  >
                    <Button variant="outline" size="sm">
                      <Eye className="h-4 w-4" />
                    </Button>
                  </Link>
                </div>
              </div>
            ))}
          </div>

          {filteredExceptions.length === 0 && (
            <div className="py-8 text-center">
              <AlertTriangle className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
              <h3 className="mb-2 text-lg font-medium">No exceptions found</h3>
              <p className="mb-4 text-muted-foreground">
                {searchQuery ||
                statusFilter !== 'all' ||
                priorityFilter !== 'all'
                  ? 'Try adjusting your search or filters'
                  : 'All reconciliation processes completed successfully'}
              </p>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
