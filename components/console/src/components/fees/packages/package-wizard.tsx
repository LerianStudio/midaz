'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Card, CardContent } from '@/components/ui/card'
import {
  Package,
  Settings,
  Calculator,
  Users,
  ChevronLeft,
  ChevronRight,
  Check
} from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { CreatePackageFormData, CalculationType } from '../types/fee-types'
import { cn } from '@/lib/utils'
import { RuleBuilder } from '../rules/rule-builder'

interface PackageWizardProps {
  onSubmit: (data: CreatePackageFormData) => void
  onCancel: () => void
  isSubmitting?: boolean
  initialData?: Partial<CreatePackageFormData>
}

const STEPS = [
  { id: 'basic', title: 'Basic Information', icon: Package },
  { id: 'rules', title: 'Fee Rules', icon: Calculator },
  { id: 'waivers', title: 'Account Waivers', icon: Users },
  { id: 'review', title: 'Review & Create', icon: Check }
]

export function PackageWizard({
  onSubmit,
  onCancel,
  isSubmitting = false,
  initialData = {}
}: PackageWizardProps) {
  const intl = useIntl()
  const [currentStep, setCurrentStep] = React.useState(0)
  const [formData, setFormData] = React.useState<CreatePackageFormData>({
    name: '',
    active: true,
    waivedAccounts: [],
    types: [],
    metadata: {},
    ...initialData
  })

  const [tempRule, setTempRule] = React.useState<Partial<CalculationType>>({
    priority: 1,
    type: 'PERCENTAGE',
    calculationType: []
  })

  const progress = ((currentStep + 1) / STEPS.length) * 100

  const handleNext = () => {
    if (currentStep < STEPS.length - 1) {
      setCurrentStep(currentStep + 1)
    }
  }

  const handlePrevious = () => {
    if (currentStep > 0) {
      setCurrentStep(currentStep - 1)
    }
  }

  const handleInputChange = (field: string, value: any) => {
    setFormData((prev) => ({
      ...prev,
      [field]: value
    }))
  }

  const handleMetadataChange = (field: string, value: any) => {
    setFormData((prev) => ({
      ...prev,
      metadata: {
        ...prev.metadata,
        [field]: value
      }
    }))
  }

  const addRule = () => {
    if (tempRule.type) {
      const newRule: CalculationType = {
        priority: tempRule.priority || 1,
        type: tempRule.type as any,
        from: [{ anyAccount: true }],
        to: [{ anyAccount: true }],
        calculationType: [
          {
            ...((tempRule.type === 'FLAT'
              ? { value: 0.3, fromTo: ['fees-account'], fromToType: 'ORIGIN' }
              : {
                  percentage: 2.5,
                  refAmount: 'ORIGINAL',
                  origin: ['fees-revenue'],
                  target: ['merchant-account']
                }) as any)
          }
        ]
      }

      setFormData((prev) => ({
        ...prev,
        types: [...prev.types, newRule]
      }))

      setTempRule({
        priority: ((prev) => prev.types.length + 1)(formData) + 1,
        type: 'PERCENTAGE',
        calculationType: []
      })
    }
  }

  const removeRule = (index: number) => {
    setFormData((prev) => ({
      ...prev,
      types: prev.types.filter((_, i) => i !== index)
    }))
  }

  const addWaivedAccount = (accountId: string) => {
    if (accountId && !formData.waivedAccounts.includes(accountId)) {
      setFormData((prev) => ({
        ...prev,
        waivedAccounts: [...prev.waivedAccounts, accountId]
      }))
    }
  }

  const removeWaivedAccount = (accountId: string) => {
    setFormData((prev) => ({
      ...prev,
      waivedAccounts: prev.waivedAccounts.filter((id) => id !== accountId)
    }))
  }

  const isStepValid = () => {
    switch (currentStep) {
      case 0:
        return formData.name.trim().length > 0
      case 1:
        return formData.types.length > 0
      case 2:
        return true // Waivers are optional
      case 3:
        return true // Review step is always valid
      default:
        return false
    }
  }

  const renderStepContent = () => {
    switch (currentStep) {
      case 0:
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Package Name *</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => handleInputChange('name', e.target.value)}
                placeholder="e.g., Standard Transaction Fees"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea
                id="description"
                value={formData.metadata?.description || ''}
                onChange={(e) =>
                  handleMetadataChange('description', e.target.value)
                }
                placeholder="Describe the purpose of this fee package..."
                rows={3}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="category">Category</Label>
              <Select
                value={formData.metadata?.category || ''}
                onValueChange={(value) =>
                  handleMetadataChange('category', value)
                }
              >
                <SelectTrigger id="category">
                  <SelectValue placeholder="Select a category" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="standard">Standard</SelectItem>
                  <SelectItem value="premium">Premium</SelectItem>
                  <SelectItem value="international">International</SelectItem>
                  <SelectItem value="micro">Micro-transactions</SelectItem>
                  <SelectItem value="custom">Custom</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-center space-x-2">
              <Switch
                id="active"
                checked={formData.active}
                onCheckedChange={(checked) =>
                  handleInputChange('active', checked)
                }
              />
              <Label htmlFor="active">Activate package immediately</Label>
            </div>
          </div>
        )

      case 1:
        return (
          <RuleBuilder
            rules={formData.types}
            onChange={(rules) => handleInputChange('types', rules)}
          />
        )

      case 2:
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Add Waived Account</Label>
              <div className="flex gap-2">
                <Input
                  placeholder="Enter account ID"
                  onKeyPress={(e) => {
                    if (e.key === 'Enter') {
                      const input = e.target as HTMLInputElement
                      addWaivedAccount(input.value)
                      input.value = ''
                    }
                  }}
                />
                <Button
                  variant="secondary"
                  onClick={() => {
                    const input = document.querySelector(
                      'input[placeholder="Enter account ID"]'
                    ) as HTMLInputElement
                    if (input) {
                      addWaivedAccount(input.value)
                      input.value = ''
                    }
                  }}
                >
                  Add
                </Button>
              </div>
            </div>

            <div className="space-y-2">
              <h4 className="font-medium">Waived Accounts</h4>
              {formData.waivedAccounts.length === 0 ? (
                <p className="rounded-lg border p-4 text-center text-sm text-muted-foreground">
                  No accounts waived. This is optional.
                </p>
              ) : (
                <div className="flex flex-wrap gap-2">
                  {formData.waivedAccounts.map((account) => (
                    <Badge
                      key={account}
                      variant="secondary"
                      className="px-3 py-1"
                    >
                      {account}
                      <button
                        className="ml-2 text-xs hover:text-destructive"
                        onClick={() => removeWaivedAccount(account)}
                      >
                        ×
                      </button>
                    </Badge>
                  ))}
                </div>
              )}
            </div>
          </div>
        )

      case 3:
        return (
          <div className="space-y-4">
            <h3 className="text-lg font-medium">Review Your Package</h3>

            <div className="space-y-4">
              <div className="space-y-3 rounded-lg border p-4">
                <h4 className="flex items-center gap-2 font-medium">
                  <Package className="h-4 w-4" />
                  Basic Information
                </h4>
                <dl className="space-y-1 text-sm">
                  <div className="flex justify-between">
                    <dt className="text-muted-foreground">Name:</dt>
                    <dd className="font-medium">{formData.name}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="text-muted-foreground">Status:</dt>
                    <dd>
                      <Badge
                        variant={formData.active ? 'default' : 'secondary'}
                      >
                        {formData.active ? 'Active' : 'Inactive'}
                      </Badge>
                    </dd>
                  </div>
                  {formData.metadata?.description && (
                    <div className="flex justify-between">
                      <dt className="text-muted-foreground">Description:</dt>
                      <dd className="max-w-xs text-right">
                        {formData.metadata.description}
                      </dd>
                    </div>
                  )}
                </dl>
              </div>

              <div className="space-y-3 rounded-lg border p-4">
                <h4 className="flex items-center gap-2 font-medium">
                  <Calculator className="h-4 w-4" />
                  Fee Rules ({formData.types.length})
                </h4>
                <div className="space-y-2">
                  {formData.types.map((rule, index) => (
                    <div
                      key={index}
                      className="flex items-center gap-2 text-sm"
                    >
                      <Badge variant="outline">Priority {rule.priority}</Badge>
                      <span>{rule.type}</span>
                    </div>
                  ))}
                </div>
              </div>

              <div className="space-y-3 rounded-lg border p-4">
                <h4 className="flex items-center gap-2 font-medium">
                  <Users className="h-4 w-4" />
                  Waived Accounts ({formData.waivedAccounts.length})
                </h4>
                {formData.waivedAccounts.length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {formData.waivedAccounts.map((account) => (
                      <Badge key={account} variant="secondary">
                        {account}
                      </Badge>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    No accounts waived
                  </p>
                )}
              </div>
            </div>
          </div>
        )
    }
  }

  return (
    <div className="space-y-6">
      {/* Steps indicator */}
      <div className="space-y-4">
        <Progress value={progress} className="h-2" />
        <div className="flex justify-between">
          {STEPS.map((step, index) => {
            const Icon = step.icon
            const isActive = index === currentStep
            const isCompleted = index < currentStep

            return (
              <div
                key={step.id}
                className={cn(
                  'flex flex-col items-center gap-2 text-sm',
                  isActive && 'text-primary',
                  !isActive && !isCompleted && 'text-muted-foreground'
                )}
              >
                <div
                  className={cn(
                    'flex h-10 w-10 items-center justify-center rounded-full border-2',
                    isActive &&
                      'border-primary bg-primary text-primary-foreground',
                    isCompleted && 'border-primary bg-primary/10 text-primary',
                    !isActive && !isCompleted && 'border-muted-foreground/30'
                  )}
                >
                  <Icon className="h-5 w-5" />
                </div>
                <span className="hidden sm:inline">{step.title}</span>
              </div>
            )
          })}
        </div>
      </div>

      {/* Step content */}
      <Card>
        <CardContent className="p-6">{renderStepContent()}</CardContent>
      </Card>

      {/* Navigation buttons */}
      <div className="flex justify-between">
        <Button
          variant="outline"
          onClick={currentStep === 0 ? onCancel : handlePrevious}
        >
          <ChevronLeft className="mr-2 h-4 w-4" />
          {currentStep === 0 ? 'Cancel' : 'Previous'}
        </Button>

        {currentStep < STEPS.length - 1 ? (
          <Button onClick={handleNext} disabled={!isStepValid()}>
            Next
            <ChevronRight className="ml-2 h-4 w-4" />
          </Button>
        ) : (
          <Button onClick={() => onSubmit(formData)} disabled={isSubmitting}>
            {isSubmitting ? 'Creating...' : 'Create Package'}
          </Button>
        )}
      </div>
    </div>
  )
}
