'use client'

import React, { useState } from 'react'
import Link from 'next/link'
import { 
  Search, 
  Filter, 
  GitMerge,
  CheckCircle,
  XCircle,
  Clock,
  Eye,
  Brain,
  Target,
  TrendingUp,
  AlertCircle
} from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { 
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'

export default function MatchesPage() {
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [typeFilter, setTypeFilter] = useState('all')
  const [confidenceFilter, setConfidenceFilter] = useState('all')
  const [selectedMatches, setSelectedMatches] = useState<string[]>([])

  // Mock data - will be replaced with real API calls
  const matches = [
    {
      id: '1',
      processId: 'proc-1',
      externalTransactionId: 'ext-txn-1',
      internalTransactionIds: ['int-txn-1'],
      matchType: 'exact',
      confidenceScore: 1.0,
      status: 'confirmed',
      externalAmount: 2547.82,
      internalAmount: 2547.82,
      externalDescription: 'Wire transfer payment REF12345',
      internalDescription: 'Wire transfer payment REF12345',
      reviewedBy: 'analyst@company.com',
      reviewedAt: '2024-12-01T11:30:00Z',
      aiInsights: null,
      createdAt: '2024-12-01T10:15:00Z'
    },
    {
      id: '2',
      processId: 'proc-1',
      externalTransactionId: 'ext-txn-2',
      internalTransactionIds: ['int-txn-2'],
      matchType: 'ai_semantic',
      confidenceScore: 0.87,
      status: 'pending',
      externalAmount: 1250.00,
      internalAmount: 1250.00,
      externalDescription: 'Online payment from customer',
      internalDescription: 'Customer payment via web portal',
      aiInsights: {
        description_similarity: 0.92,
        amount_similarity: 1.0,
        temporal_proximity: 0.85,
        suggested_review_priority: 'medium',
        explanation: 'High semantic similarity in descriptions with exact amount match'
      },
      createdAt: '2024-12-01T10:20:00Z'
    },
    {
      id: '3',
      processId: 'proc-2',
      externalTransactionId: 'ext-txn-3',
      internalTransactionIds: ['int-txn-3'],
      matchType: 'fuzzy',
      confidenceScore: 0.78,
      status: 'under_review',
      externalAmount: 875.50,
      internalAmount: 875.52,
      externalDescription: 'ACH deposit from ACME Corp',
      internalDescription: 'ACH deposit ACME Corporation',
      reviewedBy: 'senior.analyst@company.com',
      createdAt: '2024-12-01T09:45:00Z'
    },
    {
      id: '4',
      processId: 'proc-2',
      externalTransactionId: 'ext-txn-4',
      internalTransactionIds: ['int-txn-4'],
      matchType: 'rule_based',
      confidenceScore: 0.95,
      status: 'auto_approved',
      externalAmount: 432.18,
      internalAmount: 432.18,
      externalDescription: 'Card payment #1234',
      internalDescription: 'Payment card transaction 1234',
      ruleId: 'rule-001',
      ruleName: 'Exact Amount and Reference Match',
      createdAt: '2024-12-01T09:30:00Z'
    },
    {
      id: '5',
      processId: 'proc-3',
      externalTransactionId: 'ext-txn-5',
      internalTransactionIds: ['int-txn-5'],
      matchType: 'manual',
      confidenceScore: 0.65,
      status: 'rejected',
      externalAmount: 156.75,
      internalAmount: 156.78,
      externalDescription: 'Service fee payment',
      internalDescription: 'Monthly service charge',
      reviewedBy: 'analyst2@company.com',
      reviewedAt: '2024-11-30T16:45:00Z',
      reviewNotes: 'Amount variance too high, likely different transactions',
      createdAt: '2024-11-30T14:20:00Z'
    }
  ]

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'confirmed':
        return <CheckCircle className="h-4 w-4 text-green-600" />
      case 'auto_approved':
        return <CheckCircle className="h-4 w-4 text-blue-600" />
      case 'rejected':
        return <XCircle className="h-4 w-4 text-red-600" />
      case 'under_review':
        return <Clock className="h-4 w-4 text-yellow-600" />
      case 'pending':
        return <AlertCircle className="h-4 w-4 text-orange-600" />
      default:
        return <GitMerge className="h-4 w-4 text-gray-600" />
    }
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'confirmed':
        return <Badge className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400">Confirmed</Badge>
      case 'auto_approved':
        return <Badge className="bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400">Auto Approved</Badge>
      case 'rejected':
        return <Badge variant="destructive">Rejected</Badge>
      case 'under_review':
        return <Badge className="bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400">Under Review</Badge>
      case 'pending':
        return <Badge className="bg-orange-100 text-orange-800 dark:bg-orange-900/20 dark:text-orange-400">Pending</Badge>
      default:
        return <Badge variant="outline">{status}</Badge>
    }
  }

  const getMatchTypeBadge = (type: string) => {
    switch (type) {
      case 'exact':
        return <Badge className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400">Exact</Badge>
      case 'ai_semantic':
        return <Badge className="bg-purple-100 text-purple-800 dark:bg-purple-900/20 dark:text-purple-400">AI Semantic</Badge>
      case 'fuzzy':
        return <Badge className="bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400">Fuzzy</Badge>
      case 'rule_based':
        return <Badge className="bg-indigo-100 text-indigo-800 dark:bg-indigo-900/20 dark:text-indigo-400">Rule Based</Badge>
      case 'manual':
        return <Badge variant="outline">Manual</Badge>
      default:
        return <Badge variant="outline">{type}</Badge>
    }
  }

  const getConfidenceColor = (score: number) => {
    if (score >= 0.9) return 'text-green-600'
    if (score >= 0.8) return 'text-blue-600'
    if (score >= 0.7) return 'text-yellow-600'
    return 'text-red-600'
  }

  const getConfidenceLevel = (score: number) => {
    if (score >= 0.9) return 'High'
    if (score >= 0.8) return 'Medium'
    if (score >= 0.7) return 'Low'
    return 'Very Low'
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

  const handleSelectMatch = (matchId: string) => {
    setSelectedMatches(prev => 
      prev.includes(matchId) 
        ? prev.filter(id => id !== matchId)
        : [...prev, matchId]
    )
  }

  const handleSelectAll = () => {
    setSelectedMatches(
      selectedMatches.length === filteredMatches.length 
        ? [] 
        : filteredMatches.map(match => match.id)
    )
  }

  const filteredMatches = matches.filter(match => {
    const matchesSearch = match.externalDescription.toLowerCase().includes(searchQuery.toLowerCase()) ||
                         match.internalDescription.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesStatus = statusFilter === 'all' || match.status === statusFilter
    const matchesType = typeFilter === 'all' || match.matchType === typeFilter
    const matchesConfidence = confidenceFilter === 'all' || 
      (confidenceFilter === 'high' && match.confidenceScore >= 0.9) ||
      (confidenceFilter === 'medium' && match.confidenceScore >= 0.8 && match.confidenceScore < 0.9) ||
      (confidenceFilter === 'low' && match.confidenceScore < 0.8)
    return matchesSearch && matchesStatus && matchesType && matchesConfidence
  })

  const confirmedCount = matches.filter(m => m.status === 'confirmed' || m.status === 'auto_approved').length
  const pendingCount = matches.filter(m => m.status === 'pending' || m.status === 'under_review').length
  const rejectedCount = matches.filter(m => m.status === 'rejected').length
  const aiMatchCount = matches.filter(m => m.matchType === 'ai_semantic').length

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Transaction Matches</h2>
          <p className="text-muted-foreground">
            Review and approve transaction matching results
          </p>
        </div>
        <div className="flex gap-2">
          {selectedMatches.length > 0 && (
            <div className="flex gap-2">
              <Button variant="outline" size="sm" className="gap-2">
                <CheckCircle className="h-4 w-4" />
                Bulk Approve ({selectedMatches.length})
              </Button>
              <Button variant="outline" size="sm" className="gap-2">
                <XCircle className="h-4 w-4" />
                Bulk Reject
              </Button>
            </div>
          )}
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Confirmed</CardTitle>
            <CheckCircle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">{confirmedCount}</div>
            <p className="text-xs text-muted-foreground">
              Approved matches
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Pending Review</CardTitle>
            <Clock className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-orange-600">{pendingCount}</div>
            <p className="text-xs text-muted-foreground">
              Awaiting review
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">AI Matches</CardTitle>
            <Brain className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-purple-600">{aiMatchCount}</div>
            <p className="text-xs text-muted-foreground">
              AI-powered matches
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Success Rate</CardTitle>
            <Target className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-600">
              {Math.round((confirmedCount / matches.length) * 100)}%
            </div>
            <p className="text-xs text-muted-foreground">
              Match approval rate
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Matches List */}
      <Card>
        <CardHeader>
          <CardTitle>Match Results</CardTitle>
          <CardDescription>Review transaction matching results and approve or reject matches</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div className="flex flex-1 gap-2">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search matches..."
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
                  <SelectItem value="confirmed">Confirmed</SelectItem>
                  <SelectItem value="auto_approved">Auto Approved</SelectItem>
                  <SelectItem value="under_review">Under Review</SelectItem>
                  <SelectItem value="rejected">Rejected</SelectItem>
                </SelectContent>
              </Select>
              <Select value={typeFilter} onValueChange={setTypeFilter}>
                <SelectTrigger className="w-40">
                  <SelectValue placeholder="Type" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Types</SelectItem>
                  <SelectItem value="exact">Exact</SelectItem>
                  <SelectItem value="ai_semantic">AI Semantic</SelectItem>
                  <SelectItem value="fuzzy">Fuzzy</SelectItem>
                  <SelectItem value="rule_based">Rule Based</SelectItem>
                  <SelectItem value="manual">Manual</SelectItem>
                </SelectContent>
              </Select>
              <Select value={confidenceFilter} onValueChange={setConfidenceFilter}>
                <SelectTrigger className="w-40">
                  <SelectValue placeholder="Confidence" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Confidence</SelectItem>
                  <SelectItem value="high">High (90%+)</SelectItem>
                  <SelectItem value="medium">Medium (80-89%)</SelectItem>
                  <SelectItem value="low">Low (&lt;80%)</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button variant="outline" size="sm" className="gap-2">
              <Filter className="h-4 w-4" />
              Advanced Filters
            </Button>
          </div>

          {/* Bulk Selection Header */}
          {filteredMatches.length > 0 && (
            <div className="flex items-center gap-4 p-3 bg-muted rounded-lg">
              <Checkbox
                checked={selectedMatches.length === filteredMatches.length}
                onCheckedChange={handleSelectAll}
              />
              <span className="text-sm font-medium">
                {selectedMatches.length > 0 
                  ? `${selectedMatches.length} selected`
                  : 'Select all'
                }
              </span>
              {selectedMatches.length > 0 && (
                <div className="flex gap-2 ml-auto">
                  <Button variant="outline" size="sm" className="gap-2">
                    <CheckCircle className="h-4 w-4" />
                    Approve
                  </Button>
                  <Button variant="outline" size="sm" className="gap-2">
                    <XCircle className="h-4 w-4" />
                    Reject
                  </Button>
                </div>
              )}
            </div>
          )}

          {/* Match List */}
          <div className="space-y-4">
            {filteredMatches.map((match) => (
              <div key={match.id} className="flex items-center gap-4 p-4 border rounded-lg hover:bg-muted/50 transition-colors">
                <Checkbox
                  checked={selectedMatches.includes(match.id)}
                  onCheckedChange={() => handleSelectMatch(match.id)}
                />
                
                <div className="flex-shrink-0">
                  {getStatusIcon(match.status)}
                </div>
                
                <div className="flex-1 min-w-0 space-y-3">
                  <div className="flex items-center gap-3 flex-wrap">
                    <div className="flex items-center gap-2">
                      {getMatchTypeBadge(match.matchType)}
                      {getStatusBadge(match.status)}
                    </div>
                    <div className="flex items-center gap-2">
                      <span className={`text-sm font-medium ${getConfidenceColor(match.confidenceScore)}`}>
                        {(match.confidenceScore * 100).toFixed(1)}%
                      </span>
                      <Badge variant="outline" className="text-xs">
                        {getConfidenceLevel(match.confidenceScore)}
                      </Badge>
                    </div>
                    {match.matchType === 'ai_semantic' && (
                      <Badge className="bg-purple-50 text-purple-700 dark:bg-purple-950/20 dark:text-purple-400 gap-1">
                        <Brain className="h-3 w-3" />
                        AI Enhanced
                      </Badge>
                    )}
                  </div>
                  
                  {/* Transaction Comparison */}
                  <div className="grid gap-3 lg:grid-cols-2">
                    <div className="space-y-2">
                      <h5 className="text-sm font-medium text-green-700 dark:text-green-400">External Transaction</h5>
                      <div className="space-y-1 text-sm">
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">Amount:</span>
                          <span className="font-mono">${match.externalAmount.toFixed(2)}</span>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Description:</span>
                          <p className="mt-1 text-sm">{match.externalDescription}</p>
                        </div>
                      </div>
                    </div>
                    
                    <div className="space-y-2">
                      <h5 className="text-sm font-medium text-blue-700 dark:text-blue-400">Internal Transaction</h5>
                      <div className="space-y-1 text-sm">
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">Amount:</span>
                          <span className="font-mono">${match.internalAmount.toFixed(2)}</span>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Description:</span>
                          <p className="mt-1 text-sm">{match.internalDescription}</p>
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* Amount Variance Indicator */}
                  {Math.abs(match.externalAmount - match.internalAmount) > 0.01 && (
                    <div className="flex items-center gap-2 text-sm text-orange-600">
                      <AlertCircle className="h-4 w-4" />
                      Amount variance: ${Math.abs(match.externalAmount - match.internalAmount).toFixed(2)}
                    </div>
                  )}

                  {/* AI Insights */}
                  {match.aiInsights && (
                    <div className="bg-purple-50 dark:bg-purple-950/20 p-3 rounded-lg">
                      <div className="flex items-center gap-2 mb-2">
                        <Brain className="h-4 w-4 text-purple-600" />
                        <span className="text-sm font-medium text-purple-700 dark:text-purple-400">AI Insights</span>
                      </div>
                      <p className="text-sm text-purple-600 dark:text-purple-400 mb-2">{match.aiInsights.explanation}</p>
                      <div className="grid grid-cols-3 gap-4 text-xs">
                        <div>
                          <span className="text-muted-foreground">Description:</span>
                          <div className="flex items-center gap-2">
                            <Progress value={match.aiInsights.description_similarity * 100} className="h-1 flex-1" />
                            <span>{(match.aiInsights.description_similarity * 100).toFixed(0)}%</span>
                          </div>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Amount:</span>
                          <div className="flex items-center gap-2">
                            <Progress value={match.aiInsights.amount_similarity * 100} className="h-1 flex-1" />
                            <span>{(match.aiInsights.amount_similarity * 100).toFixed(0)}%</span>
                          </div>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Timing:</span>
                          <div className="flex items-center gap-2">
                            <Progress value={match.aiInsights.temporal_proximity * 100} className="h-1 flex-1" />
                            <span>{(match.aiInsights.temporal_proximity * 100).toFixed(0)}%</span>
                          </div>
                        </div>
                      </div>
                    </div>
                  )}

                  {/* Rule Information */}
                  {match.ruleId && match.ruleName && (
                    <div className="flex items-center gap-2 text-sm text-indigo-600">
                      <Target className="h-4 w-4" />
                      Matched by rule: {match.ruleName}
                    </div>
                  )}

                  {/* Review Information */}
                  <div className="flex items-center gap-4 text-sm text-muted-foreground">
                    <div>
                      <span className="font-medium">Created:</span> {formatDate(match.createdAt)}
                    </div>
                    {match.reviewedBy && (
                      <div>
                        <span className="font-medium">Reviewed by:</span> {match.reviewedBy}
                      </div>
                    )}
                    {match.reviewedAt && (
                      <div>
                        <span className="font-medium">Reviewed:</span> {formatDate(match.reviewedAt)}
                      </div>
                    )}
                  </div>

                  {/* Review Notes */}
                  {match.reviewNotes && (
                    <div className="text-sm bg-muted p-2 rounded">
                      <span className="font-medium">Review Notes:</span> {match.reviewNotes}
                    </div>
                  )}
                </div>

                <div className="flex gap-2">
                  {match.status === 'pending' && (
                    <>
                      <Button variant="outline" size="sm" className="gap-2 text-green-600">
                        <CheckCircle className="h-4 w-4" />
                        Approve
                      </Button>
                      <Button variant="outline" size="sm" className="gap-2 text-red-600">
                        <XCircle className="h-4 w-4" />
                        Reject
                      </Button>
                    </>
                  )}
                  <Link href={`/plugins/reconciliation/matches/${match.id}`}>
                    <Button variant="outline" size="sm">
                      <Eye className="h-4 w-4" />
                    </Button>
                  </Link>
                </div>
              </div>
            ))}
          </div>

          {filteredMatches.length === 0 && (
            <div className="text-center py-8">
              <GitMerge className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
              <h3 className="text-lg font-medium mb-2">No matches found</h3>
              <p className="text-muted-foreground mb-4">
                {searchQuery || statusFilter !== 'all' || typeFilter !== 'all' || confidenceFilter !== 'all'
                  ? 'Try adjusting your search or filters'
                  : 'No transaction matches available'
                }
              </p>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}