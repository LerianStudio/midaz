'use client'

import { useState } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { ValidationRulesPanel } from '@/components/accounting/compliance/validation-rules-panel'
import {
  Plus,
  Play,
  Pause,
  RefreshCw,
  Settings,
  CheckCircle,
  XCircle,
  AlertTriangle,
  Info
} from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

// Mock validation rules data
const validationRules = [
  {
    id: 'rule-001',
    name: 'Account Type Key Value Uniqueness',
    description:
      'Ensures all account type key values are unique across the system',
    category: 'account_types',
    severity: 'high' as const,
    enabled: true,
    lastExecuted: '2024-12-30T14:20:00Z',
    lastResult: 'passed' as const,
    executionCount: 1247,
    passRate: 99.2,
    condition: {
      type: 'uniqueness_check',
      field: 'keyValue',
      scope: 'global'
    },
    actions: ['block_creation', 'log_violation', 'send_alert'],
    metadata: {
      createdBy: 'system',
      lastModified: '2024-11-15T10:30:00Z',
      version: '1.2.0'
    }
  },
  {
    id: 'rule-002',
    name: 'Transaction Route Operation Balance',
    description:
      'Validates that debit and credit operations in a route are balanced',
    category: 'transaction_routes',
    severity: 'high' as const,
    enabled: true,
    lastExecuted: '2024-12-30T14:15:00Z',
    lastResult: 'passed' as const,
    executionCount: 2856,
    passRate: 98.7,
    condition: {
      type: 'balance_check',
      operation: 'sum_operations',
      tolerance: 0.01
    },
    actions: ['block_creation', 'require_approval', 'log_violation'],
    metadata: {
      createdBy: 'user-001',
      lastModified: '2024-12-01T16:45:00Z',
      version: '2.0.1'
    }
  },
  {
    id: 'rule-003',
    name: 'Domain Consistency Validation',
    description:
      'Ensures operation routes reference accounts within compatible domains',
    category: 'operation_routes',
    severity: 'medium' as const,
    enabled: true,
    lastExecuted: '2024-12-30T14:10:00Z',
    lastResult: 'warning' as const,
    executionCount: 1532,
    passRate: 95.4,
    condition: {
      type: 'domain_compatibility',
      allowedCombinations: ['ledger-ledger', 'ledger-external'],
      blockedCombinations: ['external-external']
    },
    actions: ['show_warning', 'log_violation', 'require_confirmation'],
    metadata: {
      createdBy: 'user-002',
      lastModified: '2024-11-20T09:15:00Z',
      version: '1.0.3'
    }
  },
  {
    id: 'rule-004',
    name: 'Account Type Name Format',
    description: 'Validates account type names follow naming conventions',
    category: 'account_types',
    severity: 'low' as const,
    enabled: true,
    lastExecuted: '2024-12-30T14:05:00Z',
    lastResult: 'passed' as const,
    executionCount: 892,
    passRate: 97.8,
    condition: {
      type: 'regex_match',
      pattern: '^[A-Z][a-zA-Z0-9\\s]{2,49}$',
      field: 'name'
    },
    actions: ['show_warning', 'log_violation'],
    metadata: {
      createdBy: 'user-001',
      lastModified: '2024-10-15T14:20:00Z',
      version: '1.1.0'
    }
  },
  {
    id: 'rule-005',
    name: 'Maximum Operations per Route',
    description:
      'Limits the number of operations in a single transaction route',
    category: 'transaction_routes',
    severity: 'medium' as const,
    enabled: false,
    lastExecuted: '2024-12-29T12:30:00Z',
    lastResult: 'passed' as const,
    executionCount: 234,
    passRate: 100.0,
    condition: {
      type: 'count_limit',
      field: 'operationRoutes',
      maxValue: 10
    },
    actions: ['block_creation', 'show_warning'],
    metadata: {
      createdBy: 'user-003',
      lastModified: '2024-11-10T11:00:00Z',
      version: '1.0.0'
    }
  },
  {
    id: 'rule-006',
    name: 'Fee Calculation Validation',
    description:
      'Validates fee calculation expressions are mathematically sound',
    category: 'operation_routes',
    severity: 'high' as const,
    enabled: true,
    lastExecuted: '2024-12-30T13:55:00Z',
    lastResult: 'failed' as const,
    executionCount: 456,
    passRate: 92.1,
    condition: {
      type: 'expression_validation',
      field: 'amount.expression',
      allowedVariables: ['amount', 'fee_rate', 'base_amount'],
      allowedOperators: ['+', '-', '*', '/', '(', ')']
    },
    actions: ['block_creation', 'log_violation', 'send_alert'],
    metadata: {
      createdBy: 'user-002',
      lastModified: '2024-12-15T08:30:00Z',
      version: '1.3.2'
    }
  }
]

const ruleCategories = [
  { id: 'all', name: 'All Rules', count: validationRules.length },
  {
    id: 'account_types',
    name: 'Account Types',
    count: validationRules.filter((r) => r.category === 'account_types').length
  },
  {
    id: 'transaction_routes',
    name: 'Transaction Routes',
    count: validationRules.filter((r) => r.category === 'transaction_routes')
      .length
  },
  {
    id: 'operation_routes',
    name: 'Operation Routes',
    count: validationRules.filter((r) => r.category === 'operation_routes')
      .length
  }
]

const ruleStats = {
  total: validationRules.length,
  enabled: validationRules.filter((r) => r.enabled).length,
  passed: validationRules.filter((r) => r.lastResult === 'passed').length,
  failed: validationRules.filter((r) => r.lastResult === 'failed').length,
  warnings: validationRules.filter((r) => r.lastResult === 'warning').length,
  avgPassRate:
    validationRules.reduce((sum, r) => sum + r.passRate, 0) /
    validationRules.length
}

export default function ValidationRulesPage() {
  const [selectedCategory, setSelectedCategory] = useState('all')
  const [isRunningValidation, setIsRunningValidation] = useState(false)

  const filteredRules =
    selectedCategory === 'all'
      ? validationRules
      : validationRules.filter((rule) => rule.category === selectedCategory)

  const handleRunValidation = async () => {
    setIsRunningValidation(true)
    // Simulate validation run
    await new Promise((resolve) => setTimeout(resolve, 2000))
    setIsRunningValidation(false)
  }

  const handleToggleRule = (ruleId: string, enabled: boolean) => {
    // Update rule enabled state
    console.log(`Toggle rule ${ruleId} to ${enabled}`)
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

  const getResultIcon = (result: string) => {
    switch (result) {
      case 'passed':
        return <CheckCircle className="h-4 w-4 text-green-500" />
      case 'failed':
        return <XCircle className="h-4 w-4 text-red-500" />
      case 'warning':
        return <AlertTriangle className="h-4 w-4 text-yellow-500" />
      default:
        return <Info className="h-4 w-4 text-gray-500" />
    }
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">
            Validation Rules
          </h1>
          <p className="text-muted-foreground">
            Configure and manage validation rules for accounting operations
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleRunValidation}
            disabled={isRunningValidation}
          >
            <Play
              className={`mr-2 h-4 w-4 ${isRunningValidation ? 'animate-spin' : ''}`}
            />
            {isRunningValidation ? 'Running...' : 'Run Validation'}
          </Button>
          <Button size="sm">
            <Plus className="mr-2 h-4 w-4" />
            Create Rule
          </Button>
        </div>
      </div>

      {/* Stats Overview */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-5">
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">{ruleStats.total}</div>
            <p className="text-xs text-muted-foreground">Total Rules</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold text-green-600">
              {ruleStats.enabled}
            </div>
            <p className="text-xs text-muted-foreground">Enabled</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold text-green-500">
              {ruleStats.passed}
            </div>
            <p className="text-xs text-muted-foreground">Passing</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold text-red-500">
              {ruleStats.failed}
            </div>
            <p className="text-xs text-muted-foreground">Failing</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">
              {ruleStats.avgPassRate.toFixed(1)}%
            </div>
            <p className="text-xs text-muted-foreground">Avg Pass Rate</p>
          </CardContent>
        </Card>
      </div>

      {/* Main Content */}
      <Tabs value={selectedCategory} onValueChange={setSelectedCategory}>
        <TabsList>
          {ruleCategories.map((category) => (
            <TabsTrigger key={category.id} value={category.id}>
              {category.name} ({category.count})
            </TabsTrigger>
          ))}
        </TabsList>

        <TabsContent value={selectedCategory} className="space-y-6">
          {/* Rules List */}
          <div className="space-y-4">
            {filteredRules.map((rule) => (
              <Card key={rule.id}>
                <CardHeader>
                  <div className="flex items-start justify-between">
                    <div className="space-y-1">
                      <CardTitle className="flex items-center gap-3">
                        {rule.name}
                        <div className="flex items-center gap-2">
                          <Badge variant={getSeverityColor(rule.severity)}>
                            {rule.severity}
                          </Badge>
                          <div className="flex items-center gap-1">
                            {getResultIcon(rule.lastResult)}
                            <span className="text-sm">{rule.lastResult}</span>
                          </div>
                        </div>
                      </CardTitle>
                      <CardDescription>{rule.description}</CardDescription>
                    </div>
                    <div className="flex items-center gap-2">
                      <Switch
                        checked={rule.enabled}
                        onCheckedChange={(enabled) =>
                          handleToggleRule(rule.id, enabled)
                        }
                      />
                      <Button variant="ghost" size="sm">
                        <Settings className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
                    <div>
                      <p className="text-sm font-medium">Execution Count</p>
                      <p className="text-2xl font-bold">
                        {rule.executionCount.toLocaleString()}
                      </p>
                    </div>
                    <div>
                      <p className="text-sm font-medium">Pass Rate</p>
                      <p className="text-2xl font-bold text-green-600">
                        {rule.passRate}%
                      </p>
                    </div>
                    <div>
                      <p className="text-sm font-medium">Last Executed</p>
                      <p className="text-sm text-muted-foreground">
                        {new Date(rule.lastExecuted).toLocaleString()}
                      </p>
                    </div>
                    <div>
                      <p className="text-sm font-medium">Version</p>
                      <p className="text-sm text-muted-foreground">
                        {rule.metadata.version}
                      </p>
                    </div>
                  </div>

                  {/* Rule Actions */}
                  <div className="mt-4">
                    <p className="mb-2 text-sm font-medium">Actions</p>
                    <div className="flex flex-wrap gap-2">
                      {rule.actions.map((action, index) => (
                        <Badge key={index} variant="outline">
                          {action.replace('_', ' ')}
                        </Badge>
                      ))}
                    </div>
                  </div>

                  {/* Rule Condition Preview */}
                  <div className="mt-4 rounded-lg bg-muted p-3">
                    <p className="mb-1 text-sm font-medium">Condition</p>
                    <code className="text-xs">
                      {JSON.stringify(rule.condition, null, 2)}
                    </code>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

          {filteredRules.length === 0 && (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-muted-foreground">
                  No validation rules found for this category.
                </p>
                <Button className="mt-4">
                  <Plus className="mr-2 h-4 w-4" />
                  Create First Rule
                </Button>
              </CardContent>
            </Card>
          )}
        </TabsContent>
      </Tabs>

      {/* Validation Rules Panel Component */}
      <ValidationRulesPanel rules={filteredRules} />
    </div>
  )
}
