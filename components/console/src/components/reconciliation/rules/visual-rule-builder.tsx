'use client'

import { useState, useEffect } from 'react'
import {
  Plus,
  X,
  Settings,
  Play,
  Save,
  Copy,
  Target,
  Zap,
  FileText,
  Calendar,
  DollarSign,
  Hash,
  Type,
  ToggleLeft,
  ArrowRight,
  CheckCircle,
  AlertTriangle,
  Brain,
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Slider } from '@/components/ui/slider'
import { Switch } from '@/components/ui/switch'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'

import {
  mockReconciliationRules,
  ReconciliationRule
} from '@/lib/mock-data/reconciliation-unified'

interface RuleCriteria {
  field: string
  operator: string
  value?: any
  tolerance?: number
  similarity_threshold?: number
  case_sensitive?: boolean
  date_tolerance_days?: number
  amount_tolerance_percent?: number
  additionalFields?: string[]
}

interface RuleDefinition {
  id?: string
  name: string
  description: string
  ruleType: 'amount' | 'date' | 'string' | 'regex' | 'metadata' | 'composite'
  criteria: RuleCriteria
  priority: number
  isActive: boolean
  performance?: {
    matchCount: number
    successRate: number
    averageConfidence: number
    executionTime: string
  }
}

interface TestResult {
  success: boolean
  matches: number
  executionTime: string
  sampleMatches: Array<{
    externalTxn: any
    internalTxn: any
    confidence: number
    matchedFields: string[]
  }>
  errors: string[]
  warnings: string[]
}

interface VisualRuleBuilderProps {
  ruleId?: string
  onSave?: (rule: RuleDefinition) => void
  onTest?: (rule: RuleDefinition) => Promise<TestResult>
  className?: string
}

export function VisualRuleBuilder({
  ruleId,
  onSave,
  onTest,
  className
}: VisualRuleBuilderProps) {
  const [rule, setRule] = useState<RuleDefinition>({
    name: '',
    description: '',
    ruleType: 'amount',
    criteria: {
      field: 'amount',
      operator: 'equals',
      tolerance: 0.01
    },
    priority: 1,
    isActive: true
  })
  const [testResult, setTestResult] = useState<TestResult | null>(null)
  const [isTestingRule, setIsTestingRule] = useState(false)
  const [showAdvancedOptions, setShowAdvancedOptions] = useState(false)

  useEffect(() => {
    if (ruleId) {
      // Load existing rule
      const existingRule = mockReconciliationRules.find((r) => r.id === ruleId)
      if (existingRule) {
        setRule({
          id: existingRule.id,
          name: existingRule.name,
          description: existingRule.description,
          ruleType: existingRule.ruleType,
          criteria: existingRule.criteria,
          priority: existingRule.priority,
          isActive: existingRule.isActive,
          performance: existingRule.performance
        })
      }
    }
  }, [ruleId])

  const handleFieldChange = (field: string, value: any) => {
    setRule((prev) => ({
      ...prev,
      criteria: {
        ...prev.criteria,
        [field]: value
      }
    }))
  }

  const handleTestRule = async () => {
    setIsTestingRule(true)
    try {
      if (onTest) {
        const result = await onTest(rule)
        setTestResult(result)
      } else {
        // Mock test result
        await new Promise((resolve) => setTimeout(resolve, 2000))
        setTestResult({
          success: true,
          matches: Math.floor(Math.random() * 100) + 50,
          executionTime: `${Math.floor(Math.random() * 200) + 50}ms`,
          sampleMatches: [
            {
              externalTxn: {
                id: '1',
                amount: 1000,
                description: 'Payment ABC'
              },
              internalTxn: {
                id: '2',
                amount: 1000,
                description: 'Payment ABC'
              },
              confidence: 0.95,
              matchedFields: ['amount', 'description']
            }
          ],
          errors: [],
          warnings:
            rule.criteria.tolerance && rule.criteria.tolerance > 0.05
              ? ['High tolerance value may result in false positives']
              : []
        })
      }
    } catch (error) {
      setTestResult({
        success: false,
        matches: 0,
        executionTime: '0ms',
        sampleMatches: [],
        errors: ['Failed to execute rule test'],
        warnings: []
      })
    } finally {
      setIsTestingRule(false)
    }
  }

  const handleSaveRule = () => {
    if (onSave) {
      onSave(rule)
    }
  }

  const getFieldIcon = (fieldType: string) => {
    switch (fieldType) {
      case 'amount':
        return DollarSign
      case 'date':
        return Calendar
      case 'description':
        return FileText
      case 'reference':
        return Hash
      case 'metadata':
        return Settings
      default:
        return Type
    }
  }

  const getOperatorOptions = (ruleType: string) => {
    switch (ruleType) {
      case 'amount':
        return [
          { value: 'equals', label: 'Equals' },
          { value: 'greater_than', label: 'Greater Than' },
          { value: 'less_than', label: 'Less Than' },
          { value: 'range', label: 'Within Range' }
        ]
      case 'date':
        return [
          { value: 'equals', label: 'Exact Date' },
          { value: 'range', label: 'Date Range' },
          { value: 'within_days', label: 'Within Days' }
        ]
      case 'string':
        return [
          { value: 'equals', label: 'Exact Match' },
          { value: 'contains', label: 'Contains' },
          { value: 'starts_with', label: 'Starts With' },
          { value: 'ends_with', label: 'Ends With' },
          { value: 'fuzzy_match', label: 'Fuzzy Match' }
        ]
      case 'regex':
        return [{ value: 'regex', label: 'Regular Expression' }]
      case 'metadata':
        return [
          { value: 'key_exists', label: 'Key Exists' },
          { value: 'key_value_match', label: 'Key-Value Match' }
        ]
      default:
        return [{ value: 'equals', label: 'Equals' }]
    }
  }

  const getFieldOptions = (ruleType: string) => {
    const commonFields = [
      { value: 'amount', label: 'Amount', icon: DollarSign },
      { value: 'date', label: 'Date', icon: Calendar },
      { value: 'description', label: 'Description', icon: FileText },
      { value: 'reference', label: 'Reference', icon: Hash }
    ]

    switch (ruleType) {
      case 'metadata':
        return [
          ...commonFields,
          { value: 'metadata', label: 'Metadata', icon: Settings },
          { value: 'custom_field', label: 'Custom Field', icon: Type }
        ]
      default:
        return commonFields
    }
  }

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Rule Header */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Target className="h-5 w-5 text-blue-600" />
            {ruleId ? 'Edit Rule' : 'Create New Rule'}
          </CardTitle>
          <CardDescription>
            Build reconciliation rules using visual interface with real-time
            testing
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-6">
            {/* Basic Information */}
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="ruleName">Rule Name</Label>
                <Input
                  id="ruleName"
                  placeholder="Enter rule name..."
                  value={rule.name}
                  onChange={(e) =>
                    setRule((prev) => ({ ...prev, name: e.target.value }))
                  }
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="ruleType">Rule Type</Label>
                <Select
                  value={rule.ruleType}
                  onValueChange={(value: any) =>
                    setRule((prev) => ({
                      ...prev,
                      ruleType: value,
                      criteria: {
                        field: value === 'amount' ? 'amount' : 'description',
                        operator: 'equals'
                      }
                    }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="amount">Amount Matching</SelectItem>
                    <SelectItem value="date">Date Matching</SelectItem>
                    <SelectItem value="string">String Matching</SelectItem>
                    <SelectItem value="regex">Regular Expression</SelectItem>
                    <SelectItem value="metadata">Metadata Matching</SelectItem>
                    <SelectItem value="composite">Composite Rule</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea
                id="description"
                placeholder="Describe what this rule does..."
                value={rule.description}
                onChange={(e) =>
                  setRule((prev) => ({ ...prev, description: e.target.value }))
                }
                rows={2}
              />
            </div>

            {/* Priority and Status */}
            <div className="flex items-center gap-6">
              <div className="space-y-2">
                <Label>Priority: {rule.priority}</Label>
                <Slider
                  value={[rule.priority]}
                  onValueChange={([value]) =>
                    setRule((prev) => ({ ...prev, priority: value }))
                  }
                  max={10}
                  min={1}
                  step={1}
                  className="w-32"
                />
              </div>
              <div className="flex items-center space-x-2">
                <Switch
                  id="isActive"
                  checked={rule.isActive}
                  onCheckedChange={(checked) =>
                    setRule((prev) => ({ ...prev, isActive: checked }))
                  }
                />
                <Label htmlFor="isActive">Rule Active</Label>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Rule Configuration */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Settings className="h-5 w-5 text-purple-600" />
            Rule Configuration
          </CardTitle>
          <CardDescription>
            Configure matching criteria and conditions
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-6">
            {/* Primary Field Selection */}
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label>Primary Field</Label>
                <Select
                  value={rule.criteria.field}
                  onValueChange={(value) => handleFieldChange('field', value)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {getFieldOptions(rule.ruleType).map((field) => {
                      const IconComponent = field.icon
                      return (
                        <SelectItem key={field.value} value={field.value}>
                          <div className="flex items-center gap-2">
                            <IconComponent className="h-4 w-4" />
                            {field.label}
                          </div>
                        </SelectItem>
                      )
                    })}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Operator</Label>
                <Select
                  value={rule.criteria.operator}
                  onValueChange={(value) =>
                    handleFieldChange('operator', value)
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {getOperatorOptions(rule.ruleType).map((op) => (
                      <SelectItem key={op.value} value={op.value}>
                        {op.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Operator-specific configuration */}
            {rule.criteria.operator === 'equals' &&
              rule.ruleType === 'amount' && (
                <div className="space-y-2">
                  <Label>Amount Tolerance (%)</Label>
                  <div className="flex items-center gap-4">
                    <Slider
                      value={[rule.criteria.tolerance || 0.01]}
                      onValueChange={([value]) =>
                        handleFieldChange('tolerance', value)
                      }
                      max={0.1}
                      min={0}
                      step={0.001}
                      className="flex-1"
                    />
                    <span className="w-16 font-mono text-sm">
                      {((rule.criteria.tolerance || 0.01) * 100).toFixed(1)}%
                    </span>
                  </div>
                </div>
              )}

            {rule.criteria.operator === 'fuzzy_match' && (
              <div className="space-y-4">
                <div className="space-y-2">
                  <Label>Similarity Threshold</Label>
                  <div className="flex items-center gap-4">
                    <Slider
                      value={[rule.criteria.similarity_threshold || 0.8]}
                      onValueChange={([value]) =>
                        handleFieldChange('similarity_threshold', value)
                      }
                      max={1}
                      min={0.5}
                      step={0.01}
                      className="flex-1"
                    />
                    <span className="w-16 font-mono text-sm">
                      {(
                        (rule.criteria.similarity_threshold || 0.8) * 100
                      ).toFixed(0)}
                      %
                    </span>
                  </div>
                </div>
                <div className="flex items-center space-x-2">
                  <Switch
                    id="caseSensitive"
                    checked={rule.criteria.case_sensitive || false}
                    onCheckedChange={(checked) =>
                      handleFieldChange('case_sensitive', checked)
                    }
                  />
                  <Label htmlFor="caseSensitive">Case Sensitive</Label>
                </div>
              </div>
            )}

            {rule.criteria.operator === 'within_days' && (
              <div className="space-y-2">
                <Label>Date Tolerance (Days)</Label>
                <Input
                  type="number"
                  value={rule.criteria.date_tolerance_days || 1}
                  onChange={(e) =>
                    handleFieldChange(
                      'date_tolerance_days',
                      parseInt(e.target.value)
                    )
                  }
                  min={0}
                  max={30}
                />
              </div>
            )}

            {rule.criteria.operator === 'regex' && (
              <div className="space-y-2">
                <Label>Regular Expression Pattern</Label>
                <Input
                  placeholder="Enter regex pattern..."
                  value={rule.criteria.value || ''}
                  onChange={(e) => handleFieldChange('value', e.target.value)}
                  className="font-mono"
                />
              </div>
            )}

            {/* Advanced Options */}
            <div className="space-y-4">
              <Button
                variant="outline"
                onClick={() => setShowAdvancedOptions(!showAdvancedOptions)}
                className="w-full"
              >
                <Settings className="mr-2 h-4 w-4" />
                {showAdvancedOptions ? 'Hide' : 'Show'} Advanced Options
              </Button>

              {showAdvancedOptions && (
                <div className="space-y-4 rounded-lg bg-gray-50 p-4">
                  <h4 className="font-semibold">Additional Matching Fields</h4>
                  <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
                    {[
                      'amount',
                      'date',
                      'description',
                      'reference',
                      'account_number'
                    ].map((field) => (
                      <div key={field} className="flex items-center space-x-2">
                        <Switch
                          id={`additional-${field}`}
                          checked={
                            rule.criteria.additionalFields?.includes(field) ||
                            false
                          }
                          onCheckedChange={(checked) => {
                            const currentFields =
                              rule.criteria.additionalFields || []
                            const newFields = checked
                              ? [...currentFields, field]
                              : currentFields.filter((f) => f !== field)
                            handleFieldChange('additionalFields', newFields)
                          }}
                        />
                        <Label
                          htmlFor={`additional-${field}`}
                          className="text-sm capitalize"
                        >
                          {field.replace('_', ' ')}
                        </Label>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Rule Testing */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Zap className="h-5 w-5 text-green-600" />
            Rule Testing
          </CardTitle>
          <CardDescription>
            Test your rule against sample data to validate functionality
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-6">
            <div className="flex items-center gap-4">
              <Button
                onClick={handleTestRule}
                disabled={isTestingRule || !rule.name}
                className="gap-2"
              >
                <Play className="h-4 w-4" />
                {isTestingRule ? 'Testing...' : 'Test Rule'}
              </Button>
              {rule.performance && (
                <div className="flex items-center gap-4 text-sm text-muted-foreground">
                  <span>
                    Previous performance: {rule.performance.matchCount} matches
                  </span>
                  <span>
                    Success rate:{' '}
                    {Math.round(rule.performance.successRate * 100)}%
                  </span>
                  <span>Execution: {rule.performance.executionTime}</span>
                </div>
              )}
            </div>

            {testResult && (
              <div className="space-y-4">
                <div className="flex items-center gap-2">
                  {testResult.success ? (
                    <CheckCircle className="h-5 w-5 text-green-600" />
                  ) : (
                    <AlertTriangle className="h-5 w-5 text-red-600" />
                  )}
                  <span className="font-semibold">
                    Test {testResult.success ? 'Successful' : 'Failed'}
                  </span>
                </div>

                <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                  <div className="rounded-lg bg-blue-50 p-4">
                    <div className="text-2xl font-bold text-blue-700">
                      {testResult.matches}
                    </div>
                    <div className="text-sm text-blue-600">Matches Found</div>
                  </div>
                  <div className="rounded-lg bg-green-50 p-4">
                    <div className="text-2xl font-bold text-green-700">
                      {testResult.executionTime}
                    </div>
                    <div className="text-sm text-green-600">Execution Time</div>
                  </div>
                  <div className="rounded-lg bg-purple-50 p-4">
                    <div className="text-2xl font-bold text-purple-700">
                      {testResult.sampleMatches.length}
                    </div>
                    <div className="text-sm text-purple-600">
                      Sample Matches
                    </div>
                  </div>
                </div>

                {testResult.errors.length > 0 && (
                  <div className="rounded-lg border border-red-200 bg-red-50 p-4">
                    <h5 className="mb-2 font-semibold text-red-900">Errors</h5>
                    <ul className="space-y-1">
                      {testResult.errors.map((error, index) => (
                        <li key={index} className="text-sm text-red-700">
                          • {error}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}

                {testResult.warnings.length > 0 && (
                  <div className="rounded-lg border border-yellow-200 bg-yellow-50 p-4">
                    <h5 className="mb-2 font-semibold text-yellow-900">
                      Warnings
                    </h5>
                    <ul className="space-y-1">
                      {testResult.warnings.map((warning, index) => (
                        <li key={index} className="text-sm text-yellow-700">
                          • {warning}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}

                {testResult.sampleMatches.length > 0 && (
                  <div className="space-y-4">
                    <h5 className="font-semibold">Sample Matches</h5>
                    <div className="space-y-3">
                      {testResult.sampleMatches
                        .slice(0, 3)
                        .map((match, index) => (
                          <div
                            key={index}
                            className="rounded-lg border bg-gray-50 p-4"
                          >
                            <div className="mb-3 flex items-start justify-between">
                              <span className="font-medium">
                                Match {index + 1}
                              </span>
                              <Badge className="bg-green-500">
                                {Math.round(match.confidence * 100)}% confidence
                              </Badge>
                            </div>
                            <div className="grid grid-cols-2 gap-4 text-sm">
                              <div>
                                <span className="font-medium">External:</span>
                                <div className="text-gray-600">
                                  {match.externalTxn.description} - $
                                  {match.externalTxn.amount}
                                </div>
                              </div>
                              <div>
                                <span className="font-medium">Internal:</span>
                                <div className="text-gray-600">
                                  {match.internalTxn.description} - $
                                  {match.internalTxn.amount}
                                </div>
                              </div>
                            </div>
                            <div className="mt-2">
                              <span className="text-xs text-gray-500">
                                Matched fields: {match.matchedFields.join(', ')}
                              </span>
                            </div>
                          </div>
                        ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Save Actions */}
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Button variant="outline" className="gap-2">
                <Copy className="h-4 w-4" />
                Duplicate Rule
              </Button>
              <Button variant="outline" className="gap-2">
                <Brain className="h-4 w-4" />
                AI Suggestions
              </Button>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="outline">Cancel</Button>
              <Button
                onClick={handleSaveRule}
                disabled={!rule.name}
                className="gap-2"
              >
                <Save className="h-4 w-4" />
                {ruleId ? 'Update Rule' : 'Create Rule'}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
