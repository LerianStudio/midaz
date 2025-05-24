'use client'

import React from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { DollarSign, Percent, GitBranch } from 'lucide-react'
import { cn } from '@/lib/utils'

interface CalculationTypeSelectorProps {
  onSelect: (type: 'FLAT' | 'PERCENTAGE' | 'MAX_BETWEEN_TYPES') => void
  onCancel: () => void
}

export function CalculationTypeSelector({
  onSelect,
  onCancel
}: CalculationTypeSelectorProps) {
  const types = [
    {
      id: 'FLAT' as const,
      title: 'Flat Fee',
      description: 'Charge a fixed amount regardless of transaction value',
      icon: DollarSign,
      color: 'text-blue-600 bg-blue-100',
      example: 'e.g., $0.30 per transaction'
    },
    {
      id: 'PERCENTAGE' as const,
      title: 'Percentage Fee',
      description: 'Charge a percentage of the transaction amount',
      icon: Percent,
      color: 'text-green-600 bg-green-100',
      example: 'e.g., 2.5% of transaction value'
    },
    {
      id: 'MAX_BETWEEN_TYPES' as const,
      title: 'Maximum Between Types',
      description: 'Calculate multiple fee types and apply the highest',
      icon: GitBranch,
      color: 'text-purple-600 bg-purple-100',
      example: 'e.g., Greater of $25 or 3.5%'
    }
  ]

  return (
    <Card>
      <CardContent className="p-6">
        <h3 className="mb-4 font-medium">Select Fee Type</h3>
        <div className="space-y-3">
          {types.map((type) => {
            const Icon = type.icon
            return (
              <button
                key={type.id}
                onClick={() => onSelect(type.id)}
                className="w-full rounded-lg border p-4 text-left transition-colors hover:border-primary hover:bg-accent"
              >
                <div className="flex items-start gap-3">
                  <div className={cn('rounded-lg p-2', type.color)}>
                    <Icon className="h-5 w-5" />
                  </div>
                  <div className="flex-1">
                    <h4 className="font-medium">{type.title}</h4>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {type.description}
                    </p>
                    <p className="mt-2 text-xs italic text-muted-foreground">
                      {type.example}
                    </p>
                  </div>
                </div>
              </button>
            )
          })}
        </div>
        <div className="mt-4 border-t pt-4">
          <Button variant="outline" onClick={onCancel} className="w-full">
            Cancel
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
