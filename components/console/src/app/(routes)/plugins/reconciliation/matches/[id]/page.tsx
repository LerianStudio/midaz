'use client'

import { useState, useEffect } from 'react'
import { useParams } from 'next/navigation'
import {
  ArrowLeft,
  CheckCircle,
  X,
  AlertTriangle,
  Brain,
  Zap,
  Target,
  TrendingUp,
  Eye,
  ThumbsUp,
  ThumbsDown,
  MoreHorizontal
} from 'lucide-react'
import Link from 'next/link'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Separator } from '@/components/ui/separator'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'

import { ReconciliationMockData } from '@/components/reconciliation/mock/reconciliation-mock-data'
import {
  MatchEntity,
  MatchStatus,
  MatchType
} from '@/core/domain/entities/match-entity'
import { ExternalTransactionEntity } from '@/core/domain/entities/external-transaction-entity'

const getMatchTypeColor = (type: MatchType) => {
  switch (type) {
    case 'exact':
      return 'bg-green-500'
    case 'fuzzy':
      return 'bg-blue-500'
    case 'ai_semantic':
      return 'bg-purple-500'
    case 'manual':
      return 'bg-orange-500'
    case 'rule_based':
      return 'bg-cyan-500'
    default:
      return 'bg-gray-500'
  }
}

const getMatchTypeIcon = (type: MatchType) => {
  switch (type) {
    case 'exact':
      return <Target className="h-4 w-4" />
    case 'fuzzy':
      return <Eye className="h-4 w-4" />
    case 'ai_semantic':
      return <Brain className="h-4 w-4" />
    case 'manual':
      return <Eye className="h-4 w-4" />
    case 'rule_based':
      return <Zap className="h-4 w-4" />
    default:
      return <Eye className="h-4 w-4" />
  }
}

const getStatusColor = (status: MatchStatus) => {
  switch (status) {
    case 'confirmed':
      return 'bg-green-500'
    case 'rejected':
      return 'bg-red-500'
    case 'under_review':
      return 'bg-yellow-500'
    case 'auto_approved':
      return 'bg-blue-500'
    case 'pending':
      return 'bg-gray-500'
    default:
      return 'bg-gray-500'
  }
}

const getConfidenceColor = (score: number) => {
  if (score >= 0.9) return 'text-green-600'
  if (score >= 0.7) return 'text-yellow-600'
  return 'text-red-600'
}

export default function MatchDetailPage() {
  const params = useParams()
  const matchId = params.id as string

  const [match, setMatch] = useState<MatchEntity | null>(null)
  const [externalTransaction, setExternalTransaction] =
    useState<ExternalTransactionEntity | null>(null)
  const [internalTransactions, setInternalTransactions] = useState<any[]>([])

  useEffect(() => {
    // Simulate data loading
    const matches = ReconciliationMockData.generateMatches('process-1', 100)
    const foundMatch = matches.find((m) => m.id === matchId) || matches[0]

    setMatch(foundMatch)

    // Simulate external transaction
    const externalTx = ReconciliationMockData.generateExternalTransactions(
      'import-1',
      1
    )[0]
    setExternalTransaction(externalTx)

    // Simulate internal transactions
    setInternalTransactions([
      {
        id: foundMatch.internalTransactionIds[0],
        amount: externalTx.amount + (Math.random() - 0.5) * 10,
        date: new Date(Date.now() - Math.random() * 86400000 * 3).toISOString(),
        description: 'Internal processing of external transaction',
        referenceNumber: `INT-${Math.random().toString(36).substr(2, 8).toUpperCase()}`,
        accountId: 'acc-123',
        status: 'completed'
      }
    ])
  }, [matchId])

  if (!match || !externalTransaction) {
    return (
      <div className="container mx-auto p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-8 w-1/3 rounded bg-gray-200" />
          <div className="grid grid-cols-2 gap-4">
            <div className="h-64 rounded bg-gray-200" />
            <div className="h-64 rounded bg-gray-200" />
          </div>
        </div>
      </div>
    )
  }

  const internalTx = internalTransactions[0]

  return (
    <div className="container mx-auto space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/plugins/reconciliation/matches">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-1 h-4 w-4" />
              Back to Matches
            </Button>
          </Link>
          <div className="flex items-center gap-2">
            {getMatchTypeIcon(match.matchType)}
            <h1 className="text-2xl font-bold">Match Analysis</h1>
            <Badge className={getMatchTypeColor(match.matchType)}>
              {match.matchType.replace('_', ' ').toUpperCase()}
            </Badge>
            <Badge className={getStatusColor(match.status)}>
              {match.status.replace('_', ' ').toUpperCase()}
            </Badge>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {match.status === 'pending' || match.status === 'under_review' ? (
            <>
              <Button className="gap-2">
                <CheckCircle className="h-4 w-4" />
                Approve Match
              </Button>
              <Button variant="outline" className="gap-2">
                <X className="h-4 w-4" />
                Reject Match
              </Button>
            </>
          ) : null}

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuItem>Flag for Review</DropdownMenuItem>
              <DropdownMenuItem>Request Second Opinion</DropdownMenuItem>
              <DropdownMenuItem>Export Analysis</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Confidence Score Banner */}
      <Card
        className={
          match.confidenceScore >= 0.9
            ? 'border-green-200 bg-green-50'
            : match.confidenceScore >= 0.7
              ? 'border-yellow-200 bg-yellow-50'
              : 'border-red-200 bg-red-50'
        }
      >
        <CardContent className="p-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="text-3xl font-bold">
                <span className={getConfidenceColor(match.confidenceScore)}>
                  {(match.confidenceScore * 100).toFixed(1)}%
                </span>
              </div>
              <div>
                <div className="font-medium">Confidence Score</div>
                <div className="text-sm text-muted-foreground">
                  {match.confidenceScore >= 0.9
                    ? 'High confidence match'
                    : match.confidenceScore >= 0.7
                      ? 'Medium confidence match'
                      : 'Low confidence match - requires review'}
                </div>
              </div>
            </div>

            {match.matchType === 'ai_semantic' && match.aiInsights && (
              <div className="flex items-center gap-2">
                <Brain className="h-5 w-5 text-purple-500" />
                <div className="text-right">
                  <div className="font-medium">AI Analysis Available</div>
                  <div className="text-sm text-muted-foreground">
                    Priority: {match.aiInsights.suggested_review_priority}
                  </div>
                </div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* External Transaction */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">External Transaction</CardTitle>
            <CardDescription>
              Source: {externalTransaction.sourceSystem}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="font-medium">Amount:</span>
                <div className="mt-1 text-lg font-bold">
                  {externalTransaction.currency}{' '}
                  {externalTransaction.amount.toLocaleString()}
                </div>
              </div>
              <div>
                <span className="font-medium">Date:</span>
                <div className="mt-1">
                  {new Date(externalTransaction.date).toLocaleDateString()}
                </div>
              </div>
              <div>
                <span className="font-medium">Reference:</span>
                <div className="mt-1 font-mono text-xs">
                  {externalTransaction.referenceNumber}
                </div>
              </div>
              <div>
                <span className="font-medium">Account:</span>
                <div className="mt-1">{externalTransaction.accountNumber}</div>
              </div>
            </div>

            <Separator />

            <div>
              <span className="font-medium">Description:</span>
              <p className="mt-1 text-sm">{externalTransaction.description}</p>
            </div>

            {externalTransaction.metadata && (
              <>
                <Separator />
                <div>
                  <span className="font-medium">Metadata:</span>
                  <div className="mt-2 space-y-1 text-xs">
                    <div className="flex justify-between">
                      <span>Source Bank:</span>
                      <span>{externalTransaction.metadata.sourceBank}</span>
                    </div>
                    <div className="flex justify-between">
                      <span>Processing Time:</span>
                      <span>
                        {externalTransaction.metadata.processingTime}ms
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Risk Score:</span>
                      <span>
                        {externalTransaction.metadata.riskScore.toFixed(1)}
                      </span>
                    </div>
                  </div>
                </div>
              </>
            )}
          </CardContent>
        </Card>

        {/* Internal Transaction */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Internal Transaction</CardTitle>
            <CardDescription>Matched internal record</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="font-medium">Amount:</span>
                <div className="mt-1 text-lg font-bold">
                  USD {internalTx.amount.toLocaleString()}
                </div>
              </div>
              <div>
                <span className="font-medium">Date:</span>
                <div className="mt-1">
                  {new Date(internalTx.date).toLocaleDateString()}
                </div>
              </div>
              <div>
                <span className="font-medium">Reference:</span>
                <div className="mt-1 font-mono text-xs">
                  {internalTx.referenceNumber}
                </div>
              </div>
              <div>
                <span className="font-medium">Account ID:</span>
                <div className="mt-1">{internalTx.accountId}</div>
              </div>
            </div>

            <Separator />

            <div>
              <span className="font-medium">Description:</span>
              <p className="mt-1 text-sm">{internalTx.description}</p>
            </div>

            <Separator />

            <div>
              <span className="font-medium">Status:</span>
              <Badge className="ml-2 bg-green-500">
                {internalTx.status.toUpperCase()}
              </Badge>
            </div>
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="analysis" className="w-full">
        <TabsList>
          <TabsTrigger value="analysis">Match Analysis</TabsTrigger>
          <TabsTrigger value="ai-insights">AI Insights</TabsTrigger>
          <TabsTrigger value="field-comparison">Field Comparison</TabsTrigger>
          <TabsTrigger value="history">Review History</TabsTrigger>
        </TabsList>

        <TabsContent value="analysis" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Similarity Analysis</CardTitle>
              <CardDescription>
                Detailed field-by-field matching analysis
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {match.matchedFields &&
                  Object.entries(match.matchedFields).map(
                    ([field, similarity]) => {
                      if (
                        field === 'similarity_score' ||
                        field === 'embedding_model'
                      )
                        return null

                      const score =
                        typeof similarity === 'number'
                          ? similarity
                          : typeof similarity === 'boolean'
                            ? similarity
                              ? 1
                              : 0
                            : 0.5

                      return (
                        <div key={field} className="space-y-2">
                          <div className="flex justify-between text-sm">
                            <span className="capitalize">
                              {field.replace('_', ' ')}
                            </span>
                            <span className={getConfidenceColor(score)}>
                              {typeof similarity === 'boolean'
                                ? similarity
                                  ? 'Exact Match'
                                  : 'No Match'
                                : `${(score * 100).toFixed(1)}% similarity`}
                            </span>
                          </div>
                          <Progress
                            value={score * 100}
                            className={`h-2 ${
                              score >= 0.9
                                ? '[&>div]:bg-green-500'
                                : score >= 0.7
                                  ? '[&>div]:bg-yellow-500'
                                  : '[&>div]:bg-red-500'
                            }`}
                          />
                        </div>
                      )
                    }
                  )}

                {match.similarities && (
                  <div className="mt-6 rounded-lg bg-purple-50 p-4">
                    <h4 className="mb-3 flex items-center gap-2 font-medium">
                      <Brain className="h-4 w-4 text-purple-500" />
                      AI Semantic Analysis
                    </h4>
                    <div className="space-y-2">
                      {Object.entries(match.similarities).map(
                        ([metric, score]) => (
                          <div
                            key={metric}
                            className="flex justify-between text-sm"
                          >
                            <span className="capitalize">
                              {metric.replace('_', ' ')}
                            </span>
                            <span
                              className={getConfidenceColor(score as number)}
                            >
                              {((score as number) * 100).toFixed(1)}%
                            </span>
                          </div>
                        )
                      )}
                    </div>
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="ai-insights" className="space-y-4">
          {match.aiInsights ? (
            <div className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Brain className="h-5 w-5 text-purple-500" />
                    AI Analysis Summary
                  </CardTitle>
                  <CardDescription>
                    Machine learning insights and recommendations
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="space-y-4">
                    <div className="rounded-lg bg-blue-50 p-4">
                      <p className="text-sm">{match.aiInsights.explanation}</p>
                    </div>

                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <span className="text-sm font-medium">
                          Pattern Confidence:
                        </span>
                        <div className="text-2xl font-bold text-purple-600">
                          {(match.aiInsights.pattern_confidence * 100).toFixed(
                            1
                          )}
                          %
                        </div>
                      </div>
                      <div>
                        <span className="text-sm font-medium">
                          Review Priority:
                        </span>
                        <Badge
                          className={
                            match.aiInsights.suggested_review_priority ===
                            'critical'
                              ? 'bg-red-500'
                              : match.aiInsights.suggested_review_priority ===
                                  'high'
                                ? 'bg-orange-500'
                                : match.aiInsights.suggested_review_priority ===
                                    'medium'
                                  ? 'bg-yellow-500'
                                  : 'bg-green-500'
                          }
                        >
                          {match.aiInsights.suggested_review_priority.toUpperCase()}
                        </Badge>
                      </div>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Confidence Factors</CardTitle>
                  <CardDescription>
                    Detailed breakdown of AI confidence scoring
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="space-y-4">
                    {match.aiInsights.confidence_factors?.map(
                      (factor, index) => (
                        <div key={index} className="space-y-2">
                          <div className="flex items-center justify-between">
                            <div>
                              <span className="text-sm font-medium">
                                {factor.factor}
                              </span>
                              <p className="text-xs text-muted-foreground">
                                {factor.description}
                              </p>
                            </div>
                            <div className="text-right">
                              <div className="text-sm font-medium">
                                {(factor.impact * 100).toFixed(1)}% impact
                              </div>
                              <div className="text-xs text-muted-foreground">
                                Weight: {(factor.weight * 100).toFixed(0)}%
                              </div>
                            </div>
                          </div>
                          <Progress
                            value={factor.impact * 100}
                            className="h-2"
                          />
                        </div>
                      )
                    )}
                  </div>
                </CardContent>
              </Card>
            </div>
          ) : (
            <Card>
              <CardContent className="p-8 text-center">
                <Brain className="mx-auto mb-2 h-12 w-12 text-muted-foreground" />
                <p className="text-muted-foreground">
                  AI insights are only available for semantic matches
                </p>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="field-comparison" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Field-by-Field Comparison</CardTitle>
              <CardDescription>
                Side-by-side comparison of transaction fields
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="grid grid-cols-3 gap-4 border-b pb-2 text-sm font-medium">
                  <span>Field</span>
                  <span>External Transaction</span>
                  <span>Internal Transaction</span>
                </div>

                <div className="grid grid-cols-3 gap-4 py-2 text-sm">
                  <span className="font-medium">Amount</span>
                  <span>
                    {externalTransaction.currency}{' '}
                    {externalTransaction.amount.toLocaleString()}
                  </span>
                  <span>USD {internalTx.amount.toLocaleString()}</span>
                </div>

                <div className="grid grid-cols-3 gap-4 rounded bg-gray-50 px-2 py-2 text-sm">
                  <span className="font-medium">Date</span>
                  <span>
                    {new Date(externalTransaction.date).toLocaleDateString()}
                  </span>
                  <span>{new Date(internalTx.date).toLocaleDateString()}</span>
                </div>

                <div className="grid grid-cols-3 gap-4 py-2 text-sm">
                  <span className="font-medium">Reference</span>
                  <span className="font-mono text-xs">
                    {externalTransaction.referenceNumber}
                  </span>
                  <span className="font-mono text-xs">
                    {internalTx.referenceNumber}
                  </span>
                </div>

                <div className="grid grid-cols-3 gap-4 rounded bg-gray-50 px-2 py-2 text-sm">
                  <span className="font-medium">Description</span>
                  <span>{externalTransaction.description}</span>
                  <span>{internalTx.description}</span>
                </div>

                <div className="grid grid-cols-3 gap-4 py-2 text-sm">
                  <span className="font-medium">Account</span>
                  <span>{externalTransaction.accountNumber}</span>
                  <span>{internalTx.accountId}</span>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="history" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Review History</CardTitle>
              <CardDescription>
                Timeline of reviews and decisions for this match
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="flex items-start gap-3 rounded-lg border p-3">
                  <div className="mt-2 h-2 w-2 rounded-full bg-blue-500" />
                  <div className="flex-1">
                    <div className="flex items-start justify-between">
                      <div>
                        <div className="font-medium">Match Created</div>
                        <div className="text-sm text-muted-foreground">
                          Automatically identified by{' '}
                          {match.matchType.replace('_', ' ')} matching
                        </div>
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {new Date(match.createdAt).toLocaleString()}
                      </div>
                    </div>
                  </div>
                </div>

                {match.reviewedBy && (
                  <div className="flex items-start gap-3 rounded-lg border p-3">
                    <div className="mt-2 h-2 w-2 rounded-full bg-green-500" />
                    <div className="flex-1">
                      <div className="flex items-start justify-between">
                        <div>
                          <div className="font-medium">Match Reviewed</div>
                          <div className="text-sm text-muted-foreground">
                            Reviewed by {match.reviewedBy}
                          </div>
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {match.reviewedAt
                            ? new Date(match.reviewedAt).toLocaleString()
                            : 'Recently'}
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                {match.status === 'pending' && (
                  <div className="flex items-start gap-3 rounded-lg border border-dashed p-3">
                    <div className="mt-2 h-2 w-2 rounded-full bg-gray-300" />
                    <div className="flex-1">
                      <div className="font-medium text-muted-foreground">
                        Awaiting Review
                      </div>
                      <div className="text-sm text-muted-foreground">
                        This match is pending analyst review
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Action Buttons */}
      {match.status === 'pending' || match.status === 'under_review' ? (
        <div className="flex justify-end gap-3 border-t pt-4">
          <Button variant="outline" className="gap-2">
            <ThumbsDown className="h-4 w-4" />
            Reject Match
          </Button>
          <Button className="gap-2">
            <ThumbsUp className="h-4 w-4" />
            Confirm Match
          </Button>
        </div>
      ) : null}
    </div>
  )
}
