'use client'

import React, { useState } from 'react'
import { Plus, Trash2, Calculator, Eye, Save } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
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
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { AccountSelector } from './account-selector'
import {
  type OperationRoute,
  mockAccountTypes
} from '@/components/accounting/mock/transaction-route-mock-data'

interface OperationRouteFormProps {
  operation?: OperationRoute
  onSave: (operation: OperationRoute) => void
  onCancel: () => void
  mode?: 'create' | 'edit'
}

interface Condition {
  field: string
  operator: 'equals' | 'greater_than' | 'less_than' | 'contains'
  value: string
}

export function OperationRouteForm({
  operation,
  onSave,
  onCancel,
  mode = 'create'
}: OperationRouteFormProps) {
  const [formData, setFormData] = useState<Partial<OperationRoute>>({
    id: operation?.id || `op-${Date.now()}`,
    transactionRouteId: operation?.transactionRouteId || '',
    operationType: operation?.operationType || 'debit',
    sourceAccountTypeId: operation?.sourceAccountTypeId || '',
    destinationAccountTypeId: operation?.destinationAccountTypeId || '',
    amount: operation?.amount || {
      expression: '{{amount}}',
      description: 'Transaction amount'
    },
    description: operation?.description || '',
    order: operation?.order || 1,
    conditions: operation?.conditions || []
  })

  const [testData, setTestData] = useState({
    amount: 100,
    currency: 'USD',
    accountBalance: 1000,
    customField: 'test'
  })

  const [errors, setErrors] = useState<Record<string, string>>({})

  const validateForm = (): boolean => {
    const newErrors: Record<string, string> = {}

    if (!formData.description?.trim()) {
      newErrors.description = 'Description is required'
    }

    if (!formData.sourceAccountTypeId) {
      newErrors.sourceAccountTypeId = 'Source account type is required'
    }

    if (!formData.destinationAccountTypeId) {
      newErrors.destinationAccountTypeId =
        'Destination account type is required'
    }

    if (!formData.amount?.expression?.trim()) {
      newErrors.amountExpression = 'Amount expression is required'
    }

    if (!formData.amount?.description?.trim()) {
      newErrors.amountDescription = 'Amount description is required'
    }

    if (!formData.order || formData.order < 1) {
      newErrors.order = 'Order must be a positive number'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleSave = () => {
    if (validateForm()) {
      onSave(formData as OperationRoute)
    }
  }

  const handleAddCondition = () => {
    const newConditions = [
      ...(formData.conditions || []),
      { field: 'amount', operator: 'greater_than', value: '0' } as Condition
    ]
    setFormData((prev) => ({ ...prev, conditions: newConditions }))
  }

  const handleUpdateCondition = (index: number, condition: Condition) => {
    const newConditions = [...(formData.conditions || [])]
    newConditions[index] = condition
    setFormData((prev) => ({ ...prev, conditions: newConditions }))
  }

  const handleRemoveCondition = (index: number) => {
    const newConditions = (formData.conditions || []).filter(
      (_, i) => i !== index
    )
    setFormData((prev) => ({ ...prev, conditions: newConditions }))
  }

  const calculatePreview = () => {
    try {
      if (!formData.amount?.expression) return 'N/A'

      // Simple expression evaluation for preview
      let expression = formData.amount.expression
        .replace(/\{\{amount\}\}/g, testData.amount.toString())
        .replace(/\{\{accountBalance\}\}/g, testData.accountBalance.toString())
        .replace(/\{\{(\w+)\}\}/g, (match, field) => {
          if (field in testData) {
            return (testData as any)[field].toString()
          }
          return match
        })

      // Basic math evaluation (simplified - in production use a proper expression parser)
      const result = eval(expression)
      return isNaN(result)
        ? expression
        : `${testData.currency} ${result.toFixed(2)}`
    } catch {
      return formData.amount?.expression || 'Invalid expression'
    }
  }

  const evaluateConditions = () => {
    if (!formData.conditions || formData.conditions.length === 0) {
      return { result: true, details: 'No conditions defined' }
    }

    const results = formData.conditions.map((condition, index) => {
      const fieldValue = (testData as any)[condition.field]
      let conditionMet = false

      switch (condition.operator) {
        case 'equals':
          conditionMet = fieldValue?.toString() === condition.value
          break
        case 'greater_than':
          conditionMet = parseFloat(fieldValue) > parseFloat(condition.value)
          break
        case 'less_than':
          conditionMet = parseFloat(fieldValue) < parseFloat(condition.value)
          break
        case 'contains':
          conditionMet = fieldValue?.toString().includes(condition.value)
          break
      }

      return {
        index,
        condition,
        fieldValue,
        result: conditionMet
      }
    })

    const allMet = results.every((r) => r.result)
    return { result: allMet, details: results }
  }

  const sourceAccountType = mockAccountTypes.find(
    (at) => at.id === formData.sourceAccountTypeId
  )
  const destinationAccountType = mockAccountTypes.find(
    (at) => at.id === formData.destinationAccountTypeId
  )

  const compatibilityCheck = () => {
    if (!sourceAccountType || !destinationAccountType) {
      return { compatible: true, warnings: [] }
    }

    const warnings = []

    // Check for same account type
    if (
      sourceAccountType.id === destinationAccountType.id &&
      formData.operationType === 'debit'
    ) {
      warnings.push('Source and destination are the same account type')
    }

    // Check for nature compatibility
    if (
      formData.operationType === 'debit' &&
      sourceAccountType.nature === 'credit'
    ) {
      warnings.push('Debiting a credit-nature account type may not be intended')
    }

    if (
      formData.operationType === 'credit' &&
      sourceAccountType.nature === 'debit'
    ) {
      warnings.push('Crediting a debit-nature account type may not be intended')
    }

    // Check for domain compatibility
    if (sourceAccountType.domain !== destinationAccountType.domain) {
      warnings.push('Cross-domain transfers may require additional validation')
    }

    return { compatible: warnings.length === 0, warnings }
  }

  const compatibility = compatibilityCheck()
  const conditionResults = evaluateConditions()

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">
            {mode === 'create'
              ? 'Create Operation Route'
              : 'Edit Operation Route'}
          </h2>
          <p className="text-sm text-muted-foreground">
            Configure account mapping and amount calculation for this operation.
          </p>
        </div>
        <div className="flex space-x-2">
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button onClick={handleSave}>
            <Save className="mr-2 h-4 w-4" />
            Save Operation
          </Button>
        </div>
      </div>

      <Tabs defaultValue="basic" className="w-full">
        <TabsList className="grid w-full grid-cols-4">
          <TabsTrigger value="basic">Basic Info</TabsTrigger>
          <TabsTrigger value="accounts">Account Mapping</TabsTrigger>
          <TabsTrigger value="conditions">Conditions</TabsTrigger>
          <TabsTrigger value="preview">Preview & Test</TabsTrigger>
        </TabsList>

        <TabsContent value="basic" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Operation Details</CardTitle>
              <CardDescription>
                Basic information about this operation route.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="description">Description *</Label>
                <Textarea
                  id="description"
                  placeholder="Describe what this operation does..."
                  value={formData.description}
                  onChange={(e) =>
                    setFormData((prev) => ({
                      ...prev,
                      description: e.target.value
                    }))
                  }
                  className={errors.description ? 'border-red-500' : ''}
                />
                {errors.description && (
                  <p className="text-sm text-red-600">{errors.description}</p>
                )}
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="operationType">Operation Type *</Label>
                  <Select
                    value={formData.operationType}
                    onValueChange={(value: 'debit' | 'credit') =>
                      setFormData((prev) => ({ ...prev, operationType: value }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="debit">Debit</SelectItem>
                      <SelectItem value="credit">Credit</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="order">Order *</Label>
                  <Input
                    id="order"
                    type="number"
                    min="1"
                    value={formData.order}
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        order: parseInt(e.target.value) || 1
                      }))
                    }
                    className={errors.order ? 'border-red-500' : ''}
                  />
                  {errors.order && (
                    <p className="text-sm text-red-600">{errors.order}</p>
                  )}
                </div>
              </div>

              <div className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="amountExpression">Amount Expression *</Label>
                  <Input
                    id="amountExpression"
                    placeholder="e.g., {{amount}}, {{amount}} * 0.03, {{amount}} + 5"
                    value={formData.amount?.expression}
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        amount: { ...prev.amount!, expression: e.target.value }
                      }))
                    }
                    className={errors.amountExpression ? 'border-red-500' : ''}
                  />
                  {errors.amountExpression && (
                    <p className="text-sm text-red-600">
                      {errors.amountExpression}
                    </p>
                  )}
                  <p className="text-xs text-muted-foreground">
                    Use template variables like {`{amount}`}, {`{fee}`},{' '}
                    {`{rate}`} for dynamic calculations.
                  </p>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="amountDescription">
                    Amount Description *
                  </Label>
                  <Input
                    id="amountDescription"
                    placeholder="Describe what this amount represents..."
                    value={formData.amount?.description}
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        amount: { ...prev.amount!, description: e.target.value }
                      }))
                    }
                    className={errors.amountDescription ? 'border-red-500' : ''}
                  />
                  {errors.amountDescription && (
                    <p className="text-sm text-red-600">
                      {errors.amountDescription}
                    </p>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="accounts" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Account Type Mapping</CardTitle>
              <CardDescription>
                Select the source and destination account types for this
                operation.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-2">
                <Label>Source Account Type *</Label>
                <AccountSelector
                  value={formData.sourceAccountTypeId}
                  onValueChange={(value) =>
                    setFormData((prev) => ({
                      ...prev,
                      sourceAccountTypeId: value
                    }))
                  }
                  placeholder="Select source account type..."
                />
                {errors.sourceAccountTypeId && (
                  <p className="text-sm text-red-600">
                    {errors.sourceAccountTypeId}
                  </p>
                )}
              </div>

              <div className="space-y-2">
                <Label>Destination Account Type *</Label>
                <AccountSelector
                  value={formData.destinationAccountTypeId}
                  onValueChange={(value) =>
                    setFormData((prev) => ({
                      ...prev,
                      destinationAccountTypeId: value
                    }))
                  }
                  placeholder="Select destination account type..."
                />
                {errors.destinationAccountTypeId && (
                  <p className="text-sm text-red-600">
                    {errors.destinationAccountTypeId}
                  </p>
                )}
              </div>

              {/* Compatibility Check */}
              {sourceAccountType && destinationAccountType && (
                <div className="space-y-4">
                  <Separator />
                  <div>
                    <h4 className="mb-2 font-medium">Compatibility Check</h4>
                    {compatibility.compatible ? (
                      <Alert className="border-green-200 bg-green-50">
                        <AlertDescription className="text-green-800">
                          Account types are compatible for this operation.
                        </AlertDescription>
                      </Alert>
                    ) : (
                      <Alert className="border-yellow-200 bg-yellow-50">
                        <AlertDescription className="text-yellow-800">
                          <div>
                            <strong>Potential Issues:</strong>
                            <ul className="mt-1 list-inside list-disc">
                              {compatibility.warnings.map((warning, index) => (
                                <li key={index} className="text-sm">
                                  {warning}
                                </li>
                              ))}
                            </ul>
                          </div>
                        </AlertDescription>
                      </Alert>
                    )}
                  </div>

                  <div className="grid grid-cols-2 gap-4">
                    <Card>
                      <CardHeader className="pb-2">
                        <CardTitle className="text-sm">
                          Source Account
                        </CardTitle>
                      </CardHeader>
                      <CardContent className="space-y-2">
                        <div className="text-sm">
                          <strong>{sourceAccountType.name}</strong>
                        </div>
                        <div className="flex flex-wrap gap-1">
                          <Badge variant="outline">
                            {sourceAccountType.code}
                          </Badge>
                          <Badge className="text-xs">
                            {sourceAccountType.category}
                          </Badge>
                          <Badge variant="secondary" className="text-xs">
                            {sourceAccountType.nature}
                          </Badge>
                        </div>
                        <p className="text-xs text-muted-foreground">
                          {sourceAccountType.description}
                        </p>
                      </CardContent>
                    </Card>

                    <Card>
                      <CardHeader className="pb-2">
                        <CardTitle className="text-sm">
                          Destination Account
                        </CardTitle>
                      </CardHeader>
                      <CardContent className="space-y-2">
                        <div className="text-sm">
                          <strong>{destinationAccountType.name}</strong>
                        </div>
                        <div className="flex flex-wrap gap-1">
                          <Badge variant="outline">
                            {destinationAccountType.code}
                          </Badge>
                          <Badge className="text-xs">
                            {destinationAccountType.category}
                          </Badge>
                          <Badge variant="secondary" className="text-xs">
                            {destinationAccountType.nature}
                          </Badge>
                        </div>
                        <p className="text-xs text-muted-foreground">
                          {destinationAccountType.description}
                        </p>
                      </CardContent>
                    </Card>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="conditions" className="space-y-4">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle>Conditional Logic</CardTitle>
                  <CardDescription>
                    Add conditions that must be met for this operation to
                    execute.
                  </CardDescription>
                </div>
                <Button
                  onClick={handleAddCondition}
                  variant="outline"
                  size="sm"
                >
                  <Plus className="mr-2 h-4 w-4" />
                  Add Condition
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {formData.conditions && formData.conditions.length > 0 ? (
                <div className="space-y-4">
                  {formData.conditions.map((condition, index) => (
                    <Card key={index}>
                      <CardContent className="p-4">
                        <div className="flex items-center space-x-4">
                          <div className="grid flex-1 grid-cols-3 gap-2">
                            <div>
                              <Label className="text-xs">Field</Label>
                              <Input
                                placeholder="amount"
                                value={condition.field}
                                onChange={(e) =>
                                  handleUpdateCondition(index, {
                                    ...condition,
                                    field: e.target.value
                                  })
                                }
                              />
                            </div>
                            <div>
                              <Label className="text-xs">Operator</Label>
                              <Select
                                value={condition.operator}
                                onValueChange={(value: any) =>
                                  handleUpdateCondition(index, {
                                    ...condition,
                                    operator: value
                                  })
                                }
                              >
                                <SelectTrigger>
                                  <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                  <SelectItem value="equals">Equals</SelectItem>
                                  <SelectItem value="greater_than">
                                    Greater than
                                  </SelectItem>
                                  <SelectItem value="less_than">
                                    Less than
                                  </SelectItem>
                                  <SelectItem value="contains">
                                    Contains
                                  </SelectItem>
                                </SelectContent>
                              </Select>
                            </div>
                            <div>
                              <Label className="text-xs">Value</Label>
                              <Input
                                placeholder="0"
                                value={condition.value}
                                onChange={(e) =>
                                  handleUpdateCondition(index, {
                                    ...condition,
                                    value: e.target.value
                                  })
                                }
                              />
                            </div>
                          </div>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleRemoveCondition(index)}
                            className="text-red-600 hover:text-red-700"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              ) : (
                <div className="py-8 text-center text-muted-foreground">
                  <p>
                    No conditions defined. This operation will always execute.
                  </p>
                  <Button
                    onClick={handleAddCondition}
                    variant="outline"
                    className="mt-2"
                  >
                    <Plus className="mr-2 h-4 w-4" />
                    Add First Condition
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="preview" className="space-y-4">
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center space-x-2">
                  <Calculator className="h-4 w-4" />
                  <span>Test Data</span>
                </CardTitle>
                <CardDescription>
                  Modify test values to see how your operation behaves.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <Label>Amount</Label>
                    <Input
                      type="number"
                      value={testData.amount}
                      onChange={(e) =>
                        setTestData((prev) => ({
                          ...prev,
                          amount: parseFloat(e.target.value) || 0
                        }))
                      }
                    />
                  </div>
                  <div>
                    <Label>Currency</Label>
                    <Input
                      value={testData.currency}
                      onChange={(e) =>
                        setTestData((prev) => ({
                          ...prev,
                          currency: e.target.value
                        }))
                      }
                    />
                  </div>
                  <div>
                    <Label>Account Balance</Label>
                    <Input
                      type="number"
                      value={testData.accountBalance}
                      onChange={(e) =>
                        setTestData((prev) => ({
                          ...prev,
                          accountBalance: parseFloat(e.target.value) || 0
                        }))
                      }
                    />
                  </div>
                  <div>
                    <Label>Custom Field</Label>
                    <Input
                      value={testData.customField}
                      onChange={(e) =>
                        setTestData((prev) => ({
                          ...prev,
                          customField: e.target.value
                        }))
                      }
                    />
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center space-x-2">
                  <Eye className="h-4 w-4" />
                  <span>Preview Results</span>
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div>
                  <Label className="text-sm font-medium">
                    Calculated Amount
                  </Label>
                  <div className="rounded bg-muted p-3 font-mono text-lg">
                    {calculatePreview()}
                  </div>
                </div>

                <div>
                  <Label className="text-sm font-medium">
                    Condition Results
                  </Label>
                  <div className="space-y-2">
                    {typeof conditionResults.details === 'string' ? (
                      <p className="text-sm text-muted-foreground">
                        {conditionResults.details}
                      </p>
                    ) : (
                      conditionResults.details.map(
                        (result: any, index: number) => (
                          <div
                            key={index}
                            className="flex items-center justify-between rounded bg-muted p-2"
                          >
                            <span className="text-sm">
                              {result.condition.field}{' '}
                              {result.condition.operator}{' '}
                              {result.condition.value}
                            </span>
                            <Badge
                              variant={
                                result.result ? 'default' : 'destructive'
                              }
                            >
                              {result.result ? 'Pass' : 'Fail'}
                            </Badge>
                          </div>
                        )
                      )
                    )}
                    <div className="mt-2">
                      <Badge
                        variant={
                          conditionResults.result ? 'default' : 'destructive'
                        }
                        className="text-sm"
                      >
                        Overall:{' '}
                        {conditionResults.result
                          ? 'All conditions met'
                          : 'Some conditions failed'}
                      </Badge>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}
