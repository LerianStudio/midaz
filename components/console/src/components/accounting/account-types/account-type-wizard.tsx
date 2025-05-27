'use client'

import React from 'react'
import { useState } from 'react'
import {
  Check,
  ArrowLeft,
  Database,
  ExternalLink,
  AlertCircle,
  CheckCircle
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Stepper } from '@/components/ui/stepper'
import { AccountTypeForm } from './account-type-form'

interface AccountTypeWizardData {
  name: string
  description: string
  keyValue: string
  domain: 'ledger' | 'external'
}

interface AccountTypeWizardProps {
  onSubmit: (data: AccountTypeWizardData) => Promise<void>
  onCancel?: () => void
  isSubmitting?: boolean
}

export function AccountTypeWizard({
  onSubmit,
  onCancel,
  isSubmitting = false
}: AccountTypeWizardProps) {
  const [currentStep, setCurrentStep] = useState(0)
  const [formData, setFormData] = useState<Partial<AccountTypeWizardData>>({})

  const steps = [
    {
      title: 'Basic Information',
      description: 'Account type details and key value'
    },
    {
      title: 'Review & Confirm',
      description: 'Verify your account type configuration'
    }
  ]

  const handleNext = () => {
    if (currentStep < steps.length - 1) {
      setCurrentStep(currentStep + 1)
    }
  }

  const handlePrevious = () => {
    if (currentStep > 0) {
      setCurrentStep(currentStep - 1)
    }
  }

  const handleFormSubmit = async (data: AccountTypeWizardData) => {
    setFormData(data)
    if (currentStep === 0) {
      handleNext()
    } else {
      await onSubmit(data)
    }
  }

  const renderDomainBadge = (domain: 'ledger' | 'external') => {
    return (
      <Badge
        variant={domain === 'ledger' ? 'default' : 'secondary'}
        className="gap-1"
      >
        {domain === 'ledger' ? (
          <Database className="h-3 w-3" />
        ) : (
          <ExternalLink className="h-3 w-3" />
        )}
        {domain === 'ledger' ? 'Ledger Domain' : 'External Domain'}
      </Badge>
    )
  }

  const renderBusinessRules = () => {
    if (!formData.domain) return null

    const rules =
      formData.domain === 'ledger'
        ? [
            'Full transaction history tracking enabled',
            'Real-time balance calculations supported',
            'Integrated compliance validation active',
            'Native audit trail automatically generated',
            'Double-entry bookkeeping enforced'
          ]
        : [
            'External system integration enabled',
            'Wire transfer protocols supported',
            'Third-party validation required',
            'Settlement processing configured',
            'External audit trail synchronization'
          ]

    return (
      <div className="space-y-3">
        <h4 className="font-medium text-gray-900">
          Business Rules & Validation
        </h4>
        <div className="space-y-2">
          {rules.map((rule, index) => (
            <div key={index} className="flex items-center gap-2 text-sm">
              <CheckCircle className="h-4 w-4 flex-shrink-0 text-green-500" />
              <span className="text-gray-600">{rule}</span>
            </div>
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Stepper */}
      <Stepper>
        {steps.map((step, index) => (
          <div
            key={index}
            className={`step ${index === currentStep ? 'active' : ''} ${index < currentStep ? 'completed' : ''}`}
          >
            <span className="font-medium">{step.title}</span>
            <span className="text-sm text-muted-foreground">
              {step.description}
            </span>
          </div>
        ))}
      </Stepper>

      {/* Step Content */}
      <Card>
        <CardHeader>
          <CardTitle>{steps[currentStep].title}</CardTitle>
          <CardDescription>{steps[currentStep].description}</CardDescription>
        </CardHeader>
        <CardContent>
          {currentStep === 0 && (
            <AccountTypeForm
              onSubmit={handleFormSubmit}
              mode="create"
              isSubmitting={false}
            />
          )}

          {currentStep === 1 && formData && (
            <div className="space-y-6">
              {/* Review Summary */}
              <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div className="space-y-4">
                  <h3 className="text-lg font-medium">Account Type Details</h3>

                  <div className="space-y-3">
                    <div>
                      <Label className="text-sm font-medium text-gray-600">
                        Name
                      </Label>
                      <p className="font-medium text-gray-900">
                        {formData.name}
                      </p>
                    </div>

                    <div>
                      <Label className="text-sm font-medium text-gray-600">
                        Description
                      </Label>
                      <p className="text-sm leading-relaxed text-gray-700">
                        {formData.description}
                      </p>
                    </div>

                    <div>
                      <Label className="text-sm font-medium text-gray-600">
                        Key Value
                      </Label>
                      <code className="rounded bg-gray-100 px-2 py-1 font-mono text-sm">
                        {formData.keyValue}
                      </code>
                    </div>

                    <div>
                      <Label className="text-sm font-medium text-gray-600">
                        Domain
                      </Label>
                      <div className="mt-1">
                        {formData.domain && renderDomainBadge(formData.domain)}
                      </div>
                    </div>
                  </div>
                </div>

                <div className="space-y-4">{renderBusinessRules()}</div>
              </div>

              {/* Integration Impact */}
              <Alert>
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>
                  <div className="space-y-2">
                    <div className="font-medium">Integration Impact</div>
                    <div className="text-sm">
                      Creating this account type will:
                      <ul className="mt-1 list-inside list-disc space-y-1">
                        <li>
                          Add &quot;{formData.keyValue}&quot; to the available
                          account types in transaction routes
                        </li>
                        <li>
                          Enable account creation with this type in the{' '}
                          {formData.domain} domain
                        </li>
                        <li>
                          Apply{' '}
                          {formData.domain === 'ledger'
                            ? 'internal ledger'
                            : 'external system'}{' '}
                          validation rules
                        </li>
                        <li>
                          Generate audit trail entries for all related
                          operations
                        </li>
                      </ul>
                    </div>
                  </div>
                </AlertDescription>
              </Alert>

              {/* Action Buttons */}
              <div className="flex items-center justify-between border-t pt-6">
                <Button
                  type="button"
                  variant="outline"
                  onClick={handlePrevious}
                  disabled={isSubmitting}
                >
                  <ArrowLeft className="mr-2 h-4 w-4" />
                  Previous
                </Button>

                <div className="flex items-center gap-3">
                  {onCancel && (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={onCancel}
                      disabled={isSubmitting}
                    >
                      Cancel
                    </Button>
                  )}
                  <Button
                    onClick={() =>
                      handleFormSubmit(formData as AccountTypeWizardData)
                    }
                    disabled={isSubmitting}
                  >
                    {isSubmitting ? (
                      <>
                        <div className="mr-2 h-4 w-4 animate-spin rounded-full border-b-2 border-white"></div>
                        Creating...
                      </>
                    ) : (
                      <>
                        <Check className="mr-2 h-4 w-4" />
                        Create Account Type
                      </>
                    )}
                  </Button>
                </div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function Label({
  children,
  className
}: {
  children: React.ReactNode
  className?: string
}) {
  return <div className={className}>{children}</div>
}
