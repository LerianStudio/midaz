'use client'

import { useState } from 'react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import {
  AlertTriangle,
  CheckCircle,
  XCircle,
  Info,
  X,
  Eye,
  Clock,
  RefreshCw
} from 'lucide-react'

interface ComplianceAlert {
  id: string
  type: 'violation' | 'warning' | 'info' | 'success'
  severity: 'high' | 'medium' | 'low'
  title: string
  message: string
  details?: string
  source: string
  sourceId?: string
  timestamp: string
  status: 'active' | 'acknowledged' | 'resolved'
  actions?: string[]
  metadata?: Record<string, any>
}

const mockAlerts: ComplianceAlert[] = [
  {
    id: 'alert-001',
    type: 'violation',
    severity: 'high',
    title: 'Duplicate Key Value Detected',
    message: "Account type creation failed due to duplicate key value 'CHCK'",
    details:
      "User attempted to create account type with key value 'CHCK' which already exists in the system. This violates the uniqueness constraint for account type key values.",
    source: 'Account Type Validation',
    sourceId: 'rule-001',
    timestamp: '2024-12-30T14:20:00Z',
    status: 'active',
    actions: ['review_attempt', 'check_existing', 'contact_user'],
    metadata: {
      attemptedKeyValue: 'CHCK',
      existingAccountTypeId: 'at-001',
      userId: 'user-001',
      userName: 'John Smith'
    }
  },
  {
    id: 'alert-002',
    type: 'warning',
    severity: 'medium',
    title: 'Domain Consistency Warning',
    message: 'Operation route references external-to-external account mapping',
    details:
      "Operation route in transaction 'tr-004' attempts to map between two external domain accounts, which may indicate a configuration issue.",
    source: 'Domain Validation',
    sourceId: 'rule-003',
    timestamp: '2024-12-30T13:45:00Z',
    status: 'acknowledged',
    actions: ['review_mapping', 'verify_intention'],
    metadata: {
      transactionRouteId: 'tr-004',
      sourceAccountType: 'EXT_BANK_A',
      destinationAccountType: 'EXT_BANK_B'
    }
  },
  {
    id: 'alert-003',
    type: 'info',
    severity: 'low',
    title: 'Daily Compliance Check Completed',
    message: 'Automated compliance verification completed successfully',
    details:
      'All 24 validation rules executed successfully with 96.5% overall compliance score.',
    source: 'System Check',
    timestamp: '2024-12-30T09:00:00Z',
    status: 'resolved',
    metadata: {
      rulesExecuted: 24,
      rulesPassed: 23,
      overallScore: 96.5,
      duration: '2.3s'
    }
  },
  {
    id: 'alert-004',
    type: 'violation',
    severity: 'high',
    title: 'Fee Calculation Expression Invalid',
    message:
      'Invalid mathematical expression detected in operation route amount calculation',
    details:
      "Operation route 'or-015' contains an invalid amount expression '{{amount}} * / 0.03' which fails syntax validation.",
    source: 'Expression Validation',
    sourceId: 'rule-006',
    timestamp: '2024-12-30T11:15:00Z',
    status: 'active',
    actions: ['fix_expression', 'disable_route', 'contact_creator'],
    metadata: {
      operationRouteId: 'or-015',
      invalidExpression: '{{amount}} * / 0.03',
      transactionRouteId: 'tr-005'
    }
  },
  {
    id: 'alert-005',
    type: 'success',
    severity: 'low',
    title: 'Validation Rule Updated Successfully',
    message: 'Balance validation rule updated with improved performance',
    details:
      "Rule 'Transaction Route Operation Balance' has been updated to version 2.0.1 with 15% performance improvement.",
    source: 'Rule Management',
    sourceId: 'rule-002',
    timestamp: '2024-12-29T16:30:00Z',
    status: 'resolved',
    metadata: {
      previousVersion: '2.0.0',
      newVersion: '2.0.1',
      performanceImprovement: '15%'
    }
  }
]

export function ComplianceAlerts() {
  const [alerts, setAlerts] = useState<ComplianceAlert[]>(mockAlerts)
  const [filter, setFilter] = useState<
    'all' | 'active' | 'high' | 'violations'
  >('all')

  const getAlertIcon = (type: string) => {
    switch (type) {
      case 'violation':
        return <XCircle className="h-4 w-4" />
      case 'warning':
        return <AlertTriangle className="h-4 w-4" />
      case 'info':
        return <Info className="h-4 w-4" />
      case 'success':
        return <CheckCircle className="h-4 w-4" />
      default:
        return <Info className="h-4 w-4" />
    }
  }

  const getAlertVariant = (type: string): 'default' | 'destructive' => {
    switch (type) {
      case 'violation':
        return 'destructive'
      case 'warning':
        return 'default'
      case 'info':
        return 'default'
      case 'success':
        return 'default'
      default:
        return 'default'
    }
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

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active':
        return 'destructive'
      case 'acknowledged':
        return 'default'
      case 'resolved':
        return 'secondary'
      default:
        return 'outline'
    }
  }

  const handleDismissAlert = (alertId: string) => {
    setAlerts((prev) => prev.filter((alert) => alert.id !== alertId))
  }

  const handleAcknowledgeAlert = (alertId: string) => {
    setAlerts((prev) =>
      prev.map((alert) =>
        alert.id === alertId
          ? { ...alert, status: 'acknowledged' as const }
          : alert
      )
    )
  }

  const filteredAlerts = alerts.filter((alert) => {
    switch (filter) {
      case 'active':
        return alert.status === 'active'
      case 'high':
        return alert.severity === 'high'
      case 'violations':
        return alert.type === 'violation'
      default:
        return true
    }
  })

  const alertCounts = {
    total: alerts.length,
    active: alerts.filter((a) => a.status === 'active').length,
    high: alerts.filter((a) => a.severity === 'high').length,
    violations: alerts.filter((a) => a.type === 'violation').length
  }

  return (
    <div className="space-y-4">
      {/* Alert Summary */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <AlertTriangle className="h-5 w-5" />
                Compliance Alerts
              </CardTitle>
              <CardDescription>
                Current compliance alerts and system notifications
              </CardDescription>
            </div>
            <Button variant="outline" size="sm">
              <RefreshCw className="mr-2 h-4 w-4" />
              Refresh
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {/* Alert Statistics */}
          <div className="mb-4 grid gap-4 md:grid-cols-4">
            <div className="text-center">
              <div className="text-2xl font-bold">{alertCounts.total}</div>
              <p className="text-sm text-muted-foreground">Total Alerts</p>
            </div>
            <div className="text-center">
              <div className="text-2xl font-bold text-red-600">
                {alertCounts.active}
              </div>
              <p className="text-sm text-muted-foreground">Active</p>
            </div>
            <div className="text-center">
              <div className="text-2xl font-bold text-orange-600">
                {alertCounts.high}
              </div>
              <p className="text-sm text-muted-foreground">High Severity</p>
            </div>
            <div className="text-center">
              <div className="text-2xl font-bold text-red-600">
                {alertCounts.violations}
              </div>
              <p className="text-sm text-muted-foreground">Violations</p>
            </div>
          </div>

          {/* Filter Buttons */}
          <div className="mb-4 flex gap-2">
            {[
              { key: 'all', label: 'All', count: alertCounts.total },
              { key: 'active', label: 'Active', count: alertCounts.active },
              { key: 'high', label: 'High Priority', count: alertCounts.high },
              {
                key: 'violations',
                label: 'Violations',
                count: alertCounts.violations
              }
            ].map((filterOption) => (
              <Button
                key={filterOption.key}
                variant={filter === filterOption.key ? 'default' : 'outline'}
                size="sm"
                onClick={() => setFilter(filterOption.key as any)}
              >
                {filterOption.label} ({filterOption.count})
              </Button>
            ))}
          </div>

          {/* Alerts List */}
          <div className="space-y-3">
            {filteredAlerts.map((alert) => (
              <Alert
                key={alert.id}
                variant={getAlertVariant(alert.type)}
                className="relative"
              >
                <div className="flex items-start gap-3">
                  {getAlertIcon(alert.type)}
                  <div className="flex-1 space-y-1">
                    <div className="flex items-center justify-between">
                      <AlertTitle className="flex items-center gap-2">
                        {alert.title}
                        <div className="flex items-center gap-1">
                          <Badge variant={getSeverityColor(alert.severity)}>
                            {alert.severity}
                          </Badge>
                          <Badge variant={getStatusColor(alert.status)}>
                            {alert.status}
                          </Badge>
                        </div>
                      </AlertTitle>
                      <div className="flex items-center gap-1">
                        {alert.status === 'active' && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleAcknowledgeAlert(alert.id)}
                          >
                            <Eye className="h-4 w-4" />
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleDismissAlert(alert.id)}
                        >
                          <X className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                    <AlertDescription>{alert.message}</AlertDescription>

                    {/* Alert Details */}
                    <div className="space-y-2 text-sm">
                      <div className="flex items-center gap-4 text-muted-foreground">
                        <span className="flex items-center gap-1">
                          <Clock className="h-3 w-3" />
                          {new Date(alert.timestamp).toLocaleString()}
                        </span>
                        <span>Source: {alert.source}</span>
                        {alert.sourceId && <span>ID: {alert.sourceId}</span>}
                      </div>

                      {alert.details && (
                        <p className="rounded bg-muted p-2 text-xs text-muted-foreground">
                          {alert.details}
                        </p>
                      )}

                      {alert.metadata && (
                        <div className="grid gap-1 text-xs md:grid-cols-2">
                          {Object.entries(alert.metadata).map(
                            ([key, value]) => (
                              <div key={key} className="flex justify-between">
                                <span className="font-medium">{key}:</span>
                                <span className="text-muted-foreground">
                                  {typeof value === 'object'
                                    ? JSON.stringify(value)
                                    : String(value)}
                                </span>
                              </div>
                            )
                          )}
                        </div>
                      )}

                      {alert.actions && alert.actions.length > 0 && (
                        <div className="flex flex-wrap gap-1">
                          {alert.actions.map((action, index) => (
                            <Button key={index} variant="outline" size="sm">
                              {action.replace('_', ' ').toLowerCase()}
                            </Button>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </Alert>
            ))}

            {filteredAlerts.length === 0 && (
              <div className="py-8 text-center">
                <CheckCircle className="mx-auto mb-2 h-12 w-12 text-green-500" />
                <h3 className="font-medium text-green-600">All Clear!</h3>
                <p className="text-sm text-muted-foreground">
                  No {filter !== 'all' ? `${filter} ` : ''}alerts at this time.
                </p>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
