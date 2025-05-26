'use client'

import React from 'react'
import { CalculationType, Calculation } from '../types/fee-types'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Trash2,
  ChevronDown,
  ChevronUp,
  DollarSign,
  Percent,
  GitBranch
} from 'lucide-react'
import { cn } from '@/lib/utils'

interface RuleCardProps {
  rule: CalculationType
  index: number
  onChange: (rule: CalculationType) => void
  onDelete: () => void
}

export function RuleCard({ rule, index, onChange, onDelete }: RuleCardProps) {
  const [expanded, setExpanded] = React.useState(false)

  const getIcon = () => {
    switch (rule.type) {
      case 'FLAT':
        return <DollarSign className="h-4 w-4" />
      case 'PERCENTAGE':
        return <Percent className="h-4 w-4" />
      case 'MAX_BETWEEN_TYPES':
        return <GitBranch className="h-4 w-4" />
    }
  }

  const getTypeColor = () => {
    switch (rule.type) {
      case 'FLAT':
        return 'bg-blue-100 text-blue-800'
      case 'PERCENTAGE':
        return 'bg-green-100 text-green-800'
      case 'MAX_BETWEEN_TYPES':
        return 'bg-purple-100 text-purple-800'
    }
  }

  const handleCalculationChange = (
    calcIndex: number,
    field: string,
    value: any
  ) => {
    const newCalculations = [...rule.calculationType]
    newCalculations[calcIndex] = {
      ...newCalculations[calcIndex],
      [field]: value
    }
    onChange({
      ...rule,
      calculationType: newCalculations
    })
  }

  const handleTransactionTypeChange = (field: string, value: any) => {
    onChange({
      ...rule,
      transactionType: {
        ...rule.transactionType,
        [field]: value === '' ? undefined : value
      }
    })
  }

  const renderCalculationFields = (calc: Calculation, calcIndex: number) => {
    if (rule.type === 'FLAT') {
      return (
        <div className="space-y-3">
          <div>
            <Label>Fee Amount</Label>
            <div className="relative">
              <DollarSign className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
              <Input
                type="number"
                step="0.01"
                value={calc.value || ''}
                onChange={(e) =>
                  handleCalculationChange(
                    calcIndex,
                    'value',
                    parseFloat(e.target.value)
                  )
                }
                className="pl-9"
                placeholder="0.00"
              />
            </div>
          </div>
          <div>
            <Label>Fee Account</Label>
            <Input
              value={calc.fromTo?.[0] || ''}
              onChange={(e) =>
                handleCalculationChange(calcIndex, 'fromTo', [e.target.value])
              }
              placeholder="fees-account"
            />
          </div>
        </div>
      )
    }

    if (rule.type === 'PERCENTAGE') {
      return (
        <div className="space-y-3">
          <div>
            <Label>Percentage Rate</Label>
            <div className="relative">
              <Percent className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
              <Input
                type="number"
                step="0.01"
                value={calc.percentage || ''}
                onChange={(e) =>
                  handleCalculationChange(
                    calcIndex,
                    'percentage',
                    parseFloat(e.target.value)
                  )
                }
                className="pl-9"
                placeholder="0.00"
              />
            </div>
          </div>
          <div>
            <Label>Reference Amount</Label>
            <Select
              value={calc.refAmount || 'ORIGINAL'}
              onValueChange={(value) =>
                handleCalculationChange(calcIndex, 'refAmount', value)
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ORIGINAL">Original Amount</SelectItem>
                <SelectItem value="FEES">After Previous Fees</SelectItem>
                <SelectItem value="ORIGIN_FEES">Origin Account Fees</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label>Origin Account</Label>
              <Input
                value={calc.origin?.[0] || ''}
                onChange={(e) =>
                  handleCalculationChange(calcIndex, 'origin', [e.target.value])
                }
                placeholder="fees-revenue"
              />
            </div>
            <div>
              <Label>Target Account</Label>
              <Input
                value={calc.target?.[0] || ''}
                onChange={(e) =>
                  handleCalculationChange(calcIndex, 'target', [e.target.value])
                }
                placeholder="merchant-account"
              />
            </div>
          </div>
        </div>
      )
    }

    return null
  }

  return (
    <Card className="relative">
      <CardContent className="p-4">
        <div className="mb-3 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Badge variant="outline">Priority {rule.priority}</Badge>
            <Badge className={cn('flex items-center gap-1', getTypeColor())}>
              {getIcon()}
              {rule.type}
            </Badge>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? (
                <ChevronUp className="h-4 w-4" />
              ) : (
                <ChevronDown className="h-4 w-4" />
              )}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={onDelete}
              className="text-destructive hover:text-destructive"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {expanded && (
          <div className="space-y-4 border-t pt-4">
            {/* Transaction Criteria */}
            <div>
              <h4 className="mb-3 font-medium">Transaction Criteria</h4>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label>Min Amount</Label>
                  <Input
                    type="number"
                    value={rule.transactionType?.minValue || ''}
                    onChange={(e) =>
                      handleTransactionTypeChange(
                        'minValue',
                        e.target.value ? parseFloat(e.target.value) : ''
                      )
                    }
                    placeholder="0.00"
                  />
                </div>
                <div>
                  <Label>Max Amount</Label>
                  <Input
                    type="number"
                    value={rule.transactionType?.maxValue || ''}
                    onChange={(e) =>
                      handleTransactionTypeChange(
                        'maxValue',
                        e.target.value ? parseFloat(e.target.value) : ''
                      )
                    }
                    placeholder="No limit"
                  />
                </div>
                <div>
                  <Label>Currency</Label>
                  <Select
                    value={rule.transactionType?.currency || ''}
                    onValueChange={(value) =>
                      handleTransactionTypeChange('currency', value)
                    }
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Any currency" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="">Any currency</SelectItem>
                      <SelectItem value="USD">USD</SelectItem>
                      <SelectItem value="EUR">EUR</SelectItem>
                      <SelectItem value="GBP">GBP</SelectItem>
                      <SelectItem value="BRL">BRL</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <Label>Asset Code</Label>
                  <Input
                    value={rule.transactionType?.assetCode || ''}
                    onChange={(e) =>
                      handleTransactionTypeChange('assetCode', e.target.value)
                    }
                    placeholder="Any asset"
                  />
                </div>
              </div>
            </div>

            {/* Calculation Details */}
            <div>
              <h4 className="mb-3 font-medium">Calculation Details</h4>
              {rule.type === 'MAX_BETWEEN_TYPES' ? (
                <div className="space-y-3">
                  <p className="text-sm text-muted-foreground">
                    This rule will calculate multiple fee types and apply the
                    maximum.
                  </p>
                  {rule.calculationType.map((calc, calcIndex) => (
                    <div key={calcIndex} className="rounded-lg border p-3">
                      <h5 className="mb-2 text-sm font-medium">
                        Option {calcIndex + 1}:{' '}
                        {calc.value ? 'Flat Fee' : 'Percentage Fee'}
                      </h5>
                      {renderCalculationFields(calc, calcIndex)}
                    </div>
                  ))}
                </div>
              ) : (
                rule.calculationType.map((calc, calcIndex) => (
                  <div key={calcIndex}>
                    {renderCalculationFields(calc, calcIndex)}
                  </div>
                ))
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
