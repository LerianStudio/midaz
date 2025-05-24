'use client'

import React from 'react'
import { CalculationType } from '../types/fee-types'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Plus, GripVertical } from 'lucide-react'
import { RuleCard } from './rule-card'
import { CalculationTypeSelector } from './calculation-type-selector'
import { cn } from '@/lib/utils'

interface RuleBuilderProps {
  rules: CalculationType[]
  onChange: (rules: CalculationType[]) => void
  className?: string
}

export function RuleBuilder({ rules, onChange, className }: RuleBuilderProps) {
  const [showAddRule, setShowAddRule] = React.useState(false)
  const [draggedIndex, setDraggedIndex] = React.useState<number | null>(null)

  const handleAddRule = (type: 'FLAT' | 'PERCENTAGE' | 'MAX_BETWEEN_TYPES') => {
    const newRule: CalculationType = {
      priority: rules.length + 1,
      type,
      from: [{ anyAccount: true }],
      to: [{ anyAccount: true }],
      calculationType:
        type === 'FLAT'
          ? [
              {
                value: 0.3,
                fromTo: ['fees-account'],
                fromToType: 'ORIGIN'
              }
            ]
          : type === 'PERCENTAGE'
            ? [
                {
                  percentage: 2.5,
                  refAmount: 'ORIGINAL',
                  origin: ['fees-revenue'],
                  target: ['merchant-account']
                }
              ]
            : [
                {
                  value: 25,
                  fromTo: ['fees-flat'],
                  fromToType: 'ORIGIN'
                },
                {
                  percentage: 3.5,
                  refAmount: 'ORIGINAL',
                  origin: ['fees-percentage'],
                  target: ['merchant-account']
                }
              ]
    }

    onChange([...rules, newRule])
    setShowAddRule(false)
  }

  const handleUpdateRule = (index: number, updatedRule: CalculationType) => {
    const newRules = [...rules]
    newRules[index] = updatedRule
    onChange(newRules)
  }

  const handleDeleteRule = (index: number) => {
    const newRules = rules.filter((_, i) => i !== index)
    // Recalculate priorities
    const updatedRules = newRules.map((rule, i) => ({
      ...rule,
      priority: i + 1
    }))
    onChange(updatedRules)
  }

  const handleDragStart = (e: React.DragEvent, index: number) => {
    setDraggedIndex(index)
    e.dataTransfer.effectAllowed = 'move'
  }

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
  }

  const handleDrop = (e: React.DragEvent, dropIndex: number) => {
    e.preventDefault()

    if (draggedIndex === null || draggedIndex === dropIndex) {
      return
    }

    const draggedRule = rules[draggedIndex]
    const newRules = [...rules]

    // Remove dragged item
    newRules.splice(draggedIndex, 1)

    // Insert at new position
    newRules.splice(dropIndex, 0, draggedRule)

    // Update priorities
    const updatedRules = newRules.map((rule, i) => ({
      ...rule,
      priority: i + 1
    }))

    onChange(updatedRules)
    setDraggedIndex(null)
  }

  const handleDragEnd = () => {
    setDraggedIndex(null)
  }

  return (
    <Card className={cn('', className)}>
      <CardHeader>
        <CardTitle>Fee Calculation Rules</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {rules.length === 0 && !showAddRule && (
          <div className="py-8 text-center text-muted-foreground">
            <p>No rules defined yet.</p>
            <p className="mt-2 text-sm">Add your first rule to get started.</p>
          </div>
        )}

        {/* Rules List */}
        <div className="space-y-4">
          {rules.map((rule, index) => (
            <div
              key={index}
              className={cn('relative', draggedIndex === index && 'opacity-50')}
              draggable
              onDragStart={(e) => handleDragStart(e, index)}
              onDragOver={handleDragOver}
              onDrop={(e) => handleDrop(e, index)}
              onDragEnd={handleDragEnd}
            >
              <div className="absolute left-0 top-1/2 -translate-y-1/2 cursor-move p-2">
                <GripVertical className="h-4 w-4 text-muted-foreground" />
              </div>
              <div className="pl-8">
                <RuleCard
                  rule={rule}
                  index={index}
                  onChange={(updatedRule) =>
                    handleUpdateRule(index, updatedRule)
                  }
                  onDelete={() => handleDeleteRule(index)}
                />
              </div>
            </div>
          ))}
        </div>

        {/* Add Rule Section */}
        {showAddRule ? (
          <CalculationTypeSelector
            onSelect={handleAddRule}
            onCancel={() => setShowAddRule(false)}
          />
        ) : (
          <Button
            variant="outline"
            className="w-full"
            onClick={() => setShowAddRule(true)}
          >
            <Plus className="mr-2 h-4 w-4" />
            Add Rule
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
