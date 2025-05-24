'use client'

import React, { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import {
  ArrowLeft,
  ArrowRight,
  Users,
  Building2,
  Check,
  FileText,
  Phone,
  MapPin,
  CreditCard
} from 'lucide-react'
import { CustomerWizard } from '@/components/crm/customers/customer-wizard'
import { CustomerType } from '@/components/crm/customers/customer-types'

export default function CreateCustomerPage() {
  const router = useRouter()
  const [currentStep, setCurrentStep] = useState(1)
  const [customerType, setCustomerType] = useState<CustomerType | null>(null)
  const [formData, setFormData] = useState({})

  const steps = [
    {
      id: 1,
      title: 'Customer Type',
      icon: Users,
      description: 'Choose customer type'
    },
    {
      id: 2,
      title: 'Basic Info',
      icon: FileText,
      description: 'Basic information'
    },
    { id: 3, title: 'Contact', icon: Phone, description: 'Contact details' },
    {
      id: 4,
      title: 'Address',
      icon: MapPin,
      description: 'Address information'
    },
    { id: 5, title: 'Review', icon: Check, description: 'Review and confirm' }
  ]

  const progress = ((currentStep - 1) / (steps.length - 1)) * 100

  const handleNext = () => {
    if (currentStep < steps.length) {
      setCurrentStep(currentStep + 1)
    }
  }

  const handlePrevious = () => {
    if (currentStep > 1) {
      setCurrentStep(currentStep - 1)
    }
  }

  const handleCustomerTypeSelect = (type: CustomerType) => {
    setCustomerType(type)
    setFormData({ ...formData, type })
    handleNext()
  }

  const handleFormDataUpdate = (stepData: any) => {
    setFormData({ ...formData, ...stepData })
  }

  const handleSubmit = () => {
    // Simulate customer creation
    console.log('Creating customer:', formData)

    // Navigate to customer list with success message
    router.push('/plugins/crm/customers?created=true')
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Create New Customer</h1>
          <p className="text-muted-foreground">
            Add a new customer to your CRM system
          </p>
        </div>
        <Button variant="outline" onClick={() => router.back()}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Cancel
        </Button>
      </div>

      {/* Progress */}
      <Card>
        <CardContent className="pt-6">
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">
                Step {currentStep} of {steps.length}
              </span>
              <span className="text-sm text-muted-foreground">
                {Math.round(progress)}% Complete
              </span>
            </div>
            <Progress value={progress} className="h-2" />

            {/* Step indicators */}
            <div className="flex items-center justify-between">
              {steps.map((step, index) => {
                const isActive = step.id === currentStep
                const isCompleted = step.id < currentStep
                const Icon = step.icon

                return (
                  <div
                    key={step.id}
                    className="flex flex-col items-center space-y-2"
                  >
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-full border-2 ${
                        isCompleted
                          ? 'border-primary bg-primary text-primary-foreground'
                          : isActive
                            ? 'border-primary bg-primary/10 text-primary'
                            : 'border-muted-foreground/30 text-muted-foreground'
                      } `}
                    >
                      {isCompleted ? (
                        <Check className="h-5 w-5" />
                      ) : (
                        <Icon className="h-5 w-5" />
                      )}
                    </div>
                    <div className="text-center">
                      <p
                        className={`text-xs font-medium ${
                          isActive ? 'text-primary' : 'text-muted-foreground'
                        }`}
                      >
                        {step.title}
                      </p>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Step Content */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center space-x-2">
            {React.createElement(steps[currentStep - 1].icon, {
              className: 'h-5 w-5'
            })}
            <span>{steps[currentStep - 1].title}</span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {currentStep === 1 && (
            <div className="space-y-6">
              <div>
                <h3 className="mb-2 text-lg font-medium">
                  Choose Customer Type
                </h3>
                <p className="mb-6 text-muted-foreground">
                  Select whether you're adding an individual customer or a
                  company.
                </p>
              </div>

              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <Card
                  className={`cursor-pointer border-2 transition-all hover:shadow-md ${
                    customerType === CustomerType.NATURAL_PERSON
                      ? 'border-primary bg-primary/5'
                      : 'border-border hover:border-primary/50'
                  }`}
                  onClick={() =>
                    handleCustomerTypeSelect(CustomerType.NATURAL_PERSON)
                  }
                >
                  <CardContent className="pt-6">
                    <div className="flex flex-col items-center space-y-4 text-center">
                      <div className="flex h-16 w-16 items-center justify-center rounded-full bg-blue-100 dark:bg-blue-900">
                        <Users className="h-8 w-8 text-blue-600" />
                      </div>
                      <div>
                        <h4 className="text-lg font-semibold">
                          Individual Customer
                        </h4>
                        <p className="text-sm text-muted-foreground">
                          Natural person with personal information
                        </p>
                      </div>
                      <Badge variant="outline">Natural Person</Badge>
                    </div>
                  </CardContent>
                </Card>

                <Card
                  className={`cursor-pointer border-2 transition-all hover:shadow-md ${
                    customerType === CustomerType.LEGAL_PERSON
                      ? 'border-primary bg-primary/5'
                      : 'border-border hover:border-primary/50'
                  }`}
                  onClick={() =>
                    handleCustomerTypeSelect(CustomerType.LEGAL_PERSON)
                  }
                >
                  <CardContent className="pt-6">
                    <div className="flex flex-col items-center space-y-4 text-center">
                      <div className="flex h-16 w-16 items-center justify-center rounded-full bg-purple-100 dark:bg-purple-900">
                        <Building2 className="h-8 w-8 text-purple-600" />
                      </div>
                      <div>
                        <h4 className="text-lg font-semibold">
                          Corporate Customer
                        </h4>
                        <p className="text-sm text-muted-foreground">
                          Company or organization with business information
                        </p>
                      </div>
                      <Badge variant="outline">Legal Person</Badge>
                    </div>
                  </CardContent>
                </Card>
              </div>
            </div>
          )}

          {currentStep > 1 && customerType && (
            <CustomerWizard
              step={currentStep}
              customerType={customerType}
              formData={formData}
              onDataUpdate={handleFormDataUpdate}
            />
          )}
        </CardContent>
      </Card>

      {/* Navigation */}
      <div className="flex items-center justify-between">
        <Button
          variant="outline"
          onClick={handlePrevious}
          disabled={currentStep === 1}
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Previous
        </Button>

        <div className="flex items-center space-x-2">
          {currentStep < steps.length ? (
            <Button onClick={handleNext}>
              Next
              <ArrowRight className="ml-2 h-4 w-4" />
            </Button>
          ) : (
            <Button onClick={handleSubmit}>
              <CreditCard className="mr-2 h-4 w-4" />
              Create Customer
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
