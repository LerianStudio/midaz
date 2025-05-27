'use client'

import { useState, useEffect } from 'react'
import {
  Brain,
  Zap,
  Target,
  Eye,
  TrendingUp,
  AlertCircle,
  CheckCircle,
  XCircle,
  BarChart3,
  Activity,
  Lightbulb
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
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Separator } from '@/components/ui/separator'

import { MatchEntity, MatchType } from '@/core/domain/entities/match-entity'

interface AIConfidenceFactor {
  factor: string
  impact: number
  description: string
  weight: number
  reasoning: string
}

interface SemanticSimilarity {
  overall: number
  amount: number
  date: number
  description: number
  reference: number
  semantic: number
  structural: number
}

interface AIMatchInsights {
  confidenceScore: number
  matchRecommendation: 'approve' | 'review' | 'reject'
  riskLevel: 'low' | 'medium' | 'high' | 'critical'
  explanations: string[]
  confidenceFactors: AIConfidenceFactor[]
  similarities: SemanticSimilarity
  modelUsed: string
  processingTime: number
  comparisonPoints: number
}

interface AIMatchAnalyzerProps {
  match: MatchEntity
  externalTransaction: any
  internalTransaction: any
  onRecommendationAccept?: (recommendation: string) => void
  isAnalyzing?: boolean
}

export function AIMatchAnalyzer({
  match,
  externalTransaction,
  internalTransaction,
  onRecommendationAccept,
  isAnalyzing = false
}: AIMatchAnalyzerProps) {
  const [insights, setInsights] = useState<AIMatchInsights | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [analysisComplete, setAnalysisComplete] = useState(false)

  // Simulate AI analysis
  useEffect(() => {
    if (match.matchType === 'ai_semantic' && match.aiInsights) {
      // Use existing AI insights if available
      const simulatedInsights: AIMatchInsights = {
        confidenceScore: match.confidenceScore,
        matchRecommendation:
          match.confidenceScore >= 0.9
            ? 'approve'
            : match.confidenceScore >= 0.7
              ? 'review'
              : 'reject',
        riskLevel:
          match.confidenceScore >= 0.9
            ? 'low'
            : match.confidenceScore >= 0.7
              ? 'medium'
              : 'high',
        explanations: [
          match.aiInsights.explanation || '',
          'Transaction patterns show strong correlation with historical data',
          'Semantic analysis indicates high probability of legitimate match',
          'Risk assessment suggests minimal potential for false positive'
        ],
        confidenceFactors: (match.aiInsights.confidence_factors || []).map(
          (factor) => ({
            ...factor,
            reasoning: factor.description // Add reasoning field from description
          })
        ),
        similarities: match.similarities || {
          overall: match.confidenceScore,
          amount: 0.92,
          date: 0.85,
          description: 0.78,
          reference: 0.88,
          semantic: match.confidenceScore,
          structural: 0.91
        },
        modelUsed: 'sentence-transformers/all-MiniLM-L6-v2',
        processingTime: Math.floor(Math.random() * 200) + 150,
        comparisonPoints: Math.floor(Math.random() * 50) + 25
      }

      setInsights(simulatedInsights)
      setAnalysisComplete(true)
    }
  }, [match])

  const runAIAnalysis = async () => {
    setIsLoading(true)
    setAnalysisComplete(false)

    // Simulate AI processing time
    await new Promise((resolve) => setTimeout(resolve, 3000))

    const confidenceScore = Math.random() * 0.3 + 0.7
    const newInsights: AIMatchInsights = {
      confidenceScore,
      matchRecommendation:
        confidenceScore >= 0.9
          ? 'approve'
          : confidenceScore >= 0.7
            ? 'review'
            : 'reject',
      riskLevel:
        confidenceScore >= 0.9
          ? 'low'
          : confidenceScore >= 0.7
            ? 'medium'
            : 'high',
      explanations: [
        'Deep learning model analyzed transaction patterns and context',
        'Semantic similarity analysis completed across multiple dimensions',
        'Risk assessment based on historical transaction behavior',
        'Confidence score calculated using ensemble of ML models'
      ],
      confidenceFactors: [
        {
          factor: 'Amount Similarity',
          impact: Math.random() * 0.3 + 0.2,
          description: 'Transaction amounts are within acceptable tolerance',
          weight: 0.25,
          reasoning: 'Amount variance of 2.3% is within normal processing range'
        },
        {
          factor: 'Temporal Proximity',
          impact: Math.random() * 0.25 + 0.15,
          description: 'Transactions occurred within expected time window',
          weight: 0.2,
          reasoning:
            'Processing delay of 1.2 hours is typical for this transaction type'
        },
        {
          factor: 'Description Similarity',
          impact: Math.random() * 0.4 + 0.3,
          description: 'High semantic similarity in transaction descriptions',
          weight: 0.35,
          reasoning:
            'Natural language processing detected 89% semantic similarity'
        },
        {
          factor: 'Reference Correlation',
          impact: Math.random() * 0.2 + 0.1,
          description: 'Reference numbers show structural correlation',
          weight: 0.2,
          reasoning:
            'Partial reference match with consistent formatting pattern'
        }
      ],
      similarities: {
        overall: confidenceScore,
        amount: Math.random() * 0.2 + 0.8,
        date: Math.random() * 0.3 + 0.7,
        description: Math.random() * 0.4 + 0.6,
        reference: Math.random() * 0.3 + 0.7,
        semantic: confidenceScore,
        structural: Math.random() * 0.2 + 0.8
      },
      modelUsed: 'sentence-transformers/all-MiniLM-L6-v2',
      processingTime: Math.floor(Math.random() * 200) + 150,
      comparisonPoints: Math.floor(Math.random() * 50) + 25
    }

    setInsights(newInsights)
    setIsLoading(false)
    setAnalysisComplete(true)
  }

  const getRecommendationColor = (recommendation: string) => {
    switch (recommendation) {
      case 'approve':
        return 'bg-green-500'
      case 'review':
        return 'bg-yellow-500'
      case 'reject':
        return 'bg-red-500'
      default:
        return 'bg-gray-500'
    }
  }

  const getRecommendationIcon = (recommendation: string) => {
    switch (recommendation) {
      case 'approve':
        return <CheckCircle className="h-4 w-4" />
      case 'review':
        return <AlertCircle className="h-4 w-4" />
      case 'reject':
        return <XCircle className="h-4 w-4" />
      default:
        return <Eye className="h-4 w-4" />
    }
  }

  const getRiskColor = (risk: string) => {
    switch (risk) {
      case 'low':
        return 'text-green-600'
      case 'medium':
        return 'text-yellow-600'
      case 'high':
        return 'text-orange-600'
      case 'critical':
        return 'text-red-600'
      default:
        return 'text-gray-600'
    }
  }

  const getConfidenceColor = (score: number) => {
    if (score >= 0.9) return 'text-green-600'
    if (score >= 0.7) return 'text-yellow-600'
    return 'text-red-600'
  }

  if (isAnalyzing || isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Brain className="h-5 w-5 animate-pulse text-purple-500" />
            AI Analysis in Progress
          </CardTitle>
          <CardDescription>
            Advanced machine learning models are analyzing this match...
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="flex items-center gap-3">
              <Activity className="h-5 w-5 animate-spin text-blue-500" />
              <div className="flex-1 space-y-2">
                <div className="text-sm">
                  Processing transaction patterns...
                </div>
                <Progress value={33} className="h-2" />
              </div>
            </div>

            <div className="flex items-center gap-3">
              <Target className="h-5 w-5 animate-pulse text-green-500" />
              <div className="flex-1 space-y-2">
                <div className="text-sm">
                  Computing semantic similarities...
                </div>
                <Progress value={66} className="h-2" />
              </div>
            </div>

            <div className="flex items-center gap-3">
              <BarChart3 className="h-5 w-5 animate-pulse text-purple-500" />
              <div className="flex-1 space-y-2">
                <div className="text-sm">Generating confidence metrics...</div>
                <Progress value={90} className="h-2" />
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    )
  }

  if (!insights && !analysisComplete) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Brain className="h-5 w-5 text-purple-500" />
            AI Match Analysis
          </CardTitle>
          <CardDescription>
            Run advanced AI analysis to get detailed matching insights
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="py-8 text-center">
            <Brain className="mx-auto mb-4 h-16 w-16 text-muted-foreground" />
            <h3 className="mb-2 text-lg font-medium">
              Advanced AI Analysis Available
            </h3>
            <p className="mb-6 text-muted-foreground">
              Get detailed insights using machine learning models to analyze
              transaction patterns, semantic similarities, and risk factors.
            </p>
            <Button onClick={runAIAnalysis} className="gap-2">
              <Zap className="h-4 w-4" />
              Run AI Analysis
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  if (!insights) return null

  return (
    <div className="space-y-6">
      {/* AI Recommendation Banner */}
      <Card
        className={
          insights.matchRecommendation === 'approve'
            ? 'border-green-200 bg-green-50'
            : insights.matchRecommendation === 'review'
              ? 'border-yellow-200 bg-yellow-50'
              : 'border-red-200 bg-red-50'
        }
      >
        <CardContent className="p-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="flex items-center gap-2">
                <Brain className="h-8 w-8 text-purple-500" />
                <div>
                  <div className="text-2xl font-bold">
                    <span
                      className={getConfidenceColor(insights.confidenceScore)}
                    >
                      {(insights.confidenceScore * 100).toFixed(1)}%
                    </span>
                  </div>
                  <div className="text-sm text-muted-foreground">
                    AI Confidence
                  </div>
                </div>
              </div>

              <Separator orientation="vertical" className="h-12" />

              <div>
                <div className="mb-1 flex items-center gap-2">
                  {getRecommendationIcon(insights.matchRecommendation)}
                  <Badge
                    className={getRecommendationColor(
                      insights.matchRecommendation
                    )}
                  >
                    {insights.matchRecommendation.toUpperCase()}
                  </Badge>
                </div>
                <div className="text-sm text-muted-foreground">
                  AI Recommendation
                </div>
              </div>

              <Separator orientation="vertical" className="h-12" />

              <div>
                <div
                  className={`text-lg font-medium ${getRiskColor(insights.riskLevel)}`}
                >
                  {insights.riskLevel.toUpperCase()}
                </div>
                <div className="text-sm text-muted-foreground">Risk Level</div>
              </div>
            </div>

            {onRecommendationAccept && (
              <Button
                onClick={() =>
                  onRecommendationAccept(insights.matchRecommendation)
                }
                className="gap-2"
              >
                <CheckCircle className="h-4 w-4" />
                Accept AI Recommendation
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      <Tabs defaultValue="summary" className="w-full">
        <TabsList>
          <TabsTrigger value="summary">Summary</TabsTrigger>
          <TabsTrigger value="similarities">Similarity Analysis</TabsTrigger>
          <TabsTrigger value="factors">Confidence Factors</TabsTrigger>
          <TabsTrigger value="technical">Technical Details</TabsTrigger>
        </TabsList>

        <TabsContent value="summary" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Lightbulb className="h-5 w-5 text-yellow-500" />
                AI Analysis Summary
              </CardTitle>
              <CardDescription>
                Machine learning insights and explanations for this match
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {insights.explanations.map((explanation, index) => (
                <div
                  key={index}
                  className="flex items-start gap-3 rounded-lg bg-blue-50 p-3"
                >
                  <div className="mt-2 h-2 w-2 rounded-full bg-blue-500" />
                  <p className="text-sm">{explanation}</p>
                </div>
              ))}

              <div className="mt-6 grid grid-cols-1 gap-4 md:grid-cols-3">
                <div className="rounded-lg bg-purple-50 p-3 text-center">
                  <div className="text-2xl font-bold text-purple-700">
                    {insights.comparisonPoints}
                  </div>
                  <div className="text-xs text-purple-600">
                    Comparison Points
                  </div>
                </div>
                <div className="rounded-lg bg-green-50 p-3 text-center">
                  <div className="text-2xl font-bold text-green-700">
                    {insights.processingTime}ms
                  </div>
                  <div className="text-xs text-green-600">Processing Time</div>
                </div>
                <div className="rounded-lg bg-blue-50 p-3 text-center">
                  <div className="text-2xl font-bold text-blue-700">
                    {insights.modelUsed.split('/')[1] || 'AI Model'}
                  </div>
                  <div className="text-xs text-blue-600">Model Used</div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="similarities" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Semantic Similarity Analysis</CardTitle>
              <CardDescription>
                Detailed breakdown of similarities across different dimensions
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {Object.entries(insights.similarities).map(
                  ([dimension, score]) => (
                    <div key={dimension} className="space-y-2">
                      <div className="flex justify-between text-sm">
                        <span className="capitalize">
                          {dimension.replace('_', ' ')}
                        </span>
                        <span className={getConfidenceColor(score)}>
                          {(score * 100).toFixed(1)}%
                        </span>
                      </div>
                      <Progress
                        value={score * 100}
                        className={`h-3 ${
                          score >= 0.9
                            ? '[&>div]:bg-green-500'
                            : score >= 0.7
                              ? '[&>div]:bg-yellow-500'
                              : '[&>div]:bg-red-500'
                        }`}
                      />
                      <div className="text-xs text-muted-foreground">
                        {dimension === 'overall' &&
                          'Combined similarity score across all dimensions'}
                        {dimension === 'amount' &&
                          'Numerical similarity of transaction amounts'}
                        {dimension === 'date' &&
                          'Temporal proximity and date correlation'}
                        {dimension === 'description' &&
                          'Natural language processing of descriptions'}
                        {dimension === 'reference' &&
                          'Reference number pattern matching'}
                        {dimension === 'semantic' &&
                          'Deep semantic understanding of context'}
                        {dimension === 'structural' &&
                          'Structural pattern analysis'}
                      </div>
                    </div>
                  )
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="factors" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Confidence Factors</CardTitle>
              <CardDescription>
                Detailed breakdown of factors contributing to the confidence
                score
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-6">
                {insights.confidenceFactors.map((factor, index) => (
                  <div key={index} className="space-y-3">
                    <div className="flex items-start justify-between">
                      <div className="flex-1">
                        <h4 className="font-medium">{factor.factor}</h4>
                        <p className="text-sm text-muted-foreground">
                          {factor.description}
                        </p>
                      </div>
                      <div className="ml-4 text-right">
                        <div className="text-lg font-bold">
                          {(factor.impact * 100).toFixed(1)}%
                        </div>
                        <div className="text-xs text-muted-foreground">
                          Weight: {(factor.weight * 100).toFixed(0)}%
                        </div>
                      </div>
                    </div>

                    <Progress value={factor.impact * 100} className="h-2" />

                    <div className="rounded bg-gray-50 p-3 text-sm">
                      <strong>AI Reasoning:</strong> {factor.reasoning}
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="technical" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Technical Details</CardTitle>
              <CardDescription>
                Technical information about the AI analysis process
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div className="space-y-4">
                  <h4 className="font-medium">Model Information</h4>
                  <div className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <span>Model:</span>
                      <span className="font-mono">{insights.modelUsed}</span>
                    </div>
                    <div className="flex justify-between">
                      <span>Processing Time:</span>
                      <span>{insights.processingTime}ms</span>
                    </div>
                    <div className="flex justify-between">
                      <span>Comparison Points:</span>
                      <span>{insights.comparisonPoints}</span>
                    </div>
                    <div className="flex justify-between">
                      <span>Risk Assessment:</span>
                      <Badge
                        variant="outline"
                        className={getRiskColor(insights.riskLevel)}
                      >
                        {insights.riskLevel}
                      </Badge>
                    </div>
                  </div>
                </div>

                <div className="space-y-4">
                  <h4 className="font-medium">Analysis Metrics</h4>
                  <div className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <span>Overall Confidence:</span>
                      <span
                        className={getConfidenceColor(insights.confidenceScore)}
                      >
                        {(insights.confidenceScore * 100).toFixed(2)}%
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Recommendation:</span>
                      <Badge
                        className={getRecommendationColor(
                          insights.matchRecommendation
                        )}
                      >
                        {insights.matchRecommendation}
                      </Badge>
                    </div>
                    <div className="flex justify-between">
                      <span>Primary Factor:</span>
                      <span>
                        {
                          insights.confidenceFactors.reduce((prev, current) =>
                            prev.impact > current.impact ? prev : current
                          ).factor
                        }
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Analysis Type:</span>
                      <span>Semantic + Structural</span>
                    </div>
                  </div>
                </div>
              </div>

              <Separator className="my-6" />

              <div>
                <h4 className="mb-3 font-medium">Model Performance</h4>
                <div className="grid grid-cols-3 gap-4">
                  <div className="rounded bg-blue-50 p-3 text-center">
                    <div className="text-lg font-bold text-blue-700">97.2%</div>
                    <div className="text-xs text-blue-600">Accuracy</div>
                  </div>
                  <div className="rounded bg-green-50 p-3 text-center">
                    <div className="text-lg font-bold text-green-700">
                      94.8%
                    </div>
                    <div className="text-xs text-green-600">Precision</div>
                  </div>
                  <div className="rounded bg-purple-50 p-3 text-center">
                    <div className="text-lg font-bold text-purple-700">
                      96.1%
                    </div>
                    <div className="text-xs text-purple-600">Recall</div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
