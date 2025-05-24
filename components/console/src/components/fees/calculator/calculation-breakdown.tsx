'use client'

import React from 'react'
import { FeeCalculationResponse } from '../types/fee-types'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ArrowRight, DollarSign, Percent, Info, GitBranch } from 'lucide-react'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'

interface CalculationBreakdownProps {
  result: FeeCalculationResponse
}

export function CalculationBreakdown({ result }: CalculationBreakdownProps) {
  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'FLAT':
        return <DollarSign className="h-4 w-4" />
      case 'PERCENTAGE':
        return <Percent className="h-4 w-4" />
      case 'MAX_BETWEEN_TYPES':
        return <GitBranch className="h-4 w-4" />
      default:
        return <DollarSign className="h-4 w-4" />
    }
  }

  const getTypeBadgeColor = (type: string) => {
    switch (type) {
      case 'FLAT':
        return 'bg-blue-100 text-blue-800'
      case 'PERCENTAGE':
        return 'bg-green-100 text-green-800'
      case 'MAX_BETWEEN_TYPES':
        return 'bg-purple-100 text-purple-800'
      default:
        return 'bg-gray-100 text-gray-800'
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center justify-between">
          <span>Fee Breakdown</span>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger>
                <Info className="h-4 w-4 text-muted-foreground" />
              </TooltipTrigger>
              <TooltipContent>
                <p>Detailed breakdown of how fees were calculated</p>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {result.calculatedFees.map((fee, index) => (
          <div key={index} className="space-y-3 rounded-lg border p-4">
            {/* Fee Type and Amount */}
            <div className="flex items-center justify-between">
              <Badge className={getTypeBadgeColor(fee.type)}>
                {getTypeIcon(fee.type)}
                <span className="ml-1">{fee.type}</span>
              </Badge>
              <span className="text-lg font-semibold">
                ${fee.amount.toFixed(2)}
              </span>
            </div>

            {/* Account Flow */}
            <div className="flex items-center gap-2 text-sm">
              <div className="flex items-center gap-1 rounded bg-muted px-2 py-1">
                <span className="font-mono">{fee.from}</span>
              </div>
              <ArrowRight className="h-4 w-4 text-muted-foreground" />
              <div className="flex items-center gap-1 rounded bg-muted px-2 py-1">
                <span className="font-mono">{fee.to}</span>
              </div>
            </div>

            {/* Details */}
            {fee.details && Object.keys(fee.details).length > 0 && (
              <div className="space-y-1 border-t pt-3">
                <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Calculation Details
                </p>
                {Object.entries(fee.details).map(([key, value]) => (
                  <div key={key} className="flex justify-between text-sm">
                    <span className="capitalize text-muted-foreground">
                      {key.replace(/([A-Z])/g, ' $1').trim()}:
                    </span>
                    <span className="font-medium">
                      {typeof value === 'number'
                        ? key.includes('rate') || key.includes('percentage')
                          ? `${value}%`
                          : `$${value.toFixed(2)}`
                        : String(value)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}

        {/* Summary */}
        <div className="border-t pt-4">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Total Fees Applied</span>
            <span className="text-xl font-bold text-primary">
              ${result.totalFees.toFixed(2)}
            </span>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
