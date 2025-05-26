'use client'

import React from 'react'
import { FeeCalculationResponse } from '../types/fee-types'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { DollarSign, Package, Clock, Hash } from 'lucide-react'
import { cn } from '@/lib/utils'

interface CalculationResultProps {
  result: FeeCalculationResponse
}

export function CalculationResult({ result }: CalculationResultProps) {
  const feePercentage =
    result.totalFees > 0 && result.calculatedFees[0]?.details?.baseAmount
      ? (result.totalFees / result.calculatedFees[0].details.baseAmount) * 100
      : 0

  return (
    <Card>
      <CardHeader>
        <CardTitle>Calculation Result</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Total Fee */}
        <div className="rounded-lg bg-primary/10 p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-muted-foreground">Total Fees</p>
              <p className="text-3xl font-bold">
                ${result.totalFees.toFixed(2)}
              </p>
              {feePercentage > 0 && (
                <p className="mt-1 text-sm text-muted-foreground">
                  {feePercentage.toFixed(2)}% of transaction
                </p>
              )}
            </div>
            <DollarSign className="h-12 w-12 text-primary/20" />
          </div>
        </div>

        {/* Metadata */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-1">
            <p className="flex items-center gap-1 text-sm text-muted-foreground">
              <Package className="h-3 w-3" />
              Package ID
            </p>
            <p className="truncate font-mono text-xs">{result.packageId}</p>
          </div>
          <div className="space-y-1">
            <p className="flex items-center gap-1 text-sm text-muted-foreground">
              <Hash className="h-3 w-3" />
              Transaction ID
            </p>
            <p className="truncate font-mono text-xs">
              {result.transactionId || 'N/A'}
            </p>
          </div>
        </div>

        {/* Timestamp */}
        <div className="flex items-center justify-between border-t pt-4">
          <p className="flex items-center gap-1 text-sm text-muted-foreground">
            <Clock className="h-3 w-3" />
            Calculated at
          </p>
          <p className="text-sm">
            {new Date(result.timestamp).toLocaleString()}
          </p>
        </div>

        {/* Fee Count */}
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">Fee Components</p>
          <Badge variant="secondary">
            {result.calculatedFees.length}{' '}
            {result.calculatedFees.length === 1 ? 'fee' : 'fees'}
          </Badge>
        </div>
      </CardContent>
    </Card>
  )
}
