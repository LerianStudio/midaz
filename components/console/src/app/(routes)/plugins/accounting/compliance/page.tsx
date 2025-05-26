'use client'

import { useState } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ComplianceStatusWidget } from '@/components/accounting/compliance/compliance-status-widget'
import { ComplianceAlerts } from '@/components/accounting/compliance/compliance-alerts'
import {
  Eye,
  FileText,
  Download,
  RefreshCw,
  AlertTriangle,
  CheckCircle,
  XCircle
} from 'lucide-react'
import Link from 'next/link'

// Mock compliance data
const complianceOverview = {
  overallScore: 96.5,
  totalRules: 24,
  activeRules: 22,
  violationsLast30Days: 3,
  trend: '+2.1%',
  lastAudit: '2024-12-28T10:30:00Z',
  nextAudit: '2025-02-15T09:00:00Z',
  status: 'compliant' as const
}

const riskAssessment = {
  level: 'Low',
  score: 85,
  factors: [
    { name: 'Account Type Validation', score: 95, status: 'compliant' },
    { name: 'Transaction Route Integrity', score: 92, status: 'compliant' },
    { name: 'Operation Route Mapping', score: 88, status: 'compliant' },
    { name: 'Domain Consistency', score: 90, status: 'compliant' },
    { name: 'Audit Trail Completeness', score: 98, status: 'compliant' },
    { name: 'Business Rule Enforcement', score: 75, status: 'warning' }
  ]
}

const recentActivity = [
  {
    id: 'act-001',
    type: 'violation',
    message: 'Duplicate key value detected in account type creation',
    severity: 'high',
    entity: 'Account Type',
    entityId: 'at-007',
    timestamp: '2024-12-30T14:20:00Z',
    resolved: true
  },
  {
    id: 'act-002',
    type: 'compliance',
    message: 'Daily compliance check completed successfully',
    severity: 'info',
    entity: 'System',
    entityId: null,
    timestamp: '2024-12-30T09:00:00Z',
    resolved: null
  },
  {
    id: 'act-003',
    type: 'validation',
    message: 'Transaction route validation rule updated',
    severity: 'medium',
    entity: 'Transaction Route',
    entityId: 'tr-002',
    timestamp: '2024-12-29T16:45:00Z',
    resolved: null
  },
  {
    id: 'act-004',
    type: 'audit',
    message: 'Manual audit trail export requested',
    severity: 'info',
    entity: 'Audit',
    entityId: 'audit-export-001',
    timestamp: '2024-12-29T11:30:00Z',
    resolved: null
  }
]

const complianceMetrics = [
  {
    title: 'Validation Success Rate',
    value: '99.2%',
    change: '+0.3%',
    trend: 'up'
  },
  {
    title: 'Rule Coverage',
    value: '91.7%',
    change: '+1.2%',
    trend: 'up'
  },
  {
    title: 'Response Time',
    value: '0.8s',
    change: '-0.1s',
    trend: 'up'
  },
  {
    title: 'Audit Readiness',
    value: '98.5%',
    change: '+0.5%',
    trend: 'up'
  }
]

export default function CompliancePage() {
  const [isRefreshing, setIsRefreshing] = useState(false)

  const handleRefresh = async () => {
    setIsRefreshing(true)
    // Simulate API call
    await new Promise((resolve) => setTimeout(resolve, 1000))
    setIsRefreshing(false)
  }

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'high':
        return 'destructive'
      case 'medium':
        return 'default'
      case 'low':
        return 'secondary'
      default:
        return 'outline'
    }
  }

  const getActivityIcon = (type: string) => {
    switch (type) {
      case 'violation':
        return <XCircle className="h-4 w-4 text-red-500" />
      case 'compliance':
        return <CheckCircle className="h-4 w-4 text-green-500" />
      case 'validation':
        return <AlertTriangle className="h-4 w-4 text-yellow-500" />
      case 'audit':
        return <FileText className="h-4 w-4 text-blue-500" />
      default:
        return <Eye className="h-4 w-4 text-gray-500" />
    }
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">
            Compliance Dashboard
          </h1>
          <p className="text-muted-foreground">
            Monitor compliance status, validation rules, and regulatory
            requirements
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={isRefreshing}
          >
            <RefreshCw
              className={`mr-2 h-4 w-4 ${isRefreshing ? 'animate-spin' : ''}`}
            />
            Refresh
          </Button>
          <Button variant="outline" size="sm">
            <Download className="mr-2 h-4 w-4" />
            Export Report
          </Button>
        </div>
      </div>

      {/* Alerts */}
      <ComplianceAlerts />

      {/* Main Content */}
      <Tabs defaultValue="overview" className="space-y-6">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="validation">Validation Rules</TabsTrigger>
          <TabsTrigger value="audit">Audit Trail</TabsTrigger>
          <TabsTrigger value="reports">Reports</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          {/* Status Overview */}
          <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-4">
            {complianceMetrics.map((metric, index) => (
              <Card key={index}>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    {metric.title}
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">{metric.value}</div>
                  <p
                    className={`text-xs ${
                      metric.trend === 'up' ? 'text-green-600' : 'text-red-600'
                    }`}
                  >
                    {metric.change} from last month
                  </p>
                </CardContent>
              </Card>
            ))}
          </div>

          {/* Compliance Status Widget */}
          <ComplianceStatusWidget data={complianceOverview} />

          {/* Risk Assessment */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <AlertTriangle className="h-5 w-5" />
                Risk Assessment
              </CardTitle>
              <CardDescription>
                Current risk factors and compliance health indicators
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">
                    Overall Risk Level
                  </span>
                  <Badge variant="secondary">{riskAssessment.level}</Badge>
                </div>
                <div className="space-y-3">
                  {riskAssessment.factors.map((factor, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between"
                    >
                      <span className="text-sm">{factor.name}</span>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium">
                          {factor.score}%
                        </span>
                        <Badge
                          variant={
                            factor.status === 'compliant'
                              ? 'default'
                              : 'destructive'
                          }
                          className="text-xs"
                        >
                          {factor.status}
                        </Badge>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Recent Activity */}
          <Card>
            <CardHeader>
              <CardTitle>Recent Compliance Activity</CardTitle>
              <CardDescription>
                Latest compliance events and validation results
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {recentActivity.map((activity) => (
                  <div
                    key={activity.id}
                    className="flex items-start space-x-4 rounded-lg border p-4"
                  >
                    <div className="mt-0.5">
                      {getActivityIcon(activity.type)}
                    </div>
                    <div className="flex-1 space-y-1">
                      <p className="text-sm font-medium">{activity.message}</p>
                      <div className="flex items-center space-x-2 text-xs text-muted-foreground">
                        <span>{activity.entity}</span>
                        {activity.entityId && (
                          <>
                            <span>•</span>
                            <span>{activity.entityId}</span>
                          </>
                        )}
                        <span>•</span>
                        <span>
                          {new Date(activity.timestamp).toLocaleString()}
                        </span>
                      </div>
                    </div>
                    <div className="flex items-center space-x-2">
                      <Badge variant={getSeverityColor(activity.severity)}>
                        {activity.severity}
                      </Badge>
                      {activity.resolved !== null && (
                        <Badge
                          variant={
                            activity.resolved ? 'default' : 'destructive'
                          }
                        >
                          {activity.resolved ? 'Resolved' : 'Open'}
                        </Badge>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="validation">
          <Card>
            <CardHeader>
              <CardTitle>Validation Rules Management</CardTitle>
              <CardDescription>
                Configure and monitor validation rules for accounting operations
              </CardDescription>
            </CardHeader>
            <CardContent className="py-8 text-center">
              <p className="mb-4 text-muted-foreground">
                Access detailed validation rule management
              </p>
              <Link href="/plugins/accounting/compliance/validation-rules">
                <Button>
                  <Eye className="mr-2 h-4 w-4" />
                  View Validation Rules
                </Button>
              </Link>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="audit">
          <Card>
            <CardHeader>
              <CardTitle>Audit Trail</CardTitle>
              <CardDescription>
                Complete audit trail of all accounting operations and changes
              </CardDescription>
            </CardHeader>
            <CardContent className="py-8 text-center">
              <p className="mb-4 text-muted-foreground">
                Access detailed audit trail with advanced filtering
              </p>
              <Link href="/plugins/accounting/compliance/audit-trail">
                <Button>
                  <FileText className="mr-2 h-4 w-4" />
                  View Audit Trail
                </Button>
              </Link>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="reports">
          <Card>
            <CardHeader>
              <CardTitle>Compliance Reports</CardTitle>
              <CardDescription>
                Generate and export compliance reports for regulatory
                requirements
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid gap-4 md:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle className="text-base">
                      Monthly Compliance Report
                    </CardTitle>
                    <CardDescription>
                      Comprehensive monthly compliance status and metrics
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Button variant="outline" className="w-full">
                      <Download className="mr-2 h-4 w-4" />
                      Generate Report
                    </Button>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="text-base">
                      Audit Trail Export
                    </CardTitle>
                    <CardDescription>
                      Export filtered audit trail for external review
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Button variant="outline" className="w-full">
                      <Download className="mr-2 h-4 w-4" />
                      Export Data
                    </Button>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="text-base">
                      Validation Summary
                    </CardTitle>
                    <CardDescription>
                      Summary of validation rule performance and coverage
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Button variant="outline" className="w-full">
                      <Download className="mr-2 h-4 w-4" />
                      Download Summary
                    </Button>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle className="text-base">
                      Risk Assessment Report
                    </CardTitle>
                    <CardDescription>
                      Detailed risk analysis and compliance recommendations
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Button variant="outline" className="w-full">
                      <Download className="mr-2 h-4 w-4" />
                      Generate Assessment
                    </Button>
                  </CardContent>
                </Card>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
