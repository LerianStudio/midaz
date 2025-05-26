'use client'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { Button } from '@/components/ui/button'
import {
  CheckCircle,
  AlertTriangle,
  XCircle,
  Shield,
  TrendingUp,
  TrendingDown,
  Calendar,
  Clock
} from 'lucide-react'

interface ComplianceData {
  overallScore: number
  totalRules: number
  activeRules: number
  violationsLast30Days: number
  trend: string
  lastAudit: string
  nextAudit: string
  status: 'compliant' | 'warning' | 'non-compliant'
}

interface ComplianceStatusWidgetProps {
  data: ComplianceData
}

export function ComplianceStatusWidget({ data }: ComplianceStatusWidgetProps) {
  const getStatusColor = (status: string) => {
    switch (status) {
      case 'compliant':
        return 'text-green-600'
      case 'warning':
        return 'text-yellow-600'
      case 'non-compliant':
        return 'text-red-600'
      default:
        return 'text-gray-600'
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'compliant':
        return <CheckCircle className="h-5 w-5 text-green-600" />
      case 'warning':
        return <AlertTriangle className="h-5 w-5 text-yellow-600" />
      case 'non-compliant':
        return <XCircle className="h-5 w-5 text-red-600" />
      default:
        return <Shield className="h-5 w-5 text-gray-600" />
    }
  }

  const getScoreColor = (score: number) => {
    if (score >= 95) return 'text-green-600'
    if (score >= 85) return 'text-yellow-600'
    return 'text-red-600'
  }

  const getTrendIcon = (trend: string) => {
    if (trend.startsWith('+')) {
      return <TrendingUp className="h-4 w-4 text-green-600" />
    } else if (trend.startsWith('-')) {
      return <TrendingDown className="h-4 w-4 text-red-600" />
    }
    return null
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Shield className="h-5 w-5" />
          Compliance Status
        </CardTitle>
        <CardDescription>
          Overall compliance health and regulatory status
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Overall Status */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            {getStatusIcon(data.status)}
            <div>
              <p className="font-medium capitalize">
                {data.status.replace('-', ' ')}
              </p>
              <p className="text-sm text-muted-foreground">Current Status</p>
            </div>
          </div>
          <Badge
            variant={data.status === 'compliant' ? 'default' : 'destructive'}
          >
            {data.status === 'compliant' ? 'Compliant' : 'Needs Attention'}
          </Badge>
        </div>

        {/* Compliance Score */}
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Compliance Score</span>
            <div className="flex items-center gap-2">
              <span
                className={`text-2xl font-bold ${getScoreColor(data.overallScore)}`}
              >
                {data.overallScore}%
              </span>
              {getTrendIcon(data.trend)}
              <span className="text-sm text-muted-foreground">
                {data.trend}
              </span>
            </div>
          </div>
          <Progress value={data.overallScore} className="h-2" />
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>0%</span>
            <span>50%</span>
            <span>100%</span>
          </div>
        </div>

        {/* Rules Status */}
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-sm">Total Rules</span>
              <span className="font-medium">{data.totalRules}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm">Active Rules</span>
              <span className="font-medium text-green-600">
                {data.activeRules}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm">Coverage</span>
              <span className="font-medium">
                {Math.round((data.activeRules / data.totalRules) * 100)}%
              </span>
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-sm">Violations (30d)</span>
              <span
                className={`font-medium ${data.violationsLast30Days > 0 ? 'text-red-600' : 'text-green-600'}`}
              >
                {data.violationsLast30Days}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm">Success Rate</span>
              <span className="font-medium text-green-600">
                {Math.round((1 - data.violationsLast30Days / 100) * 100)}%
              </span>
            </div>
          </div>
        </div>

        {/* Audit Information */}
        <div className="space-y-3 border-t pt-4">
          <h4 className="flex items-center gap-2 font-medium">
            <Calendar className="h-4 w-4" />
            Audit Schedule
          </h4>

          <div className="grid gap-3 md:grid-cols-2">
            <div className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-muted-foreground" />
              <div>
                <p className="text-sm font-medium">Last Audit</p>
                <p className="text-xs text-muted-foreground">
                  {new Date(data.lastAudit).toLocaleDateString()}
                </p>
              </div>
            </div>

            <div className="flex items-center gap-2">
              <Calendar className="h-4 w-4 text-muted-foreground" />
              <div>
                <p className="text-sm font-medium">Next Audit</p>
                <p className="text-xs text-muted-foreground">
                  {new Date(data.nextAudit).toLocaleDateString()}
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* Quick Actions */}
        <div className="flex gap-2">
          <Button variant="outline" size="sm" className="flex-1">
            <Shield className="mr-2 h-4 w-4" />
            Run Check
          </Button>
          <Button variant="outline" size="sm" className="flex-1">
            <AlertTriangle className="mr-2 h-4 w-4" />
            View Issues
          </Button>
        </div>

        {/* Status Indicators */}
        <div className="grid gap-2 text-xs">
          <div className="flex items-center gap-2">
            <div className="h-2 w-2 rounded-full bg-green-500" />
            <span>95-100%: Fully Compliant</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="h-2 w-2 rounded-full bg-yellow-500" />
            <span>85-94%: Minor Issues</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="h-2 w-2 rounded-full bg-red-500" />
            <span>&lt;85%: Action Required</span>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
