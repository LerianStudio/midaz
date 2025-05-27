'use client'

import { useState } from 'react'
import {
  Plus,
  X,
  Zap,
  Target,
  Calendar,
  Hash,
  Type,
  Settings,
  Eye,
  TestTube,
  Save,
  Play,
  AlertCircle,
  CheckCircle
} from 'lucide-react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Label } from '@/components/ui/label'
import { Slider } from '@/components/ui/slider'

import { ReconciliationRuleType } from '@/core/domain/entities/reconciliation-rule-entity'

interface RuleCondition {
  id: string
  field: string
  operator: string
  value: string | number
  tolerance?: number
  caseSensitive?: boolean
}

interface RuleGroup {
  id: string
  conditions: RuleCondition[]
  operator: 'and' | 'or'
}

interface RuleBuilderState {
  name: string
  description: string
  ruleType: ReconciliationRuleType
  priority: number
  isActive: boolean
  groups: RuleGroup[]
  testResults?: {
    isValid: boolean
    matches: number
    errors: string[]
  }
}

interface RuleBuilderProps {
  initialRule?: Partial<RuleBuilderState>
  onSave?: (rule: RuleBuilderState) => void
  onTest?: (rule: RuleBuilderState) => Promise<any>
  isLoading?: boolean
}

export function RuleBuilder({
  initialRule,
  onSave,
  onTest,
  isLoading = false
}: RuleBuilderProps) {
  const [rule, setRule] = useState<RuleBuilderState>({
    name: '',
    description: '',
    ruleType: 'amount',
    priority: 5,
    isActive: true,
    groups: [
      {
        id: crypto.randomUUID(),
        conditions: [
          {
            id: crypto.randomUUID(),
            field: 'amount',
            operator: 'equals',
            value: ''
          }
        ],
        operator: 'and'
      }
    ],
    ...initialRule
  })

  const [activeTab, setActiveTab] = useState('builder')
  const [isTestingRule, setIsTestingRule] = useState(false)

  const fieldOptions = [
    {
      value: 'amount',
      label: 'Amount',
      type: 'number',
      icon: <Hash className="h-4 w-4" />
    },
    {
      value: 'transaction_date',
      label: 'Transaction Date',
      type: 'date',
      icon: <Calendar className="h-4 w-4" />
    },
    {
      value: 'description',
      label: 'Description',
      type: 'string',
      icon: <Type className="h-4 w-4" />
    },
    {
      value: 'reference_number',
      label: 'Reference Number',
      type: 'string',
      icon: <Type className="h-4 w-4" />
    },
    {
      value: 'account_number',
      label: 'Account Number',
      type: 'string',
      icon: <Type className="h-4 w-4" />
    },
    {
      value: 'source_system',
      label: 'Source System',
      type: 'string',
      icon: <Type className="h-4 w-4" />
    },
    {
      value: 'metadata',
      label: 'Metadata Field',
      type: 'object',
      icon: <Settings className="h-4 w-4" />
    }
  ]

  const operatorOptions = {
    number: [
      { value: 'equals', label: 'Equals' },
      { value: 'greater_than', label: 'Greater Than' },
      { value: 'less_than', label: 'Less Than' },
      { value: 'between', label: 'Between' },
      { value: 'within_range', label: 'Within Range' }
    ],
    string: [
      { value: 'equals', label: 'Equals' },
      { value: 'contains', label: 'Contains' },
      { value: 'starts_with', label: 'Starts With' },
      { value: 'ends_with', label: 'Ends With' },
      { value: 'fuzzy_match', label: 'Fuzzy Match' },
      { value: 'regex_match', label: 'Regex Match' }
    ],
    date: [
      { value: 'equals', label: 'Equals' },
      { value: 'before', label: 'Before' },
      { value: 'after', label: 'After' },
      { value: 'within_date_range', label: 'Within Date Range' }
    ],
    object: [
      { value: 'has_key', label: 'Has Key' },
      { value: 'key_equals', label: 'Key Equals' },
      { value: 'key_contains', label: 'Key Contains' }
    ]
  }

  const addCondition = (groupId: string) => {
    setRule((prev) => ({
      ...prev,
      groups: prev.groups.map((group) =>
        group.id === groupId
          ? {
              ...group,
              conditions: [
                ...group.conditions,
                {
                  id: crypto.randomUUID(),
                  field: 'amount',
                  operator: 'equals',
                  value: ''
                }
              ]
            }
          : group
      )
    }))
  }

  const removeCondition = (groupId: string, conditionId: string) => {
    setRule((prev) => ({
      ...prev,
      groups: prev.groups.map((group) =>
        group.id === groupId
          ? {
              ...group,
              conditions: group.conditions.filter((c) => c.id !== conditionId)
            }
          : group
      )
    }))
  }

  const updateCondition = (
    groupId: string,
    conditionId: string,
    updates: Partial<RuleCondition>
  ) => {
    setRule((prev) => ({
      ...prev,
      groups: prev.groups.map((group) =>
        group.id === groupId
          ? {
              ...group,
              conditions: group.conditions.map((condition) =>
                condition.id === conditionId
                  ? { ...condition, ...updates }
                  : condition
              )
            }
          : group
      )
    }))
  }

  const addRuleGroup = () => {
    setRule((prev) => ({
      ...prev,
      groups: [
        ...prev.groups,
        {
          id: crypto.randomUUID(),
          conditions: [
            {
              id: crypto.randomUUID(),
              field: 'amount',
              operator: 'equals',
              value: ''
            }
          ],
          operator: 'and'
        }
      ]
    }))
  }

  const removeRuleGroup = (groupId: string) => {
    setRule((prev) => ({
      ...prev,
      groups: prev.groups.filter((g) => g.id !== groupId)
    }))
  }

  const testRule = async () => {
    setIsTestingRule(true)

    // Simulate rule testing
    await new Promise((resolve) => setTimeout(resolve, 2000))

    const testResults = {
      isValid: Math.random() > 0.3,
      matches: Math.floor(Math.random() * 1000) + 100,
      errors:
        Math.random() > 0.7
          ? [
              'Field "amount" requires numeric value',
              'Regex pattern is invalid for reference_number field'
            ]
          : []
    }

    setRule((prev) => ({ ...prev, testResults }))
    setIsTestingRule(false)

    if (onTest) {
      await onTest(rule)
    }
  }

  const saveRule = () => {
    if (onSave) {
      onSave(rule)
    }
  }

  const getFieldType = (fieldName: string) => {
    const field = fieldOptions.find((f) => f.value === fieldName)
    return field?.type || 'string'
  }

  const getOperatorsForField = (fieldName: string) => {
    const fieldType = getFieldType(fieldName)
    return (
      operatorOptions[fieldType as keyof typeof operatorOptions] ||
      operatorOptions.string
    )
  }

  return (
    <div className="space-y-6">
      {/* Rule Header */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Zap className="h-5 w-5 text-blue-500" />
            Rule Builder
          </CardTitle>
          <CardDescription>
            Create and configure reconciliation matching rules with visual
            interface
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <Label htmlFor="rule-name">Rule Name</Label>
              <Input
                id="rule-name"
                placeholder="Enter rule name..."
                value={rule.name}
                onChange={(e) =>
                  setRule((prev) => ({ ...prev, name: e.target.value }))
                }
              />
            </div>

            <div>
              <Label htmlFor="rule-type">Rule Type</Label>
              <Select
                value={rule.ruleType}
                onValueChange={(value: ReconciliationRuleType) =>
                  setRule((prev) => ({ ...prev, ruleType: value }))
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select rule type" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="amount">Amount Matching</SelectItem>
                  <SelectItem value="date">Date Matching</SelectItem>
                  <SelectItem value="string">String Matching</SelectItem>
                  <SelectItem value="regex">Regex Pattern</SelectItem>
                  <SelectItem value="metadata">Metadata Matching</SelectItem>
                  <SelectItem value="composite">Composite Rule</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div>
            <Label htmlFor="rule-description">Description</Label>
            <Textarea
              id="rule-description"
              placeholder="Describe what this rule matches..."
              value={rule.description}
              onChange={(e) =>
                setRule((prev) => ({ ...prev, description: e.target.value }))
              }
            />
          </div>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <Label>Priority ({rule.priority})</Label>
              <Slider
                value={[rule.priority]}
                onValueChange={([value]: number[]) =>
                  setRule((prev) => ({ ...prev, priority: value }))
                }
                max={10}
                min={1}
                step={1}
                className="mt-2"
              />
              <div className="mt-1 flex justify-between text-xs text-muted-foreground">
                <span>Low</span>
                <span>High</span>
              </div>
            </div>

            <div className="flex items-center space-x-2">
              <Switch
                id="rule-active"
                checked={rule.isActive}
                onCheckedChange={(checked) =>
                  setRule((prev) => ({ ...prev, isActive: checked }))
                }
              />
              <Label htmlFor="rule-active">Rule is active</Label>
            </div>
          </div>
        </CardContent>
      </Card>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="builder">Visual Builder</TabsTrigger>
          <TabsTrigger value="preview">Rule Preview</TabsTrigger>
          <TabsTrigger value="test">Test & Validate</TabsTrigger>
        </TabsList>

        <TabsContent value="builder" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Rule Conditions</CardTitle>
              <CardDescription>
                Define the conditions that must be met for this rule to match
                transactions
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {rule.groups.map((group, groupIndex) => (
                <div key={group.id} className="space-y-4 rounded-lg border p-4">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Badge variant="outline">Group {groupIndex + 1}</Badge>
                      <Select
                        value={group.operator}
                        onValueChange={(value: 'and' | 'or') =>
                          setRule((prev) => ({
                            ...prev,
                            groups: prev.groups.map((g) =>
                              g.id === group.id ? { ...g, operator: value } : g
                            )
                          }))
                        }
                      >
                        <SelectTrigger className="w-20">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="and">AND</SelectItem>
                          <SelectItem value="or">OR</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>

                    {rule.groups.length > 1 && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => removeRuleGroup(group.id)}
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    )}
                  </div>

                  <div className="space-y-3">
                    {group.conditions.map((condition, conditionIndex) => {
                      const fieldType = getFieldType(condition.field)
                      const operators = getOperatorsForField(condition.field)

                      return (
                        <div
                          key={condition.id}
                          className="flex items-center gap-3 rounded bg-gray-50 p-3"
                        >
                          {conditionIndex > 0 && (
                            <Badge variant="secondary" className="text-xs">
                              {group.operator.toUpperCase()}
                            </Badge>
                          )}

                          <Select
                            value={condition.field}
                            onValueChange={(value) =>
                              updateCondition(group.id, condition.id, {
                                field: value,
                                operator:
                                  getOperatorsForField(value)[0]?.value ||
                                  'equals'
                              })
                            }
                          >
                            <SelectTrigger className="w-40">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {fieldOptions.map((field) => (
                                <SelectItem
                                  key={field.value}
                                  value={field.value}
                                >
                                  <div className="flex items-center gap-2">
                                    {field.icon}
                                    {field.label}
                                  </div>
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>

                          <Select
                            value={condition.operator}
                            onValueChange={(value) =>
                              updateCondition(group.id, condition.id, {
                                operator: value
                              })
                            }
                          >
                            <SelectTrigger className="w-32">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {operators.map((op) => (
                                <SelectItem key={op.value} value={op.value}>
                                  {op.label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>

                          <Input
                            placeholder={
                              fieldType === 'number'
                                ? '0'
                                : fieldType === 'date'
                                  ? 'YYYY-MM-DD'
                                  : 'Enter value...'
                            }
                            type={fieldType === 'number' ? 'number' : 'text'}
                            value={condition.value}
                            onChange={(e) =>
                              updateCondition(group.id, condition.id, {
                                value: e.target.value
                              })
                            }
                            className="flex-1"
                          />

                          {(condition.operator === 'within_range' ||
                            condition.operator === 'fuzzy_match') && (
                            <div className="flex items-center gap-2">
                              <span className="text-sm">±</span>
                              <Input
                                type="number"
                                placeholder="0.1"
                                value={condition.tolerance || ''}
                                onChange={(e) =>
                                  updateCondition(group.id, condition.id, {
                                    tolerance: parseFloat(e.target.value) || 0
                                  })
                                }
                                className="w-20"
                              />
                            </div>
                          )}

                          {group.conditions.length > 1 && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() =>
                                removeCondition(group.id, condition.id)
                              }
                            >
                              <X className="h-4 w-4" />
                            </Button>
                          )}
                        </div>
                      )
                    })}
                  </div>

                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => addCondition(group.id)}
                    className="gap-2"
                  >
                    <Plus className="h-4 w-4" />
                    Add Condition
                  </Button>
                </div>
              ))}

              <div className="flex gap-2">
                <Button
                  variant="outline"
                  onClick={addRuleGroup}
                  className="gap-2"
                >
                  <Plus className="h-4 w-4" />
                  Add Group
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="preview" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Eye className="h-5 w-5" />
                Rule Preview
              </CardTitle>
              <CardDescription>
                Visual representation of your rule logic
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="rounded-lg bg-gray-50 p-4 font-mono text-sm">
                <div className="mb-2 font-bold">IF:</div>
                {rule.groups.map((group, groupIndex) => (
                  <div key={group.id} className="mb-2 ml-4">
                    {groupIndex > 0 && (
                      <div className="mb-1 font-bold text-blue-600">OR</div>
                    )}
                    <div className="border-l-2 border-blue-300 pl-4">
                      {group.conditions.map((condition, conditionIndex) => (
                        <div key={condition.id} className="mb-1">
                          {conditionIndex > 0 && (
                            <span className="mr-2 font-bold text-green-600">
                              {group.operator.toUpperCase()}
                            </span>
                          )}
                          <span className="text-purple-600">
                            {condition.field}
                          </span>
                          <span className="mx-2 text-orange-600">
                            {condition.operator}
                          </span>
                          <span className="text-red-600">
                            &quot;{condition.value}&quot;
                          </span>
                          {condition.tolerance && (
                            <span className="text-gray-600">
                              {' '}
                              (±{condition.tolerance})
                            </span>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
                <div className="mt-4 font-bold">THEN: Create Match</div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="test" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <TestTube className="h-5 w-5" />
                Test & Validate Rule
              </CardTitle>
              <CardDescription>
                Test your rule against sample data to validate functionality
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <Button
                onClick={testRule}
                disabled={isTestingRule}
                className="gap-2"
              >
                {isTestingRule ? (
                  <Play className="h-4 w-4 animate-spin" />
                ) : (
                  <TestTube className="h-4 w-4" />
                )}
                {isTestingRule ? 'Testing Rule...' : 'Test Rule'}
              </Button>

              {rule.testResults && (
                <div className="space-y-3">
                  <div
                    className={`rounded-lg p-4 ${
                      rule.testResults.isValid
                        ? 'border border-green-200 bg-green-50'
                        : 'border border-red-200 bg-red-50'
                    }`}
                  >
                    <div className="mb-2 flex items-center gap-2">
                      {rule.testResults.isValid ? (
                        <CheckCircle className="h-5 w-5 text-green-600" />
                      ) : (
                        <AlertCircle className="h-5 w-5 text-red-600" />
                      )}
                      <span className="font-medium">
                        {rule.testResults.isValid
                          ? 'Rule is Valid'
                          : 'Rule has Issues'}
                      </span>
                    </div>

                    {rule.testResults.isValid ? (
                      <p className="text-sm text-green-700">
                        Rule matched {rule.testResults.matches} transactions in
                        test dataset
                      </p>
                    ) : (
                      <div className="space-y-1">
                        {rule.testResults.errors.map((error, index) => (
                          <p key={index} className="text-sm text-red-700">
                            • {error}
                          </p>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Actions */}
      <div className="flex justify-end gap-3">
        <Button variant="outline">Cancel</Button>
        <Button
          onClick={saveRule}
          disabled={!rule.name || isLoading}
          className="gap-2"
        >
          <Save className="h-4 w-4" />
          {isLoading ? 'Saving...' : 'Save Rule'}
        </Button>
      </div>
    </div>
  )
}
