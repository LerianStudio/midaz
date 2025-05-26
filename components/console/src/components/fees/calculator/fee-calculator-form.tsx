'use client'

import React from 'react'
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
import { FeeCalculationRequest } from '../types/fee-types'
import { getActivePackages } from '../mock/fee-mock-data'
import { Calculator } from 'lucide-react'
import { useIntl } from 'react-intl'

interface FeeCalculatorFormProps {
  onCalculate: (request: FeeCalculationRequest) => void
  isCalculating?: boolean
}

export function FeeCalculatorForm({
  onCalculate,
  isCalculating = false
}: FeeCalculatorFormProps) {
  const intl = useIntl()
  const activePackages = getActivePackages()

  const [formData, setFormData] = React.useState<FeeCalculationRequest>({
    ledgerId: 'main-ledger',
    amount: 0,
    currency: 'USD',
    from: '',
    to: '',
    packageId: activePackages[0]?.id || ''
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    onCalculate(formData)
  }

  const handleInputChange = (
    field: keyof FeeCalculationRequest,
    value: any
  ) => {
    setFormData((prev) => ({
      ...prev,
      [field]: value
    }))
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="amount">Transaction Amount *</Label>
          <div className="relative">
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground">
              $
            </span>
            <Input
              id="amount"
              name="amount"
              type="number"
              step="0.01"
              min="0"
              value={formData.amount || ''}
              onChange={(e) =>
                handleInputChange('amount', parseFloat(e.target.value) || 0)
              }
              className="pl-8"
              placeholder="0.00"
              required
            />
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="currency">Currency</Label>
          <Select
            value={formData.currency}
            onValueChange={(value) => handleInputChange('currency', value)}
          >
            <SelectTrigger id="currency">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="USD">USD - US Dollar</SelectItem>
              <SelectItem value="EUR">EUR - Euro</SelectItem>
              <SelectItem value="GBP">GBP - British Pound</SelectItem>
              <SelectItem value="BRL">BRL - Brazilian Real</SelectItem>
              <SelectItem value="JPY">JPY - Japanese Yen</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="packageId">Fee Package</Label>
        <Select
          value={formData.packageId}
          onValueChange={(value) => handleInputChange('packageId', value)}
        >
          <SelectTrigger id="packageId">
            <SelectValue placeholder="Select a fee package" />
          </SelectTrigger>
          <SelectContent>
            {activePackages.map((pkg) => (
              <SelectItem key={pkg.id} value={pkg.id}>
                {pkg.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="from">From Account *</Label>
          <Input
            id="from"
            name="from"
            value={formData.from}
            onChange={(e) => handleInputChange('from', e.target.value)}
            placeholder="e.g., customer-001"
            required
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="to">To Account *</Label>
          <Input
            id="to"
            name="to"
            value={formData.to}
            onChange={(e) => handleInputChange('to', e.target.value)}
            placeholder="e.g., merchant-001"
            required
          />
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="transactionId">Transaction ID (Optional)</Label>
        <Input
          id="transactionId"
          value={formData.transactionId || ''}
          onChange={(e) => handleInputChange('transactionId', e.target.value)}
          placeholder="For reference only"
        />
      </div>

      <Button
        type="submit"
        className="w-full"
        disabled={
          isCalculating || !formData.amount || !formData.from || !formData.to
        }
      >
        <Calculator className="mr-2 h-4 w-4" />
        {isCalculating ? 'Calculating...' : 'Calculate Fees'}
      </Button>
    </form>
  )
}
