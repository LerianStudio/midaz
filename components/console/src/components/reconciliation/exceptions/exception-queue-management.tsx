'use client'

import { useState, useEffect } from 'react'
import {
  AlertTriangle,
  Clock,
  User,
  Filter,
  Download,
  CheckCircle,
  XCircle,
  Eye,
  MoreHorizontal,
  ArrowUpDown,
  Search,
  Calendar,
  TrendingUp,
  Users,
  Target,
  Zap
} from 'lucide-react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'

import {
  mockReconciliationExceptions,
  mockExternalTransactions,
  mockInternalTransactions,
  ReconciliationException
} from '@/lib/mock-data/reconciliation-unified'
import { ExceptionResolutionWorkflow } from './exception-resolution-workflow'

interface ExceptionQueueManagementProps {
  processId?: string
  className?: string
}

interface ExceptionFilters {
  status:
    | 'all'
    | 'pending'
    | 'assigned'
    | 'investigating'
    | 'resolved'
    | 'escalated'
  category:
    | 'all'
    | 'unmatched'
    | 'duplicate'
    | 'amount_mismatch'
    | 'date_mismatch'
    | 'validation_error'
    | 'system_error'
  priority: 'all' | 'low' | 'medium' | 'high' | 'critical'
  assignee: 'all' | 'unassigned' | string
  escalationLevel: 'all' | '0' | '1' | '2' | '3+'
}

export function ExceptionQueueManagement({
  processId,
  className
}: ExceptionQueueManagementProps) {
  const [exceptions, setExceptions] = useState<ReconciliationException[]>(
    mockReconciliationExceptions
  )
  const [selectedExceptions, setSelectedExceptions] = useState<string[]>([])
  const [selectedExceptionId, setSelectedExceptionId] = useState<string | null>(
    null
  )
  const [filters, setFilters] = useState<ExceptionFilters>({
    status: 'all',
    category: 'all',
    priority: 'all',
    assignee: 'all',
    escalationLevel: 'all'
  })
  const [searchTerm, setSearchTerm] = useState('')
  const [sortField, setSortField] = useState<
    'priority' | 'createdAt' | 'dueDate' | 'escalationLevel'
  >('priority')
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('desc')

  // Filter exceptions based on current filters
  const filteredExceptions = exceptions.filter((exception) => {
    const statusMatch =
      filters.status === 'all' || exception.status === filters.status
    const categoryMatch =
      filters.category === 'all' || exception.category === filters.category
    const priorityMatch =
      filters.priority === 'all' || exception.priority === filters.priority
    const assigneeMatch =
      filters.assignee === 'all' ||
      (filters.assignee === 'unassigned'
        ? !exception.assignedTo
        : exception.assignedTo === filters.assignee)
    const escalationMatch =
      filters.escalationLevel === 'all' ||
      (filters.escalationLevel === '3+'
        ? exception.escalationLevel >= 3
        : exception.escalationLevel.toString() === filters.escalationLevel)

    const searchMatch =
      searchTerm === '' ||
      exception.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
      exception.reason.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (exception.externalTransactionId &&
        exception.externalTransactionId
          .toLowerCase()
          .includes(searchTerm.toLowerCase()))

    return (
      statusMatch &&
      categoryMatch &&
      priorityMatch &&
      assigneeMatch &&
      escalationMatch &&
      searchMatch
    )
  })

  // Sort exceptions
  const sortedExceptions = [...filteredExceptions].sort((a, b) => {
    let aValue: any = a[sortField]
    let bValue: any = b[sortField]

    if (sortField === 'priority') {
      const priorityOrder = { critical: 4, high: 3, medium: 2, low: 1 }
      aValue = priorityOrder[a.priority as keyof typeof priorityOrder] || 0
      bValue = priorityOrder[b.priority as keyof typeof priorityOrder] || 0
    } else if (sortField === 'createdAt' || sortField === 'dueDate') {
      aValue = new Date(aValue || 0).getTime()
      bValue = new Date(bValue || 0).getTime()
    }

    if (sortDirection === 'asc') {
      return aValue > bValue ? 1 : -1
    } else {
      return aValue < bValue ? 1 : -1
    }
  })

  const getPriorityBadge = (priority: string) => {
    switch (priority) {
      case 'critical':
        return <Badge className="bg-red-500">Critical</Badge>
      case 'high':
        return <Badge className="bg-orange-500">High</Badge>
      case 'medium':
        return <Badge className="bg-yellow-500">Medium</Badge>
      case 'low':
        return <Badge variant="outline">Low</Badge>
      default:
        return <Badge variant="outline">{priority}</Badge>
    }
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'pending':
        return (
          <Badge
            variant="outline"
            className="border-yellow-200 bg-yellow-50 text-yellow-700"
          >
            Pending
          </Badge>
        )
      case 'assigned':
        return (
          <Badge
            variant="outline"
            className="border-blue-200 bg-blue-50 text-blue-700"
          >
            Assigned
          </Badge>
        )
      case 'investigating':
        return (
          <Badge
            variant="outline"
            className="border-purple-200 bg-purple-50 text-purple-700"
          >
            Investigating
          </Badge>
        )
      case 'resolved':
        return (
          <Badge
            variant="outline"
            className="border-green-200 bg-green-50 text-green-700"
          >
            Resolved
          </Badge>
        )
      case 'escalated':
        return (
          <Badge
            variant="outline"
            className="border-red-200 bg-red-50 text-red-700"
          >
            Escalated
          </Badge>
        )
      default:
        return <Badge variant="outline">{status}</Badge>
    }
  }

  const getCategoryBadge = (category: string) => {
    const categoryConfig = {
      unmatched: {
        label: 'Unmatched',
        color: 'bg-gray-50 text-gray-700 border-gray-200'
      },
      duplicate: {
        label: 'Duplicate',
        color: 'bg-orange-50 text-orange-700 border-orange-200'
      },
      amount_mismatch: {
        label: 'Amount Mismatch',
        color: 'bg-red-50 text-red-700 border-red-200'
      },
      date_mismatch: {
        label: 'Date Mismatch',
        color: 'bg-yellow-50 text-yellow-700 border-yellow-200'
      },
      validation_error: {
        label: 'Validation Error',
        color: 'bg-purple-50 text-purple-700 border-purple-200'
      },
      system_error: {
        label: 'System Error',
        color: 'bg-red-50 text-red-700 border-red-200'
      }
    }

    const config = categoryConfig[category as keyof typeof categoryConfig] || {
      label: category,
      color: 'bg-gray-50 text-gray-700 border-gray-200'
    }
    return (
      <Badge variant="outline" className={config.color}>
        {config.label}
      </Badge>
    )
  }

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedExceptions(sortedExceptions.map((e) => e.id))
    } else {
      setSelectedExceptions([])
    }
  }

  const handleSelectException = (exceptionId: string, checked: boolean) => {
    if (checked) {
      setSelectedExceptions((prev) => [...prev, exceptionId])
    } else {
      setSelectedExceptions((prev) => prev.filter((id) => id !== exceptionId))
    }
  }

  const handleBulkAction = (action: 'assign' | 'escalate' | 'priority') => {
    // Implementation would update the exceptions based on the action
    console.log(`Bulk ${action} action for:`, selectedExceptions)
    setSelectedExceptions([])
  }

  const getExternalTransaction = (id?: string) => {
    if (!id) return null
    return mockExternalTransactions.find((t) => t.id === id)
  }

  const getInternalTransaction = (id?: string) => {
    if (!id) return null
    return mockInternalTransactions.find((t) => t.id === id)
  }

  const getDaysOverdue = (dueDate?: string) => {
    if (!dueDate) return 0
    const due = new Date(dueDate)
    const now = new Date()
    const diffTime = now.getTime() - due.getTime()
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24))
    return Math.max(0, diffDays)
  }

  const stats = {
    total: exceptions.length,
    pending: exceptions.filter((e) => e.status === 'pending').length,
    assigned: exceptions.filter((e) => e.status === 'assigned').length,
    investigating: exceptions.filter((e) => e.status === 'investigating')
      .length,
    resolved: exceptions.filter((e) => e.status === 'resolved').length,
    escalated: exceptions.filter((e) => e.status === 'escalated').length,
    critical: exceptions.filter((e) => e.priority === 'critical').length,
    overdue: exceptions.filter(
      (e) => e.dueDate && getDaysOverdue(e.dueDate) > 0
    ).length
  }

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Header with Stats */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <AlertTriangle className="h-5 w-5 text-orange-600" />
                Exception Queue
              </CardTitle>
              <CardDescription>
                Manage and resolve reconciliation exceptions
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm">
                <Download className="mr-2 h-4 w-4" />
                Export
              </Button>
              <Button variant="outline" size="sm">
                <Filter className="mr-2 h-4 w-4" />
                Advanced Filters
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Stats Grid */}
          <div className="mb-6 grid grid-cols-2 gap-4 md:grid-cols-4 lg:grid-cols-8">
            <div className="rounded-lg bg-blue-50 p-3 text-center">
              <div className="text-2xl font-bold text-blue-700">
                {stats.total}
              </div>
              <div className="text-xs text-blue-600">Total</div>
            </div>
            <div className="rounded-lg bg-yellow-50 p-3 text-center">
              <div className="text-2xl font-bold text-yellow-700">
                {stats.pending}
              </div>
              <div className="text-xs text-yellow-600">Pending</div>
            </div>
            <div className="rounded-lg bg-blue-50 p-3 text-center">
              <div className="text-2xl font-bold text-blue-700">
                {stats.assigned}
              </div>
              <div className="text-xs text-blue-600">Assigned</div>
            </div>
            <div className="rounded-lg bg-purple-50 p-3 text-center">
              <div className="text-2xl font-bold text-purple-700">
                {stats.investigating}
              </div>
              <div className="text-xs text-purple-600">Investigating</div>
            </div>
            <div className="rounded-lg bg-green-50 p-3 text-center">
              <div className="text-2xl font-bold text-green-700">
                {stats.resolved}
              </div>
              <div className="text-xs text-green-600">Resolved</div>
            </div>
            <div className="rounded-lg bg-red-50 p-3 text-center">
              <div className="text-2xl font-bold text-red-700">
                {stats.escalated}
              </div>
              <div className="text-xs text-red-600">Escalated</div>
            </div>
            <div className="rounded-lg bg-red-50 p-3 text-center">
              <div className="text-2xl font-bold text-red-700">
                {stats.critical}
              </div>
              <div className="text-xs text-red-600">Critical</div>
            </div>
            <div className="rounded-lg bg-orange-50 p-3 text-center">
              <div className="text-2xl font-bold text-orange-700">
                {stats.overdue}
              </div>
              <div className="text-xs text-orange-600">Overdue</div>
            </div>
          </div>

          {/* Filters and Search */}
          <div className="mb-6 flex flex-wrap items-center gap-4">
            <div className="max-w-sm flex-1">
              <Input
                placeholder="Search exceptions..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
              />
            </div>
            <Select
              value={filters.status}
              onValueChange={(value: any) =>
                setFilters((prev) => ({ ...prev, status: value }))
              }
            >
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
            <Select
              value={filters.priority}
              onValueChange={(value: any) =>
                setFilters((prev) => ({ ...prev, priority: value }))
              }
            >
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
            <Select
              value={filters.category}
              onValueChange={(value: any) =>
                setFilters((prev) => ({ ...prev, category: value }))
              }
            >
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Category" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Categories</SelectItem>
                <SelectItem value="unmatched">Unmatched</SelectItem>
                <SelectItem value="amount_mismatch">Amount Mismatch</SelectItem>
                <SelectItem value="date_mismatch">Date Mismatch</SelectItem>
                <SelectItem value="duplicate">Duplicate</SelectItem>
                <SelectItem value="validation_error">
                  Validation Error
                </SelectItem>
                <SelectItem value="system_error">System Error</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Bulk Actions */}
          {selectedExceptions.length > 0 && (
            <div className="mb-6 flex items-center gap-4 rounded-lg bg-orange-50 p-4">
              <span className="text-sm font-medium">
                {selectedExceptions.length} exception(s) selected
              </span>
              <div className="flex gap-2">
                <Button size="sm" onClick={() => handleBulkAction('assign')}>
                  <User className="mr-1 h-4 w-4" />
                  Bulk Assign
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleBulkAction('escalate')}
                >
                  <TrendingUp className="mr-1 h-4 w-4" />
                  Escalate
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleBulkAction('priority')}
                >
                  <Target className="mr-1 h-4 w-4" />
                  Set Priority
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Exceptions Table */}
      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-12">
                  <Checkbox
                    checked={
                      selectedExceptions.length === sortedExceptions.length &&
                      sortedExceptions.length > 0
                    }
                    onCheckedChange={handleSelectAll}
                  />
                </TableHead>
                <TableHead>Exception ID</TableHead>
                <TableHead>Category</TableHead>
                <TableHead
                  className="cursor-pointer"
                  onClick={() => {
                    setSortField('priority')
                    setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
                  }}
                >
                  <div className="flex items-center gap-1">
                    Priority
                    <ArrowUpDown className="h-4 w-4" />
                  </div>
                </TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Assignee</TableHead>
                <TableHead>Amount</TableHead>
                <TableHead
                  className="cursor-pointer"
                  onClick={() => {
                    setSortField('dueDate')
                    setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
                  }}
                >
                  <div className="flex items-center gap-1">
                    Due Date
                    <ArrowUpDown className="h-4 w-4" />
                  </div>
                </TableHead>
                <TableHead>Escalation</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedExceptions.map((exception) => {
                const externalTxn = getExternalTransaction(
                  exception.externalTransactionId
                )
                const internalTxn = getInternalTransaction(
                  exception.internalTransactionId
                )
                const daysOverdue = getDaysOverdue(exception.dueDate)

                return (
                  <TableRow
                    key={exception.id}
                    className={daysOverdue > 0 ? 'bg-red-50' : ''}
                  >
                    <TableCell>
                      <Checkbox
                        checked={selectedExceptions.includes(exception.id)}
                        onCheckedChange={(checked) =>
                          handleSelectException(
                            exception.id,
                            checked as boolean
                          )
                        }
                      />
                    </TableCell>
                    <TableCell className="font-mono text-sm">
                      <div className="flex items-center gap-2">
                        {exception.id.slice(-8)}
                        {exception.escalationLevel > 0 && (
                          <Badge variant="outline" className="text-xs">
                            L{exception.escalationLevel}
                          </Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      {getCategoryBadge(exception.category)}
                    </TableCell>
                    <TableCell>
                      {getPriorityBadge(exception.priority)}
                    </TableCell>
                    <TableCell>{getStatusBadge(exception.status)}</TableCell>
                    <TableCell>
                      {exception.assignedTo ? (
                        <div className="flex items-center gap-2">
                          <User className="h-4 w-4 text-muted-foreground" />
                          <span className="text-sm">
                            {exception.assignedTo.split('@')[0]}
                          </span>
                        </div>
                      ) : (
                        <span className="text-sm text-muted-foreground">
                          Unassigned
                        </span>
                      )}
                    </TableCell>
                    <TableCell>
                      {externalTxn && (
                        <span className="font-medium">
                          {externalTxn.currency}{' '}
                          {externalTxn.amount.toLocaleString()}
                        </span>
                      )}
                      {internalTxn && !externalTxn && (
                        <span className="font-medium">
                          {internalTxn.currency}{' '}
                          {internalTxn.amount.toLocaleString()}
                        </span>
                      )}
                    </TableCell>
                    <TableCell>
                      {exception.dueDate ? (
                        <div className="space-y-1">
                          <span
                            className={`text-sm ${daysOverdue > 0 ? 'font-medium text-red-600' : ''}`}
                          >
                            {new Date(exception.dueDate).toLocaleDateString()}
                          </span>
                          {daysOverdue > 0 && (
                            <div className="text-xs text-red-600">
                              {daysOverdue} days overdue
                            </div>
                          )}
                        </div>
                      ) : (
                        <span className="text-sm text-muted-foreground">
                          No due date
                        </span>
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <span className="text-sm">
                          Level {exception.escalationLevel}
                        </span>
                        {exception.escalationLevel > 0 && (
                          <Progress
                            value={exception.escalationLevel * 25}
                            className="h-2 w-16"
                          />
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Dialog>
                          <DialogTrigger asChild>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() =>
                                setSelectedExceptionId(exception.id)
                              }
                            >
                              <Eye className="h-4 w-4" />
                            </Button>
                          </DialogTrigger>
                          <DialogContent className="max-h-[90vh] max-w-6xl overflow-auto">
                            <DialogHeader>
                              <DialogTitle>Exception Resolution</DialogTitle>
                              <DialogDescription>
                                Resolve exception {exception.id}
                              </DialogDescription>
                            </DialogHeader>
                            {selectedExceptionId && (
                              <ExceptionResolutionWorkflow
                                exceptionId={selectedExceptionId}
                                onResolutionComplete={(resolution) => {
                                  console.log(
                                    'Resolution completed:',
                                    resolution
                                  )
                                  // Update exception status
                                }}
                              />
                            )}
                          </DialogContent>
                        </Dialog>

                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button size="sm" variant="outline">
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent>
                            <DropdownMenuItem>
                              <User className="mr-2 h-4 w-4" />
                              Assign
                            </DropdownMenuItem>
                            <DropdownMenuItem>
                              <TrendingUp className="mr-2 h-4 w-4" />
                              Escalate
                            </DropdownMenuItem>
                            <DropdownMenuItem>
                              <Calendar className="mr-2 h-4 w-4" />
                              Set Due Date
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem>
                              <Zap className="mr-2 h-4 w-4" />
                              Quick Resolve
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}
