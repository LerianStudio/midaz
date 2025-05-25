'use client'

import { useState, useEffect } from 'react'
import {
  Check,
  X,
  Eye,
  Filter,
  Download,
  ChevronDown,
  ChevronRight,
  Brain,
  Target,
  AlertTriangle,
  CheckCircle,
  Clock,
  MoreHorizontal,
  ArrowUpDown,
  ExternalLink
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
import { Progress } from '@/components/ui/progress'
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
import { Separator } from '@/components/ui/separator'

import {
  mockReconciliationMatches,
  mockExternalTransactions,
  mockInternalTransactions,
  ReconciliationMatch
} from '@/lib/mock-data/reconciliation-unified'
import { AIMatchAnalyzer } from '../matching/ai-match-analyzer'

interface MatchResultsInterfaceProps {
  processId?: string
  className?: string
}

interface MatchFilters {
  status: 'all' | 'pending' | 'confirmed' | 'rejected' | 'under_review'
  matchType: 'all' | 'exact' | 'fuzzy' | 'ai_semantic' | 'manual' | 'rule_based'
  confidenceRange: [number, number]
  reviewPriority: 'all' | 'high' | 'medium' | 'low'
}

export function MatchResultsInterface({
  processId,
  className
}: MatchResultsInterfaceProps) {
  const [matches, setMatches] = useState<ReconciliationMatch[]>(
    mockReconciliationMatches
  )
  const [selectedMatches, setSelectedMatches] = useState<string[]>([])
  const [expandedMatch, setExpandedMatch] = useState<string | null>(null)
  const [filters, setFilters] = useState<MatchFilters>({
    status: 'all',
    matchType: 'all',
    confidenceRange: [0, 1],
    reviewPriority: 'all'
  })
  const [searchTerm, setSearchTerm] = useState('')
  const [sortField, setSortField] = useState<
    'confidenceScore' | 'createdAt' | 'matchType'
  >('confidenceScore')
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('desc')

  // Filter matches based on current filters
  const filteredMatches = matches.filter((match) => {
    const statusMatch =
      filters.status === 'all' || match.status === filters.status
    const typeMatch =
      filters.matchType === 'all' || match.matchType === filters.matchType
    const confidenceMatch =
      match.confidenceScore >= filters.confidenceRange[0] &&
      match.confidenceScore <= filters.confidenceRange[1]
    const priorityMatch =
      filters.reviewPriority === 'all' ||
      match.aiInsights?.suggested_review_priority === filters.reviewPriority

    const searchMatch =
      searchTerm === '' ||
      match.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
      match.externalTransactionId
        .toLowerCase()
        .includes(searchTerm.toLowerCase())

    return (
      statusMatch &&
      typeMatch &&
      confidenceMatch &&
      priorityMatch &&
      searchMatch
    )
  })

  // Sort matches
  const sortedMatches = [...filteredMatches].sort((a, b) => {
    let aValue: any = a[sortField]
    let bValue: any = b[sortField]

    if (sortField === 'createdAt') {
      aValue = new Date(aValue).getTime()
      bValue = new Date(bValue).getTime()
    }

    if (sortDirection === 'asc') {
      return aValue > bValue ? 1 : -1
    } else {
      return aValue < bValue ? 1 : -1
    }
  })

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
      case 'confirmed':
        return (
          <Badge
            variant="outline"
            className="border-green-200 bg-green-50 text-green-700"
          >
            Confirmed
          </Badge>
        )
      case 'rejected':
        return (
          <Badge
            variant="outline"
            className="border-red-200 bg-red-50 text-red-700"
          >
            Rejected
          </Badge>
        )
      case 'under_review':
        return (
          <Badge
            variant="outline"
            className="border-blue-200 bg-blue-50 text-blue-700"
          >
            Under Review
          </Badge>
        )
      default:
        return <Badge variant="outline">{status}</Badge>
    }
  }

  const getMatchTypeBadge = (type: string) => {
    switch (type) {
      case 'exact':
        return (
          <Badge
            variant="outline"
            className="border-green-200 bg-green-50 text-green-700"
          >
            Exact
          </Badge>
        )
      case 'fuzzy':
        return (
          <Badge
            variant="outline"
            className="border-blue-200 bg-blue-50 text-blue-700"
          >
            Fuzzy
          </Badge>
        )
      case 'ai_semantic':
        return (
          <Badge
            variant="outline"
            className="border-purple-200 bg-purple-50 text-purple-700"
          >
            AI Semantic
          </Badge>
        )
      case 'manual':
        return (
          <Badge
            variant="outline"
            className="border-gray-200 bg-gray-50 text-gray-700"
          >
            Manual
          </Badge>
        )
      case 'rule_based':
        return (
          <Badge
            variant="outline"
            className="border-indigo-200 bg-indigo-50 text-indigo-700"
          >
            Rule Based
          </Badge>
        )
      default:
        return <Badge variant="outline">{type}</Badge>
    }
  }

  const getConfidenceBadge = (score: number) => {
    if (score >= 0.9) {
      return (
        <Badge className="bg-green-500">
          High ({Math.round(score * 100)}%)
        </Badge>
      )
    } else if (score >= 0.7) {
      return (
        <Badge className="bg-yellow-500">
          Medium ({Math.round(score * 100)}%)
        </Badge>
      )
    } else {
      return (
        <Badge className="bg-red-500">Low ({Math.round(score * 100)}%)</Badge>
      )
    }
  }

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedMatches(sortedMatches.map((m) => m.id))
    } else {
      setSelectedMatches([])
    }
  }

  const handleSelectMatch = (matchId: string, checked: boolean) => {
    if (checked) {
      setSelectedMatches((prev) => [...prev, matchId])
    } else {
      setSelectedMatches((prev) => prev.filter((id) => id !== matchId))
    }
  }

  const handleBulkAction = (action: 'confirm' | 'reject' | 'review') => {
    const updatedMatches = matches.map((match) => {
      if (selectedMatches.includes(match.id)) {
        return {
          ...match,
          status:
            action === 'confirm'
              ? 'confirmed'
              : action === 'reject'
                ? 'rejected'
                : 'under_review',
          reviewedAt: new Date().toISOString(),
          reviewedBy: 'current-user@company.com'
        }
      }
      return match
    })
    setMatches(updatedMatches)
    setSelectedMatches([])
  }

  const handleSingleAction = (
    matchId: string,
    action: 'confirm' | 'reject' | 'review'
  ) => {
    const updatedMatches = matches.map((match) => {
      if (match.id === matchId) {
        return {
          ...match,
          status:
            action === 'confirm'
              ? 'confirmed'
              : action === 'reject'
                ? 'rejected'
                : 'under_review',
          reviewedAt: new Date().toISOString(),
          reviewedBy: 'current-user@company.com'
        }
      }
      return match
    })
    setMatches(updatedMatches)
  }

  const getExternalTransaction = (id: string) => {
    return mockExternalTransactions.find((t) => t.id === id)
  }

  const getInternalTransaction = (id: string) => {
    return mockInternalTransactions.find((t) => t.id === id)
  }

  const stats = {
    total: matches.length,
    pending: matches.filter((m) => m.status === 'pending').length,
    confirmed: matches.filter((m) => m.status === 'confirmed').length,
    rejected: matches.filter((m) => m.status === 'rejected').length,
    highConfidence: matches.filter((m) => m.confidenceScore >= 0.9).length,
    aiMatches: matches.filter((m) => m.matchType === 'ai_semantic').length
  }

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Header with Stats */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Target className="h-5 w-5 text-blue-600" />
                Match Results
              </CardTitle>
              <CardDescription>
                Review and manage transaction matches from reconciliation
                process
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm">
                <Download className="mr-2 h-4 w-4" />
                Export
              </Button>
              <Button variant="outline" size="sm">
                <Filter className="mr-2 h-4 w-4" />
                Filters
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Stats Grid */}
          <div className="mb-6 grid grid-cols-2 gap-4 md:grid-cols-6">
            <div className="rounded-lg bg-blue-50 p-3 text-center">
              <div className="text-2xl font-bold text-blue-700">
                {stats.total}
              </div>
              <div className="text-xs text-blue-600">Total Matches</div>
            </div>
            <div className="rounded-lg bg-yellow-50 p-3 text-center">
              <div className="text-2xl font-bold text-yellow-700">
                {stats.pending}
              </div>
              <div className="text-xs text-yellow-600">Pending Review</div>
            </div>
            <div className="rounded-lg bg-green-50 p-3 text-center">
              <div className="text-2xl font-bold text-green-700">
                {stats.confirmed}
              </div>
              <div className="text-xs text-green-600">Confirmed</div>
            </div>
            <div className="rounded-lg bg-red-50 p-3 text-center">
              <div className="text-2xl font-bold text-red-700">
                {stats.rejected}
              </div>
              <div className="text-xs text-red-600">Rejected</div>
            </div>
            <div className="rounded-lg bg-emerald-50 p-3 text-center">
              <div className="text-2xl font-bold text-emerald-700">
                {stats.highConfidence}
              </div>
              <div className="text-xs text-emerald-600">High Confidence</div>
            </div>
            <div className="rounded-lg bg-purple-50 p-3 text-center">
              <div className="text-2xl font-bold text-purple-700">
                {stats.aiMatches}
              </div>
              <div className="text-xs text-purple-600">AI Matches</div>
            </div>
          </div>

          {/* Filters and Search */}
          <div className="mb-6 flex flex-wrap items-center gap-4">
            <div className="max-w-sm flex-1">
              <Input
                placeholder="Search matches..."
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
                <SelectItem value="confirmed">Confirmed</SelectItem>
                <SelectItem value="rejected">Rejected</SelectItem>
                <SelectItem value="under_review">Under Review</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={filters.matchType}
              onValueChange={(value: any) =>
                setFilters((prev) => ({ ...prev, matchType: value }))
              }
            >
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Match Type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Types</SelectItem>
                <SelectItem value="exact">Exact</SelectItem>
                <SelectItem value="fuzzy">Fuzzy</SelectItem>
                <SelectItem value="ai_semantic">AI Semantic</SelectItem>
                <SelectItem value="manual">Manual</SelectItem>
                <SelectItem value="rule_based">Rule Based</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Bulk Actions */}
          {selectedMatches.length > 0 && (
            <div className="mb-6 flex items-center gap-4 rounded-lg bg-blue-50 p-4">
              <span className="text-sm font-medium">
                {selectedMatches.length} match(es) selected
              </span>
              <div className="flex gap-2">
                <Button size="sm" onClick={() => handleBulkAction('confirm')}>
                  <Check className="mr-1 h-4 w-4" />
                  Confirm All
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleBulkAction('reject')}
                >
                  <X className="mr-1 h-4 w-4" />
                  Reject All
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleBulkAction('review')}
                >
                  <Eye className="mr-1 h-4 w-4" />
                  Mark for Review
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Matches Table */}
      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-12">
                  <Checkbox
                    checked={
                      selectedMatches.length === sortedMatches.length &&
                      sortedMatches.length > 0
                    }
                    onCheckedChange={handleSelectAll}
                  />
                </TableHead>
                <TableHead className="w-12"></TableHead>
                <TableHead>Transaction ID</TableHead>
                <TableHead>Match Type</TableHead>
                <TableHead
                  className="cursor-pointer"
                  onClick={() => {
                    setSortField('confidenceScore')
                    setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
                  }}
                >
                  <div className="flex items-center gap-1">
                    Confidence
                    <ArrowUpDown className="h-4 w-4" />
                  </div>
                </TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Amount</TableHead>
                <TableHead>Date</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedMatches.map((match) => {
                const externalTxn = getExternalTransaction(
                  match.externalTransactionId
                )
                const internalTxns = match.internalTransactionIds
                  .map((id) => getInternalTransaction(id))
                  .filter(Boolean)
                const isExpanded = expandedMatch === match.id

                return (
                  <>
                    <TableRow
                      key={match.id}
                      className={isExpanded ? 'bg-muted/50' : ''}
                    >
                      <TableCell>
                        <Checkbox
                          checked={selectedMatches.includes(match.id)}
                          onCheckedChange={(checked) =>
                            handleSelectMatch(match.id, checked as boolean)
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() =>
                            setExpandedMatch(isExpanded ? null : match.id)
                          }
                        >
                          {isExpanded ? (
                            <ChevronDown className="h-4 w-4" />
                          ) : (
                            <ChevronRight className="h-4 w-4" />
                          )}
                        </Button>
                      </TableCell>
                      <TableCell className="font-mono text-sm">
                        <div className="flex items-center gap-2">
                          {match.externalTransactionId.slice(-8)}
                          {match.matchType === 'ai_semantic' && (
                            <Brain className="h-4 w-4 text-purple-500" />
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        {getMatchTypeBadge(match.matchType)}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          {getConfidenceBadge(match.confidenceScore)}
                          {match.aiInsights?.suggested_review_priority ===
                            'high' && (
                            <AlertTriangle className="h-4 w-4 text-orange-500" />
                          )}
                        </div>
                      </TableCell>
                      <TableCell>{getStatusBadge(match.status)}</TableCell>
                      <TableCell>
                        {externalTxn && (
                          <span className="font-medium">
                            {externalTxn.currency}{' '}
                            {externalTxn.amount.toLocaleString()}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        {externalTxn &&
                          new Date(externalTxn.date).toLocaleDateString()}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() =>
                              handleSingleAction(match.id, 'confirm')
                            }
                            disabled={match.status === 'confirmed'}
                          >
                            <Check className="h-4 w-4" />
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() =>
                              handleSingleAction(match.id, 'reject')
                            }
                            disabled={match.status === 'rejected'}
                          >
                            <X className="h-4 w-4" />
                          </Button>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button size="sm" variant="outline">
                                <MoreHorizontal className="h-4 w-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent>
                              <DropdownMenuItem
                                onClick={() =>
                                  handleSingleAction(match.id, 'review')
                                }
                              >
                                <Eye className="mr-2 h-4 w-4" />
                                Mark for Review
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem>
                                <ExternalLink className="mr-2 h-4 w-4" />
                                View Details
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </div>
                      </TableCell>
                    </TableRow>

                    {/* Expanded Details */}
                    {isExpanded && (
                      <TableRow>
                        <TableCell colSpan={9} className="p-0">
                          <div className="border-t bg-muted/20 p-6">
                            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
                              {/* Transaction Details */}
                              <div className="space-y-4">
                                <h4 className="flex items-center gap-2 font-semibold">
                                  <Target className="h-4 w-4" />
                                  Transaction Comparison
                                </h4>

                                {/* External Transaction */}
                                <div className="rounded-lg bg-blue-50 p-4">
                                  <h5 className="mb-2 font-medium text-blue-900">
                                    External Transaction
                                  </h5>
                                  {externalTxn && (
                                    <div className="space-y-1 text-sm">
                                      <div className="flex justify-between">
                                        <span>Amount:</span>
                                        <span className="font-medium">
                                          {externalTxn.currency}{' '}
                                          {externalTxn.amount}
                                        </span>
                                      </div>
                                      <div className="flex justify-between">
                                        <span>Date:</span>
                                        <span>
                                          {new Date(
                                            externalTxn.date
                                          ).toLocaleString()}
                                        </span>
                                      </div>
                                      <div className="flex justify-between">
                                        <span>Description:</span>
                                        <span className="max-w-40 truncate text-right">
                                          {externalTxn.description}
                                        </span>
                                      </div>
                                      <div className="flex justify-between">
                                        <span>Reference:</span>
                                        <span className="font-mono">
                                          {externalTxn.reference}
                                        </span>
                                      </div>
                                    </div>
                                  )}
                                </div>

                                {/* Internal Transactions */}
                                {internalTxns.map((internalTxn, index) => (
                                  <div
                                    key={index}
                                    className="rounded-lg bg-green-50 p-4"
                                  >
                                    <h5 className="mb-2 font-medium text-green-900">
                                      Internal Transaction {index + 1}
                                    </h5>
                                    <div className="space-y-1 text-sm">
                                      <div className="flex justify-between">
                                        <span>Amount:</span>
                                        <span className="font-medium">
                                          {internalTxn.currency}{' '}
                                          {internalTxn.amount}
                                        </span>
                                      </div>
                                      <div className="flex justify-between">
                                        <span>Date:</span>
                                        <span>
                                          {new Date(
                                            internalTxn.date
                                          ).toLocaleString()}
                                        </span>
                                      </div>
                                      <div className="flex justify-between">
                                        <span>Description:</span>
                                        <span className="max-w-40 truncate text-right">
                                          {internalTxn.description}
                                        </span>
                                      </div>
                                      <div className="flex justify-between">
                                        <span>Reference:</span>
                                        <span className="font-mono">
                                          {internalTxn.reference}
                                        </span>
                                      </div>
                                    </div>
                                  </div>
                                ))}

                                {/* Matched Fields */}
                                <div className="rounded-lg bg-gray-50 p-4">
                                  <h5 className="mb-2 font-medium">
                                    Matched Fields
                                  </h5>
                                  <div className="flex flex-wrap gap-2">
                                    {Object.entries(match.matchedFields).map(
                                      ([field, matched]) => (
                                        <Badge
                                          key={field}
                                          variant={
                                            matched ? 'default' : 'outline'
                                          }
                                          className={
                                            matched
                                              ? 'bg-green-500'
                                              : 'bg-gray-200'
                                          }
                                        >
                                          {field.replace('_', ' ')}
                                        </Badge>
                                      )
                                    )}
                                  </div>
                                </div>
                              </div>

                              {/* AI Analysis */}
                              {match.matchType === 'ai_semantic' &&
                                match.aiInsights && (
                                  <div className="space-y-4">
                                    <h4 className="flex items-center gap-2 font-semibold">
                                      <Brain className="h-4 w-4 text-purple-500" />
                                      AI Analysis
                                    </h4>

                                    <div className="rounded-lg bg-purple-50 p-4">
                                      <div className="mb-4 grid grid-cols-2 gap-4">
                                        <div>
                                          <span className="text-sm text-purple-600">
                                            Description Similarity
                                          </span>
                                          <div className="text-lg font-bold text-purple-900">
                                            {Math.round(
                                              (match.aiInsights
                                                .description_similarity || 0) *
                                                100
                                            )}
                                            %
                                          </div>
                                        </div>
                                        <div>
                                          <span className="text-sm text-purple-600">
                                            Amount Similarity
                                          </span>
                                          <div className="text-lg font-bold text-purple-900">
                                            {Math.round(
                                              (match.aiInsights
                                                .amount_similarity || 0) * 100
                                            )}
                                            %
                                          </div>
                                        </div>
                                        <div>
                                          <span className="text-sm text-purple-600">
                                            Temporal Proximity
                                          </span>
                                          <div className="text-lg font-bold text-purple-900">
                                            {Math.round(
                                              (match.aiInsights
                                                .temporal_proximity || 0) * 100
                                            )}
                                            %
                                          </div>
                                        </div>
                                        <div>
                                          <span className="text-sm text-purple-600">
                                            Review Priority
                                          </span>
                                          <Badge
                                            variant="outline"
                                            className="mt-1"
                                          >
                                            {
                                              match.aiInsights
                                                .suggested_review_priority
                                            }
                                          </Badge>
                                        </div>
                                      </div>
                                      {match.aiInsights.explanation && (
                                        <p className="rounded bg-purple-100 p-3 text-sm text-purple-800">
                                          <strong>AI Explanation:</strong>{' '}
                                          {match.aiInsights.explanation}
                                        </p>
                                      )}
                                    </div>
                                  </div>
                                )}
                            </div>
                          </div>
                        </TableCell>
                      </TableRow>
                    )}
                  </>
                )
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}
