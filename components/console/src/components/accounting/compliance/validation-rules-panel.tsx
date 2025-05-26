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
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  AlertTriangle,
  CheckCircle,
  XCircle,
  Play,
  Settings,
  Plus,
  Edit,
  Trash2,
  TestTube
} from 'lucide-react'

interface ValidationRule {
  id: string
  name: string
  description: string
  category: string
  severity: 'high' | 'medium' | 'low'
  enabled: boolean
  lastExecuted: string
  lastResult: 'passed' | 'failed' | 'warning'
  executionCount: number
  passRate: number
  condition: Record<string, any>
  actions: string[]
  metadata: {
    createdBy: string
    lastModified: string
    version: string
  }
}

interface ValidationRulesPanelProps {
  rules: ValidationRule[]
}

export function ValidationRulesPanel({ rules }: ValidationRulesPanelProps) {
  const [selectedRule, setSelectedRule] = useState<ValidationRule | null>(null)
  const [isTestDialogOpen, setIsTestDialogOpen] = useState(false)
  const [testResult, setTestResult] = useState<any>(null)
  const [isTestRunning, setIsTestRunning] = useState(false)

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
        return null
    }
  }

  const handleTestRule = async (rule: ValidationRule) => {
    setIsTestRunning(true)
    setSelectedRule(rule)
    setIsTestDialogOpen(true)

    // Simulate rule testing
    await new Promise((resolve) => setTimeout(resolve, 2000))

    // Mock test result
    const mockResult = {
      ruleId: rule.id,
      success: Math.random() > 0.3,
      executionTime: Math.random() * 1000 + 100,
      message:
        Math.random() > 0.3
          ? 'Rule validation passed successfully'
          : 'Rule validation failed: Invalid condition detected',
      details: {
        testedConditions: Object.keys(rule.condition).length,
        passedConditions:
          Math.floor(Math.random() * Object.keys(rule.condition).length) + 1,
        warnings:
          Math.random() > 0.7 ? ['Potential performance impact detected'] : []
      }
    }

    setTestResult(mockResult)
    setIsTestRunning(false)
  }

  const handleToggleRule = (ruleId: string, enabled: boolean) => {
    console.log(`Toggle rule ${ruleId} to ${enabled}`)
  }

  const handleDeleteRule = (ruleId: string) => {
    console.log(`Delete rule ${ruleId}`)
  }

  return (
    <div className="space-y-6">
      {/* Test Dialog */}
      <Dialog open={isTestDialogOpen} onOpenChange={setIsTestDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Test Validation Rule</DialogTitle>
            <DialogDescription>
              Testing rule: {selectedRule?.name}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            {isTestRunning ? (
              <div className="py-8 text-center">
                <div className="mx-auto mb-4 animate-spin">
                  <TestTube className="h-8 w-8" />
                </div>
                <p>Running validation test...</p>
              </div>
            ) : testResult ? (
              <div className="space-y-4">
                <div className="flex items-center gap-2">
                  {testResult.success ? (
                    <CheckCircle className="h-5 w-5 text-green-500" />
                  ) : (
                    <XCircle className="h-5 w-5 text-red-500" />
                  )}
                  <span
                    className={`font-medium ${
                      testResult.success ? 'text-green-600' : 'text-red-600'
                    }`}
                  >
                    {testResult.success ? 'Test Passed' : 'Test Failed'}
                  </span>
                </div>

                <p className="text-sm text-muted-foreground">
                  {testResult.message}
                </p>

                <div className="grid gap-4 md:grid-cols-2">
                  <div>
                    <p className="text-sm font-medium">Execution Time</p>
                    <p className="text-sm text-muted-foreground">
                      {testResult.executionTime.toFixed(0)}ms
                    </p>
                  </div>
                  <div>
                    <p className="text-sm font-medium">Conditions Tested</p>
                    <p className="text-sm text-muted-foreground">
                      {testResult.details.passedConditions}/
                      {testResult.details.testedConditions}
                    </p>
                  </div>
                </div>

                {testResult.details.warnings.length > 0 && (
                  <div>
                    <p className="text-sm font-medium">Warnings</p>
                    <ul className="text-sm text-muted-foreground">
                      {testResult.details.warnings.map(
                        (warning: string, index: number) => (
                          <li key={index}>• {warning}</li>
                        )
                      )}
                    </ul>
                  </div>
                )}
              </div>
            ) : null}
          </div>
        </DialogContent>
      </Dialog>

      {/* Rule Testing Panel */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <TestTube className="h-5 w-5" />
            Rule Testing
          </CardTitle>
          <CardDescription>
            Test validation rules against sample data or custom conditions
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="grid gap-4 md:grid-cols-2">
              <div>
                <label className="text-sm font-medium">Test Data Type</label>
                <Select defaultValue="sample">
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="sample">Sample Data</SelectItem>
                    <SelectItem value="custom">Custom Data</SelectItem>
                    <SelectItem value="production">
                      Production Sample
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <label className="text-sm font-medium">Test Scope</label>
                <Select defaultValue="all">
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Rules</SelectItem>
                    <SelectItem value="enabled">Enabled Rules Only</SelectItem>
                    <SelectItem value="failed">Previously Failed</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div>
              <label className="text-sm font-medium">
                Custom Test Data (JSON)
              </label>
              <Textarea
                placeholder="Enter custom test data in JSON format..."
                className="mt-1"
                rows={4}
              />
            </div>

            <div className="flex gap-2">
              <Button>
                <Play className="mr-2 h-4 w-4" />
                Run All Tests
              </Button>
              <Button variant="outline">
                <TestTube className="mr-2 h-4 w-4" />
                Validate Configuration
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Quick Actions */}
      <Card>
        <CardHeader>
          <CardTitle>Quick Actions</CardTitle>
          <CardDescription>
            Common operations for validation rule management
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 md:grid-cols-3">
            <Button variant="outline" className="h-auto p-4">
              <div className="text-center">
                <Plus className="mx-auto mb-2 h-6 w-6" />
                <div className="font-medium">Create Rule</div>
                <div className="text-sm text-muted-foreground">
                  Add new validation rule
                </div>
              </div>
            </Button>

            <Button variant="outline" className="h-auto p-4">
              <div className="text-center">
                <Settings className="mx-auto mb-2 h-6 w-6" />
                <div className="font-medium">Bulk Operations</div>
                <div className="text-sm text-muted-foreground">
                  Enable/disable multiple rules
                </div>
              </div>
            </Button>

            <Button variant="outline" className="h-auto p-4">
              <div className="text-center">
                <TestTube className="mx-auto mb-2 h-6 w-6" />
                <div className="font-medium">Run Diagnostics</div>
                <div className="text-sm text-muted-foreground">
                  Check rule performance
                </div>
              </div>
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Rule Templates */}
      <Card>
        <CardHeader>
          <CardTitle>Rule Templates</CardTitle>
          <CardDescription>
            Pre-built validation rule templates for common scenarios
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 md:grid-cols-2">
            {[
              {
                name: 'Unique Key Value',
                description: 'Ensures account type key values are unique',
                category: 'account_types',
                difficulty: 'Beginner'
              },
              {
                name: 'Balance Validation',
                description: 'Validates transaction route balance',
                category: 'transaction_routes',
                difficulty: 'Intermediate'
              },
              {
                name: 'Domain Consistency',
                description: 'Checks operation route domain compatibility',
                category: 'operation_routes',
                difficulty: 'Advanced'
              },
              {
                name: 'Amount Range Check',
                description: 'Validates amount expressions and ranges',
                category: 'operation_routes',
                difficulty: 'Intermediate'
              }
            ].map((template, index) => (
              <div key={index} className="space-y-2 rounded-lg border p-4">
                <div className="flex items-center justify-between">
                  <h4 className="font-medium">{template.name}</h4>
                  <Badge variant="outline">{template.difficulty}</Badge>
                </div>
                <p className="text-sm text-muted-foreground">
                  {template.description}
                </p>
                <div className="flex items-center justify-between">
                  <Badge variant="secondary">{template.category}</Badge>
                  <Button size="sm" variant="outline">
                    Use Template
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Rule Performance */}
      <Card>
        <CardHeader>
          <CardTitle>Rule Performance</CardTitle>
          <CardDescription>
            Performance metrics for validation rules
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {rules.slice(0, 5).map((rule) => (
              <div
                key={rule.id}
                className="flex items-center justify-between rounded-lg border p-3"
              >
                <div className="flex items-center gap-3">
                  <div className="flex items-center gap-2">
                    {getResultIcon(rule.lastResult)}
                    <Badge variant={getSeverityColor(rule.severity)}>
                      {rule.severity}
                    </Badge>
                  </div>
                  <div>
                    <p className="font-medium">{rule.name}</p>
                    <p className="text-sm text-muted-foreground">
                      {rule.executionCount.toLocaleString()} executions
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-4">
                  <div className="text-right">
                    <p className="font-medium text-green-600">
                      {rule.passRate}%
                    </p>
                    <p className="text-sm text-muted-foreground">Pass Rate</p>
                  </div>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleTestRule(rule)}
                    >
                      <TestTube className="h-4 w-4" />
                    </Button>
                    <Switch
                      checked={rule.enabled}
                      onCheckedChange={(enabled) =>
                        handleToggleRule(rule.id, enabled)
                      }
                    />
                  </div>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
